package parser

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

func TestNewPool(t *testing.T) {
	p := NewPool(4)
	defer p.Shutdown()

	if p.size != 4 {
		t.Errorf("expected pool size 4, got %d", p.size)
	}
	if len(p.parsers) != 4 {
		t.Errorf("expected 4 parsers in channel, got %d", len(p.parsers))
	}
}

func TestNewPool_MinSize(t *testing.T) {
	p := NewPool(0)
	defer p.Shutdown()

	if p.size != 1 {
		t.Errorf("expected pool size clamped to 1, got %d", p.size)
	}
}

func TestPool_Parse_TypeScript(t *testing.T) {
	p := NewPool(1)
	defer p.Shutdown()

	code := []byte(`function greet(name: string): string { return "hello " + name; }`)
	tree, err := p.Parse(context.Background(), code, typescript.GetLanguage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("root node is nil")
	}
	if root.Type() != "program" {
		t.Errorf("root type = %q, want %q", root.Type(), "program")
	}
	if root.ChildCount() == 0 {
		t.Error("expected at least one child node")
	}
}

func TestPool_Parse_MultipleLanguages(t *testing.T) {
	p := NewPool(2)
	defer p.Shutdown()

	tests := []struct {
		langID string
		code   string
	}{
		{"typescript", `const x: number = 42;`},
		{"javascript", `const x = 42;`},
		{"python", `def hello(): pass`},
		{"go", `package main; func main() {}`},
		{"rust", `fn main() {}`},
		{"java", `class Main { public static void main(String[] args) {} }`},
		{"cpp", `int main() { return 0; }`},
	}

	for _, tt := range tests {
		t.Run(tt.langID, func(t *testing.T) {
			grammar := GetGrammar(tt.langID)
			if grammar == nil {
				t.Fatalf("no grammar for %s", tt.langID)
			}
			tree, err := p.Parse(context.Background(), []byte(tt.code), grammar)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			root := tree.RootNode()
			if root == nil {
				t.Fatal("root node is nil")
			}
		})
	}
}

func TestPool_Parse_EmptyContent(t *testing.T) {
	p := NewPool(1)
	defer p.Shutdown()

	tree, err := p.Parse(context.Background(), []byte{}, typescript.GetLanguage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("root node is nil")
	}
	if root.ChildCount() != 0 {
		t.Errorf("expected 0 children for empty content, got %d", root.ChildCount())
	}
}

func TestPool_Parse_SyntaxError(t *testing.T) {
	p := NewPool(1)
	defer p.Shutdown()

	code := []byte(`function( { ] }`)
	tree, err := p.Parse(context.Background(), code, typescript.GetLanguage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("root node is nil")
	}
	if !root.HasChanges() && root.ChildCount() == 0 {
		t.Error("expected partial tree with nodes for invalid syntax")
	}
}

func TestPool_Parse_Concurrent(t *testing.T) {
	poolSize := 2
	goroutines := 10
	p := NewPool(poolSize)
	defer p.Shutdown()

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code := []byte(`const x = 42;`)
			tree, err := p.Parse(context.Background(), code, javascript.GetLanguage())
			if err != nil {
				errs <- err
				return
			}
			if tree.RootNode() == nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent parse error: %v", err)
	}
}

func TestPool_Parse_ContextTimeout(t *testing.T) {
	p := NewPool(1)
	defer p.Shutdown()

	// Acquire the only parser to force the next call to block.
	parser := <-p.parsers
	defer func() { p.parsers <- parser }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Parse(ctx, []byte(`const x = 1;`), typescript.GetLanguage())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestPool_Parse_ContextCancelled(t *testing.T) {
	p := NewPool(1)
	defer p.Shutdown()

	// Acquire the only parser.
	parser := <-p.parsers
	defer func() { p.parsers <- parser }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := p.Parse(ctx, []byte(`const x = 1;`), typescript.GetLanguage())
	if err == nil {
		t.Fatal("expected cancel error, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected Canceled, got %v", err)
	}
}

func TestPool_Parse_NilLanguage(t *testing.T) {
	p := NewPool(1)
	defer p.Shutdown()

	_, err := p.Parse(context.Background(), []byte(`hello`), nil)
	if err == nil {
		t.Fatal("expected error for nil language, got nil")
	}
}

func TestPool_Shutdown(t *testing.T) {
	p := NewPool(3)
	p.Shutdown()

	_, err := p.Parse(context.Background(), []byte(`x`), typescript.GetLanguage())
	if err == nil {
		t.Fatal("expected error after shutdown, got nil")
	}
}

func TestPool_Shutdown_Idempotent(t *testing.T) {
	p := NewPool(2)
	p.Shutdown()
	p.Shutdown() // should not panic
}

func TestGetGrammar_AllRegistered(t *testing.T) {
	for _, id := range GrammarLanguageIDs() {
		t.Run(id, func(t *testing.T) {
			g := GetGrammar(id)
			if g == nil {
				t.Fatalf("GetGrammar(%q) returned nil", id)
			}
		})
	}
}

func TestHasGrammar(t *testing.T) {
	if !HasGrammar("typescript") {
		t.Error("expected HasGrammar(typescript) = true")
	}
	if HasGrammar("scss") {
		t.Error("expected HasGrammar(scss) = false")
	}
	if HasGrammar("graphql") {
		t.Error("expected HasGrammar(graphql) = false")
	}
	if HasGrammar("xml") {
		t.Error("expected HasGrammar(xml) = false")
	}
	if HasGrammar("json") {
		t.Error("expected HasGrammar(json) = false")
	}
}

func TestGrammarLanguageIDs_Sorted(t *testing.T) {
	ids := GrammarLanguageIDs()
	for i := 1; i < len(ids); i++ {
		if ids[i] < ids[i-1] {
			t.Errorf("not sorted: %q before %q", ids[i-1], ids[i])
		}
	}
}

func TestGrammar_SpecificLanguages(t *testing.T) {
	tests := []struct {
		langID   string
		code     string
		wantRoot string
	}{
		{"go", "package main", "source_file"},
		{"python", "x = 1", "module"},
		{"rust", "fn main() {}", "source_file"},
	}

	p := NewPool(1)
	defer p.Shutdown()

	for _, tt := range tests {
		t.Run(tt.langID, func(t *testing.T) {
			tree, err := p.Parse(context.Background(), []byte(tt.code), GetGrammar(tt.langID))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if got := tree.RootNode().Type(); got != tt.wantRoot {
				t.Errorf("root type = %q, want %q", got, tt.wantRoot)
			}
		})
	}
}

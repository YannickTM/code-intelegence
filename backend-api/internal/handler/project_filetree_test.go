package handler

import (
	"testing"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

func makeFile(path string) db.File {
	return db.File{
		FilePath: path,
		Language: pgtype.Text{String: "", Valid: false},
		SizeBytes: pgtype.Int8{Int64: 100, Valid: true},
	}
}

func TestBuildFileTree_Empty(t *testing.T) {
	root := buildFileTree(nil)
	if root.Path != "." || root.NodeType != "directory" {
		t.Fatalf("expected root directory, got %+v", root)
	}
	if len(root.Children) != 0 {
		t.Fatalf("expected 0 children, got %d", len(root.Children))
	}
}

func TestBuildFileTree_SingleFile(t *testing.T) {
	root := buildFileTree([]db.File{makeFile("README.md")})
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	child := root.Children[0]
	if child.Name != "README.md" || child.NodeType != "file" {
		t.Fatalf("unexpected child: %+v", child)
	}
}

func TestBuildFileTree_NestedPaths(t *testing.T) {
	files := []db.File{
		makeFile("src/main.go"),
		makeFile("src/utils/helper.go"),
	}
	root := buildFileTree(files)

	// root should have one child: src/
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child at root, got %d", len(root.Children))
	}
	src := root.Children[0]
	if src.Name != "src" || src.NodeType != "directory" {
		t.Fatalf("expected src directory, got %+v", src)
	}

	// src/ should have main.go and utils/
	if len(src.Children) != 2 {
		t.Fatalf("expected 2 children in src, got %d", len(src.Children))
	}
	// directories first: utils/ before main.go
	if src.Children[0].Name != "utils" || src.Children[0].NodeType != "directory" {
		t.Fatalf("expected utils directory first, got %+v", src.Children[0])
	}
	if src.Children[1].Name != "main.go" || src.Children[1].NodeType != "file" {
		t.Fatalf("expected main.go second, got %+v", src.Children[1])
	}

	// utils/ should have helper.go
	utils := src.Children[0]
	if len(utils.Children) != 1 || utils.Children[0].Name != "helper.go" {
		t.Fatalf("expected helper.go in utils, got %+v", utils.Children)
	}
}

func TestBuildFileTree_DirectoriesFirstSorting(t *testing.T) {
	files := []db.File{
		makeFile("a.txt"),
		makeFile("b/file.txt"),
		makeFile("c.txt"),
		makeFile("d/file.txt"),
	}
	root := buildFileTree(files)

	// Should be: b/, d/, a.txt, c.txt
	if len(root.Children) != 4 {
		t.Fatalf("expected 4 children, got %d", len(root.Children))
	}

	expected := []struct {
		name     string
		nodeType string
	}{
		{"b", "directory"},
		{"d", "directory"},
		{"a.txt", "file"},
		{"c.txt", "file"},
	}
	for i, exp := range expected {
		got := root.Children[i]
		if got.Name != exp.name || got.NodeType != exp.nodeType {
			t.Errorf("child[%d]: expected %s (%s), got %s (%s)", i, exp.name, exp.nodeType, got.Name, got.NodeType)
		}
	}
}

func TestBuildFileTree_DeeplyNested(t *testing.T) {
	files := []db.File{
		makeFile("a/b/c/d/e.txt"),
	}
	root := buildFileTree(files)

	node := root
	for _, expected := range []string{"a", "b", "c", "d"} {
		if len(node.Children) != 1 {
			t.Fatalf("expected 1 child at %s, got %d", node.Path, len(node.Children))
		}
		node = node.Children[0]
		if node.Name != expected || node.NodeType != "directory" {
			t.Fatalf("expected directory %s, got %+v", expected, node)
		}
	}
	if len(node.Children) != 1 || node.Children[0].Name != "e.txt" {
		t.Fatalf("expected e.txt leaf, got %+v", node.Children)
	}
}

func TestBuildFileTree_FileLanguageAndSize(t *testing.T) {
	f := db.File{
		FilePath:  "main.go",
		Language:  pgtype.Text{String: "go", Valid: true},
		SizeBytes: pgtype.Int8{Int64: 2048, Valid: true},
	}
	root := buildFileTree([]db.File{f})

	child := root.Children[0]
	if child.Language != "go" {
		t.Errorf("expected language 'go', got %q", child.Language)
	}
	if child.Size != 2048 {
		t.Errorf("expected size 2048, got %d", child.Size)
	}
}

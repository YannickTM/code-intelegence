package parser

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
)

// Pool manages a bounded set of tree-sitter parsers for concurrent use.
// sitter.Parser is NOT goroutine-safe; the pool guarantees exclusive access
// via a buffered channel.
type Pool struct {
	parsers chan *sitter.Parser
	size    int
	done    chan struct{} // closed by Shutdown to unblock waiting acquires
	closed  atomic.Bool
}

// NewPool creates a pool with the given number of pre-allocated parsers.
func NewPool(size int) *Pool {
	if size < 1 {
		size = 1
	}
	p := &Pool{
		parsers: make(chan *sitter.Parser, size),
		size:    size,
		done:    make(chan struct{}),
	}
	for range size {
		p.parsers <- sitter.NewParser()
	}
	return p
}

// Parse acquires a parser from the pool, sets the language, parses content,
// and returns the parser to the pool. It blocks when all parsers are in use
// and respects context cancellation.
func (p *Pool) Parse(ctx context.Context, content []byte, lang *sitter.Language) (*sitter.Tree, error) {
	if lang == nil {
		return nil, errors.New("parser: language must not be nil")
	}
	if p.closed.Load() {
		return nil, errors.New("parser: pool is shut down")
	}

	// Acquire a parser, respecting context cancellation and shutdown.
	var parser *sitter.Parser
	select {
	case parser = <-p.parsers:
	case <-p.done:
		return nil, errors.New("parser: pool is shut down")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Re-check after acquiring: select is non-deterministic when multiple
	// cases are ready, so we may have taken a parser despite shutdown or
	// context cancellation.
	if p.closed.Load() {
		p.parsers <- parser
		return nil, errors.New("parser: pool is shut down")
	}
	if err := ctx.Err(); err != nil {
		p.parsers <- parser
		return nil, err
	}

	// From here on, always return the parser to the pool.
	defer func() { p.parsers <- parser }()

	// Reset clears the parser's incremental-parse state so it always parses
	// from scratch.
	parser.Reset()
	parser.SetLanguage(lang)
	parseCtx, stopParseCtx := stableParseContext(ctx)
	defer stopParseCtx()

	tree, err := parser.ParseCtx(parseCtx, nil, content)

	// go-tree-sitter's ParseCtx spawns a goroutine that watches the parse
	// context and sets the parser's internal cancellation flag when fired.
	// Yielding here gives that goroutine a chance to exit cleanly via the
	// parseComplete path before the parser is reused.
	runtime.Gosched()

	if err != nil {
		return nil, err
	}
	return tree, nil
}

// stableParseContext prevents caller-driven cancellation from firing after
// ParseCtx returns, which can otherwise poison a parser that has already been
// returned to the pool and reused.
func stableParseContext(ctx context.Context) (context.Context, func()) {
	parseCtx, cancel := context.WithCancel(context.Background())
	stop := make(chan struct{})

	go func() {
		var (
			timer   *time.Timer
			timerCh <-chan time.Time
		)
		if deadline, ok := ctx.Deadline(); ok {
			timer = time.NewTimer(time.Until(deadline))
			timerCh = timer.C
		}
		defer func() {
			if timer == nil {
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}()

		select {
		case <-ctx.Done():
			cancel()
		case <-timerCh:
			cancel()
		case <-stop:
		}
	}()

	return parseCtx, func() {
		close(stop)
	}
}

// Size returns the number of parsers in the pool.
func (p *Pool) Size() int { return p.size }

// Shutdown closes all parsers in the pool. After Shutdown returns, Parse
// calls will return an error.
func (p *Pool) Shutdown() {
	if !p.closed.CompareAndSwap(false, true) {
		return // already shut down
	}
	close(p.done) // unblock any Parse calls waiting to acquire a parser
	// Drain all parsers and close each one.
	for range p.size {
		parser := <-p.parsers
		parser.Close()
	}
}

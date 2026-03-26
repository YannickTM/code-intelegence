package health

import (
	"context"
	"testing"
)

func TestPostgresChecker_NilDB(t *testing.T) {
	c := NewPostgresChecker(nil)

	if c.Name() != "postgres" {
		t.Errorf("Name() = %q, want %q", c.Name(), "postgres")
	}

	result := c.Check(context.Background())
	if result.Status != StatusSkipped {
		t.Errorf("Status = %q, want %q", result.Status, StatusSkipped)
	}
}

func TestStubChecker(t *testing.T) {
	tests := []string{"redis", "ollama"}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			c := NewStubChecker(name)

			if c.Name() != name {
				t.Errorf("Name() = %q, want %q", c.Name(), name)
			}

			result := c.Check(context.Background())
			if result.Status != StatusSkipped {
				t.Errorf("Status = %q, want %q", result.Status, StatusSkipped)
			}
		})
	}
}

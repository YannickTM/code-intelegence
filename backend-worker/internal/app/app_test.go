package app

import (
	"strings"
	"testing"

	"myjungle/backend-worker/internal/config"
)

func TestNew_FailsOnBadDSN(t *testing.T) {
	cfg := config.LoadForTest()
	cfg.Postgres.DSN = "not-a-valid-dsn"

	a, err := New(cfg)
	if a != nil {
		defer a.Close()
	}
	if err == nil {
		t.Fatal("expected error for invalid DSN, got nil")
	}
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("error = %q, want mention of 'database'", err.Error())
	}
}

func TestClose_Idempotent(t *testing.T) {
	// Calling Close on a partially constructed App should not panic.
	a := &App{}
	a.Close()
	a.Close() // second call should also be safe
}

func TestClose_Nil(t *testing.T) {
	var a *App
	a.Close() // nil receiver should not panic
}

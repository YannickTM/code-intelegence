package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestJSONHandler_Output(t *testing.T) {
	var buf bytes.Buffer
	// Create a JSON handler writing to buf so we can inspect output.
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	l := slog.New(h)
	l.Info("hello", slog.String("key", "value"))

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	for _, field := range []string{"time", "level", "msg"} {
		if _, ok := m[field]; !ok {
			t.Errorf("JSON output missing %q field", field)
		}
	}
	if m["msg"] != "hello" {
		t.Errorf("msg = %v, want %q", m["msg"], "hello")
	}
	if m["key"] != "value" {
		t.Errorf("key = %v, want %q", m["key"], "value")
	}
}

func TestTextHandler_Output(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	l := slog.New(h)
	l.Info("hello", slog.String("key", "value"))

	out := buf.String()
	if !strings.Contains(out, "level=INFO") {
		t.Errorf("text output missing level=INFO: %s", out)
	}
	if !strings.Contains(out, `msg=hello`) {
		t.Errorf("text output missing msg=hello: %s", out)
	}
	if !strings.Contains(out, `key=value`) {
		t.Errorf("text output missing key=value: %s", out)
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	l := slog.New(h)
	l.Debug("should be suppressed")

	if buf.Len() != 0 {
		t.Errorf("DEBUG message should be suppressed at INFO level, got: %s", buf.String())
	}

	l.Info("should appear")
	if buf.Len() == 0 {
		t.Error("INFO message should appear at INFO level")
	}
}

func TestNew_SetsDefault(t *testing.T) {
	prev := slog.Default()
	defer slog.SetDefault(prev)

	l := New(Config{Level: "debug", Format: "text"})
	if slog.Default() != l {
		t.Error("New() should set the slog default logger")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		if got := parseLevel(tt.input); got != tt.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFromContext_Default(t *testing.T) {
	ctx := context.Background()
	l := FromContext(ctx)
	if l != slog.Default() {
		t.Error("FromContext should return slog.Default() when no logger in context")
	}
}

func TestFromContext_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, nil)
	custom := slog.New(h)

	ctx := WithLogger(context.Background(), custom)
	l := FromContext(ctx)
	if l != custom {
		t.Error("FromContext should return the logger stored via WithLogger")
	}
}

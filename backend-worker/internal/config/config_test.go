package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// setRequiredEnvs sets the minimum env vars needed for Load() to succeed.
func setRequiredEnvs(t *testing.T) {
	t.Helper()
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/testdb?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("SSH_KEY_ENCRYPTION_SECRET", "test-secret")
}

// clearOptionalEnvs ensures optional env vars are empty so defaults apply.
func clearOptionalEnvs(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"LOG_LEVEL", "LOG_FORMAT",
		"POSTGRES_MAX_CONNS", "POSTGRES_MIN_CONNS", "POSTGRES_MAX_CONN_LIFE",
		"QUEUE_NAME", "WORKER_CONCURRENCY", "QUEUE_SHUTDOWN_TIMEOUT",
		"PARSER_POOL_SIZE", "PARSER_TIMEOUT", "PARSER_TIMEOUT_PER_FILE", "PARSER_MAX_FILE_SIZE",
		"OLLAMA_URL", "OLLAMA_MODEL",
		"QDRANT_URL",
		"REPO_CACHE_DIR",
	} {
		t.Setenv(key, "")
	}
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredEnvs(t)
	clearOptionalEnvs(t)

	cfg := Load()

	// Log defaults.
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}

	// Postgres defaults.
	if cfg.Postgres.MaxConns != DefaultPostgresMaxConns {
		t.Errorf("Postgres.MaxConns = %d, want %d", cfg.Postgres.MaxConns, DefaultPostgresMaxConns)
	}
	if cfg.Postgres.MinConns != DefaultPostgresMinConns {
		t.Errorf("Postgres.MinConns = %d, want %d", cfg.Postgres.MinConns, DefaultPostgresMinConns)
	}
	if cfg.Postgres.MaxConnLife != DefaultPostgresMaxConnLife {
		t.Errorf("Postgres.MaxConnLife = %v, want %v", cfg.Postgres.MaxConnLife, DefaultPostgresMaxConnLife)
	}

	// Queue defaults.
	if cfg.Queue.Name != DefaultQueueName {
		t.Errorf("Queue.Name = %q, want %q", cfg.Queue.Name, DefaultQueueName)
	}
	if cfg.Queue.Concurrency != DefaultWorkerConcurrency {
		t.Errorf("Queue.Concurrency = %d, want %d", cfg.Queue.Concurrency, DefaultWorkerConcurrency)
	}
	if cfg.Queue.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("Queue.ShutdownTimeout = %v, want %v", cfg.Queue.ShutdownTimeout, DefaultShutdownTimeout)
	}

	// Parser defaults.
	if cfg.Parser.PoolSize != DefaultParserPoolSize {
		t.Errorf("Parser.PoolSize = %d, want %d", cfg.Parser.PoolSize, DefaultParserPoolSize)
	}
	if cfg.Parser.Timeout != DefaultParserTimeout {
		t.Errorf("Parser.Timeout = %v, want %v", cfg.Parser.Timeout, DefaultParserTimeout)
	}
	if cfg.Parser.TimeoutPerFile != DefaultParserTimeoutPerFile {
		t.Errorf("Parser.TimeoutPerFile = %v, want %v", cfg.Parser.TimeoutPerFile, DefaultParserTimeoutPerFile)
	}
	if cfg.Parser.MaxFileSize != DefaultParserMaxFileSize {
		t.Errorf("Parser.MaxFileSize = %d, want %d", cfg.Parser.MaxFileSize, DefaultParserMaxFileSize)
	}

	// Embedding defaults.
	if cfg.Embedding.OllamaURL != DefaultOllamaURL {
		t.Errorf("Embedding.OllamaURL = %q, want %q", cfg.Embedding.OllamaURL, DefaultOllamaURL)
	}
	if cfg.Embedding.OllamaModel != DefaultOllamaModel {
		t.Errorf("Embedding.OllamaModel = %q, want %q", cfg.Embedding.OllamaModel, DefaultOllamaModel)
	}

	// Qdrant defaults.
	if cfg.Qdrant.URL != DefaultQdrantURL {
		t.Errorf("Qdrant.URL = %q, want %q", cfg.Qdrant.URL, DefaultQdrantURL)
	}

	// Workspace defaults.
	if cfg.Workspace.RepoCacheDir != DefaultRepoCacheDir {
		t.Errorf("Workspace.RepoCacheDir = %q, want %q", cfg.Workspace.RepoCacheDir, DefaultRepoCacheDir)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnvs(t)
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("POSTGRES_MAX_CONNS", "20")
	t.Setenv("POSTGRES_MIN_CONNS", "5")
	t.Setenv("POSTGRES_MAX_CONN_LIFE", "15m")
	t.Setenv("QUEUE_NAME", "indexing")
	t.Setenv("WORKER_CONCURRENCY", "8")
	t.Setenv("QUEUE_SHUTDOWN_TIMEOUT", "1m")
	t.Setenv("PARSER_POOL_SIZE", "8")
	t.Setenv("PARSER_TIMEOUT", "1m")
	t.Setenv("PARSER_TIMEOUT_PER_FILE", "15s")
	t.Setenv("PARSER_MAX_FILE_SIZE", "5242880")
	t.Setenv("OLLAMA_URL", "http://localhost:11434")
	t.Setenv("OLLAMA_MODEL", "custom-model")
	t.Setenv("QDRANT_URL", "http://localhost:6333")
	t.Setenv("REPO_CACHE_DIR", "/custom/repos")

	cfg := Load()

	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "text")
	}
	if cfg.Postgres.MaxConns != 20 {
		t.Errorf("Postgres.MaxConns = %d, want 20", cfg.Postgres.MaxConns)
	}
	if cfg.Postgres.MinConns != 5 {
		t.Errorf("Postgres.MinConns = %d, want 5", cfg.Postgres.MinConns)
	}
	if cfg.Postgres.MaxConnLife != 15*time.Minute {
		t.Errorf("Postgres.MaxConnLife = %v, want 15m", cfg.Postgres.MaxConnLife)
	}
	if cfg.Queue.Name != "indexing" {
		t.Errorf("Queue.Name = %q, want %q", cfg.Queue.Name, "indexing")
	}
	if cfg.Queue.Concurrency != 8 {
		t.Errorf("Queue.Concurrency = %d, want 8", cfg.Queue.Concurrency)
	}
	if cfg.Queue.ShutdownTimeout != 1*time.Minute {
		t.Errorf("Queue.ShutdownTimeout = %v, want 1m", cfg.Queue.ShutdownTimeout)
	}
	if cfg.Parser.PoolSize != 8 {
		t.Errorf("Parser.PoolSize = %d, want 8", cfg.Parser.PoolSize)
	}
	if cfg.Parser.Timeout != 1*time.Minute {
		t.Errorf("Parser.Timeout = %v, want 1m", cfg.Parser.Timeout)
	}
	if cfg.Parser.TimeoutPerFile != 15*time.Second {
		t.Errorf("Parser.TimeoutPerFile = %v, want 15s", cfg.Parser.TimeoutPerFile)
	}
	if cfg.Parser.MaxFileSize != 5242880 {
		t.Errorf("Parser.MaxFileSize = %d, want 5242880", cfg.Parser.MaxFileSize)
	}
	if cfg.Embedding.OllamaURL != "http://localhost:11434" {
		t.Errorf("Embedding.OllamaURL = %q, want %q", cfg.Embedding.OllamaURL, "http://localhost:11434")
	}
	if cfg.Embedding.OllamaModel != "custom-model" {
		t.Errorf("Embedding.OllamaModel = %q, want %q", cfg.Embedding.OllamaModel, "custom-model")
	}
	if cfg.Qdrant.URL != "http://localhost:6333" {
		t.Errorf("Qdrant.URL = %q, want %q", cfg.Qdrant.URL, "http://localhost:6333")
	}
	if cfg.Workspace.RepoCacheDir != "/custom/repos" {
		t.Errorf("Workspace.RepoCacheDir = %q, want %q", cfg.Workspace.RepoCacheDir, "/custom/repos")
	}
}

func TestLoad_MissingPostgresDSN(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("SSH_KEY_ENCRYPTION_SECRET", "test-secret")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing POSTGRES_DSN")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "POSTGRES_DSN") {
			t.Errorf("panic message = %v, want mention of POSTGRES_DSN", r)
		}
	}()
	Load()
}

func TestLoad_MissingRedisURL(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/testdb?sslmode=disable")
	t.Setenv("REDIS_URL", "")
	t.Setenv("SSH_KEY_ENCRYPTION_SECRET", "test-secret")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing REDIS_URL")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "REDIS_URL") {
			t.Errorf("panic message = %v, want mention of REDIS_URL", r)
		}
	}()
	Load()
}

func TestLoad_MissingSSHSecret(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/testdb?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("SSH_KEY_ENCRYPTION_SECRET", "")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing SSH_KEY_ENCRYPTION_SECRET")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "SSH_KEY_ENCRYPTION_SECRET") {
			t.Errorf("panic message = %v, want mention of SSH_KEY_ENCRYPTION_SECRET", r)
		}
	}()
	Load()
}

func TestLoad_InvalidConcurrency(t *testing.T) {
	setRequiredEnvs(t)
	t.Setenv("WORKER_CONCURRENCY", "0")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for WORKER_CONCURRENCY=0")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "WORKER_CONCURRENCY") {
			t.Errorf("panic message = %v, want mention of WORKER_CONCURRENCY", r)
		}
	}()
	Load()
}

func TestLoad_MinConnsExceedsMaxConns(t *testing.T) {
	setRequiredEnvs(t)
	t.Setenv("POSTGRES_MAX_CONNS", "2")
	t.Setenv("POSTGRES_MIN_CONNS", "5")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for MinConns > MaxConns")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "POSTGRES_MIN_CONNS") {
			t.Errorf("panic message = %v, want mention of POSTGRES_MIN_CONNS", r)
		}
	}()
	Load()
}

func TestLoadForTest(t *testing.T) {
	cfg := LoadForTest()
	if cfg == nil {
		t.Fatal("LoadForTest returned nil")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("LoadForTest Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Queue.Concurrency < 1 {
		t.Errorf("LoadForTest Queue.Concurrency = %d, want positive", cfg.Queue.Concurrency)
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("TEST_KEY_SET", "hello")
	t.Setenv("TEST_KEY_EMPTY", "")
	t.Setenv("TEST_KEY_SPACES", "   ")

	if got := envOrDefault("TEST_KEY_SET", "fallback"); got != "hello" {
		t.Errorf("envOrDefault(set) = %q, want %q", got, "hello")
	}
	if got := envOrDefault("TEST_KEY_EMPTY", "fallback"); got != "fallback" {
		t.Errorf("envOrDefault(empty) = %q, want %q", got, "fallback")
	}
	if got := envOrDefault("TEST_KEY_SPACES", "fallback"); got != "fallback" {
		t.Errorf("envOrDefault(spaces) = %q, want %q", got, "fallback")
	}
	if got := envOrDefault("TEST_KEY_UNSET", "fallback"); got != "fallback" {
		t.Errorf("envOrDefault(unset) = %q, want %q", got, "fallback")
	}
}

func TestParseInt(t *testing.T) {
	if got := parseInt("42", 0); got != 42 {
		t.Errorf("parseInt(42) = %d, want 42", got)
	}
	if got := parseInt("notanumber", 10); got != 10 {
		t.Errorf("parseInt(invalid) = %d, want fallback 10", got)
	}
	if got := parseInt("", 5); got != 5 {
		t.Errorf("parseInt(empty) = %d, want fallback 5", got)
	}
}

func TestParseDuration(t *testing.T) {
	if got := parseDuration("5m", 0); got != 5*time.Minute {
		t.Errorf("parseDuration(5m) = %v, want 5m", got)
	}
	if got := parseDuration("invalid", 10*time.Second); got != 10*time.Second {
		t.Errorf("parseDuration(invalid) = %v, want fallback 10s", got)
	}
	if got := parseDuration("", 30*time.Second); got != 30*time.Second {
		t.Errorf("parseDuration(empty) = %v, want fallback 30s", got)
	}
}

func TestRequiredEnv_Panics(t *testing.T) {
	t.Setenv("REQUIRED_TEST_KEY", "")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty required env")
		}
	}()
	requiredEnv("REQUIRED_TEST_KEY")
}

func TestRequiredEnv_ReturnsValue(t *testing.T) {
	t.Setenv("REQUIRED_TEST_KEY", "myvalue")

	if got := requiredEnv("REQUIRED_TEST_KEY"); got != "myvalue" {
		t.Errorf("requiredEnv = %q, want %q", got, "myvalue")
	}
}

func TestRedactDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "url style with password",
			dsn:  "postgres://user:secret@localhost:5432/db?sslmode=disable",
			want: "postgres://user:***@localhost:5432/db?sslmode=disable",
		},
		{
			name: "url style without password",
			dsn:  "postgres://localhost:5432/db",
			want: "postgres://localhost:5432/db",
		},
		{
			name: "key value unquoted",
			dsn:  "host=localhost password=secret dbname=mydb",
			want: "host=localhost password=*** dbname=mydb",
		},
		{
			name: "key value single quoted",
			dsn:  "host=localhost password='secret' dbname=mydb",
			want: "host=localhost password='***' dbname=mydb",
		},
		{
			name: "empty",
			dsn:  "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactDSN(tt.dsn)
			if got != tt.want {
				t.Errorf("redactDSN(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "with userinfo",
			raw:  "redis://user:pass@localhost:6379/0",
			want: "redis://localhost:6379/0",
		},
		{
			name: "without userinfo",
			raw:  "http://localhost:6333",
			want: "http://localhost:6333",
		},
		{
			name: "with query stripped",
			raw:  "http://localhost:6333?token=abc",
			want: "http://localhost:6333",
		},
		{
			name: "with fragment stripped",
			raw:  "http://localhost:6333#section",
			want: "http://localhost:6333",
		},
		{
			name: "empty",
			raw:  "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactURL(tt.raw)
			if got != tt.want {
				t.Errorf("redactURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestLogSummary_NoSecrets(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(orig)

	cfg := &Config{
		Log: LogConfig{Level: "info", Format: "json"},
		Postgres: PostgresConfig{
			DSN:         "postgres://user:supersecret@localhost:5432/db?sslmode=disable",
			MaxConns:    10,
			MinConns:    2,
			MaxConnLife: 30 * time.Minute,
		},
		Redis: RedisConfig{
			URL: "redis://authuser:redispass@localhost:6379/0",
		},
		Queue: QueueConfig{
			Name:            "default",
			Concurrency:     4,
			ShutdownTimeout: 30 * time.Second,
		},
		Parser: ParserConfig{
			PoolSize:       4,
			Timeout:        30 * time.Second,
			TimeoutPerFile: 30 * time.Second,
			MaxFileSize:    10 * 1024 * 1024,
		},
		Embedding: EmbeddingConfig{
			OllamaURL:   "http://localhost:11434",
			OllamaModel: "jina/jina-embeddings-v2-base-en",
		},
		Qdrant: QdrantConfig{
			URL: "http://localhost:6333",
		},
		SSH: SSHConfig{
			EncryptionSecret: "my-super-secret-key",
		},
		Workspace: WorkspaceConfig{
			RepoCacheDir: "/var/lib/myjungle/repos",
		},
	}

	logSummary(cfg)

	output := buf.String()
	if strings.Contains(output, "supersecret") {
		t.Error("log output contains Postgres password 'supersecret'")
	}
	if strings.Contains(output, "redispass") {
		t.Error("log output contains Redis password 'redispass'")
	}
	if strings.Contains(output, "my-super-secret-key") {
		t.Error("log output contains SSH encryption secret")
	}
	if !strings.Contains(output, "***") {
		t.Error("log output does not contain redacted markers")
	}
}

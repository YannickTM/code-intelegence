package config

import (
	"strings"
	"testing"
	"time"
)

// setRequiredEnvs sets the required env vars to prevent Load() from panicking.
func setRequiredEnvs(t *testing.T) {
	t.Helper()
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/testdb?sslmode=disable")
	t.Setenv("SSH_KEY_ENCRYPTION_SECRET", "test-secret")
	t.Setenv("PROVIDER_ENCRYPTION_SECRET", "provider-secret")
}

// clearOptionalEnvs clears all optional config env vars so defaults apply.
func clearOptionalEnvs(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SERVER_PORT", "SERVER_READ_TIMEOUT", "SERVER_WRITE_TIMEOUT",
		"SERVER_IDLE_TIMEOUT", "SERVER_SHUTDOWN_TIMEOUT", "SERVER_READ_HEADER_TIMEOUT",
		"CORS_ALLOWED_ORIGINS", "CORS_WILDCARD", "SERVER_BODY_LIMIT_BYTES",
		"POSTGRES_MAX_CONNS", "POSTGRES_MIN_CONNS", "POSTGRES_MAX_CONN_LIFE",
		"REDIS_URL", "REDIS_POOL_SIZE",
		"EMBEDDING_PROVIDER", "OLLAMA_URL", "OLLAMA_MODEL", "OLLAMA_DIMENSIONS", "EMBED_BATCH_SIZE",
		"LLM_PROVIDER", "LLM_URL", "LLM_MODEL",
		"INDEXING_DEFAULT_BRANCH", "INDEXING_MAX_PARALLEL_FILES", "INDEXING_MAX_CHUNK_TOKENS", "REPO_CACHE_DIR",
		"WORKER_CONCURRENCY", "INDEX_FULL_TIMEOUT", "INDEX_INCREMENTAL_TIMEOUT", "MAX_RETRIES", "UNIQUE_WINDOW",
		"SSE_KEEPALIVE_INTERVAL", "MAX_SSE_CONNECTIONS",
		"SESSION_TTL", "SESSION_COOKIE_NAME", "SESSION_SECURE_COOKIE",
	} {
		t.Setenv(key, "")
	}
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredEnvs(t)
	clearOptionalEnvs(t)

	cfg := Load()

	// Server
	if cfg.Server.Port != DefaultServerPort {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, DefaultServerPort)
	}
	if cfg.Server.ReadTimeout != DefaultReadTimeout {
		t.Errorf("Server.ReadTimeout = %v, want %v", cfg.Server.ReadTimeout, DefaultReadTimeout)
	}
	if cfg.Server.WriteTimeout != DefaultWriteTimeout {
		t.Errorf("Server.WriteTimeout = %v, want %v", cfg.Server.WriteTimeout, DefaultWriteTimeout)
	}
	if cfg.Server.IdleTimeout != DefaultIdleTimeout {
		t.Errorf("Server.IdleTimeout = %v, want %v", cfg.Server.IdleTimeout, DefaultIdleTimeout)
	}
	if cfg.Server.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("Server.ShutdownTimeout = %v, want %v", cfg.Server.ShutdownTimeout, DefaultShutdownTimeout)
	}
	if cfg.Server.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Errorf("Server.ReadHeaderTimeout = %v, want %v", cfg.Server.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	}
	if len(cfg.Server.CORSAllowedOrigins) != 0 {
		t.Errorf("Server.CORSAllowedOrigins = %v, want empty", cfg.Server.CORSAllowedOrigins)
	}
	if cfg.Server.CORSWildcard {
		t.Error("Server.CORSWildcard = true, want false")
	}
	if cfg.Server.BodyLimitBytes != DefaultBodyLimitBytes {
		t.Errorf("Server.BodyLimitBytes = %d, want %d", cfg.Server.BodyLimitBytes, DefaultBodyLimitBytes)
	}

	// Postgres (DSN from required env)
	if cfg.Postgres.DSN != "postgres://test:test@localhost:5432/testdb?sslmode=disable" {
		t.Errorf("Postgres.DSN = %q, unexpected", cfg.Postgres.DSN)
	}
	if cfg.Postgres.MaxConns != DefaultPostgresMaxConns {
		t.Errorf("Postgres.MaxConns = %d, want %d", cfg.Postgres.MaxConns, DefaultPostgresMaxConns)
	}
	if cfg.Postgres.MinConns != DefaultPostgresMinConns {
		t.Errorf("Postgres.MinConns = %d, want %d", cfg.Postgres.MinConns, DefaultPostgresMinConns)
	}
	if cfg.Postgres.MaxConnLife != DefaultPostgresMaxConnLife {
		t.Errorf("Postgres.MaxConnLife = %v, want %v", cfg.Postgres.MaxConnLife, DefaultPostgresMaxConnLife)
	}

	// Redis
	if cfg.Redis.URL != "" {
		t.Errorf("Redis.URL = %q, want empty", cfg.Redis.URL)
	}
	if cfg.Redis.PoolSize != DefaultRedisPoolSize {
		t.Errorf("Redis.PoolSize = %d, want %d", cfg.Redis.PoolSize, DefaultRedisPoolSize)
	}

	// SSH (from required env)
	if cfg.SSH.EncryptionSecret != "test-secret" {
		t.Errorf("SSH.EncryptionSecret = %q, unexpected", cfg.SSH.EncryptionSecret)
	}
	if cfg.ProviderEncryptionSecret != "provider-secret" {
		t.Errorf("ProviderEncryptionSecret = %q, want %q", cfg.ProviderEncryptionSecret, "provider-secret")
	}

	// Embedding
	if cfg.Embedding.Provider != DefaultEmbeddingProvider {
		t.Errorf("Embedding.Provider = %q, want %q", cfg.Embedding.Provider, DefaultEmbeddingProvider)
	}
	if cfg.Embedding.EndpointURL != DefaultEmbeddingEndpointURL {
		t.Errorf("Embedding.EndpointURL = %q, want %q", cfg.Embedding.EndpointURL, DefaultEmbeddingEndpointURL)
	}
	if cfg.Embedding.Model != DefaultEmbeddingModel {
		t.Errorf("Embedding.Model = %q, want %q", cfg.Embedding.Model, DefaultEmbeddingModel)
	}
	if cfg.Embedding.Dimensions != DefaultEmbeddingDimensions {
		t.Errorf("Embedding.Dimensions = %d, want %d", cfg.Embedding.Dimensions, DefaultEmbeddingDimensions)
	}
	if cfg.Embedding.BatchSize != DefaultEmbeddingBatchSize {
		t.Errorf("Embedding.BatchSize = %d, want %d", cfg.Embedding.BatchSize, DefaultEmbeddingBatchSize)
	}
	if cfg.LLM.Provider != DefaultLLMProvider {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, DefaultLLMProvider)
	}
	if cfg.LLM.EndpointURL != DefaultLLMEndpointURL {
		t.Errorf("LLM.EndpointURL = %q, want %q", cfg.LLM.EndpointURL, DefaultLLMEndpointURL)
	}
	if cfg.LLM.Model != DefaultLLMModel {
		t.Errorf("LLM.Model = %q, want %q", cfg.LLM.Model, DefaultLLMModel)
	}

	// Indexing
	if cfg.Indexing.DefaultBranch != DefaultIndexingDefaultBranch {
		t.Errorf("Indexing.DefaultBranch = %q, want %q", cfg.Indexing.DefaultBranch, DefaultIndexingDefaultBranch)
	}
	if cfg.Indexing.MaxParallelFiles != DefaultIndexingMaxParallelFiles {
		t.Errorf("Indexing.MaxParallelFiles = %d, want %d", cfg.Indexing.MaxParallelFiles, DefaultIndexingMaxParallelFiles)
	}
	if cfg.Indexing.MaxChunkTokens != DefaultIndexingMaxChunkTokens {
		t.Errorf("Indexing.MaxChunkTokens = %d, want %d", cfg.Indexing.MaxChunkTokens, DefaultIndexingMaxChunkTokens)
	}
	if cfg.Indexing.RepoCacheDir != DefaultRepoCacheDir {
		t.Errorf("Indexing.RepoCacheDir = %q, want %q", cfg.Indexing.RepoCacheDir, DefaultRepoCacheDir)
	}

	// Jobs
	if cfg.Jobs.WorkerConcurrency != DefaultWorkerConcurrency {
		t.Errorf("Jobs.WorkerConcurrency = %d, want %d", cfg.Jobs.WorkerConcurrency, DefaultWorkerConcurrency)
	}
	if cfg.Jobs.IndexFullTimeout != DefaultIndexFullTimeout {
		t.Errorf("Jobs.IndexFullTimeout = %v, want %v", cfg.Jobs.IndexFullTimeout, DefaultIndexFullTimeout)
	}
	if cfg.Jobs.IndexIncrementalTimeout != DefaultIndexIncrementalTimeout {
		t.Errorf("Jobs.IndexIncrementalTimeout = %v, want %v", cfg.Jobs.IndexIncrementalTimeout, DefaultIndexIncrementalTimeout)
	}
	if cfg.Jobs.MaxRetries != DefaultMaxRetries {
		t.Errorf("Jobs.MaxRetries = %d, want %d", cfg.Jobs.MaxRetries, DefaultMaxRetries)
	}
	if cfg.Jobs.UniqueWindow != DefaultUniqueWindow {
		t.Errorf("Jobs.UniqueWindow = %v, want %v", cfg.Jobs.UniqueWindow, DefaultUniqueWindow)
	}

	// Events
	if cfg.Events.SSEKeepaliveInterval != DefaultSSEKeepaliveInterval {
		t.Errorf("Events.SSEKeepaliveInterval = %v, want %v", cfg.Events.SSEKeepaliveInterval, DefaultSSEKeepaliveInterval)
	}
	if cfg.Events.MaxSSEConnections != DefaultMaxSSEConnections {
		t.Errorf("Events.MaxSSEConnections = %d, want %d", cfg.Events.MaxSSEConnections, DefaultMaxSSEConnections)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnvs(t)
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("SERVER_READ_TIMEOUT", "10s")
	// SERVER_WRITE_TIMEOUT must stay 0 (unlimited) when SSE is enabled;
	// validate that the default is preserved when not overridden.
	t.Setenv("SERVER_IDLE_TIMEOUT", "60s")
	t.Setenv("SERVER_SHUTDOWN_TIMEOUT", "5s")
	t.Setenv("SERVER_READ_HEADER_TIMEOUT", "2s")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000, http://example.com")
	t.Setenv("CORS_WILDCARD", "true")
	t.Setenv("SERVER_BODY_LIMIT_BYTES", "2097152")
	t.Setenv("POSTGRES_MAX_CONNS", "50")
	t.Setenv("POSTGRES_MIN_CONNS", "10")
	t.Setenv("POSTGRES_MAX_CONN_LIFE", "1h")
	t.Setenv("REDIS_URL", "redis://custom:6380/1")
	t.Setenv("REDIS_POOL_SIZE", "20")
	t.Setenv("PROVIDER_ENCRYPTION_SECRET", "provider-secret")
	t.Setenv("OLLAMA_URL", "http://custom-ollama:11434")
	t.Setenv("OLLAMA_MODEL", "custom-model")
	t.Setenv("OLLAMA_DIMENSIONS", "512")
	t.Setenv("EMBED_BATCH_SIZE", "128")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("LLM_URL", "https://custom-llm:11434")
	t.Setenv("LLM_MODEL", "llama3.1")
	t.Setenv("INDEXING_DEFAULT_BRANCH", "develop")
	t.Setenv("INDEXING_MAX_PARALLEL_FILES", "16")
	t.Setenv("INDEXING_MAX_CHUNK_TOKENS", "1024")
	t.Setenv("REPO_CACHE_DIR", "/custom/repos")
	t.Setenv("WORKER_CONCURRENCY", "8")
	t.Setenv("INDEX_FULL_TIMEOUT", "1h")
	t.Setenv("INDEX_INCREMENTAL_TIMEOUT", "20m")
	t.Setenv("MAX_RETRIES", "5")
	t.Setenv("UNIQUE_WINDOW", "2h")
	t.Setenv("SSE_KEEPALIVE_INTERVAL", "15s")
	t.Setenv("MAX_SSE_CONNECTIONS", "200")

	cfg := Load()

	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 10*time.Second {
		t.Errorf("Server.ReadTimeout = %v, want 10s", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != DefaultWriteTimeout {
		t.Errorf("Server.WriteTimeout = %v, want %v", cfg.Server.WriteTimeout, DefaultWriteTimeout)
	}
	if cfg.Server.IdleTimeout != 60*time.Second {
		t.Errorf("Server.IdleTimeout = %v, want 1m", cfg.Server.IdleTimeout)
	}
	if cfg.Server.ShutdownTimeout != 5*time.Second {
		t.Errorf("Server.ShutdownTimeout = %v, want 5s", cfg.Server.ShutdownTimeout)
	}
	if cfg.Server.ReadHeaderTimeout != 2*time.Second {
		t.Errorf("Server.ReadHeaderTimeout = %v, want 2s", cfg.Server.ReadHeaderTimeout)
	}
	if len(cfg.Server.CORSAllowedOrigins) != 2 {
		t.Fatalf("CORSAllowedOrigins len = %d, want 2", len(cfg.Server.CORSAllowedOrigins))
	}
	if cfg.Server.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("CORSAllowedOrigins[0] = %q", cfg.Server.CORSAllowedOrigins[0])
	}
	if cfg.Server.CORSAllowedOrigins[1] != "http://example.com" {
		t.Errorf("CORSAllowedOrigins[1] = %q, want %q", cfg.Server.CORSAllowedOrigins[1], "http://example.com")
	}
	if !cfg.Server.CORSWildcard {
		t.Error("CORSWildcard = false, want true")
	}
	if cfg.Server.BodyLimitBytes != 2097152 {
		t.Errorf("Server.BodyLimitBytes = %d, want 2097152", cfg.Server.BodyLimitBytes)
	}
	if cfg.Postgres.MaxConns != 50 {
		t.Errorf("Postgres.MaxConns = %d, want 50", cfg.Postgres.MaxConns)
	}
	if cfg.Postgres.MinConns != 10 {
		t.Errorf("Postgres.MinConns = %d, want 10", cfg.Postgres.MinConns)
	}
	if cfg.Postgres.MaxConnLife != 1*time.Hour {
		t.Errorf("Postgres.MaxConnLife = %v, want 1h", cfg.Postgres.MaxConnLife)
	}
	if cfg.Redis.URL != "redis://custom:6380/1" {
		t.Errorf("Redis.URL = %q", cfg.Redis.URL)
	}
	if cfg.Redis.PoolSize != 20 {
		t.Errorf("Redis.PoolSize = %d, want 20", cfg.Redis.PoolSize)
	}
	if cfg.ProviderEncryptionSecret != "provider-secret" {
		t.Errorf("ProviderEncryptionSecret = %q, want %q", cfg.ProviderEncryptionSecret, "provider-secret")
	}
	if cfg.Embedding.EndpointURL != "http://custom-ollama:11434" {
		t.Errorf("Embedding.EndpointURL = %q", cfg.Embedding.EndpointURL)
	}
	if cfg.Embedding.Model != "custom-model" {
		t.Errorf("Embedding.Model = %q", cfg.Embedding.Model)
	}
	if cfg.Embedding.Dimensions != 512 {
		t.Errorf("Embedding.Dimensions = %d, want 512", cfg.Embedding.Dimensions)
	}
	if cfg.Embedding.BatchSize != 128 {
		t.Errorf("Embedding.BatchSize = %d, want 128", cfg.Embedding.BatchSize)
	}
	if cfg.LLM.Provider != "ollama" {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "ollama")
	}
	if cfg.LLM.EndpointURL != "https://custom-llm:11434" {
		t.Errorf("LLM.EndpointURL = %q, want %q", cfg.LLM.EndpointURL, "https://custom-llm:11434")
	}
	if cfg.LLM.Model != "llama3.1" {
		t.Errorf("LLM.Model = %q, want %q", cfg.LLM.Model, "llama3.1")
	}
	if cfg.Indexing.DefaultBranch != "develop" {
		t.Errorf("Indexing.DefaultBranch = %q", cfg.Indexing.DefaultBranch)
	}
	if cfg.Indexing.MaxParallelFiles != 16 {
		t.Errorf("Indexing.MaxParallelFiles = %d", cfg.Indexing.MaxParallelFiles)
	}
	if cfg.Indexing.MaxChunkTokens != 1024 {
		t.Errorf("Indexing.MaxChunkTokens = %d", cfg.Indexing.MaxChunkTokens)
	}
	if cfg.Indexing.RepoCacheDir != "/custom/repos" {
		t.Errorf("Indexing.RepoCacheDir = %q", cfg.Indexing.RepoCacheDir)
	}
	if cfg.Jobs.WorkerConcurrency != 8 {
		t.Errorf("Jobs.WorkerConcurrency = %d", cfg.Jobs.WorkerConcurrency)
	}
	if cfg.Jobs.IndexFullTimeout != 1*time.Hour {
		t.Errorf("Jobs.IndexFullTimeout = %v", cfg.Jobs.IndexFullTimeout)
	}
	if cfg.Jobs.IndexIncrementalTimeout != 20*time.Minute {
		t.Errorf("Jobs.IndexIncrementalTimeout = %v", cfg.Jobs.IndexIncrementalTimeout)
	}
	if cfg.Jobs.MaxRetries != 5 {
		t.Errorf("Jobs.MaxRetries = %d", cfg.Jobs.MaxRetries)
	}
	if cfg.Jobs.UniqueWindow != 2*time.Hour {
		t.Errorf("Jobs.UniqueWindow = %v", cfg.Jobs.UniqueWindow)
	}
	if cfg.Events.SSEKeepaliveInterval != 15*time.Second {
		t.Errorf("Events.SSEKeepaliveInterval = %v", cfg.Events.SSEKeepaliveInterval)
	}
	if cfg.Events.MaxSSEConnections != 200 {
		t.Errorf("Events.MaxSSEConnections = %d", cfg.Events.MaxSSEConnections)
	}
}

func TestLoad_PanicsOnMissingPostgresDSN(t *testing.T) {
	clearOptionalEnvs(t)
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("SSH_KEY_ENCRYPTION_SECRET", "test-secret")
	t.Setenv("PROVIDER_ENCRYPTION_SECRET", "provider-secret")

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

func TestLoad_PanicsOnMissingSSHSecret(t *testing.T) {
	clearOptionalEnvs(t)
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/testdb")
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

func TestLoad_PanicsOnInvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port string
	}{
		{"zero", "0"},
		{"too_high", "70000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnvs(t)
			clearOptionalEnvs(t)
			t.Setenv("SERVER_PORT", tt.port)

			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic for invalid port")
				}
				msg, ok := r.(string)
				if !ok || !strings.Contains(msg, "SERVER_PORT") {
					t.Errorf("panic message = %v, want mention of SERVER_PORT", r)
				}
			}()
			Load()
		})
	}
}

func TestLoad_PanicsOnMinConnsExceedsMaxConns(t *testing.T) {
	setRequiredEnvs(t)
	clearOptionalEnvs(t)
	t.Setenv("POSTGRES_MIN_CONNS", "50")
	t.Setenv("POSTGRES_MAX_CONNS", "10")

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

func TestLoad_PanicsOnInvalidLLMURL(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "malformed", value: "http://%zz", want: "LLM_URL"},
		{name: "missing_host", value: "http://", want: "LLM_URL"},
		{name: "hostless", value: "http://:11434", want: "LLM_URL"},
		{name: "userinfo", value: "http://user:pass@localhost:11434", want: "LLM_URL"},
		{name: "bad_scheme", value: "ftp://localhost:11434", want: "LLM_URL"},
		{name: "query", value: "http://localhost:11434?token=secret", want: "LLM_URL"},
		{name: "fragment", value: "http://localhost:11434#secret", want: "LLM_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnvs(t)
			clearOptionalEnvs(t)
			t.Setenv("LLM_URL", tt.value)

			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic for invalid LLM_URL")
				}
				msg, ok := r.(string)
				if !ok || !strings.Contains(msg, tt.want) {
					t.Errorf("panic message = %v, want mention of %s", r, tt.want)
				}
			}()
			Load()
		})
	}
}

func TestLoad_NonNumericPortFallsBackToDefault(t *testing.T) {
	setRequiredEnvs(t)
	clearOptionalEnvs(t)
	t.Setenv("SERVER_PORT", "abc")

	cfg := Load()

	if cfg.Server.Port != DefaultServerPort {
		t.Errorf("Server.Port = %d, want default %d for non-numeric input", cfg.Server.Port, DefaultServerPort)
	}
}

func TestLoadForTest(t *testing.T) {
	cfg := LoadForTest()

	if cfg.Server.Port != 0 {
		t.Errorf("test Port = %d, want 0", cfg.Server.Port)
	}
	if cfg.Postgres.DSN != "" {
		t.Errorf("test Postgres.DSN = %q, want empty (tests skip DB connection)", cfg.Postgres.DSN)
	}
	if cfg.SSH.EncryptionSecret == "" {
		t.Error("test SSH.EncryptionSecret is empty")
	}
	if cfg.ProviderEncryptionSecret == "" {
		t.Error("test ProviderEncryptionSecret is empty")
	}
	if cfg.Embedding.Model == "" {
		t.Error("test Embedding.Model is empty")
	}
	if cfg.Embedding.Provider != DefaultEmbeddingProvider {
		t.Errorf("test Embedding.Provider = %q, want %q", cfg.Embedding.Provider, DefaultEmbeddingProvider)
	}
	if cfg.LLM.Provider != DefaultLLMProvider {
		t.Errorf("test LLM.Provider = %q, want %q", cfg.LLM.Provider, DefaultLLMProvider)
	}
	if cfg.LLM.EndpointURL != DefaultLLMEndpointURL {
		t.Errorf("test LLM.EndpointURL = %q, want %q", cfg.LLM.EndpointURL, DefaultLLMEndpointURL)
	}
	if cfg.Redis.URL == "" {
		t.Error("test Redis.URL is empty")
	}
}

func TestLoad_MissingProviderEncryptionSecretPanics(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/testdb?sslmode=disable")
	t.Setenv("SSH_KEY_ENCRYPTION_SECRET", "test-secret")
	t.Setenv("PROVIDER_ENCRYPTION_SECRET", "")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing PROVIDER_ENCRYPTION_SECRET")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "PROVIDER_ENCRYPTION_SECRET") {
			t.Errorf("panic message = %v, want mention of PROVIDER_ENCRYPTION_SECRET", r)
		}
	}()

	Load()
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"t", true},
		{"T", true},
		{"false", false},
		{"FALSE", false},
		{"0", false},
		{"f", false},
		{"F", false},
		{"junk", false},
		{"", false},
		{"yes", true},
		{"YES", true},
		{"y", true},
		{"on", true},
		{"ON", true},
		{"no", false},
		{"n", false},
		{"off", false},
		{"OFF", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseBool(tt.input); got != tt.want {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseOrigins(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single", "http://localhost:3000", 1},
		{"multiple", "http://a.com,http://b.com,http://c.com", 3},
		{"with_whitespace", " http://a.com , http://b.com ", 2},
		{"trailing_comma", "http://a.com,", 1},
		{"only_commas", ",,,", 0},
		{"whitespace_only", "  ,  ,  ", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOrigins(tt.input)
			if len(got) != tt.want {
				t.Errorf("parseOrigins(%q) len = %d, want %d (got %v)", tt.input, len(got), tt.want, got)
			}
		})
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		t.Setenv("TEST_KEY_SET", "custom")
		if got := envOrDefault("TEST_KEY_SET", "fallback"); got != "custom" {
			t.Errorf("got %q, want %q", got, "custom")
		}
	})

	t.Run("unset", func(t *testing.T) {
		t.Setenv("TEST_KEY_UNSET", "")
		if got := envOrDefault("TEST_KEY_UNSET", "fallback"); got != "fallback" {
			t.Errorf("got %q, want %q", got, "fallback")
		}
	})

	t.Run("whitespace_only", func(t *testing.T) {
		t.Setenv("TEST_KEY_WS", "   ")
		if got := envOrDefault("TEST_KEY_WS", "fallback"); got != "fallback" {
			t.Errorf("got %q, want %q", got, "fallback")
		}
	})
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback int
		want     int
	}{
		{"valid", "42", 0, 42},
		{"empty", "", 10, 10},
		{"invalid", "abc", 10, 10},
		{"negative", "-5", 0, -5},
		{"zero", "0", 99, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseInt(tt.input, tt.fallback); got != tt.want {
				t.Errorf("parseInt(%q, %d) = %d, want %d", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback time.Duration
		want     time.Duration
	}{
		{"seconds", "5s", 0, 5 * time.Second},
		{"minutes", "30m", 0, 30 * time.Minute},
		{"hours", "2h", 0, 2 * time.Hour},
		{"zero", "0s", time.Second, 0},
		{"empty_fallback", "", 10 * time.Second, 10 * time.Second},
		{"invalid_fallback", "invalid", 10 * time.Second, 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDuration(tt.input, tt.fallback); got != tt.want {
				t.Errorf("parseDuration(%q, %v) = %v, want %v", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestRedactDSN(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"password_redacted",
			"postgres://app:secret@postgres:5432/codeintel?sslmode=disable",
			"postgres://app:***@postgres:5432/codeintel?sslmode=disable",
		},
		{
			"no_password",
			"postgres://app@postgres:5432/codeintel",
			"postgres://app@postgres:5432/codeintel",
		},
		{
			"not_a_url",
			"not-a-url",
			"not-a-url",
		},
		{
			"no_at_sign",
			"postgres://localdb",
			"postgres://localdb",
		},
		{
			"empty",
			"",
			"",
		},
		{
			"kv_unquoted",
			"host=localhost user=app password=secret dbname=codeintel sslmode=disable",
			"host=localhost user=app password=*** dbname=codeintel sslmode=disable",
		},
		{
			"kv_quoted",
			"host=localhost user=app password='my secret' dbname=codeintel",
			"host=localhost user=app password='***' dbname=codeintel",
		},
		{
			"kv_case_insensitive",
			"host=localhost Password=topsecret dbname=mydb",
			"host=localhost Password=*** dbname=mydb",
		},
		{
			"kv_spaced_equals",
			"host=localhost user=app password = secret dbname=codeintel",
			"host=localhost user=app password = *** dbname=codeintel",
		},
		{
			"kv_double_quoted",
			`host=localhost user=app password="secret" dbname=codeintel`,
			`host=localhost user=app password="***" dbname=codeintel`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactDSN(tt.input); got != tt.want {
				t.Errorf("redactDSN(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"no_userinfo", "http://localhost:11434", "http://localhost:11434"},
		{"query_redacted", "http://localhost:11434?token=secret", "http://localhost:11434"},
		{"fragment_redacted", "http://localhost:11434#secret", "http://localhost:11434"},
		{"with_userinfo", "postgres://app:secret@host:5432/db", "postgres://host:5432/db"},
		{"userinfo_query_redacted", "postgres://app:secret@host:5432/db?password=secret#frag", "postgres://host:5432/db"},
		{"redis_with_pass", "redis://user:pass@localhost:6379/0", "redis://localhost:6379/0"},
		{"user_only", "postgres://app@host:5432/db", "postgres://host:5432/db"},
		{"not_a_url", "not-a-url", "not-a-url"},
		{"not_a_url_query_redacted", "not-a-url?token=secret", "not-a-url"},
		{"not_a_url_fragment_redacted", "not-a-url#secret", "not-a-url"},
		{"path_only", "/var/lib/data", "/var/lib/data"},
		{"parse_error", "http://app:secret@%zz", "http://***@%zz"},
		{"parse_error_query_redacted", "http://app:secret@%zz?token=secret#frag", "http://***@%zz"},
		{"parse_error_no_userinfo_query_redacted", "http://%zz?token=secret#frag", "http://%zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactURL(tt.input); got != tt.want {
				t.Errorf("redactURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRequiredEnv_Panics(t *testing.T) {
	t.Setenv("REQUIRED_TEST_KEY", "")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing required env")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "REQUIRED_TEST_KEY") {
			t.Errorf("panic message = %v, want mention of REQUIRED_TEST_KEY", r)
		}
	}()
	requiredEnv("REQUIRED_TEST_KEY")
}

func TestRequiredEnv_ReturnsValue(t *testing.T) {
	t.Setenv("REQUIRED_TEST_KEY2", "my-value")

	got := requiredEnv("REQUIRED_TEST_KEY2")
	if got != "my-value" {
		t.Errorf("requiredEnv() = %q, want %q", got, "my-value")
	}
}

// Package config loads application configuration from the environment.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"myjungle/backend-api/internal/logger"
)

// Config holds application configuration.
type Config struct {
	Log                      logger.Config
	Server                   ServerConfig
	Postgres                 PostgresConfig
	Redis                    RedisConfig
	SSH                      SSHConfig
	ProviderEncryptionSecret string
	Embedding                EmbeddingDefaults
	LLM                      LLMDefaults
	Indexing                 IndexingConfig
	Jobs                     JobsConfig
	Events                   EventsConfig
	Reaper                   ReaperConfig
	Session                  SessionConfig
	PlatformAdminUsernames   []string
}

// SessionConfig holds session authentication settings.
type SessionConfig struct {
	TTL        time.Duration
	CookieName string
	// SecureCookie controls the Secure flag on session cookies.
	// Defaults to false for local development; set SESSION_SECURE_COOKIE=true
	// in production to ensure cookies are sent only over HTTPS.
	SecureCookie bool
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port               int
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	ShutdownTimeout    time.Duration
	ReadHeaderTimeout  time.Duration
	CORSAllowedOrigins []string
	CORSWildcard       bool
	BodyLimitBytes     int64
}

// PostgresConfig holds database connection settings.
type PostgresConfig struct {
	DSN         string
	MaxConns    int
	MinConns    int
	MaxConnLife time.Duration
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL      string
	PoolSize int
}

// SSHConfig holds SSH key encryption settings.
type SSHConfig struct {
	EncryptionSecret string
}

// EmbeddingDefaults holds default embedding provider settings.
type EmbeddingDefaults struct {
	Provider    string
	EndpointURL string
	Model       string
	Dimensions  int
	MaxTokens   int
	BatchSize   int
}

// LLMDefaults holds default LLM provider settings.
type LLMDefaults struct {
	Provider    string
	EndpointURL string
	Model       string
}

// IndexingConfig holds code indexing settings.
type IndexingConfig struct {
	DefaultBranch    string
	MaxParallelFiles int
	MaxChunkTokens   int
	RepoCacheDir     string
}

// JobsConfig holds async job worker settings.
type JobsConfig struct {
	WorkerConcurrency       int
	IndexFullTimeout        time.Duration
	IndexIncrementalTimeout time.Duration
	MaxRetries              int
	UniqueWindow            time.Duration
}

// ReaperConfig holds lazy job reaper settings.
type ReaperConfig struct {
	StaleThreshold time.Duration
}

// EventsConfig holds SSE event stream settings.
type EventsConfig struct {
	SSEKeepaliveInterval      time.Duration
	MaxSSEConnections         int
	MembershipRefreshInterval time.Duration
}

// Load reads configuration from environment variables.
// It panics if required environment variables are missing or values are invalid.
func Load() *Config {
	sshSecret := requiredEnv("SSH_KEY_ENCRYPTION_SECRET")
	providerSecret := requiredEnv("PROVIDER_ENCRYPTION_SECRET")
	slog.Info("config: using explicit PROVIDER_ENCRYPTION_SECRET")

	cfg := &Config{
		Log: logger.Config{
			Level:  envOrDefault("LOG_LEVEL", "info"),
			Format: envOrDefault("LOG_FORMAT", "json"),
		},
		Server: ServerConfig{
			Port:               parseInt(envOrDefault("SERVER_PORT", strconv.Itoa(DefaultServerPort)), DefaultServerPort),
			ReadTimeout:        parseDuration(envOrDefault("SERVER_READ_TIMEOUT", DefaultReadTimeout.String()), DefaultReadTimeout),
			WriteTimeout:       parseDuration(envOrDefault("SERVER_WRITE_TIMEOUT", DefaultWriteTimeout.String()), DefaultWriteTimeout),
			IdleTimeout:        parseDuration(envOrDefault("SERVER_IDLE_TIMEOUT", DefaultIdleTimeout.String()), DefaultIdleTimeout),
			ShutdownTimeout:    parseDuration(envOrDefault("SERVER_SHUTDOWN_TIMEOUT", DefaultShutdownTimeout.String()), DefaultShutdownTimeout),
			ReadHeaderTimeout:  parseDuration(envOrDefault("SERVER_READ_HEADER_TIMEOUT", DefaultReadHeaderTimeout.String()), DefaultReadHeaderTimeout),
			CORSAllowedOrigins: parseOrigins(envOrDefault("CORS_ALLOWED_ORIGINS", "")),
			CORSWildcard:       parseBool(envOrDefault("CORS_WILDCARD", "false")),
			BodyLimitBytes:     parseInt64(envOrDefault("SERVER_BODY_LIMIT_BYTES", strconv.FormatInt(DefaultBodyLimitBytes, 10)), DefaultBodyLimitBytes),
		},
		Postgres: PostgresConfig{
			DSN:         requiredEnv("POSTGRES_DSN"),
			MaxConns:    parseInt(envOrDefault("POSTGRES_MAX_CONNS", strconv.Itoa(DefaultPostgresMaxConns)), DefaultPostgresMaxConns),
			MinConns:    parseInt(envOrDefault("POSTGRES_MIN_CONNS", strconv.Itoa(DefaultPostgresMinConns)), DefaultPostgresMinConns),
			MaxConnLife: parseDuration(envOrDefault("POSTGRES_MAX_CONN_LIFE", DefaultPostgresMaxConnLife.String()), DefaultPostgresMaxConnLife),
		},
		Redis: RedisConfig{
			URL:      envOrDefault("REDIS_URL", ""),
			PoolSize: parseInt(envOrDefault("REDIS_POOL_SIZE", strconv.Itoa(DefaultRedisPoolSize)), DefaultRedisPoolSize),
		},
		SSH: SSHConfig{
			EncryptionSecret: sshSecret,
		},
		ProviderEncryptionSecret: providerSecret,
		Embedding: EmbeddingDefaults{
			Provider:    envOrDefault("EMBEDDING_PROVIDER", DefaultEmbeddingProvider),
			EndpointURL: envOrDefault("OLLAMA_URL", DefaultEmbeddingEndpointURL),
			Model:       envOrDefault("OLLAMA_MODEL", DefaultEmbeddingModel),
			Dimensions:  parseInt(envOrDefault("OLLAMA_DIMENSIONS", strconv.Itoa(DefaultEmbeddingDimensions)), DefaultEmbeddingDimensions),
			MaxTokens:   parseInt(envOrDefault("EMBEDDING_MAX_TOKENS", strconv.Itoa(DefaultEmbeddingMaxTokens)), DefaultEmbeddingMaxTokens),
			BatchSize:   parseInt(envOrDefault("EMBED_BATCH_SIZE", strconv.Itoa(DefaultEmbeddingBatchSize)), DefaultEmbeddingBatchSize),
		},
		LLM: LLMDefaults{
			Provider:    envOrDefault("LLM_PROVIDER", DefaultLLMProvider),
			EndpointURL: envOrDefault("LLM_URL", DefaultLLMEndpointURL),
			Model:       envOrDefault("LLM_MODEL", DefaultLLMModel),
		},
		Indexing: IndexingConfig{
			DefaultBranch:    envOrDefault("INDEXING_DEFAULT_BRANCH", DefaultIndexingDefaultBranch),
			MaxParallelFiles: parseInt(envOrDefault("INDEXING_MAX_PARALLEL_FILES", strconv.Itoa(DefaultIndexingMaxParallelFiles)), DefaultIndexingMaxParallelFiles),
			MaxChunkTokens:   parseInt(envOrDefault("INDEXING_MAX_CHUNK_TOKENS", strconv.Itoa(DefaultIndexingMaxChunkTokens)), DefaultIndexingMaxChunkTokens),
			RepoCacheDir:     envOrDefault("REPO_CACHE_DIR", DefaultRepoCacheDir),
		},
		Jobs: JobsConfig{
			WorkerConcurrency:       parseInt(envOrDefault("WORKER_CONCURRENCY", strconv.Itoa(DefaultWorkerConcurrency)), DefaultWorkerConcurrency),
			IndexFullTimeout:        parseDuration(envOrDefault("INDEX_FULL_TIMEOUT", DefaultIndexFullTimeout.String()), DefaultIndexFullTimeout),
			IndexIncrementalTimeout: parseDuration(envOrDefault("INDEX_INCREMENTAL_TIMEOUT", DefaultIndexIncrementalTimeout.String()), DefaultIndexIncrementalTimeout),
			MaxRetries:              parseInt(envOrDefault("MAX_RETRIES", strconv.Itoa(DefaultMaxRetries)), DefaultMaxRetries),
			UniqueWindow:            parseDuration(envOrDefault("UNIQUE_WINDOW", DefaultUniqueWindow.String()), DefaultUniqueWindow),
		},
		Reaper: ReaperConfig{
			StaleThreshold: parseDuration(envOrDefault("REAPER_STALE_THRESHOLD", DefaultReaperStaleThreshold.String()), DefaultReaperStaleThreshold),
		},
		Events: EventsConfig{
			SSEKeepaliveInterval:      parseDuration(envOrDefault("SSE_KEEPALIVE_INTERVAL", DefaultSSEKeepaliveInterval.String()), DefaultSSEKeepaliveInterval),
			MaxSSEConnections:         parseInt(envOrDefault("MAX_SSE_CONNECTIONS", strconv.Itoa(DefaultMaxSSEConnections)), DefaultMaxSSEConnections),
			MembershipRefreshInterval: parseDuration(envOrDefault("MEMBERSHIP_REFRESH_INTERVAL", DefaultMembershipRefreshInterval.String()), DefaultMembershipRefreshInterval),
		},
		Session: SessionConfig{
			TTL:          parseDuration(envOrDefault("SESSION_TTL", DefaultSessionTTL.String()), DefaultSessionTTL),
			CookieName:   envOrDefault("SESSION_COOKIE_NAME", DefaultSessionCookieName),
			SecureCookie: mustParseBool(envOrDefault("SESSION_SECURE_COOKIE", "false"), "SESSION_SECURE_COOKIE"),
		},
	}

	cfg.PlatformAdminUsernames = parsePlatformAdminUsernames(
		envOrDefault("PLATFORM_ADMIN_USERNAMES", ""))

	validate(cfg)
	logSummary(cfg)
	return cfg
}

// LoadForTest returns a Config with sensible test defaults.
// No environment variables are read and validate() is NOT called,
// so fields may hold values (e.g. Port=0) that would fail normal
// validation. Required fields are populated with placeholder values
// suitable for unit tests.
func LoadForTest() *Config {
	return &Config{
		Log: logger.Config{Level: "debug", Format: "text"},
		Server: ServerConfig{
			Port:               0, // OS picks a free port in tests.
			ReadTimeout:        DefaultReadTimeout,
			WriteTimeout:       DefaultWriteTimeout,
			IdleTimeout:        DefaultIdleTimeout,
			ShutdownTimeout:    DefaultShutdownTimeout,
			ReadHeaderTimeout:  DefaultReadHeaderTimeout,
			CORSAllowedOrigins: nil,
			CORSWildcard:       true,
			BodyLimitBytes:     DefaultBodyLimitBytes,
		},
		Postgres: PostgresConfig{
			DSN:         "", // empty: unit tests skip DB connection
			MaxConns:    5,
			MinConns:    1,
			MaxConnLife: 5 * time.Minute,
		},
		Redis: RedisConfig{
			URL:      "redis://localhost:6379/15",
			PoolSize: 2,
		},
		SSH: SSHConfig{
			EncryptionSecret: "test-secret-do-not-use-in-production",
		},
		ProviderEncryptionSecret: "test-provider-secret-do-not-use-in-production",
		Embedding: EmbeddingDefaults{
			Provider:    DefaultEmbeddingProvider,
			EndpointURL: DefaultEmbeddingEndpointURL,
			Model:       DefaultEmbeddingModel,
			Dimensions:  DefaultEmbeddingDimensions,
			MaxTokens:   DefaultEmbeddingMaxTokens,
			BatchSize:   DefaultEmbeddingBatchSize,
		},
		LLM: LLMDefaults{
			Provider:    DefaultLLMProvider,
			EndpointURL: DefaultLLMEndpointURL,
			Model:       DefaultLLMModel,
		},
		Indexing: IndexingConfig{
			DefaultBranch:    DefaultIndexingDefaultBranch,
			MaxParallelFiles: DefaultIndexingMaxParallelFiles,
			MaxChunkTokens:   DefaultIndexingMaxChunkTokens,
			RepoCacheDir:     "/tmp/myjungle-test-repos",
		},
		Jobs: JobsConfig{
			WorkerConcurrency:       1,
			IndexFullTimeout:        1 * time.Minute,
			IndexIncrementalTimeout: 30 * time.Second,
			MaxRetries:              1,
			UniqueWindow:            1 * time.Minute,
		},
		Reaper: ReaperConfig{
			StaleThreshold: DefaultReaperStaleThreshold,
		},
		Events: EventsConfig{
			SSEKeepaliveInterval:      5 * time.Second,
			MaxSSEConnections:         5,
			MembershipRefreshInterval: 10 * time.Second,
		},
		Session: SessionConfig{
			TTL:          1 * time.Hour,
			CookieName:   DefaultSessionCookieName,
			SecureCookie: false,
		},
	}
}

// validate panics if config values are out of acceptable ranges.
func validate(cfg *Config) {
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(cfg.Log.Level)] {
		panic(fmt.Sprintf("LOG_LEVEL must be debug, info, warn, or error, got %q", cfg.Log.Level))
	}
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[strings.ToLower(cfg.Log.Format)] {
		panic(fmt.Sprintf("LOG_FORMAT must be json or text, got %q", cfg.Log.Format))
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		panic(fmt.Sprintf("SERVER_PORT must be between 1 and 65535, got %d", cfg.Server.Port))
	}
	if cfg.Server.ReadTimeout < 0 {
		panic(fmt.Sprintf("SERVER_READ_TIMEOUT must not be negative, got %v", cfg.Server.ReadTimeout))
	}
	if cfg.Server.WriteTimeout < 0 {
		panic(fmt.Sprintf("SERVER_WRITE_TIMEOUT must not be negative, got %v", cfg.Server.WriteTimeout))
	}
	if cfg.Server.IdleTimeout <= 0 {
		panic(fmt.Sprintf("SERVER_IDLE_TIMEOUT must be positive, got %v", cfg.Server.IdleTimeout))
	}
	if cfg.Server.ShutdownTimeout <= 0 {
		panic(fmt.Sprintf("SERVER_SHUTDOWN_TIMEOUT must be positive, got %v", cfg.Server.ShutdownTimeout))
	}
	if cfg.Server.ReadHeaderTimeout <= 0 {
		panic(fmt.Sprintf("SERVER_READ_HEADER_TIMEOUT must be positive, got %v", cfg.Server.ReadHeaderTimeout))
	}
	if cfg.Server.BodyLimitBytes <= 0 {
		panic(fmt.Sprintf("SERVER_BODY_LIMIT_BYTES must be positive, got %d", cfg.Server.BodyLimitBytes))
	}
	if cfg.Postgres.MaxConns < 1 {
		panic(fmt.Sprintf("POSTGRES_MAX_CONNS must be positive, got %d", cfg.Postgres.MaxConns))
	}
	if cfg.Redis.PoolSize < 1 {
		panic(fmt.Sprintf("REDIS_POOL_SIZE must be positive, got %d", cfg.Redis.PoolSize))
	}
	if cfg.Jobs.WorkerConcurrency < 1 {
		panic(fmt.Sprintf("WORKER_CONCURRENCY must be positive, got %d", cfg.Jobs.WorkerConcurrency))
	}
	if cfg.Embedding.Dimensions < 1 {
		panic(fmt.Sprintf("OLLAMA_DIMENSIONS must be positive, got %d", cfg.Embedding.Dimensions))
	}
	if cfg.Embedding.MaxTokens < 1 {
		panic(fmt.Sprintf("EMBEDDING_MAX_TOKENS must be positive, got %d", cfg.Embedding.MaxTokens))
	}
	if cfg.Embedding.BatchSize < 1 {
		panic(fmt.Sprintf("EMBED_BATCH_SIZE must be positive, got %d", cfg.Embedding.BatchSize))
	}
	if strings.TrimSpace(cfg.ProviderEncryptionSecret) == "" {
		panic("PROVIDER_ENCRYPTION_SECRET must not be empty")
	}
	validateProviderDefaultEndpointURL(cfg.Embedding.EndpointURL, "OLLAMA_URL")
	if strings.TrimSpace(cfg.LLM.Provider) == "" {
		panic("LLM_PROVIDER must not be empty")
	}
	validateProviderDefaultEndpointURL(cfg.LLM.EndpointURL, "LLM_URL")
	if cfg.Postgres.MinConns < 0 {
		panic(fmt.Sprintf("POSTGRES_MIN_CONNS must not be negative, got %d", cfg.Postgres.MinConns))
	}
	if cfg.Postgres.MinConns > cfg.Postgres.MaxConns {
		panic(fmt.Sprintf("POSTGRES_MIN_CONNS (%d) must not exceed POSTGRES_MAX_CONNS (%d)", cfg.Postgres.MinConns, cfg.Postgres.MaxConns))
	}
	if cfg.Postgres.MaxConnLife <= 0 {
		panic(fmt.Sprintf("POSTGRES_MAX_CONN_LIFE must be positive, got %v", cfg.Postgres.MaxConnLife))
	}
	if cfg.Indexing.MaxParallelFiles < 1 {
		panic(fmt.Sprintf("INDEXING_MAX_PARALLEL_FILES must be positive, got %d", cfg.Indexing.MaxParallelFiles))
	}
	if cfg.Indexing.MaxChunkTokens < 1 {
		panic(fmt.Sprintf("INDEXING_MAX_CHUNK_TOKENS must be positive, got %d", cfg.Indexing.MaxChunkTokens))
	}
	if cfg.Jobs.IndexFullTimeout <= 0 {
		panic(fmt.Sprintf("INDEX_FULL_TIMEOUT must be positive, got %v", cfg.Jobs.IndexFullTimeout))
	}
	if cfg.Jobs.IndexIncrementalTimeout <= 0 {
		panic(fmt.Sprintf("INDEX_INCREMENTAL_TIMEOUT must be positive, got %v", cfg.Jobs.IndexIncrementalTimeout))
	}
	if cfg.Jobs.MaxRetries < 0 {
		panic(fmt.Sprintf("MAX_RETRIES must not be negative, got %d", cfg.Jobs.MaxRetries))
	}
	if cfg.Jobs.UniqueWindow <= 0 {
		panic(fmt.Sprintf("UNIQUE_WINDOW must be positive, got %v", cfg.Jobs.UniqueWindow))
	}
	if cfg.Events.SSEKeepaliveInterval <= 0 {
		panic(fmt.Sprintf("SSE_KEEPALIVE_INTERVAL must be positive, got %v", cfg.Events.SSEKeepaliveInterval))
	}
	if cfg.Events.MaxSSEConnections < 1 {
		panic(fmt.Sprintf("MAX_SSE_CONNECTIONS must be positive, got %d", cfg.Events.MaxSSEConnections))
	}
	if cfg.Events.MembershipRefreshInterval <= 0 {
		panic(fmt.Sprintf("MEMBERSHIP_REFRESH_INTERVAL must be positive, got %v", cfg.Events.MembershipRefreshInterval))
	}
	if cfg.Events.MaxSSEConnections > 0 && cfg.Server.WriteTimeout > 0 {
		panic(fmt.Sprintf(
			"SERVER_WRITE_TIMEOUT must be 0 (unlimited) when SSE is enabled, got %v",
			cfg.Server.WriteTimeout))
	}
	if cfg.Session.TTL <= 0 {
		panic(fmt.Sprintf("SESSION_TTL must be positive, got %v", cfg.Session.TTL))
	}
	if cfg.Session.CookieName == "" {
		panic("SESSION_COOKIE_NAME must not be empty")
	}
	if !validCookieName.MatchString(cfg.Session.CookieName) {
		panic(fmt.Sprintf("SESSION_COOKIE_NAME %q contains invalid characters (must be HTTP token chars)", cfg.Session.CookieName))
	}
}

func validateProviderDefaultEndpointURL(raw, envKey string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		panic(fmt.Sprintf("%s must not be empty", envKey))
	}

	u, err := url.Parse(value)
	if err != nil {
		panic(fmt.Sprintf("%s must be a valid URL: %v", envKey, err))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		panic(fmt.Sprintf("%s must use http or https scheme", envKey))
	}
	if u.Hostname() == "" {
		panic(fmt.Sprintf("%s must include a host", envKey))
	}
	if u.User != nil {
		panic(fmt.Sprintf("%s must not contain user credentials", envKey))
	}
	if u.RawQuery != "" {
		panic(fmt.Sprintf("%s must not contain a query string", envKey))
	}
	if u.Fragment != "" {
		panic(fmt.Sprintf("%s must not contain a fragment", envKey))
	}
}

// logSummary logs the loaded configuration as a single structured event. Secrets are redacted.
func logSummary(cfg *Config) {
	slog.Info("config loaded",
		slog.Group("log",
			slog.String("level", cfg.Log.Level),
			slog.String("format", cfg.Log.Format),
		),
		slog.Group("server",
			slog.Int("port", cfg.Server.Port),
			slog.Duration("read_timeout", cfg.Server.ReadTimeout),
			slog.Duration("write_timeout", cfg.Server.WriteTimeout),
			slog.Duration("idle_timeout", cfg.Server.IdleTimeout),
			slog.Duration("shutdown_timeout", cfg.Server.ShutdownTimeout),
			slog.Bool("cors_wildcard", cfg.Server.CORSWildcard),
			slog.Any("cors_origins", cfg.Server.CORSAllowedOrigins),
			slog.Int64("body_limit_bytes", cfg.Server.BodyLimitBytes),
		),
		slog.Group("postgres",
			slog.String("dsn", redactDSN(cfg.Postgres.DSN)),
			slog.Int("max_conns", cfg.Postgres.MaxConns),
			slog.Int("min_conns", cfg.Postgres.MinConns),
			slog.Duration("max_conn_life", cfg.Postgres.MaxConnLife),
		),
		slog.Group("redis",
			slog.String("url", redactURL(cfg.Redis.URL)),
			slog.Int("pool_size", cfg.Redis.PoolSize),
		),
		slog.Group("embedding",
			slog.String("provider", cfg.Embedding.Provider),
			slog.String("endpoint_url", redactURL(cfg.Embedding.EndpointURL)),
			slog.String("model", cfg.Embedding.Model),
			slog.Int("dimensions", cfg.Embedding.Dimensions),
			slog.Int("batch_size", cfg.Embedding.BatchSize),
		),
		slog.Group("llm",
			slog.String("provider", cfg.LLM.Provider),
			slog.String("endpoint_url", redactURL(cfg.LLM.EndpointURL)),
			slog.String("model", cfg.LLM.Model),
		),
		slog.Group("indexing",
			slog.String("default_branch", cfg.Indexing.DefaultBranch),
			slog.Int("max_parallel_files", cfg.Indexing.MaxParallelFiles),
			slog.Int("max_chunk_tokens", cfg.Indexing.MaxChunkTokens),
			slog.String("repo_cache_dir", cfg.Indexing.RepoCacheDir),
		),
		slog.Group("jobs",
			slog.Int("worker_concurrency", cfg.Jobs.WorkerConcurrency),
			slog.Duration("index_full_timeout", cfg.Jobs.IndexFullTimeout),
			slog.Duration("index_incremental_timeout", cfg.Jobs.IndexIncrementalTimeout),
			slog.Int("max_retries", cfg.Jobs.MaxRetries),
			slog.Duration("unique_window", cfg.Jobs.UniqueWindow),
		),
		slog.Group("events",
			slog.Duration("sse_keepalive", cfg.Events.SSEKeepaliveInterval),
			slog.Int("max_sse_connections", cfg.Events.MaxSSEConnections),
			slog.Duration("membership_refresh", cfg.Events.MembershipRefreshInterval),
		),
		slog.Group("session",
			slog.Duration("ttl", cfg.Session.TTL),
			slog.String("cookie_name", cfg.Session.CookieName),
			slog.Bool("secure_cookie", cfg.Session.SecureCookie),
		),
	)
}

// validCookieName matches RFC 6265 cookie-name token characters.
var validCookieName = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)

// kvPasswordDetect detects key-value password fields with optional whitespace around '='.
var kvPasswordDetect = regexp.MustCompile(`(?i)password\s*=`)
var kvPasswordSingleQuoted = regexp.MustCompile(`(?i)(password\s*=\s*)'[^']*'`)
var kvPasswordDoubleQuoted = regexp.MustCompile(`(?i)(password\s*=\s*)"[^"]*"`)
var kvPasswordUnquoted = regexp.MustCompile(`(?i)(password\s*=\s*)\S+`)

// redactDSN masks the password in a Postgres DSN.
// It handles both URL-style (postgres://user:pass@host/db) and
// key-value style (host=localhost password=secret dbname=mydb).
// Quoted (single/double) and unquoted values are supported, with
// optional whitespace around '='.
func redactDSN(dsn string) string {
	// Key-value format: contains "password=" (case-insensitive, optional whitespace)
	if kvPasswordDetect.MatchString(dsn) {
		// Try single-quoted: password='secret'
		result := kvPasswordSingleQuoted.ReplaceAllString(dsn, "${1}'***'")
		if result != dsn {
			return result
		}
		// Try double-quoted: password="secret"
		result = kvPasswordDoubleQuoted.ReplaceAllString(dsn, `${1}"***"`)
		if result != dsn {
			return result
		}
		// Unquoted fallback: password=secret
		return kvPasswordUnquoted.ReplaceAllString(dsn, "${1}***")
	}

	// URL-style format
	idx := strings.Index(dsn, "://")
	if idx < 0 {
		return dsn
	}
	rest := dsn[idx+3:]
	atIdx := strings.Index(rest, "@")
	if atIdx < 0 {
		return dsn
	}
	userPass := rest[:atIdx]
	colonIdx := strings.Index(userPass, ":")
	if colonIdx < 0 {
		return dsn
	}
	return dsn[:idx+3] + userPass[:colonIdx+1] + "***" + rest[atIdx:]
}

// redactURL strips userinfo from a URL for safe logging.
// On parse error it falls back to manual stripping so credentials
// are never returned verbatim.
func redactURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		// Fallback: strip userinfo manually to avoid leaking credentials and
		// never return query strings or fragments verbatim.
		idx := strings.Index(raw, "://")
		if idx < 0 {
			return trimURLSecrets(raw)
		}
		rest := raw[idx+3:]
		atIdx := strings.Index(rest, "@")
		if atIdx < 0 {
			return raw[:idx+3] + trimURLSecrets(rest)
		}
		return raw[:idx+3] + "***@" + trimURLSecrets(rest[atIdx+1:])
	}
	if u.User == nil {
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func trimURLSecrets(raw string) string {
	if idx := strings.IndexAny(raw, "?#"); idx >= 0 {
		return raw[:idx]
	}
	return raw
}

// --- helpers ---

// parsePlatformAdminUsernames splits a comma-separated list of usernames,
// trims whitespace, lowercases, and drops empty entries.
func parsePlatformAdminUsernames(raw string) []string {
	var usernames []string
	for _, u := range strings.Split(raw, ",") {
		u = strings.TrimSpace(strings.ToLower(u))
		if u != "" {
			usernames = append(usernames, u)
		}
	}
	return usernames
}

// parseOrigins splits a comma-separated origins string into a slice.
func parseOrigins(raw string) []string {
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// mustParseBool is a strict bool parser for security-sensitive settings.
// It accepts true/false, 1/0, yes/no, on/off (case-insensitive) and panics
// on any unrecognised value so misconfigurations are caught at startup.
func mustParseBool(s, key string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "t", "yes", "y", "on":
		return true
	case "false", "0", "f", "no", "n", "off":
		return false
	default:
		panic(fmt.Sprintf("%s: invalid bool value %q (expected true/false, yes/no, on/off, 1/0)", key, s))
	}
}

// parseBool interprets common truthy values.
// It first tries strconv.ParseBool ("true", "1", "t", "false", "0", "f")
// and falls back to matching "yes", "y", "on" as true and "no", "n", "off" as false.
// Unrecognised values are treated as false.
func parseBool(s string) bool {
	v, err := strconv.ParseBool(strings.TrimSpace(s))
	if err == nil {
		return v
	}
	trimmed := strings.ToLower(strings.TrimSpace(s))
	switch trimmed {
	case "yes", "y", "on":
		return true
	case "no", "n", "off":
		return false
	}
	if trimmed != "" {
		slog.Warn("unrecognised bool value, treating as false", slog.String("value", trimmed))
	}
	return false
}

func envOrDefault(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

// parseInt64 parses s as an int64; returns fallback on failure.
func parseInt64(s string, fallback int64) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		slog.Warn("failed to parse int64, using fallback", slog.String("value", s), slog.Any("error", err), slog.Int64("fallback", fallback))
		return fallback
	}
	return v
}

// parseInt parses s as an int; returns fallback on failure.
func parseInt(s string, fallback int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		slog.Warn("failed to parse int, using fallback", slog.String("value", s), slog.Any("error", err), slog.Int("fallback", fallback))
		return fallback
	}
	return v
}

// parseDuration parses s as a time.Duration; returns fallback on failure.
func parseDuration(s string, fallback time.Duration) time.Duration {
	v, err := time.ParseDuration(s)
	if err != nil {
		slog.Warn("failed to parse duration, using fallback", slog.String("value", s), slog.Any("error", err), slog.Duration("fallback", fallback))
		return fallback
	}
	return v
}

// requiredEnv reads an env var and panics with a descriptive message if empty.
func requiredEnv(key string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return val
}

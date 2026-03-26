// Package config loads worker configuration from the environment.
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
)

// Config holds worker configuration.
type Config struct {
	Log       LogConfig
	Postgres  PostgresConfig
	Redis     RedisConfig
	Queue     QueueConfig
	Parser    ParserConfig
	Embedding EmbeddingConfig
	Qdrant    QdrantConfig
	SSH       SSHConfig
	Workspace WorkspaceConfig
	Reaper    ReaperConfig
}

// LogConfig controls log output format and minimum level.
type LogConfig struct {
	Level  string // "debug", "info", "warn", "error"
	Format string // "json" or "text"
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
	URL string
}

// QueueConfig holds asynq worker settings.
type QueueConfig struct {
	Name            string
	Concurrency     int
	ShutdownTimeout time.Duration
}

// ParserConfig holds parser engine settings.
type ParserConfig struct {
	PoolSize       int           // worker pool size; 0 = runtime.NumCPU()
	Timeout        time.Duration // overall batch timeout
	TimeoutPerFile time.Duration // per-file parse timeout
	MaxFileSize    int64         // max bytes per file
}

// EmbeddingConfig holds embedding provider settings.
type EmbeddingConfig struct {
	OllamaURL   string
	OllamaModel string
}

// QdrantConfig holds Qdrant connection settings.
type QdrantConfig struct {
	URL string
}

// SSHConfig holds SSH key encryption settings.
type SSHConfig struct {
	EncryptionSecret string
}

// ReaperConfig holds stuck-job reaper settings.
type ReaperConfig struct {
	StaleThreshold time.Duration
}

// WorkspaceConfig holds workspace / repo-cache settings.
type WorkspaceConfig struct {
	RepoCacheDir string
}

// Load reads configuration from environment variables.
// It panics if required environment variables are missing or values are invalid.
func Load() *Config {
	cfg := &Config{
		Log: LogConfig{
			Level:  envOrDefault("LOG_LEVEL", "info"),
			Format: envOrDefault("LOG_FORMAT", "json"),
		},
		Postgres: PostgresConfig{
			DSN:         requiredEnv("POSTGRES_DSN"),
			MaxConns:    parseInt(envOrDefault("POSTGRES_MAX_CONNS", strconv.Itoa(DefaultPostgresMaxConns)), DefaultPostgresMaxConns),
			MinConns:    parseInt(envOrDefault("POSTGRES_MIN_CONNS", strconv.Itoa(DefaultPostgresMinConns)), DefaultPostgresMinConns),
			MaxConnLife: parseDuration(envOrDefault("POSTGRES_MAX_CONN_LIFE", DefaultPostgresMaxConnLife.String()), DefaultPostgresMaxConnLife),
		},
		Redis: RedisConfig{
			URL: requiredEnv("REDIS_URL"),
		},
		Queue: QueueConfig{
			Name:            envOrDefault("QUEUE_NAME", DefaultQueueName),
			Concurrency:     parseInt(envOrDefault("WORKER_CONCURRENCY", strconv.Itoa(DefaultWorkerConcurrency)), DefaultWorkerConcurrency),
			ShutdownTimeout: parseDuration(envOrDefault("QUEUE_SHUTDOWN_TIMEOUT", DefaultShutdownTimeout.String()), DefaultShutdownTimeout),
		},
		Parser: ParserConfig{
			PoolSize:       parseInt(envOrDefault("PARSER_POOL_SIZE", strconv.Itoa(DefaultParserPoolSize)), DefaultParserPoolSize),
			Timeout:        parseDuration(envOrDefault("PARSER_TIMEOUT", DefaultParserTimeout.String()), DefaultParserTimeout),
			TimeoutPerFile: parseDuration(envOrDefault("PARSER_TIMEOUT_PER_FILE", DefaultParserTimeoutPerFile.String()), DefaultParserTimeoutPerFile),
			MaxFileSize:    parseInt64(envOrDefault("PARSER_MAX_FILE_SIZE", strconv.FormatInt(DefaultParserMaxFileSize, 10)), DefaultParserMaxFileSize),
		},
		Embedding: EmbeddingConfig{
			OllamaURL:   envOrDefault("OLLAMA_URL", DefaultOllamaURL),
			OllamaModel: envOrDefault("OLLAMA_MODEL", DefaultOllamaModel),
		},
		Qdrant: QdrantConfig{
			URL: envOrDefault("QDRANT_URL", DefaultQdrantURL),
		},
		SSH: SSHConfig{
			EncryptionSecret: requiredEnv("SSH_KEY_ENCRYPTION_SECRET"),
		},
		Workspace: WorkspaceConfig{
			RepoCacheDir: envOrDefault("REPO_CACHE_DIR", DefaultRepoCacheDir),
		},
		Reaper: ReaperConfig{
			StaleThreshold: parseDuration(envOrDefault("REAPER_STALE_THRESHOLD", DefaultReaperStaleThreshold.String()), DefaultReaperStaleThreshold),
		},
	}

	// Normalize case-insensitive fields before validation.
	cfg.Log.Level = strings.ToLower(cfg.Log.Level)
	cfg.Log.Format = strings.ToLower(cfg.Log.Format)

	validate(cfg)
	logSummary(cfg)
	return cfg
}

// LoadForTest returns a Config with sensible test defaults.
// No environment variables are read and validate() is NOT called.
func LoadForTest() *Config {
	return &Config{
		Log: LogConfig{Level: "debug", Format: "text"},
		Postgres: PostgresConfig{
			DSN:         "",
			MaxConns:    5,
			MinConns:    1,
			MaxConnLife: 5 * time.Minute,
		},
		Redis: RedisConfig{
			URL: "redis://localhost:6379/15",
		},
		Queue: QueueConfig{
			Name:            DefaultQueueName,
			Concurrency:     1,
			ShutdownTimeout: 5 * time.Second,
		},
		Parser: ParserConfig{
			PoolSize:       2,
			Timeout:        5 * time.Second,
			TimeoutPerFile: 5 * time.Second,
			MaxFileSize:    10 * 1024 * 1024,
		},
		Embedding: EmbeddingConfig{
			OllamaURL:   DefaultOllamaURL,
			OllamaModel: DefaultOllamaModel,
		},
		Qdrant: QdrantConfig{
			URL: DefaultQdrantURL,
		},
		SSH: SSHConfig{
			EncryptionSecret: "test-secret-do-not-use-in-production",
		},
		Workspace: WorkspaceConfig{
			RepoCacheDir: "/tmp/myjungle-test-repos",
		},
		Reaper: ReaperConfig{
			StaleThreshold: 5 * time.Minute,
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
	if cfg.Postgres.MaxConns < 1 {
		panic(fmt.Sprintf("POSTGRES_MAX_CONNS must be positive, got %d", cfg.Postgres.MaxConns))
	}
	if cfg.Postgres.MinConns < 0 {
		panic(fmt.Sprintf("POSTGRES_MIN_CONNS must not be negative, got %d", cfg.Postgres.MinConns))
	}
	if cfg.Postgres.MinConns > cfg.Postgres.MaxConns {
		panic(fmt.Sprintf("POSTGRES_MIN_CONNS (%d) must not exceed POSTGRES_MAX_CONNS (%d)",
			cfg.Postgres.MinConns, cfg.Postgres.MaxConns))
	}
	if cfg.Postgres.MaxConnLife <= 0 {
		panic(fmt.Sprintf("POSTGRES_MAX_CONN_LIFE must be positive, got %v", cfg.Postgres.MaxConnLife))
	}
	if cfg.Queue.Concurrency < 1 {
		panic(fmt.Sprintf("WORKER_CONCURRENCY must be positive, got %d", cfg.Queue.Concurrency))
	}
	if cfg.Queue.ShutdownTimeout <= 0 {
		panic(fmt.Sprintf("QUEUE_SHUTDOWN_TIMEOUT must be positive, got %v", cfg.Queue.ShutdownTimeout))
	}
	if cfg.Parser.PoolSize < 0 {
		panic(fmt.Sprintf("PARSER_POOL_SIZE must not be negative, got %d", cfg.Parser.PoolSize))
	}
	if cfg.Parser.Timeout <= 0 {
		panic(fmt.Sprintf("PARSER_TIMEOUT must be positive, got %v", cfg.Parser.Timeout))
	}
	if cfg.Parser.TimeoutPerFile <= 0 {
		panic(fmt.Sprintf("PARSER_TIMEOUT_PER_FILE must be positive, got %v", cfg.Parser.TimeoutPerFile))
	}
	if cfg.Parser.MaxFileSize <= 0 {
		panic(fmt.Sprintf("PARSER_MAX_FILE_SIZE must be positive, got %d", cfg.Parser.MaxFileSize))
	}
	if cfg.Reaper.StaleThreshold <= 0 {
		panic(fmt.Sprintf("REAPER_STALE_THRESHOLD must be positive, got %v", cfg.Reaper.StaleThreshold))
	}
	validateURL(cfg.Embedding.OllamaURL, "OLLAMA_URL")
	validateURL(cfg.Qdrant.URL, "QDRANT_URL")
}

func validateURL(raw, envKey string) {
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
}

// logSummary logs the loaded configuration as a single structured event. Secrets are redacted.
func logSummary(cfg *Config) {
	slog.Info("config loaded",
		slog.Group("log",
			slog.String("level", cfg.Log.Level),
			slog.String("format", cfg.Log.Format),
		),
		slog.Group("postgres",
			slog.String("dsn", redactDSN(cfg.Postgres.DSN)),
			slog.Int("max_conns", cfg.Postgres.MaxConns),
			slog.Int("min_conns", cfg.Postgres.MinConns),
			slog.Duration("max_conn_life", cfg.Postgres.MaxConnLife),
		),
		slog.Group("redis",
			slog.String("url", redactURL(cfg.Redis.URL)),
		),
		slog.Group("queue",
			slog.String("name", cfg.Queue.Name),
			slog.Int("concurrency", cfg.Queue.Concurrency),
			slog.Duration("shutdown_timeout", cfg.Queue.ShutdownTimeout),
		),
		slog.Group("parser",
			slog.Int("pool_size", cfg.Parser.PoolSize),
			slog.Duration("timeout", cfg.Parser.Timeout),
			slog.Duration("timeout_per_file", cfg.Parser.TimeoutPerFile),
			slog.Int64("max_file_size", cfg.Parser.MaxFileSize),
		),
		slog.Group("embedding",
			slog.String("ollama_url", redactURL(cfg.Embedding.OllamaURL)),
			slog.String("ollama_model", cfg.Embedding.OllamaModel),
		),
		slog.Group("qdrant",
			slog.String("url", redactURL(cfg.Qdrant.URL)),
		),
		slog.String("ssh_encryption_secret", "***"),
		slog.Group("workspace",
			slog.String("repo_cache_dir", cfg.Workspace.RepoCacheDir),
		),
		slog.Group("reaper",
			slog.Duration("stale_threshold", cfg.Reaper.StaleThreshold),
		),
	)
}

// kvPasswordDetect detects key-value password fields with optional whitespace around '='.
var kvPasswordDetect = regexp.MustCompile(`(?i)password\s*=`)
var kvPasswordSingleQuoted = regexp.MustCompile(`(?i)(password\s*=\s*)'[^']*'`)
var kvPasswordDoubleQuoted = regexp.MustCompile(`(?i)(password\s*=\s*)"[^"]*"`)
var kvPasswordUnquoted = regexp.MustCompile(`(?i)(password\s*=\s*)\S+`)

// redactDSN masks the password in a Postgres DSN.
// It handles both URL-style (postgres://user:pass@host/db) and
// key-value style (host=localhost password=secret dbname=mydb).
func redactDSN(dsn string) string {
	if kvPasswordDetect.MatchString(dsn) {
		result := kvPasswordSingleQuoted.ReplaceAllString(dsn, "${1}'***'")
		if result != dsn {
			return result
		}
		result = kvPasswordDoubleQuoted.ReplaceAllString(dsn, `${1}"***"`)
		if result != dsn {
			return result
		}
		return kvPasswordUnquoted.ReplaceAllString(dsn, "${1}***")
	}

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
func redactURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
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

func envOrDefault(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
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

// parseInt64 parses s as an int64; returns fallback on failure.
func parseInt64(s string, fallback int64) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		slog.Warn("failed to parse int64, using fallback", slog.String("value", s), slog.Any("error", err), slog.Int64("fallback", fallback))
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

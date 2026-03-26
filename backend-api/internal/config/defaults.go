package config

import "time"

// Server defaults.
const (
	DefaultServerPort              = 8080
	DefaultReadTimeout             = 5 * time.Second
	DefaultWriteTimeout            = time.Duration(0) // SSE requires unlimited writes.
	DefaultIdleTimeout             = 120 * time.Second
	DefaultShutdownTimeout         = 10 * time.Second
	DefaultReadHeaderTimeout       = 5 * time.Second
	DefaultBodyLimitBytes    int64 = 1 << 20 // 1 MB
)

// Postgres defaults.
const (
	DefaultPostgresMaxConns    = 25
	DefaultPostgresMinConns    = 5
	DefaultPostgresMaxConnLife = 30 * time.Minute
)

// Redis defaults.
const (
	DefaultRedisPoolSize = 10
)

// Embedding defaults.
const (
	DefaultEmbeddingProvider    = "ollama"
	DefaultEmbeddingEndpointURL = "http://host.docker.internal:11434"
	DefaultEmbeddingModel       = "jina/jina-embeddings-v2-base-en"
	DefaultEmbeddingDimensions  = 768
	DefaultEmbeddingMaxTokens   = 8000
	DefaultEmbeddingBatchSize   = 64
)

// LLM defaults.
const (
	DefaultPlatformProviderConfigName = "Platform Ollama" // TODO: extend when more platform default providers are supported.
	DefaultLLMProvider                = "ollama"
	DefaultLLMEndpointURL             = "http://host.docker.internal:11434"
	DefaultLLMModel                   = "codellama:7b"
)

// Indexing defaults.
const (
	DefaultIndexingDefaultBranch    = "main"
	DefaultIndexingMaxParallelFiles = 8
	DefaultIndexingMaxChunkTokens   = 512
	DefaultRepoCacheDir             = "/var/lib/myjungle/repos"
)

// Jobs defaults.
const (
	DefaultWorkerConcurrency       = 4
	DefaultIndexFullTimeout        = 2 * time.Hour
	DefaultIndexIncrementalTimeout = 30 * time.Minute
	DefaultMaxRetries              = 3
	DefaultUniqueWindow            = 1 * time.Hour
)

// Events defaults.
const (
	DefaultSSEKeepaliveInterval      = 30 * time.Second
	DefaultMaxSSEConnections         = 100
	DefaultMembershipRefreshInterval = 60 * time.Second
)

// Reaper defaults.
const (
	DefaultReaperStaleThreshold = 5 * time.Minute
)

// Session defaults.
const (
	DefaultSessionTTL        = 24 * time.Hour
	DefaultSessionCookieName = "session"
)

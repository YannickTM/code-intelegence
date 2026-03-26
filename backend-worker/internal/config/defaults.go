package config

import "time"

// Postgres defaults.
const (
	DefaultPostgresMaxConns    = 10
	DefaultPostgresMinConns    = 2
	DefaultPostgresMaxConnLife = 30 * time.Minute
)

// Queue (asynq) defaults.
const (
	DefaultQueueName         = "default"
	DefaultWorkerConcurrency = 4
	DefaultShutdownTimeout   = 30 * time.Second
)

// Parser defaults.
const (
	DefaultParserTimeout        = 5 * time.Minute
	DefaultParserPoolSize       = 0                  // 0 means runtime.NumCPU()
	DefaultParserTimeoutPerFile = 30 * time.Second
	DefaultParserMaxFileSize    = 10 * 1024 * 1024   // 10 MB
)

// Embedding defaults.
const (
	DefaultOllamaURL   = "http://host.docker.internal:11434"
	DefaultOllamaModel = "jina/jina-embeddings-v2-base-en"
)

// Qdrant defaults.
const (
	DefaultQdrantURL = "http://qdrant:6333"
)

// Reaper defaults.
const (
	DefaultReaperStaleThreshold = 5 * time.Minute
)

// Workspace defaults.
const (
	DefaultRepoCacheDir = "/var/lib/myjungle/repos"
)

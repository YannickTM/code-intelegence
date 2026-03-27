// Package app wires dependencies and runs the worker lifecycle.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"myjungle/backend-worker/internal/artifact"
	"myjungle/backend-worker/internal/config"
	"myjungle/backend-worker/internal/gitclient"
	"myjungle/backend-worker/internal/indexing"
	"myjungle/backend-worker/internal/notify"
	"myjungle/backend-worker/internal/parser/engine"
	"myjungle/backend-worker/internal/queue"
	"myjungle/backend-worker/internal/reaper"
	"myjungle/backend-worker/internal/registry"
	"myjungle/backend-worker/internal/repository"
	"myjungle/backend-worker/internal/sshenv"
	"myjungle/backend-worker/internal/storage/postgres"
	"myjungle/backend-worker/internal/vectorstore"
	"myjungle/backend-worker/internal/workflow"
	"myjungle/backend-worker/internal/workflow/commits"
	"myjungle/backend-worker/internal/workflow/fullindex"
	"myjungle/backend-worker/internal/workflow/incremental"
	"myjungle/backend-worker/internal/workspace"
	sqlcdb "myjungle/datastore/postgres/sqlc"
)

// supportedWorkflows lists the workflow names this worker handles.
var supportedWorkflows = []string{"full-index", "incremental-index"}

// App holds all worker dependencies and manages the lifecycle.
type App struct {
	Config     *config.Config
	DB         *postgres.DB
	Queue      *queue.Client
	Registry   *registry.Registry
	Dispatcher *workflow.Dispatcher
	Repo       *repository.JobRepository
	Parser     *engine.Engine
	Reaper     *reaper.Reaper

	reaperRedis *redis.Client
	reaperPub   *notify.EventPublisher
}

// New creates and wires all dependencies.
// It returns an error if any dependency fails to initialize.
func New(cfg *config.Config) (*App, error) {
	db, err := postgres.New(context.Background(), cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	queueClient, err := queue.New(cfg.Redis, cfg.Queue)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("queue: %w", err)
	}

	workerID := resolveWorkerID()

	reg, err := registry.New(cfg.Redis.URL, workerID, supportedWorkflows)
	if err != nil {
		queueClient.Server.Shutdown()
		db.Close()
		return nil, fmt.Errorf("registry: %w", err)
	}

	// PinnedQueryFunc acquires a dedicated connection for session-scoped
	// operations (advisory locks). Release returns it to the pool; Destroy
	// removes it from the pool entirely (used when session state like
	// advisory locks could not be cleaned up).
	pinnedFn := repository.PinnedQueryFunc(func(ctx context.Context) (*repository.PinnedConn, error) {
		conn, err := db.Pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		return &repository.PinnedConn{
			Querier: sqlcdb.New(conn),
			Release: func() { conn.Release() },
			Destroy: func() { conn.Hijack().Close(context.Background()) },
		}, nil
	})

	repo, err := repository.New(db.Queries, cfg.SSH.EncryptionSecret, pinnedFn)
	if err != nil {
		reg.Close()
		queueClient.Server.Shutdown()
		db.Close()
		return nil, fmt.Errorf("repository: %w", err)
	}

	// Reaper dependencies: separate Redis client and event publisher.
	reaperRedisOpts, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		reg.Close()
		queueClient.Server.Shutdown()
		db.Close()
		return nil, fmt.Errorf("reaper redis: %w", err)
	}
	reaperRedis := redis.NewClient(reaperRedisOpts)

	reaperPub, pubErr := notify.NewEventPublisher(cfg.Redis.URL)
	if pubErr != nil {
		slog.Warn("reaper: event publisher unavailable, SSE notifications disabled",
			slog.Any("error", pubErr))
	}

	rpr := reaper.New(
		reaper.Config{StaleThreshold: cfg.Reaper.StaleThreshold},
		db.Queries,
		reaperRedis,
		reaperPub,
	)

	// Workspace dependencies.
	gitClient := gitclient.New(gitclient.ExecRunner{})
	scanner := sshenv.ExecKeyscanner{}
	wsMgr := workspace.New(cfg.Workspace.RepoCacheDir, gitClient, scanner)

	// Embedded parser engine.
	parserEngine, err := engine.New(engine.Config{
		PoolSize:       cfg.Parser.PoolSize,
		TimeoutPerFile: cfg.Parser.TimeoutPerFile,
		MaxFileSize:    cfg.Parser.MaxFileSize,
	})
	if err != nil {
		reaperRedis.Close()
		if reaperPub != nil {
			reaperPub.Close()
		}
		reg.Close()
		queueClient.Server.Shutdown()
		db.Close()
		return nil, fmt.Errorf("parser: %w", err)
	}

	// Vector store and artifact writer (shared across jobs).
	qdrantClient := vectorstore.NewQdrantClient(cfg.Qdrant.URL)
	artWriter := artifact.NewWriter(db.Queries)

	// Commit indexer (shared across jobs).
	commitIdx := commits.New(db.Queries, gitClient)

	// Register workflow handlers.
	activate := indexing.TxActivator(db.Pool)
	fullIndexHandler := fullindex.NewHandler(repo, wsMgr, parserEngine, db.Queries, activate, artWriter, qdrantClient, commitIdx)

	incrementalHandler := incremental.NewHandler(repo, wsMgr, parserEngine, db.Queries, activate, artWriter, qdrantClient, gitClient, commitIdx)

	d := workflow.NewDispatcher()
	d.Register("full-index", fullIndexHandler)
	d.Register("incremental-index", incrementalHandler)

	return &App{
		Config:      cfg,
		DB:          db,
		Queue:       queueClient,
		Registry:    reg,
		Dispatcher:  d,
		Repo:        repo,
		Parser:      parserEngine,
		Reaper:      rpr,
		reaperRedis: reaperRedis,
		reaperPub:   reaperPub,
	}, nil
}

// Run starts the queue consumer and blocks until a shutdown signal is received.
func (a *App) Run() error {
	defer a.Close()

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	a.Registry.SetStatus(registry.StatusStarting)
	a.Registry.StartHeartbeat(ctx)

	// Startup reap: clean up any jobs orphaned by a previous crash.
	if a.Reaper != nil {
		reapCtx, reapCancel := context.WithTimeout(ctx, 30*time.Second)
		defer reapCancel()
		if n, err := a.Reaper.RunOnce(reapCtx); err != nil {
			slog.Warn("startup reap failed", slog.Any("error", err))
		} else if n > 0 {
			slog.Info("startup reap complete", slog.Int("reaped_jobs", n))
		}
	}

	mux := queue.BuildServeMux(a.Dispatcher.Handlers(), a.Registry)

	if err := a.Queue.Server.Start(mux); err != nil {
		return fmt.Errorf("app: start queue consumer: %w", err)
	}

	a.Registry.SetStatus(registry.StatusIdle)

	slog.Info("backend-worker started",
		slog.String("worker_id", a.Registry.WorkerID()))

	<-ctx.Done()

	a.Registry.SetStatus(registry.StatusDraining)
	slog.Info("backend-worker shutting down")
	return nil
}

// Close releases all held resources.
func (a *App) Close() {
	if a == nil {
		return
	}
	if a.Queue != nil && a.Queue.Server != nil {
		a.Queue.Server.Shutdown()
	}
	if a.Registry != nil {
		a.Registry.Close()
	}
	if a.Parser != nil {
		a.Parser.Close()
	}
	if a.reaperPub != nil {
		a.reaperPub.Close()
	}
	if a.reaperRedis != nil {
		a.reaperRedis.Close()
	}
	if a.DB != nil {
		a.DB.Close()
	}
	slog.Info("backend-worker cleanup complete")
}

// resolveWorkerID returns the worker identifier from WORKER_ID env var,
// falling back to os.Hostname().
func resolveWorkerID() string {
	if id := strings.TrimSpace(os.Getenv("WORKER_ID")); id != "" {
		return id
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

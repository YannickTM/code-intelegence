// Package app wires dependencies and runs the HTTP server.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"myjungle/backend-api/internal/apikey"
	"myjungle/backend-api/internal/config"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/embedding"
	"myjungle/backend-api/internal/handler"
	"myjungle/backend-api/internal/health"
	"myjungle/backend-api/internal/jobhealth"
	"myjungle/backend-api/internal/llm"
	"myjungle/backend-api/internal/metrics"
	"myjungle/backend-api/internal/middleware"
	"myjungle/backend-api/internal/queue"
	"myjungle/backend-api/internal/redisclient"
	"myjungle/backend-api/internal/sse"
	"myjungle/backend-api/internal/sshkey"
	"myjungle/backend-api/internal/storage/postgres"

	"github.com/go-chi/chi/v5"
)

// App holds all dependencies and the HTTP router.
type App struct {
	Config       *config.Config
	DB           *postgres.DB
	Router       chi.Router
	SSEHub       *sse.Hub // exported for downstream subscriber wiring
	publisher    queue.JobEnqueuer
	subscriber   *sse.Subscriber     // Redis pub/sub → SSE Hub bridge
	evtPublisher *sse.EventPublisher // Redis pub for membership events

	health      *handler.HealthHandler
	user        *handler.UserHandler
	auth        *handler.AuthHandler
	project     *handler.ProjectHandler
	membership  *handler.MembershipHandler
	sshKey      *handler.SSHKeyHandler
	apiKey      *handler.APIKeyHandler
	provider    *handler.ProviderHandler
	embedding   *handler.EmbeddingHandler
	llm         *handler.LLMHandler
	dashboard   *handler.DashboardHandler
	event       *handler.EventHandler
	commits     *handler.CommitHandler
	admin       *handler.AdminHandler
	worker      *handler.WorkerHandler
	redisReader *redisclient.Reader
}

// New creates and wires a new App instance.
// It returns an error if the database connection cannot be established.
func New(cfg *config.Config) (*App, error) {
	startedAt := time.Now().UTC()

	// Generate a random instance ID used to tag locally-originated SSE
	// events so the subscriber can skip re-broadcasting its own events.
	instanceID, err := generateInstanceID()
	if err != nil {
		return nil, fmt.Errorf("generate instance id: %w", err)
	}

	// Open database connection pool (skipped when DSN is empty, e.g. in tests).
	var db *postgres.DB
	if cfg.Postgres.DSN != "" {
		var err error
		db, err = postgres.New(context.Background(), cfg.Postgres)
		if err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		if err := embedding.BootstrapDefaultConfig(context.Background(), db, cfg.Embedding); err != nil {
			db.Close()
			return nil, fmt.Errorf("bootstrap embedding defaults: %w", err)
		}
		if err := llm.BootstrapDefaultConfig(context.Background(), db, cfg.LLM); err != nil {
			db.Close()
			return nil, fmt.Errorf("bootstrap llm defaults: %w", err)
		}

		// Bootstrap platform admin roles from environment variable.
		for _, username := range cfg.PlatformAdminUsernames {
			result, err := db.Queries.UpsertPlatformRoleByUsername(
				context.Background(), username)
			if err != nil {
				slog.Warn("bootstrap platform admin: insert failed",
					slog.String("username", username), slog.Any("error", err))
			} else if result.RowsAffected() == 0 {
				slog.Warn("bootstrap platform admin: no active user found",
					slog.String("username", username))
			} else {
				slog.Info("bootstrap platform admin: ensured platform_admin role",
					slog.String("username", username))
			}
		}
	}

	// Metrics collector and health checkers.
	collector := metrics.NewCollector(startedAt)
	checkers := []health.Checker{
		health.NewPostgresChecker(db),
		health.NewStubChecker("redis"),
		health.NewStubChecker("ollama"),
	}

	sshSvc, err := sshkey.NewService(cfg.SSH.EncryptionSecret)
	if err != nil {
		if db != nil {
			db.Close()
		}
		return nil, fmt.Errorf("ssh key service: %w", err)
	}
	providerHTTPClient := &http.Client{Timeout: 10 * time.Second}
	embeddingSvc, err := embedding.NewService(db, providerHTTPClient, cfg.ProviderEncryptionSecret)
	if err != nil {
		if db != nil {
			db.Close()
		}
		return nil, fmt.Errorf("embedding service: %w", err)
	}
	llmSvc, err := llm.NewService(db, providerHTTPClient, cfg.ProviderEncryptionSecret)
	if err != nil {
		if db != nil {
			db.Close()
		}
		return nil, fmt.Errorf("llm service: %w", err)
	}

	// Queue publisher (nil when REDIS_URL is not configured).
	// When REDIS_URL is set, publisher init failure is fatal — jobs would be
	// created in Postgres but never enqueued, leaving them stuck in "queued".
	var pub queue.JobEnqueuer
	if cfg.Redis.URL != "" {
		p, err := queue.NewPublisher(cfg.Redis.URL, queue.PublisherConfig{
			IndexFullTimeout:        cfg.Jobs.IndexFullTimeout,
			IndexIncrementalTimeout: cfg.Jobs.IndexIncrementalTimeout,
			MaxRetries:              cfg.Jobs.MaxRetries,
		})
		if err != nil {
			if db != nil {
				db.Close()
			}
			return nil, fmt.Errorf("queue publisher: %w", err)
		}
		pub = p
	}

	sseHub := sse.NewHub(cfg.Events.MaxSSEConnections)

	// Install a membership loader so the Hub can periodically reconcile
	// in-memory project sets against the database (guards against missed
	// Redis pub/sub deltas).
	if db != nil {
		sseHub.SetMembershipLoader(func(ctx context.Context, userID string) (map[string]struct{}, error) {
			uuid, err := dbconv.StringToPgUUID(userID)
			if err != nil {
				return nil, err
			}
			pgIDs, err := db.Queries.ListUserProjectIDs(ctx, uuid)
			if err != nil {
				return nil, err
			}
			pids := make(map[string]struct{}, len(pgIDs))
			for _, pid := range pgIDs {
				pids[dbconv.PgUUIDToString(pid)] = struct{}{}
			}
			return pids, nil
		})
	}

	// cleanup closes partially-initialised resources on error paths.
	cleanup := func() {
		if db != nil {
			db.Close()
		}
		if pub != nil {
			_ = pub.Close()
		}
	}

	// SSE subscriber (optional — skip when REDIS_URL is empty).
	var sub *sse.Subscriber
	if cfg.Redis.URL != "" {
		var subErr error
		sub, subErr = sse.NewSubscriber(sseHub, cfg.Redis.URL, cfg.Redis.PoolSize, instanceID)
		if subErr != nil {
			cleanup()
			return nil, fmt.Errorf("SSE subscriber: %w", subErr)
		}
	}

	// SSE event publisher (optional — skip when REDIS_URL is empty).
	// Publishes membership events to Redis so remote API instances receive
	// them via their Subscriber and update their Hub state accordingly.
	var evtPub *sse.EventPublisher
	if cfg.Redis.URL != "" {
		var epErr error
		evtPub, epErr = sse.NewEventPublisher(cfg.Redis.URL, cfg.Redis.PoolSize)
		if epErr != nil {
			_ = sub.Close()
			cleanup()
			return nil, fmt.Errorf("SSE event publisher: %w", epErr)
		}
	}

	// Redis reader for worker status (optional — skip when REDIS_URL is empty).
	var redisReader *redisclient.Reader
	if cfg.Redis.URL != "" {
		var rrErr error
		redisReader, rrErr = redisclient.NewReader(cfg.Redis.URL, 3) // small pool: admin-only reads
		if rrErr != nil {
			_ = evtPub.Close()
			_ = sub.Close()
			cleanup()
			return nil, fmt.Errorf("redis reader: %w", rrErr)
		}
	}

	// Lazy job reaper: checks worker heartbeats on API read paths.
	var jh *jobhealth.Checker
	if redisReader != nil && db != nil {
		jh = jobhealth.NewChecker(redisReader, db, evtPub, cfg.Reaper.StaleThreshold)
	}

	a := &App{
		Config:       cfg,
		DB:           db,
		Router:       chi.NewRouter(),
		SSEHub:       sseHub,
		publisher:    pub,
		subscriber:   sub,
		evtPublisher: evtPub,
		health:       handler.NewHealthHandler(startedAt, checkers, collector),
		user:         handler.NewUserHandler(db, jh),
		auth:         handler.NewAuthHandler(db, cfg.Session),
		project:      handler.NewProjectHandler(db, sshSvc, embeddingSvc, llmSvc, pub, jh),
		membership:   handler.NewMembershipHandler(db, sseHub, evtPub, instanceID),
		sshKey:       handler.NewSSHKeyHandler(db, sshSvc),
		apiKey:       handler.NewAPIKeyHandler(apikey.NewService(db)),
		provider:     handler.NewProviderHandler(),
		embedding:    handler.NewEmbeddingHandler(embeddingSvc),
		llm:          handler.NewLLMHandler(llmSvc),
		dashboard:    handler.NewDashboardHandler(db),
		event:        handler.NewEventHandler(sseHub, db, cfg.Events.SSEKeepaliveInterval),
		commits:      handler.NewCommitHandler(db),
		admin:        handler.NewAdminHandler(db),
		redisReader:  redisReader,
	}

	// Worker handler requires Redis; remains nil when REDIS_URL is not set.
	if redisReader != nil {
		a.worker = handler.NewWorkerHandler(redisReader)
	}

	// Middleware chain: requestid → logging → metrics → recover → CORS → bodylimit → routes
	a.Router.Use(middleware.RequestID)
	a.Router.Use(middleware.Logging)
	a.Router.Use(middleware.Metrics(collector))
	a.Router.Use(middleware.Recover)
	a.Router.Use(middleware.NewCORS(cfg.Server.CORSAllowedOrigins, cfg.Server.CORSWildcard))
	bodyLimit := cfg.Server.BodyLimitBytes
	if bodyLimit <= 0 {
		bodyLimit = config.DefaultBodyLimitBytes
	}
	a.Router.Use(middleware.BodyLimit(bodyLimit))

	a.registerRoutes()
	return a, nil
}

// SetPublisher replaces the queue publisher on both the App and the ProjectHandler.
// This is intended for integration tests that need to inject a fake publisher.
func (a *App) SetPublisher(p queue.JobEnqueuer) {
	a.publisher = p
	a.project.SetPublisher(p)
}

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It returns a non-nil error if the server fails to start (e.g. port in use).
func (a *App) Run() error {
	if a.DB != nil {
		defer a.DB.Close()
	}
	if a.publisher != nil {
		defer func() {
			if err := a.publisher.Close(); err != nil {
				slog.Error("queue publisher close error", slog.Any("error", err))
			}
		}()
	}

	// Close SSE event publisher on shutdown.
	if a.evtPublisher != nil {
		defer func() {
			if err := a.evtPublisher.Close(); err != nil {
				slog.Error("SSE event publisher close error", slog.Any("error", err))
			}
		}()
	}

	// Close Redis reader on shutdown.
	if a.redisReader != nil {
		defer func() {
			if err := a.redisReader.Close(); err != nil {
				slog.Error("redis reader close error", slog.Any("error", err))
			}
		}()
	}

	// Start SSE subscriber goroutine for real-time worker events.
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()
	if a.subscriber != nil {
		// After a Redis reconnect, refresh every connected client's project
		// membership from the database to correct any missed deltas. Uses
		// subCtx (cancelled on shutdown) with an additional timeout so the
		// reconciliation is bounded and shutdown-aware.
		a.subscriber.OnReconnect = func() {
			slog.Info("SSE subscriber reconnected, refreshing Hub memberships")
			ctx, cancel := context.WithTimeout(subCtx, 30*time.Second)
			defer cancel()
			a.SSEHub.RefreshAllMemberships(ctx)
		}
		go a.subscriber.Listen(subCtx)
		defer func() {
			if err := a.subscriber.Close(); err != nil {
				slog.Error("SSE subscriber close error", slog.Any("error", err))
			}
		}()
	}

	// Periodic membership refresh: reconcile Hub's in-memory project sets
	// against the database to correct missed Redis pub/sub deltas.
	go a.SSEHub.RunPeriodicRefresh(subCtx, a.Config.Events.MembershipRefreshInterval)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", a.Config.Server.Port),
		Handler:           a.Router,
		ReadHeaderTimeout: a.Config.Server.ReadHeaderTimeout,
		ReadTimeout:       a.Config.Server.ReadTimeout,
		WriteTimeout:      a.Config.Server.WriteTimeout,
		IdleTimeout:       a.Config.Server.IdleTimeout,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		slog.Info("backend-api listening", slog.Int("port", a.Config.Server.Port))
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	var serverErr error
	select {
	case sig := <-stop:
		slog.Info("received shutdown signal", slog.String("signal", sig.String()))
	case serverErr = <-serverErrCh:
		// Will be returned after graceful shutdown.
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.Config.Server.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", slog.Any("error", err))
	}
	return serverErr
}

// generateInstanceID returns a random 16-hex-character string that uniquely
// identifies this API process. Used as the Origin field in SSE events so
// that the local Subscriber can skip re-broadcasting its own events.
func generateInstanceID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

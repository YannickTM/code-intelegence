// Package queue provides the asynq-based task consumer for the worker.
package queue

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/hibiken/asynq"

	"myjungle/backend-worker/internal/config"
)

// Client holds the asynq Server (consumer) that will be started later.
// In Task 01 the server is created but not started; Task 02 will register
// handlers on a ServeMux and call srv.Run(mux).
type Client struct {
	Server *asynq.Server
}

// New creates an asynq Server from the worker config.
// It parses the Redis URL and configures concurrency/queues/shutdown.
// The server is NOT started; call Client.Server.Run(mux) after registering handlers.
func New(redisCfg config.RedisConfig, queueCfg config.QueueConfig) (*Client, error) {
	opt, err := asynq.ParseRedisURI(redisCfg.URL)
	if err != nil {
		return nil, fmt.Errorf("queue: parse redis url: %w", err)
	}

	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency:     queueCfg.Concurrency,
		Queues:          map[string]int{queueCfg.Name: 1},
		ShutdownTimeout: queueCfg.ShutdownTimeout,
		Logger:          newAsynqLogger(),
	})

	slog.Info("queue client created",
		slog.Int("concurrency", queueCfg.Concurrency),
		slog.String("queue", queueCfg.Name))

	return &Client{
		Server: srv,
	}, nil
}

// asynqLogger adapts slog to asynq's Logger interface.
type asynqLogger struct{}

func newAsynqLogger() *asynqLogger { return &asynqLogger{} }

func (l *asynqLogger) Debug(args ...interface{}) { slog.Debug(fmt.Sprint(args...)) }
func (l *asynqLogger) Info(args ...interface{})  { slog.Info(fmt.Sprint(args...)) }
func (l *asynqLogger) Warn(args ...interface{})  { slog.Warn(fmt.Sprint(args...)) }
func (l *asynqLogger) Error(args ...interface{}) { slog.Error(fmt.Sprint(args...)) }
func (l *asynqLogger) Fatal(args ...interface{}) {
	slog.Error(fmt.Sprint(args...))
	os.Exit(1)
}

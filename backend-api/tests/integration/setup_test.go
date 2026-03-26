//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"myjungle/backend-api/internal/app"
	"myjungle/backend-api/internal/config"
	"myjungle/backend-api/internal/embedding"
	"myjungle/backend-api/internal/llm"
	"myjungle/backend-api/internal/storage/postgres"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// testDSN holds the connection string used by all integration tests.
// It is set once in TestMain and never modified afterwards.
var testDSN string

// testContainer is the testcontainers container, nil when using an external DSN.
var testContainer testcontainers.Container

// testApp is a shared app instance reused across integration tests.
// Tests are not marked parallel, so reusing the app avoids repeated DB pool
// creation and startup pings without introducing cross-test concurrency.
var testApp *app.App

func TestMain(m *testing.M) {
	// Use structured text output so test logs are human-readable.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	exitCode := 0
	defer func() {
		if testApp != nil && testApp.DB != nil {
			testApp.DB.Close()
		}
		// Capture panics so os.Exit doesn't mask the failure.
		if r := recover(); r != nil {
			slog.Error("TestMain panic", slog.Any("error", r))
			exitCode = 1
		}
		// Cleanup: terminate container if we started one.
		if testContainer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = testContainer.Terminate(ctx)
		}
		os.Exit(exitCode)
	}()

	ctx := context.Background()

	// Option B: external database (CI override).
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		testDSN = dsn
	} else {
		// Option A: spin up a Postgres container via testcontainers-go.
		ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
			tcpostgres.WithDatabase("myjungle_test"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			tcpostgres.BasicWaitStrategies(),
		)
		// Assign immediately: Run can return a non-nil container even on error
		// (e.g. container started but wait strategy failed). The deferred
		// cleanup will call Terminate if testContainer is non-nil.
		testContainer = ctr
		if err != nil {
			slog.Error("testcontainers: start postgres failed", slog.Any("error", err))
			exitCode = 1
			return
		}

		dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			slog.Error("testcontainers: connection string failed", slog.Any("error", err))
			exitCode = 1
			return
		}
		testDSN = dsn
	}

	// Run migrations against the test database.
	if err := postgres.Migrate(testDSN); err != nil {
		slog.Error("migrate failed", slog.Any("error", err))
		exitCode = 1
		return
	}

	cfg := config.LoadForTest()
	cfg.Postgres.DSN = testDSN

	var err error
	testApp, err = app.New(cfg)
	if err != nil {
		slog.Error("app.New failed", slog.Any("error", err))
		exitCode = 1
		return
	}

	exitCode = m.Run()
}

// setupTestApp returns the shared App backed by the test database.
func setupTestApp(t *testing.T) *app.App {
	t.Helper()

	if testApp == nil {
		t.Fatal("setupTestApp: shared test app is not initialized")
	}
	return testApp
}

// truncateAll removes all rows from user-data tables. Foreign key cascades
// ensure referential integrity is maintained.
func truncateAll(t *testing.T, a *app.App) {
	t.Helper()

	tables := []string{
		"sessions",
		"query_log",
		"api_keys",
		"dependencies",
		"code_chunks",
		"symbols",
		"files",
		"indexing_jobs",
		"index_snapshots",
		"embedding_versions",
		"llm_provider_configs",
		"embedding_provider_configs",
		"project_ssh_key_assignments",
		"ssh_keys",
		"project_members",
		"projects",
		"users",
	}
	query := "TRUNCATE " + strings.Join(tables, ", ") + " CASCADE"
	_, err := a.DB.Pool.Exec(context.Background(), query)
	if err != nil {
		t.Fatalf("truncateAll: %v", err)
	}
	if err := embedding.BootstrapDefaultConfig(context.Background(), a.DB, a.Config.Embedding); err != nil {
		t.Fatalf("truncateAll bootstrap embedding: %v", err)
	}
	if err := llm.BootstrapDefaultConfig(context.Background(), a.DB, a.Config.LLM); err != nil {
		t.Fatalf("truncateAll bootstrap llm: %v", err)
	}
}

// doRequest sends an HTTP request through the app router and returns the recorder.
func doRequest(t *testing.T, a *app.App, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("doRequest: marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	a.Router.ServeHTTP(w, req)
	return w
}

// decodeJSON decodes the response body into a map.
func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decodeJSON: %v (body=%s)", err, w.Body.String())
	}
	return m
}

// registerUser registers a user and returns the decoded response body.
func registerUser(t *testing.T, a *app.App, username string) map[string]any {
	t.Helper()

	w := doRequest(t, a, http.MethodPost, "/v1/users", map[string]any{
		"username": username,
		"email":    username + "@example.com",
	}, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("registerUser(%q): status=%d, body=%s", username, w.Code, w.Body.String())
	}
	return decodeJSON(t, w)
}

// loginUser logs in and returns the raw session token.
func loginUser(t *testing.T, a *app.App, username string) string {
	t.Helper()

	w := doRequest(t, a, http.MethodPost, "/v1/auth/login", map[string]any{
		"username": username,
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("loginUser(%q): status=%d, body=%s", username, w.Code, w.Body.String())
	}

	// Decode inline instead of using decodeJSON because we only need the token.
	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("loginUser(%q): decode: %v", username, err)
	}
	token, ok := m["token"].(string)
	if !ok || token == "" {
		t.Fatalf("loginUser(%q): missing or empty token in response", username)
	}
	return token
}

// authHeader returns a header map with a Bearer token.
func authHeader(token string) map[string]string {
	return map[string]string{"Authorization": fmt.Sprintf("Bearer %s", token)}
}

// doRequestDirect sends an HTTP request through the app router and returns the
// recorder. Unlike doRequest it does not use testing.T, which avoids panics if
// a goroutine finishes after the test frame returns. The goroutines must still
// complete within the current test's lifetime because they share testApp and
// the database.
func doRequestDirect(a *app.App, method, path string, body any, headers map[string]string) (*httptest.ResponseRecorder, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	a.Router.ServeHTTP(w, req)
	return w, nil
}

// doRawRequest sends an HTTP request with a raw string body (not JSON-marshaled).
// Use this instead of doRequest when you need control over the exact body bytes
// (e.g. whitespace-only bodies, empty JSON objects, or oversized payloads).
func doRawRequest(t *testing.T, a *app.App, method, path, rawBody string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody io.Reader
	if rawBody != "" {
		reqBody = strings.NewReader(rawBody)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if rawBody != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	a.Router.ServeHTTP(w, req)
	return w
}

// createSSHKey creates an SSH key via the API, asserts 201, and returns the key ID.
func createSSHKey(t *testing.T, a *app.App, name, token string) string {
	t.Helper()
	w := doRequest(t, a, http.MethodPost, "/v1/ssh-keys", map[string]any{"name": name}, authHeader(token))
	if w.Code != http.StatusCreated {
		t.Fatalf("create ssh key %q: status = %d, want %d (body=%s)", name, w.Code, http.StatusCreated, w.Body.String())
	}
	m := decodeJSON(t, w)
	id, ok := m["id"].(string)
	if !ok || id == "" {
		t.Fatalf("create ssh key %q: missing or empty id in response", name)
	}
	return id
}

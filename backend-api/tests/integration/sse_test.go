//go:build integration

package integration_test

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"context"
)

// readSSEEvent reads lines from an SSE response body until a blank line
// (end of an SSE event block) or the context deadline is reached.
// Returns the concatenated lines of the first event.
func readSSEEvent(t *testing.T, body io.Reader) string {
	t.Helper()

	scanner := bufio.NewScanner(body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" && len(lines) > 0 {
			break // blank line = end of SSE event
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil && !strings.Contains(err.Error(), "context") {
		t.Fatalf("readSSEEvent: scan error: %v", err)
	}
	return strings.Join(lines, "\n")
}

func TestSSE_Stream_Unauthenticated(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	w := doRequest(t, a, http.MethodGet, "/v1/events/stream", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestSSE_Stream_ConnectedEvent(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")

	ts := httptest.NewServer(a.Router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", cc, "no-cache")
	}

	event := readSSEEvent(t, resp.Body)
	cancel()

	if !strings.Contains(event, "event: connected") {
		t.Errorf("expected 'event: connected' in %q", event)
	}
	if !strings.Contains(event, `"status":"connected"`) {
		t.Errorf("expected connected status in %q", event)
	}
	// Must NOT contain old stub event.
	if strings.Contains(event, "job:started") {
		t.Errorf("should not contain old stub event 'job:started' in %q", event)
	}
}

func TestSSE_LogStream_Member_ConnectedEvent(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	token := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", token)
	proj := createProject(t, a, "sse-test-proj", "https://github.com/example/repo.git", keyID, token)
	projID := mustString(t, proj, "id")

	ts := httptest.NewServer(a.Router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/v1/projects/%s/logs/stream", ts.URL, projID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	event := readSSEEvent(t, resp.Body)
	cancel()

	if !strings.Contains(event, "event: log:connected") {
		t.Errorf("expected 'event: log:connected' in %q", event)
	}
	if !strings.Contains(event, fmt.Sprintf(`"project_id":"%s"`, projID)) {
		t.Errorf("expected project_id in %q", event)
	}
}

func TestSSE_LogStream_NonMember_Forbidden(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// alice owns the project
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", aliceToken)
	proj := createProject(t, a, "sse-forbidden-proj", "https://github.com/example/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// bob is not a member
	registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")

	w := doRequest(t, a, http.MethodGet,
		fmt.Sprintf("/v1/projects/%s/logs/stream", projID),
		nil, authHeader(bobToken))

	// Non-member should get 404 from RequireProjectRole middleware
	// (project existence is hidden per ADR-008).
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestSSE_LogStream_APIKey_Member(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", aliceToken)
	proj := createProject(t, a, "sse-apikey-proj", "https://github.com/example/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Create a project API key with read access.
	_, apiKey := createProjectAPIKey(t, a, projID, "ci-key", "read", aliceToken)

	ts := httptest.NewServer(a.Router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/v1/projects/%s/logs/stream", ts.URL, projID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}

	event := readSSEEvent(t, resp.Body)
	cancel()

	if !strings.Contains(event, "event: log:connected") {
		t.Errorf("expected 'event: log:connected' in %q", event)
	}
}

//go:build integration

package integration_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSSE_MemberAdded_EventDelivered(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Setup: alice creates a project, bob will be added.
	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "deploy-key", aliceToken)
	proj := createProject(t, a, "sse-member-proj", "https://github.com/example/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)

	// Connect bob as SSE client (he is the targeted user for the notification).
	// Bob has no project memberships yet, but SendToUser routes by UserID.
	ts := httptest.NewServer(a.Router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+bobToken)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}

	// Read the initial "connected" event.
	connEvent := readSSEEvent(t, resp.Body)
	if !strings.Contains(connEvent, "event: connected") {
		t.Fatalf("expected connected event, got: %q", connEvent)
	}

	// Add bob as member via API (this triggers member:added SSE event).
	// Use doRequestDirect so that the goroutine never touches testing.T.
	apiErr := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		w, err := doRequestDirect(a, http.MethodPost,
			fmt.Sprintf("/v1/projects/%s/members", projID),
			map[string]any{"user_id": bobID, "role": "member"},
			authHeader(aliceToken))
		if err != nil {
			apiErr <- err
			return
		}
		if w.Code != http.StatusCreated {
			apiErr <- fmt.Errorf("add member: status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
			return
		}
		apiErr <- nil
	}()

	// Read the member:added event — bob receives it as a personal notification.
	memberEvent := readSSEEvent(t, resp.Body)
	cancel()

	if err := <-apiErr; err != nil {
		t.Error(err)
	}

	if !strings.Contains(memberEvent, "event: member:added") {
		t.Errorf("expected 'event: member:added' in %q", memberEvent)
	}
	if !strings.Contains(memberEvent, fmt.Sprintf(`"project_id":"%s"`, projID)) {
		t.Errorf("expected project_id in %q", memberEvent)
	}
	if !strings.Contains(memberEvent, fmt.Sprintf(`"user_id":"%s"`, bobID)) {
		t.Errorf("expected user_id (bob) in %q", memberEvent)
	}
	if !strings.Contains(memberEvent, fmt.Sprintf(`"actor_user_id":"%s"`, aliceID)) {
		t.Errorf("expected actor_user_id (alice) in %q", memberEvent)
	}
	if !strings.Contains(memberEvent, `"role":"member"`) {
		t.Errorf("expected role 'member' in %q", memberEvent)
	}
}

func TestSSE_MemberRoleUpdated_EventDelivered(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Setup: alice creates project, bob is a member.
	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "deploy-key", aliceToken)
	proj := createProject(t, a, "sse-role-proj", "https://github.com/example/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// Connect bob as SSE client (he is the targeted user).
	ts := httptest.NewServer(a.Router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+bobToken)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}

	// Read connected event.
	readSSEEvent(t, resp.Body)

	// Update bob's role to admin.
	// Use doRequestDirect so the goroutine never touches testing.T.
	apiErr := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		w, err := doRequestDirect(a, http.MethodPatch,
			fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
			map[string]any{"role": "admin"},
			authHeader(aliceToken))
		if err != nil {
			apiErr <- err
			return
		}
		if w.Code != http.StatusOK {
			apiErr <- fmt.Errorf("update role: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
			return
		}
		apiErr <- nil
	}()

	// Read the member:role_updated event — bob receives it as a personal notification.
	roleEvent := readSSEEvent(t, resp.Body)
	cancel()

	if err := <-apiErr; err != nil {
		t.Error(err)
	}

	if !strings.Contains(roleEvent, "event: member:role_updated") {
		t.Errorf("expected 'event: member:role_updated' in %q", roleEvent)
	}
	if !strings.Contains(roleEvent, fmt.Sprintf(`"project_id":"%s"`, projID)) {
		t.Errorf("expected project_id in %q", roleEvent)
	}
	if !strings.Contains(roleEvent, fmt.Sprintf(`"user_id":"%s"`, bobID)) {
		t.Errorf("expected user_id (bob) in %q", roleEvent)
	}
	if !strings.Contains(roleEvent, fmt.Sprintf(`"actor_user_id":"%s"`, aliceID)) {
		t.Errorf("expected actor_user_id (alice) in %q", roleEvent)
	}
	if !strings.Contains(roleEvent, `"role":"admin"`) {
		t.Errorf("expected role 'admin' in %q", roleEvent)
	}
}

func TestSSE_MemberRemoved_EventDelivered(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Setup: alice creates project, bob is a member.
	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "deploy-key", aliceToken)
	proj := createProject(t, a, "sse-remove-proj", "https://github.com/example/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// Connect bob as SSE client (he is the targeted user).
	ts := httptest.NewServer(a.Router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+bobToken)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}

	// Read connected event.
	readSSEEvent(t, resp.Body)

	// Remove bob.
	// Use doRequestDirect so the goroutine never touches testing.T.
	apiErr := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		w, err := doRequestDirect(a, http.MethodDelete,
			fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
			nil, authHeader(aliceToken))
		if err != nil {
			apiErr <- err
			return
		}
		if w.Code != http.StatusNoContent {
			apiErr <- fmt.Errorf("remove member: status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
			return
		}
		apiErr <- nil
	}()

	// Read the member:removed event — bob receives it as a personal notification.
	removeEvent := readSSEEvent(t, resp.Body)
	cancel()

	if err := <-apiErr; err != nil {
		t.Error(err)
	}

	if !strings.Contains(removeEvent, "event: member:removed") {
		t.Errorf("expected 'event: member:removed' in %q", removeEvent)
	}
	if !strings.Contains(removeEvent, fmt.Sprintf(`"user_id":"%s"`, bobID)) {
		t.Errorf("expected user_id (bob) in %q", removeEvent)
	}
	if !strings.Contains(removeEvent, fmt.Sprintf(`"actor_user_id":"%s"`, aliceID)) {
		t.Errorf("expected actor_user_id (alice) in %q", removeEvent)
	}
}

// readSSERaw reads lines from an SSE response body until a blank line
// (end of an SSE event block) or an error. Unlike readSSEEvent it does not
// use testing.T, making it safe to call from a goroutine that may outlive the test.
func readSSERaw(body io.Reader) (string, error) {
	scanner := bufio.NewScanner(body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" && len(lines) > 0 {
			break
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n"), scanner.Err()
}

func TestSSE_MemberEvent_NonMember_NotDelivered(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Setup: alice creates project A, charlie creates project B.
	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "deploy-key", aliceToken)
	createProject(t, a, "sse-proj-a", "https://github.com/example/a.git", keyID, aliceToken)

	registerUser(t, a, "charlie")
	charlieToken := loginUser(t, a, "charlie")
	keyID2 := createSSHKey(t, a, "deploy-key-2", charlieToken)
	projB := createProject(t, a, "sse-proj-b", "https://github.com/example/b.git", keyID2, charlieToken)
	projBID := mustString(t, projB, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)

	// Connect alice to SSE (she is only a member of project A).
	ts := httptest.NewServer(a.Router)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+aliceToken)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Read connected event.
	readSSEEvent(t, resp.Body)

	// Add bob to project B via API as charlie. The member:added event is a
	// personal notification for bob — alice should NOT receive it.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projBID),
		map[string]any{"user_id": bobID, "role": "member"},
		authHeader(charlieToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("add bob to proj B: status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	// Try to read the next SSE event with a short timeout. Because the
	// member:added event is a personal notification for bob, alice should
	// not receive anything.
	// We use readSSERaw (not readSSEEvent) so the goroutine never touches
	// testing.T — all assertions stay on the main goroutine.
	ch := make(chan string, 1)
	go func() {
		ev, _ := readSSERaw(resp.Body)
		ch <- ev
	}()

	select {
	case ev := <-ch:
		if ev != "" {
			t.Errorf("alice should NOT receive personal notification for bob, got: %q", ev)
		}
	case <-time.After(300 * time.Millisecond):
		// Expected: no event delivered within timeout.
		// Close the body to unblock the goroutine, then drain ch so the
		// goroutine exits cleanly and doesn't access t after the test ends.
		resp.Body.Close()
		<-ch
	}
}

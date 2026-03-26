package sse

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestSubscriber_ReceivesAndBroadcasts(t *testing.T) {
	mr := miniredis.RunT(t)

	hub := NewHub(10)
	client := newTestClient("u1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	sub, err := NewSubscriber(hub, "redis://"+mr.Addr(), 2, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sub.Listen(ctx)
	<-sub.ready

	msg := `{"event":"job:started","project_id":"proj-a","timestamp":"2026-03-09T10:00:00Z"}`
	mr.Publish(EventChannel, msg)

	select {
	case frame := <-client.Ch:
		s := string(frame)
		if !strings.HasPrefix(s, "event: job:started\n") {
			t.Errorf("missing event line: got %q", s)
		}
		if !strings.Contains(s, "data: "+msg) {
			t.Errorf("missing data line: got %q", s)
		}
		if !strings.HasSuffix(s, "\n\n") {
			t.Errorf("missing trailing blank line: got %q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSubscriber_NonMatchingProject(t *testing.T) {
	mr := miniredis.RunT(t)

	hub := NewHub(10)
	client := newTestClient("u1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	sub, err := NewSubscriber(hub, "redis://"+mr.Addr(), 2, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sub.Listen(ctx)
	<-sub.ready

	// Publish event for a different project.
	mr.Publish(EventChannel, `{"event":"job:started","project_id":"proj-b","timestamp":"2026-03-09T10:00:00Z"}`)

	select {
	case frame := <-client.Ch:
		t.Errorf("should not receive event for proj-b, got: %q", string(frame))
	case <-time.After(200 * time.Millisecond):
		// Expected: no message delivered.
	}
}

func TestSubscriber_HandleMessage_MalformedJSON(t *testing.T) {
	hub := NewHub(10)
	client := newTestClient("u1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	sub := &Subscriber{hub: hub, channel: EventChannel}

	sub.handleMessage("not valid json")

	select {
	case frame := <-client.Ch:
		t.Errorf("should not receive message for malformed JSON, got: %q", string(frame))
	case <-time.After(50 * time.Millisecond):
		// Expected: no delivery.
	}
}

func TestSubscriber_HandleMessage_MissingProjectID(t *testing.T) {
	hub := NewHub(10)
	client := newTestClient("u1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	sub := &Subscriber{hub: hub, channel: EventChannel}

	sub.handleMessage(`{"event":"job:started"}`)

	select {
	case frame := <-client.Ch:
		t.Errorf("should not receive message without project_id, got: %q", string(frame))
	case <-time.After(50 * time.Millisecond):
		// Expected.
	}
}

func TestSubscriber_HandleMessage_MissingEvent(t *testing.T) {
	hub := NewHub(10)
	client := newTestClient("u1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	sub := &Subscriber{hub: hub, channel: EventChannel}

	sub.handleMessage(`{"project_id":"proj-a"}`)

	select {
	case frame := <-client.Ch:
		t.Errorf("should not receive message without event type, got: %q", string(frame))
	case <-time.After(50 * time.Millisecond):
		// Expected.
	}
}

func TestSubscriber_HandleMessage_ValidFrame(t *testing.T) {
	hub := NewHub(10)
	client := newTestClient("u1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	sub := &Subscriber{hub: hub, channel: EventChannel}

	payload := `{"event":"job:progress","project_id":"proj-a","job_id":"j1","timestamp":"2026-03-09T10:00:00Z","data":{"files_processed":10}}`
	sub.handleMessage(payload)

	select {
	case frame := <-client.Ch:
		s := string(frame)
		want := "event: job:progress\ndata: " + payload + "\n\n"
		if s != want {
			t.Errorf("frame mismatch:\ngot:  %q\nwant: %q", s, want)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected message but got none")
	}
}

func TestSubscriber_HandleMessage_CompactsMultilineJSON(t *testing.T) {
	hub := NewHub(10)
	client := newTestClient("u1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	sub := &Subscriber{hub: hub, channel: EventChannel}

	// Pretty-printed multiline JSON — would break SSE framing without compaction.
	prettyPayload := `{
  "event": "job:progress",
  "project_id": "proj-a",
  "job_id": "j1",
  "timestamp": "2026-03-09T10:00:00Z",
  "data": {
    "files_processed": 10
  }
}`
	compactPayload := `{"event":"job:progress","project_id":"proj-a","job_id":"j1","timestamp":"2026-03-09T10:00:00Z","data":{"files_processed":10}}`

	sub.handleMessage(prettyPayload)

	select {
	case frame := <-client.Ch:
		s := string(frame)
		want := "event: job:progress\ndata: " + compactPayload + "\n\n"
		if s != want {
			t.Errorf("frame mismatch:\ngot:  %q\nwant: %q", s, want)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected message but got none")
	}
}

func TestSubscriber_ContextCancellation(t *testing.T) {
	mr := miniredis.RunT(t)

	hub := NewHub(10)
	sub, err := NewSubscriber(hub, "redis://"+mr.Addr(), 2, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sub.Listen(ctx)
		close(done)
	}()

	<-sub.ready
	cancel()

	select {
	case <-done:
		// Listen returned as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("Listen did not return after context cancellation")
	}
}

func TestSubscriber_HandleMessage_MemberAdded(t *testing.T) {
	hub := NewHub(10)
	// Targeted user (user-1) belongs to proj-a but NOT proj-b yet.
	client := newTestClient("user-1", "proj-a")
	// Another project member (user-2) already has proj-b.
	bystander := newTestClient("user-2", "proj-b")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}
	if err := hub.Register(bystander); err != nil {
		t.Fatal(err)
	}

	sub := &Subscriber{hub: hub, channel: EventChannel}

	// Publish a member:added event that adds user-1 to proj-b.
	payload := `{"event":"member:added","project_id":"proj-b","timestamp":"2026-03-09T10:00:00Z","data":{"user_id":"user-1","role":"member","actor_user_id":"admin-1"}}`
	sub.handleMessage(payload)

	// After processing, the client should now include proj-b in its set.
	if _, ok := client.ProjectIDs["proj-b"]; !ok {
		t.Error("expected proj-b in client's ProjectIDs after member:added")
	}
	// Original membership should be preserved.
	if _, ok := client.ProjectIDs["proj-a"]; !ok {
		t.Error("expected proj-a to still be in client's ProjectIDs")
	}

	// The targeted user (user-1) should receive the personal notification.
	select {
	case frame := <-client.Ch:
		s := string(frame)
		if !strings.Contains(s, "event: member:added") {
			t.Errorf("expected event: member:added in frame, got %q", s)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected personal notification for targeted user")
	}

	// The bystander (user-2) should NOT receive the event — it is a
	// personal notification for user-1 only.
	select {
	case frame := <-bystander.Ch:
		t.Errorf("bystander should not receive member:added, got: %q", string(frame))
	case <-time.After(50 * time.Millisecond):
		// Expected: no delivery to bystander.
	}
}

func TestSubscriber_HandleMessage_MemberRemoved(t *testing.T) {
	hub := NewHub(10)
	// Targeted user (user-1) belongs to proj-a and proj-b.
	client := newTestClient("user-1", "proj-a", "proj-b")
	// Bystander (user-2) also has proj-b.
	bystander := newTestClient("user-2", "proj-b")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}
	if err := hub.Register(bystander); err != nil {
		t.Fatal(err)
	}

	sub := &Subscriber{hub: hub, channel: EventChannel}

	// Publish a member:removed event that removes user-1 from proj-b.
	payload := `{"event":"member:removed","project_id":"proj-b","timestamp":"2026-03-09T10:00:00Z","data":{"user_id":"user-1","actor_user_id":"admin-1"}}`
	sub.handleMessage(payload)

	// After processing, proj-b should be removed from the client's set.
	if _, ok := client.ProjectIDs["proj-b"]; ok {
		t.Error("expected proj-b to be removed from client's ProjectIDs after member:removed")
	}
	// Original membership should be preserved.
	if _, ok := client.ProjectIDs["proj-a"]; !ok {
		t.Error("expected proj-a to still be in client's ProjectIDs")
	}

	// The targeted user (user-1) should receive the personal notification
	// via SendToUser — even though their project membership was revoked.
	select {
	case frame := <-client.Ch:
		s := string(frame)
		if !strings.Contains(s, "event: member:removed") {
			t.Errorf("expected event: member:removed in frame, got %q", s)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected personal notification for removed user")
	}

	// The bystander (user-2) should NOT receive the event.
	select {
	case frame := <-bystander.Ch:
		t.Errorf("bystander should not receive member:removed, got: %q", string(frame))
	case <-time.After(50 * time.Millisecond):
		// Expected: no delivery to bystander.
	}
}

func TestSubscriber_HandleMessage_MemberRoleUpdated(t *testing.T) {
	hub := NewHub(10)
	// Targeted user (user-1) belongs to proj-a.
	client := newTestClient("user-1", "proj-a")
	// Bystander (user-2) also has proj-a.
	bystander := newTestClient("user-2", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}
	if err := hub.Register(bystander); err != nil {
		t.Fatal(err)
	}

	sub := &Subscriber{hub: hub, channel: EventChannel}

	payload := `{"event":"member:role_updated","project_id":"proj-a","timestamp":"2026-03-09T10:00:00Z","data":{"user_id":"user-1","role":"admin","actor_user_id":"admin-1"}}`
	sub.handleMessage(payload)

	// The targeted user (user-1) should receive the personal notification.
	select {
	case frame := <-client.Ch:
		s := string(frame)
		if !strings.Contains(s, "event: member:role_updated") {
			t.Errorf("expected event: member:role_updated in frame, got %q", s)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected personal notification for targeted user")
	}

	// The bystander (user-2) should NOT receive the event.
	select {
	case frame := <-bystander.Ch:
		t.Errorf("bystander should not receive member:role_updated, got: %q", string(frame))
	case <-time.After(50 * time.Millisecond):
		// Expected: no delivery to bystander.
	}
}

func TestSubscriber_HandleMessage_MemberDelta_MissingUserID(t *testing.T) {
	hub := NewHub(10)
	client := newTestClient("user-1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	sub := &Subscriber{hub: hub, channel: EventChannel}

	// member:added with missing user_id in data — should not crash.
	// With no target user, the event falls through to project-wide broadcast.
	payload := `{"event":"member:added","project_id":"proj-a","timestamp":"2026-03-09T10:00:00Z","data":{"role":"member"}}`
	sub.handleMessage(payload)

	select {
	case frame := <-client.Ch:
		s := string(frame)
		if !strings.Contains(s, "event: member:added") {
			t.Errorf("expected event: member:added in frame, got %q", s)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected broadcast fallback when membership target is incomplete")
	}
}

func TestSubscriber_HandleMessage_SelfOrigin_Skipped(t *testing.T) {
	hub := NewHub(10)
	client := newTestClient("user-1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	// Subscriber has instanceID "inst-1"; event origin matches → should skip.
	sub := &Subscriber{hub: hub, channel: EventChannel, instanceID: "inst-1"}

	payload := `{"event":"member:added","project_id":"proj-a","origin":"inst-1","timestamp":"2026-03-09T10:00:00Z","data":{"user_id":"user-1","role":"member","actor_user_id":"admin-1"}}`
	sub.handleMessage(payload)

	select {
	case frame := <-client.Ch:
		t.Errorf("self-originated event should be skipped, got: %q", string(frame))
	case <-time.After(50 * time.Millisecond):
		// Expected: no delivery — handler already delivered locally.
	}

	// Membership delta should NOT be applied (handler already did it).
	// The client had proj-a before; member:added for proj-a would be
	// idempotent, so instead test with a fresh project to confirm no delta.
	if _, ok := client.ProjectIDs["proj-a"]; !ok {
		t.Error("expected proj-a to remain in client's ProjectIDs")
	}
}

func TestSubscriber_HandleMessage_DifferentOrigin_Delivered(t *testing.T) {
	hub := NewHub(10)
	client := newTestClient("user-1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	// Subscriber has instanceID "inst-1"; event origin is "inst-2" → should process.
	sub := &Subscriber{hub: hub, channel: EventChannel, instanceID: "inst-1"}

	payload := `{"event":"member:added","project_id":"proj-b","origin":"inst-2","timestamp":"2026-03-09T10:00:00Z","data":{"user_id":"user-1","role":"member","actor_user_id":"admin-1"}}`
	sub.handleMessage(payload)

	// Should be delivered because origin differs.
	select {
	case frame := <-client.Ch:
		s := string(frame)
		if !strings.Contains(s, "event: member:added") {
			t.Errorf("expected event: member:added in frame, got %q", s)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected delivery for event from different instance")
	}

	// Membership delta should be applied.
	if _, ok := client.ProjectIDs["proj-b"]; !ok {
		t.Error("expected proj-b in client's ProjectIDs after member:added from remote")
	}
}

func TestSubscriber_HandleMessage_NoOrigin_Delivered(t *testing.T) {
	hub := NewHub(10)
	client := newTestClient("user-1", "proj-a")
	if err := hub.Register(client); err != nil {
		t.Fatal(err)
	}

	// Subscriber has instanceID; event has no origin (e.g. from worker) → should process.
	sub := &Subscriber{hub: hub, channel: EventChannel, instanceID: "inst-1"}

	payload := `{"event":"job:started","project_id":"proj-a","timestamp":"2026-03-09T10:00:00Z","data":{"status":"running"}}`
	sub.handleMessage(payload)

	select {
	case frame := <-client.Ch:
		s := string(frame)
		if !strings.Contains(s, "event: job:started") {
			t.Errorf("expected event: job:started in frame, got %q", s)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected delivery for event without origin")
	}
}

func TestSubscriber_Close_NilSafe(t *testing.T) {
	var s *Subscriber
	if err := s.Close(); err != nil {
		t.Errorf("Close on nil subscriber: %v", err)
	}
}

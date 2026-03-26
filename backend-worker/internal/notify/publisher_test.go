package notify

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// helper starts miniredis, creates a publisher, and returns both along with
// a subscriber already listening on Channel.
func setup(t *testing.T) (*EventPublisher, *redis.Client, *redis.PubSub) {
	t.Helper()
	mr := miniredis.RunT(t)

	pub, err := NewEventPublisher("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("NewEventPublisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	sub := rdb.Subscribe(context.Background(), Channel)
	t.Cleanup(func() { sub.Close() })

	// Wait for subscription to be active.
	_, err = sub.Receive(context.Background())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	return pub, rdb, sub
}

// receive reads one message from the subscription with a timeout.
func receive(t *testing.T, sub *redis.PubSub) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("receive message: %v", err)
	}

	if msg.Channel != Channel {
		t.Errorf("channel = %q, want %q", msg.Channel, Channel)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

func TestPublish_SerializesAndPublishes(t *testing.T) {
	pub, _, sub := setup(t)

	evt := SSEEvent{
		Event:     "job:started",
		ProjectID: "proj-1",
		JobID:     "job-1",
		Timestamp: "2026-03-18T10:00:00Z",
		Data:      map[string]any{"status": "running"},
	}

	if err := pub.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	payload := receive(t, sub)

	if got := payload["event"]; got != "job:started" {
		t.Errorf("event = %v, want job:started", got)
	}
	if got := payload["project_id"]; got != "proj-1" {
		t.Errorf("project_id = %v, want proj-1", got)
	}
	if got := payload["job_id"]; got != "job-1" {
		t.Errorf("job_id = %v, want job-1", got)
	}
	if got := payload["timestamp"]; got != "2026-03-18T10:00:00Z" {
		t.Errorf("timestamp = %v, want 2026-03-18T10:00:00Z", got)
	}
}

func TestPublish_RequiredFields(t *testing.T) {
	pub, _, sub := setup(t)

	evt := SSEEvent{
		Event:     "job:completed",
		ProjectID: "proj-2",
		Timestamp: "2026-03-18T11:00:00Z",
	}

	if err := pub.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	payload := receive(t, sub)

	for _, field := range []string{"event", "project_id", "timestamp"} {
		if _, ok := payload[field]; !ok {
			t.Errorf("required field %q missing", field)
		}
	}
	// Optional fields should be absent when not set.
	if _, ok := payload["job_id"]; ok {
		t.Error("job_id should be omitted when empty")
	}
}

func TestPublishJobStarted(t *testing.T) {
	pub, _, sub := setup(t)
	pub.PublishJobStarted(context.Background(), "proj-1", "job-1", "full")

	payload := receive(t, sub)

	if got := payload["event"]; got != "job:started" {
		t.Errorf("event = %v, want job:started", got)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatal("data field missing or wrong type")
	}
	if got := data["status"]; got != "running" {
		t.Errorf("status = %v, want running", got)
	}
	if got := data["job_type"]; got != "full" {
		t.Errorf("job_type = %v, want full", got)
	}
}

func TestPublishJobProgress(t *testing.T) {
	pub, _, sub := setup(t)
	pub.PublishJobProgress(context.Background(), "proj-1", "job-1", "full", 10, 100, 50)

	payload := receive(t, sub)

	if got := payload["event"]; got != "job:progress" {
		t.Errorf("event = %v, want job:progress", got)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatal("data field missing or wrong type")
	}
	if got := data["files_processed"]; got != float64(10) {
		t.Errorf("files_processed = %v, want 10", got)
	}
	if got := data["files_total"]; got != float64(100) {
		t.Errorf("files_total = %v, want 100", got)
	}
	if got := data["chunks_upserted"]; got != float64(50) {
		t.Errorf("chunks_upserted = %v, want 50", got)
	}
}

func TestPublishJobCompleted(t *testing.T) {
	pub, _, sub := setup(t)
	pub.PublishJobCompleted(context.Background(), "proj-1", "job-1", "incremental", 200, 800, 30)

	payload := receive(t, sub)

	if got := payload["event"]; got != "job:completed" {
		t.Errorf("event = %v, want job:completed", got)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatal("data field missing or wrong type")
	}
	if got := data["status"]; got != "completed" {
		t.Errorf("status = %v, want completed", got)
	}
	if got := data["files_processed"]; got != float64(200) {
		t.Errorf("files_processed = %v, want 200", got)
	}
	if got := data["chunks_upserted"]; got != float64(800) {
		t.Errorf("chunks_upserted = %v, want 800", got)
	}
	if got := data["vectors_deleted"]; got != float64(30) {
		t.Errorf("vectors_deleted = %v, want 30", got)
	}
}

func TestPublishJobFailed(t *testing.T) {
	pub, _, sub := setup(t)
	pub.PublishJobFailed(context.Background(), "proj-1", "job-1", "full", "parser timeout")

	payload := receive(t, sub)

	if got := payload["event"]; got != "job:failed" {
		t.Errorf("event = %v, want job:failed", got)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatal("data field missing or wrong type")
	}
	if got := data["status"]; got != "failed" {
		t.Errorf("status = %v, want failed", got)
	}
	if got := data["error_message"]; got != "parser timeout" {
		t.Errorf("error_message = %v, want parser timeout", got)
	}
}

func TestPublishSnapshotActivated(t *testing.T) {
	pub, _, sub := setup(t)
	pub.PublishSnapshotActivated(context.Background(), "proj-1", "snap-1", "abc123")

	payload := receive(t, sub)

	if got := payload["event"]; got != "snapshot:activated" {
		t.Errorf("event = %v, want snapshot:activated", got)
	}
	if got := payload["snapshot_id"]; got != "snap-1" {
		t.Errorf("snapshot_id = %v, want snap-1", got)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatal("data field missing or wrong type")
	}
	if got := data["active_commit"]; got != "abc123" {
		t.Errorf("active_commit = %v, want abc123", got)
	}
}

func TestNilPublisher(t *testing.T) {
	var p *EventPublisher

	// None of these should panic.
	if err := p.Publish(context.Background(), SSEEvent{}); err != nil {
		t.Errorf("nil Publish returned error: %v", err)
	}
	p.PublishJobStarted(context.Background(), "p", "j", "full")
	p.PublishJobProgress(context.Background(), "p", "j", "full", 1, 2, 3)
	p.PublishJobCompleted(context.Background(), "p", "j", "full", 1, 2, 3)
	p.PublishJobFailed(context.Background(), "p", "j", "full", "err")
	p.PublishSnapshotActivated(context.Background(), "p", "s", "abc")
	if err := p.Close(); err != nil {
		t.Errorf("nil Close returned error: %v", err)
	}
}

func TestClose(t *testing.T) {
	mr := miniredis.RunT(t)
	pub, err := NewEventPublisher("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("NewEventPublisher: %v", err)
	}
	if err := pub.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

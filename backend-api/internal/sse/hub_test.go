package sse

import (
	"strings"
	"testing"
)

func newTestClient(userID string, projectIDs ...string) *Client {
	pids := make(map[string]struct{}, len(projectIDs))
	for _, pid := range projectIDs {
		pids[pid] = struct{}{}
	}
	return NewClient(userID, pids)
}

func TestHub_Register_UnderCapacity(t *testing.T) {
	h := NewHub(2)
	c := newTestClient("u1", "p1")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: unexpected error: %v", err)
	}
	if h.ClientCount() != 1 {
		t.Errorf("ClientCount = %d, want 1", h.ClientCount())
	}
}

func TestHub_Register_AtCapacity(t *testing.T) {
	h := NewHub(1)
	c1 := newTestClient("u1", "p1")
	c2 := newTestClient("u2", "p2")

	if err := h.Register(c1); err != nil {
		t.Fatalf("Register c1: unexpected error: %v", err)
	}

	err := h.Register(c2)
	if err == nil {
		t.Fatal("Register c2: expected error at capacity, got nil")
	}
	if h.ClientCount() != 1 {
		t.Errorf("ClientCount = %d, want 1", h.ClientCount())
	}
}

func TestHub_Unregister(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "p1")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	h.Unregister(c)

	if h.ClientCount() != 0 {
		t.Errorf("ClientCount = %d, want 0", h.ClientCount())
	}

	// done channel should be closed.
	select {
	case <-c.Done:
	default:
		t.Error("done channel not closed after Unregister")
	}
}

func TestHub_Unregister_Idempotent(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "p1")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	h.Unregister(c)
	// Second unregister should not panic.
	h.Unregister(c)

	if h.ClientCount() != 0 {
		t.Errorf("ClientCount = %d, want 0", h.ClientCount())
	}
}

func TestHub_Broadcast_MatchingClient(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "proj-a")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	msg := []byte("event: test\ndata: hello\n\n")
	h.Broadcast("proj-a", msg)

	select {
	case got := <-c.Ch:
		if string(got) != string(msg) {
			t.Errorf("got %q, want %q", got, msg)
		}
	default:
		t.Error("expected message on client channel, got nothing")
	}
}

func TestHub_Broadcast_SkipsNonMatchingClient(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "proj-a")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	h.Broadcast("proj-b", []byte("should not arrive"))

	select {
	case msg := <-c.Ch:
		t.Errorf("expected no message, got %q", msg)
	default:
		// OK — no message delivered.
	}
}

func TestHub_Broadcast_DropsOnFullChannel(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "proj-a")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Fill the channel.
	for i := 0; i < clientBufferSize; i++ {
		c.Ch <- []byte("filler")
	}

	// This should not block even though the channel is full.
	h.Broadcast("proj-a", []byte("dropped"))

	// Drain and verify we only have the filler messages.
	count := 0
	for {
		select {
		case <-c.Ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != clientBufferSize {
		t.Errorf("drained %d messages, want %d", count, clientBufferSize)
	}
}

func TestHub_Publish_FormatsSSEFrame(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "proj-a")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	h.Publish(SSEEvent{
		Event:     "job:started",
		ProjectID: "proj-a",
		Timestamp: "2026-03-09T10:00:00Z",
		Data:      map[string]any{"status": "running"},
	})

	select {
	case msg := <-c.Ch:
		s := string(msg)
		if !strings.HasPrefix(s, "event: job:started\n") {
			t.Errorf("missing event line, got: %q", s)
		}
		if !strings.Contains(s, "data: {") {
			t.Errorf("missing data line, got: %q", s)
		}
		if !strings.HasSuffix(s, "\n\n") {
			t.Errorf("frame should end with \\n\\n, got: %q", s)
		}
	default:
		t.Error("expected message on client channel")
	}
}

func TestHub_Publish_NilHub(t *testing.T) {
	var h *Hub
	// Should not panic.
	h.Publish(SSEEvent{
		Event:     "test",
		ProjectID: "proj-a",
		Timestamp: "2026-03-09T10:00:00Z",
	})
}

func TestHub_Publish_MarshalFailure(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "proj-a")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// A channel is not JSON-marshalable, so json.Marshal on SSEEvent will fail.
	h.Publish(SSEEvent{
		Event:     "bad:event",
		ProjectID: "proj-a",
		Timestamp: "2026-03-09T10:00:00Z",
		Data:      map[string]any{"broken": make(chan int)},
	})

	// No message should be delivered on marshal failure.
	select {
	case msg := <-c.Ch:
		t.Errorf("expected no message on marshal failure, got %q", msg)
	default:
		// OK — marshal failed, nothing sent.
	}
}

func TestHub_Broadcast_MultipleClients(t *testing.T) {
	h := NewHub(10)
	c1 := newTestClient("u1", "proj-a", "proj-b")
	c2 := newTestClient("u2", "proj-b")
	c3 := newTestClient("u3", "proj-c")

	for _, c := range []*Client{c1, c2, c3} {
		if err := h.Register(c); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	msg := []byte("event: test\ndata: for proj-b\n\n")
	h.Broadcast("proj-b", msg)

	// c1 and c2 should receive (both have proj-b).
	for _, c := range []*Client{c1, c2} {
		select {
		case got := <-c.Ch:
			if string(got) != string(msg) {
				t.Errorf("client %s: got %q, want %q", c.UserID, got, msg)
			}
		default:
			t.Errorf("client %s: expected message, got nothing", c.UserID)
		}
	}

	// c3 should not receive.
	select {
	case got := <-c3.Ch:
		t.Errorf("client %s: unexpected message: %q", c3.UserID, got)
	default:
	}
}

func TestHub_SendToUser_MatchingUser(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "proj-a")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	msg := []byte("event: member:added\ndata: hello\n\n")
	h.SendToUser("u1", msg)

	select {
	case got := <-c.Ch:
		if string(got) != string(msg) {
			t.Errorf("got %q, want %q", got, msg)
		}
	default:
		t.Error("expected message on client channel, got nothing")
	}
}

func TestHub_SendToUser_NonMatchingUser(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "proj-a")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	h.SendToUser("u2", []byte("should not arrive"))

	select {
	case msg := <-c.Ch:
		t.Errorf("expected no message, got %q", msg)
	default:
		// OK — no message delivered.
	}
}

func TestHub_SendToUser_MultipleClientsForSameUser(t *testing.T) {
	h := NewHub(10)
	c1 := newTestClient("u1", "proj-a")
	c2 := newTestClient("u1", "proj-b") // same user, different project
	c3 := newTestClient("u2", "proj-a") // different user

	for _, c := range []*Client{c1, c2, c3} {
		if err := h.Register(c); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	msg := []byte("event: member:added\ndata: for u1\n\n")
	h.SendToUser("u1", msg)

	// c1 and c2 should receive (both belong to u1).
	for _, c := range []*Client{c1, c2} {
		select {
		case got := <-c.Ch:
			if string(got) != string(msg) {
				t.Errorf("client %s: got %q, want %q", c.UserID, got, msg)
			}
		default:
			t.Errorf("client %s: expected message, got nothing", c.UserID)
		}
	}

	// c3 should not receive (belongs to u2).
	select {
	case got := <-c3.Ch:
		t.Errorf("client %s: unexpected message: %q", c3.UserID, got)
	default:
	}
}

func TestHub_SendToUser_DropsOnFullChannel(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "proj-a")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Fill the channel.
	for i := 0; i < clientBufferSize; i++ {
		c.Ch <- []byte("filler")
	}

	// This should not block even though the channel is full.
	h.SendToUser("u1", []byte("dropped"))

	// Drain and verify we only have the filler messages.
	count := 0
	for {
		select {
		case <-c.Ch:
			count++
		default:
			goto done2
		}
	}
done2:
	if count != clientBufferSize {
		t.Errorf("drained %d messages, want %d", count, clientBufferSize)
	}
}

func TestHub_SendToUser_NilHub(t *testing.T) {
	var h *Hub
	// Should not panic.
	h.SendToUser("u1", []byte("hello"))
}

func TestHub_PublishToUser_FormatsSSEFrame(t *testing.T) {
	h := NewHub(10)
	c := newTestClient("u1", "proj-a")

	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}

	h.PublishToUser("u1", SSEEvent{
		Event:     "member:added",
		ProjectID: "proj-a",
		Timestamp: "2026-03-09T10:00:00Z",
		Data:      map[string]any{"user_id": "u1", "role": "member"},
	})

	select {
	case msg := <-c.Ch:
		s := string(msg)
		if !strings.HasPrefix(s, "event: member:added\n") {
			t.Errorf("missing event line, got: %q", s)
		}
		if !strings.Contains(s, "data: {") {
			t.Errorf("missing data line, got: %q", s)
		}
		if !strings.HasSuffix(s, "\n\n") {
			t.Errorf("frame should end with \\n\\n, got: %q", s)
		}
	default:
		t.Error("expected message on client channel")
	}
}

func TestHub_PublishToUser_NilHub(t *testing.T) {
	var h *Hub
	// Should not panic.
	h.PublishToUser("u1", SSEEvent{
		Event:     "member:added",
		ProjectID: "proj-a",
		Timestamp: "2026-03-09T10:00:00Z",
	})
}

func TestHub_HasCapacity(t *testing.T) {
	h := NewHub(1)
	if !h.HasCapacity() {
		t.Error("expected capacity on empty hub")
	}

	c := newTestClient("u1", "p1")
	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if h.HasCapacity() {
		t.Error("expected no capacity when at max")
	}

	h.Unregister(c)
	if !h.HasCapacity() {
		t.Error("expected capacity after unregister")
	}
}

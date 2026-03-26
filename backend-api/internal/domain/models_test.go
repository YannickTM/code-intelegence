package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUser_JSONRoundTrip(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	u := User{
		ID:          "abc-123",
		Username:    "alice",
		DisplayName: "Alice Smith",
		AvatarURL:   "https://example.com/avatar.png",
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	wantKeys := []string{"id", "username", "display_name", "avatar_url", "is_active", "created_at", "updated_at"}
	for _, k := range wantKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing JSON key %q", k)
		}
	}

	if m["id"] != "abc-123" {
		t.Errorf("id = %v, want %q", m["id"], "abc-123")
	}
	if m["username"] != "alice" {
		t.Errorf("username = %v, want %q", m["username"], "alice")
	}
	if m["is_active"] != true {
		t.Errorf("is_active = %v, want true", m["is_active"])
	}
}

func TestUser_JSONOmitsEmpty(t *testing.T) {
	u := User{
		ID:       "abc-123",
		Username: "alice",
		IsActive: true,
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if _, ok := m["display_name"]; ok {
		t.Error("expected display_name to be omitted when empty")
	}
	if _, ok := m["avatar_url"]; ok {
		t.Error("expected avatar_url to be omitted when empty")
	}
}

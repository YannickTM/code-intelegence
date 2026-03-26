package dbconv

import (
	"testing"
	"time"

	"myjungle/backend-api/internal/domain"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestDBUserToDomain(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	u := makeDBUser(t, "alice", "alice@example.com", "Alice A", "https://example.com/alice.png", now)

	d := DBUserToDomain(u)

	if d.Username != "alice" {
		t.Errorf("Username = %q, want %q", d.Username, "alice")
	}
	if d.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", d.Email, "alice@example.com")
	}
	if d.DisplayName != "Alice A" {
		t.Errorf("DisplayName = %q, want %q", d.DisplayName, "Alice A")
	}
	if d.AvatarURL != "https://example.com/alice.png" {
		t.Errorf("AvatarURL = %q, want %q", d.AvatarURL, "https://example.com/alice.png")
	}
	if !d.IsActive {
		t.Error("IsActive = false, want true")
	}
	const wantID = "00000000-0000-0000-0000-000000000001"
	if d.ID != wantID {
		t.Errorf("ID = %q, want %q", d.ID, wantID)
	}
	if !d.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", d.CreatedAt, now)
	}
	if !d.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", d.UpdatedAt, now)
	}
}

func TestDBUserToDomain_NullOptionals(t *testing.T) {
	u := makeDBUser(t, "bob", "bob@example.com", "", "", time.Now())
	u.DisplayName = pgtype.Text{}
	u.AvatarUrl = pgtype.Text{}

	d := DBUserToDomain(u)

	if d.DisplayName != "" {
		t.Errorf("DisplayName = %q, want empty", d.DisplayName)
	}
	if d.AvatarURL != "" {
		t.Errorf("AvatarURL = %q, want empty", d.AvatarURL)
	}
}

func TestPgUUIDToString(t *testing.T) {
	uuidStr := "12345678-abcd-ef01-2345-6789abcdef01"
	pgUUID, err := StringToPgUUID(uuidStr)
	if err != nil {
		t.Fatalf("StringToPgUUID: %v", err)
	}
	got := PgUUIDToString(pgUUID)
	if got != uuidStr {
		t.Errorf("PgUUIDToString = %q, want %q", got, uuidStr)
	}
}

func TestPgUUIDToString_Invalid(t *testing.T) {
	var zero pgtype.UUID // Valid == false
	if got := PgUUIDToString(zero); got != "" {
		t.Errorf("PgUUIDToString(invalid) = %q, want empty", got)
	}
}

func TestStringToPgUUID_Invalid(t *testing.T) {
	_, err := StringToPgUUID("not-a-uuid")
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
}

func TestDBMemberToDomain(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	projectID, err := StringToPgUUID("00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("StringToPgUUID projectID: %v", err)
	}
	userID, err := StringToPgUUID("00000000-0000-0000-0000-000000000002")
	if err != nil {
		t.Fatalf("StringToPgUUID userID: %v", err)
	}
	memberID, err := StringToPgUUID("00000000-0000-0000-0000-000000000003")
	if err != nil {
		t.Fatalf("StringToPgUUID memberID: %v", err)
	}

	m := db.ProjectMember{
		ID:        memberID,
		ProjectID: projectID,
		UserID:    userID,
		Role:      domain.RoleAdmin,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}

	d := DBMemberToDomain(m)

	if d.Role != domain.RoleAdmin {
		t.Errorf("Role = %q, want %q", d.Role, domain.RoleAdmin)
	}
	if d.ProjectID != PgUUIDToString(projectID) {
		t.Errorf("ProjectID = %q, want %q", d.ProjectID, PgUUIDToString(projectID))
	}
	if d.UserID != PgUUIDToString(userID) {
		t.Errorf("UserID = %q, want %q", d.UserID, PgUUIDToString(userID))
	}
	if d.ID != PgUUIDToString(memberID) {
		t.Errorf("ID = %q, want %q", d.ID, PgUUIDToString(memberID))
	}
	if !d.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", d.CreatedAt, now)
	}
	if !d.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", d.UpdatedAt, now)
	}
}

func TestSessionRowToUser(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	userID, err := StringToPgUUID("00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("StringToPgUUID: %v", err)
	}

	row := db.GetSessionByTokenHashRow{
		UserID:        userID,
		Username:      "alice",
		Email:         "alice@example.com",
		DisplayName:   pgtype.Text{String: "Alice A", Valid: true},
		AvatarUrl:     pgtype.Text{String: "https://example.com/alice.png", Valid: true},
		IsActive:      true,
		UserCreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		UserUpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}

	d := SessionRowToUser(row)

	if d.ID != PgUUIDToString(userID) {
		t.Errorf("ID = %q, want %q", d.ID, PgUUIDToString(userID))
	}
	if d.Username != "alice" {
		t.Errorf("Username = %q, want %q", d.Username, "alice")
	}
	if d.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", d.Email, "alice@example.com")
	}
	if d.DisplayName != "Alice A" {
		t.Errorf("DisplayName = %q, want %q", d.DisplayName, "Alice A")
	}
	if d.AvatarURL != "https://example.com/alice.png" {
		t.Errorf("AvatarURL = %q, want %q", d.AvatarURL, "https://example.com/alice.png")
	}
	if !d.IsActive {
		t.Error("IsActive = false, want true")
	}
	if !d.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", d.CreatedAt, now)
	}
	if !d.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", d.UpdatedAt, now)
	}
}

func TestSessionRowToUser_NullOptionals(t *testing.T) {
	row := db.GetSessionByTokenHashRow{
		Username: "bob",
		IsActive: true,
	}

	d := SessionRowToUser(row)

	if d.ID != "" {
		t.Errorf("ID = %q, want empty", d.ID)
	}
	if d.DisplayName != "" {
		t.Errorf("DisplayName = %q, want empty", d.DisplayName)
	}
	if d.AvatarURL != "" {
		t.Errorf("AvatarURL = %q, want empty", d.AvatarURL)
	}
}

func TestDBSSHKeyToDomain(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	rotated := now.Add(time.Hour)

	id, err := StringToPgUUID("00000000-0000-0000-0000-000000000010")
	if err != nil {
		t.Fatalf("StringToPgUUID id: %v", err)
	}
	createdBy, err := StringToPgUUID("00000000-0000-0000-0000-000000000020")
	if err != nil {
		t.Fatalf("StringToPgUUID createdBy: %v", err)
	}

	dbKey := db.SshKey{
		ID:          id,
		Name:        "my-key",
		PublicKey:   "ssh-ed25519 AAAA...",
		Fingerprint: "SHA256:abc",
		KeyType:     "ed25519",
		IsActive:    true,
		CreatedBy:   createdBy,
		CreatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
		RotatedAt:   pgtype.Timestamptz{Time: rotated, Valid: true},
	}

	d := DBSSHKeyToDomain(dbKey)

	if d.ID != PgUUIDToString(id) {
		t.Errorf("ID = %q, want %q", d.ID, PgUUIDToString(id))
	}
	if d.Name != "my-key" {
		t.Errorf("Name = %q, want %q", d.Name, "my-key")
	}
	if d.PublicKey != "ssh-ed25519 AAAA..." {
		t.Errorf("PublicKey = %q, want %q", d.PublicKey, "ssh-ed25519 AAAA...")
	}
	if d.Fingerprint != "SHA256:abc" {
		t.Errorf("Fingerprint = %q, want %q", d.Fingerprint, "SHA256:abc")
	}
	if d.KeyType != "ed25519" {
		t.Errorf("KeyType = %q, want %q", d.KeyType, "ed25519")
	}
	if !d.IsActive {
		t.Error("IsActive = false, want true")
	}
	if d.CreatedBy != "" {
		t.Errorf("CreatedBy = %q, want empty (not exposed in API)", d.CreatedBy)
	}
	if !d.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", d.CreatedAt, now)
	}
	if d.RotatedAt == nil {
		t.Fatal("RotatedAt = nil, want non-nil")
	}
	if !d.RotatedAt.Equal(rotated) {
		t.Errorf("RotatedAt = %v, want %v", *d.RotatedAt, rotated)
	}
}

func TestDBSSHKeyToDomain_NullRotatedAt(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	id, err := StringToPgUUID("00000000-0000-0000-0000-000000000010")
	if err != nil {
		t.Fatalf("StringToPgUUID: %v", err)
	}

	dbKey := db.SshKey{
		ID:        id,
		Name:      "my-key",
		PublicKey: "ssh-ed25519 AAAA...",
		KeyType:   "ed25519",
		IsActive:  true,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		RotatedAt: pgtype.Timestamptz{}, // NULL
	}

	d := DBSSHKeyToDomain(dbKey)

	if d.RotatedAt != nil {
		t.Errorf("RotatedAt = %v, want nil", d.RotatedAt)
	}
}

// --- test helpers ---

func makeDBUser(t *testing.T, username, email, displayName, avatarURL string, ts time.Time) db.User {
	t.Helper()
	id, err := StringToPgUUID("00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("StringToPgUUID: %v", err)
	}
	u := db.User{
		ID:        id,
		Username:  username,
		Email:     email,
		IsActive:  true,
		CreatedAt: pgtype.Timestamptz{Time: ts, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: ts, Valid: true},
	}
	if displayName != "" {
		u.DisplayName = pgtype.Text{String: displayName, Valid: true}
	}
	if avatarURL != "" {
		u.AvatarUrl = pgtype.Text{String: avatarURL, Valid: true}
	}
	return u
}

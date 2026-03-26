package auth

import (
	"context"
	"testing"
	"time"

	"myjungle/backend-api/internal/domain"
)

func TestUserContext_RoundTrip(t *testing.T) {
	u := &domain.User{
		ID:       "user-1",
		Username: "alice",
		IsActive: true,
	}

	ctx := ContextWithUser(context.Background(), u)
	got, ok := UserFromContext(ctx)
	if !ok {
		t.Fatal("UserFromContext returned false")
	}
	if got.ID != u.ID {
		t.Errorf("ID = %q, want %q", got.ID, u.ID)
	}
	if got.Username != u.Username {
		t.Errorf("Username = %q, want %q", got.Username, u.Username)
	}
}

func TestUserContext_Missing(t *testing.T) {
	got, ok := UserFromContext(context.Background())
	if ok {
		t.Error("UserFromContext returned true for empty context")
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

func TestMembershipContext_RoundTrip(t *testing.T) {
	now := time.Now()
	m := &domain.ProjectMember{
		ID:        "pm-1",
		ProjectID: "proj-1",
		UserID:    "user-1",
		Role:      domain.RoleAdmin,
		CreatedAt: now,
		UpdatedAt: now,
	}

	ctx := ContextWithMembership(context.Background(), m)
	got, ok := MembershipFromContext(ctx)
	if !ok {
		t.Fatal("MembershipFromContext returned false")
	}
	if got.ID != m.ID {
		t.Errorf("ID = %q, want %q", got.ID, m.ID)
	}
	if got.Role != domain.RoleAdmin {
		t.Errorf("Role = %q, want %q", got.Role, domain.RoleAdmin)
	}
}

func TestMembershipContext_Missing(t *testing.T) {
	got, ok := MembershipFromContext(context.Background())
	if ok {
		t.Error("MembershipFromContext returned true for empty context")
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

func TestUserContext_TypedNil(t *testing.T) {
	ctx := ContextWithUser(context.Background(), nil)
	got, ok := UserFromContext(ctx)
	if ok {
		t.Error("UserFromContext returned true for typed-nil")
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

func TestMembershipContext_TypedNil(t *testing.T) {
	ctx := ContextWithMembership(context.Background(), nil)
	got, ok := MembershipFromContext(ctx)
	if ok {
		t.Error("MembershipFromContext returned true for typed-nil")
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

func TestBothContextValues_Independent(t *testing.T) {
	u := &domain.User{ID: "user-1", Username: "alice"}
	m := &domain.ProjectMember{ID: "pm-1", Role: domain.RoleMember}

	ctx := ContextWithUser(context.Background(), u)
	ctx = ContextWithMembership(ctx, m)

	gotU, okU := UserFromContext(ctx)
	gotM, okM := MembershipFromContext(ctx)

	if !okU {
		t.Fatal("UserFromContext returned false")
	}
	if gotU.ID != "user-1" {
		t.Errorf("user ID = %q, want %q", gotU.ID, "user-1")
	}
	if !okM {
		t.Fatal("MembershipFromContext returned false")
	}
	if gotM.ID != "pm-1" {
		t.Errorf("membership ID = %q, want %q", gotM.ID, "pm-1")
	}
}

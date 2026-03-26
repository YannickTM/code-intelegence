package middleware

import (
	"testing"

	"myjungle/backend-api/internal/domain"
)

func TestRoleSufficient(t *testing.T) {
	tests := []struct {
		userRole string
		minRole  string
		want     bool
	}{
		{domain.RoleOwner, domain.RoleOwner, true},
		{domain.RoleOwner, domain.RoleAdmin, true},
		{domain.RoleOwner, domain.RoleMember, true},
		{domain.RoleAdmin, domain.RoleOwner, false},
		{domain.RoleAdmin, domain.RoleAdmin, true},
		{domain.RoleAdmin, domain.RoleMember, true},
		{domain.RoleMember, domain.RoleOwner, false},
		{domain.RoleMember, domain.RoleAdmin, false},
		{domain.RoleMember, domain.RoleMember, true},
		{"unknown", domain.RoleMember, false},
		{domain.RoleMember, "unknown", false}, // fail closed: unknown minRole always denies
		{"unknown", "unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.userRole+"_vs_"+tt.minRole, func(t *testing.T) {
			if got := domain.RoleSufficient(tt.userRole, tt.minRole); got != tt.want {
				t.Errorf("RoleSufficient(%q, %q) = %v, want %v", tt.userRole, tt.minRole, got, tt.want)
			}
		})
	}
}

func TestRequireProjectRole_InvalidMinRole(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for invalid minRole")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want string", r)
		}
		want := "RequireProjectRole: invalid minRole bogus"
		if msg != want {
			t.Errorf("panic message = %q, want %q", msg, want)
		}
	}()
	RequireProjectRole(nil, "bogus")
}

func TestRoleRank(t *testing.T) {
	tests := []struct {
		role string
		want int
	}{
		{domain.RoleOwner, 3},
		{domain.RoleAdmin, 2},
		{domain.RoleMember, 1},
		{"unknown", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := domain.RoleRank(tt.role); got != tt.want {
				t.Errorf("RoleRank(%q) = %d, want %d", tt.role, got, tt.want)
			}
		})
	}
}

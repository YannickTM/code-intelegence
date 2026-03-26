// Package auth provides session context helpers for user identity and project membership.
package auth

import (
	"context"

	"myjungle/backend-api/internal/domain"
)

type contextKey string

const (
	userKey       contextKey = "auth.user"
	membershipKey contextKey = "auth.membership"
	apiKeyKey     contextKey = "auth.apikey"
)

// ContextWithUser stores a User in the request context.
func ContextWithUser(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFromContext retrieves the User from the request context.
// Returns nil and false if no user is present (including typed-nil).
func UserFromContext(ctx context.Context) (*domain.User, bool) {
	u, ok := ctx.Value(userKey).(*domain.User)
	if !ok || u == nil {
		return nil, false
	}
	return u, true
}

// ContextWithMembership stores a ProjectMember in the request context.
func ContextWithMembership(ctx context.Context, m *domain.ProjectMember) context.Context {
	return context.WithValue(ctx, membershipKey, m)
}

// MembershipFromContext retrieves the ProjectMember from the request context.
// Returns nil and false if no membership is present (including typed-nil).
func MembershipFromContext(ctx context.Context) (*domain.ProjectMember, bool) {
	m, ok := ctx.Value(membershipKey).(*domain.ProjectMember)
	if !ok || m == nil {
		return nil, false
	}
	return m, true
}

// ContextWithAPIKey stores an APIKeyIdentity in the request context.
func ContextWithAPIKey(ctx context.Context, k *domain.APIKeyIdentity) context.Context {
	return context.WithValue(ctx, apiKeyKey, k)
}

// APIKeyFromContext retrieves the APIKeyIdentity from the request context.
// Returns nil and false if no API key identity is present (including typed-nil).
func APIKeyFromContext(ctx context.Context) (*domain.APIKeyIdentity, bool) {
	k, ok := ctx.Value(apiKeyKey).(*domain.APIKeyIdentity)
	if !ok || k == nil {
		return nil, false
	}
	return k, true
}

const platformRolesKey contextKey = "auth.platform_roles"

// ContextWithPlatformRoles stores the user's platform roles in the context.
func ContextWithPlatformRoles(ctx context.Context, roles []string) context.Context {
	return context.WithValue(ctx, platformRolesKey, roles)
}

// PlatformRolesFromContext retrieves the user's platform roles from the context.
func PlatformRolesFromContext(ctx context.Context) []string {
	roles, _ := ctx.Value(platformRolesKey).([]string)
	return roles
}

// IsPlatformAdmin checks if the context user has the platform_admin role.
func IsPlatformAdmin(ctx context.Context) bool {
	for _, r := range PlatformRolesFromContext(ctx) {
		if r == domain.PlatformRoleAdmin {
			return true
		}
	}
	return false
}

package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// RequireProjectRole enforces project-scoped RBAC. It must run after
// IdentityResolver (which resolves the caller as a user or API key).
//
// It extracts {projectID} from the chi URL params and checks the caller's
// access to that project:
//
//   - User callers: looked up via project_members table
//   - API key callers: looked up via api_key_project_access view
//
// Responses:
//   - 401 if no identity at all (neither user nor API key in context)
//   - 404 if the caller has no access (hides project existence per ADR-008)
//   - 403 if the caller's role is insufficient
//   - On success, a ProjectMember is stored in context for handler use
func RequireProjectRole(pdb *postgres.DB, minRole string) func(http.Handler) http.Handler {
	if !domain.RoleKnown(minRole) {
		panic("RequireProjectRole: invalid minRole " + minRole)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, userOK := auth.UserFromContext(r.Context())
			apiKey, keyOK := auth.APIKeyFromContext(r.Context())

			if !userOK && !keyOK {
				writeMiddlewareError(r.Context(), w, domain.ErrUnauthorized)
				return
			}

			if pdb == nil || pdb.Queries == nil {
				writeMiddlewareError(r.Context(), w, domain.ErrInternal)
				return
			}

			projectID := chi.URLParam(r, "projectID")
			pgProjectID, err := dbconv.StringToPgUUID(projectID)
			if err != nil {
				writeMiddlewareError(r.Context(), w, domain.NotFound("project not found"))
				return
			}

			// User-based access: look up membership in project_members.
			if userOK {
				pgUserID, err := dbconv.StringToPgUUID(user.ID)
				if err != nil {
					slog.ErrorContext(r.Context(), "projectrole: invalid user UUID", slog.String("user_id", user.ID), slog.Any("error", err))
					writeMiddlewareError(r.Context(), w, domain.ErrInternal)
					return
				}

				// Platform admin short-circuit: treat as owner-level for any project.
				// Verify project exists to avoid passing phantom IDs to handlers.
				if auth.IsPlatformAdmin(r.Context()) {
					if _, err := pdb.Queries.GetProject(r.Context(), pgProjectID); err != nil {
						if errors.Is(err, pgx.ErrNoRows) {
							writeMiddlewareError(r.Context(), w, domain.NotFound("project not found"))
							return
						}
						slog.ErrorContext(r.Context(), "projectrole: GetProject failed", slog.Any("error", err))
						writeMiddlewareError(r.Context(), w, domain.ErrInternal)
						return
					}
					synthetic := &domain.ProjectMember{
						ProjectID: projectID,
						UserID:    user.ID,
						Role:      domain.RoleOwner,
					}
					ctx := auth.ContextWithMembership(r.Context(), synthetic)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				// If platform roles haven't been loaded yet (request didn't go through
				// RequirePlatformAdmin first), do a DB check and cache in context.
				if roles := auth.PlatformRolesFromContext(r.Context()); roles == nil {
					hasRole, err := pdb.Queries.HasPlatformRole(r.Context(), db.HasPlatformRoleParams{
						UserID: pgUserID,
						Role:   domain.PlatformRoleAdmin,
					})
					if err != nil {
						slog.WarnContext(r.Context(), "projectrole: HasPlatformRole failed, falling through", slog.Any("error", err))
					} else if hasRole {
						if _, err := pdb.Queries.GetProject(r.Context(), pgProjectID); err != nil {
							if errors.Is(err, pgx.ErrNoRows) {
								writeMiddlewareError(r.Context(), w, domain.NotFound("project not found"))
								return
							}
							slog.ErrorContext(r.Context(), "projectrole: GetProject failed", slog.Any("error", err))
							writeMiddlewareError(r.Context(), w, domain.ErrInternal)
							return
						}
						ctx := auth.ContextWithPlatformRoles(r.Context(), []string{domain.PlatformRoleAdmin})
						synthetic := &domain.ProjectMember{
							ProjectID: projectID,
							UserID:    user.ID,
							Role:      domain.RoleOwner,
						}
						ctx = auth.ContextWithMembership(ctx, synthetic)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}

				member, err := pdb.Queries.GetProjectMember(r.Context(), db.GetProjectMemberParams{
					ProjectID: pgProjectID,
					UserID:    pgUserID,
				})
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						writeMiddlewareError(r.Context(), w, domain.NotFound("project not found"))
						return
					}
					slog.ErrorContext(r.Context(), "projectrole: GetProjectMember failed", slog.Any("error", err))
					writeMiddlewareError(r.Context(), w, domain.ErrInternal)
					return
				}

				if !domain.RoleSufficient(member.Role, minRole) {
					writeMiddlewareError(r.Context(), w, domain.Forbidden("insufficient project role"))
					return
				}

				dm := dbconv.DBMemberToDomain(member)
				ctx := auth.ContextWithMembership(r.Context(), &dm)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// API key–based access: look up via api_key_project_access view.

			// Map the membership-based minRole to the equivalent key role.
			// Owner-level routes are never accessible via API keys.
			minKeyRole, allowed := domain.MembershipRoleToKeyRole(minRole)
			if !allowed {
				writeMiddlewareError(r.Context(), w, domain.Forbidden("api keys cannot access owner-level routes"))
				return
			}

			effectiveRole, err := pdb.Queries.CheckAPIKeyProjectAccess(r.Context(), db.CheckAPIKeyProjectAccessParams{
				KeyHash:   apiKey.KeyHash,
				ProjectID: pgProjectID,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeMiddlewareError(r.Context(), w, domain.NotFound("project not found"))
					return
				}
				slog.ErrorContext(r.Context(), "projectrole: CheckAPIKeyProjectAccess failed", slog.Any("error", err))
				writeMiddlewareError(r.Context(), w, domain.ErrInternal)
				return
			}

			if !domain.KeyRoleSufficient(effectiveRole, minKeyRole) {
				writeMiddlewareError(r.Context(), w, domain.Forbidden("insufficient api key role"))
				return
			}

			// Store a synthetic ProjectMember for handler compatibility.
			synthetic := &domain.ProjectMember{
				ProjectID: projectID,
				Role:      effectiveRole,
			}
			ctx := auth.ContextWithMembership(r.Context(), synthetic)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Package membership implements project membership management with role
// hierarchy enforcement and ownership invariants.
package membership

import (
	"context"
	"errors"
	"fmt"

	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/storage/postgres"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Service provides project membership operations with RBAC enforcement.
type Service struct {
	db *postgres.DB
}

// NewService creates a new membership Service.
func NewService(db *postgres.DB) *Service {
	return &Service{db: db}
}

// List returns all members of a project with joined user info.
func (s *Service) List(ctx context.Context, projectID pgtype.UUID) ([]domain.ProjectMemberWithUser, error) {
	rows, err := s.db.Queries.ListProjectMembers(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project members: %w", err)
	}
	out := make([]domain.ProjectMemberWithUser, len(rows))
	for i, row := range rows {
		out[i] = dbconv.DBMemberWithUserToDomain(row)
	}
	return out, nil
}

// validateActor performs defense-in-depth checks on the actor: ensures it is
// non-nil and belongs to the target project. These conditions are normally
// guaranteed by the middleware, but we verify here so the service is safe
// to call from other contexts.
func validateActor(actor *domain.ProjectMember, projectID pgtype.UUID) *domain.AppError {
	if actor == nil {
		return domain.BadRequest("missing actor context")
	}
	if actor.ProjectID != dbconv.PgUUIDToString(projectID) {
		return domain.Forbidden("actor not authorized for this project")
	}
	return nil
}

// UpdateRole changes a member's role, enforcing role hierarchy and ownership
// invariants. The actor is the authenticated user performing the change.
//
// Returns the updated membership row on success.
//
// The entire read-check-update sequence runs inside a single transaction with
// a FOR UPDATE lock on the target membership row to prevent TOCTOU races
// (e.g. a concurrent promotion could change the target's role between the
// permission check and the update).
func (s *Service) UpdateRole(ctx context.Context, projectID, targetUserID pgtype.UUID, newRole string, actor *domain.ProjectMember) (domain.ProjectMember, *domain.AppError) {
	// 0. Actor validation (defense-in-depth).
	if appErr := validateActor(actor, projectID); appErr != nil {
		return domain.ProjectMember{}, appErr
	}

	// 1. Validate new role (pure logic — no DB needed).
	if !domain.RoleKnown(newRole) {
		return domain.ProjectMember{}, domain.BadRequest("role must be one of: owner, admin, member")
	}

	// 2. Self-role-change prevention (pure logic).
	if actor.UserID == dbconv.PgUUIDToString(targetUserID) {
		return domain.ProjectMember{}, domain.BadRequest("cannot change your own role")
	}

	// 3. Defense-in-depth: verify actor is at least admin.
	// The route middleware already enforces this, but we check here too
	// so the service is safe to call from other contexts.
	if !domain.RoleSufficient(actor.Role, domain.RoleAdmin) {
		return domain.ProjectMember{}, domain.Forbidden("requires admin role to update member roles")
	}

	// 4. Admin cannot promote to owner (pure logic on actor + newRole).
	if actor.Role == domain.RoleAdmin && newRole == domain.RoleOwner {
		return domain.ProjectMember{}, domain.Forbidden("requires owner role for this action")
	}

	// 5. Transactional read-check-update with row lock.
	var result domain.ProjectMember
	txErr := s.db.WithTx(ctx, func(q *db.Queries) error {
		// Lock the target membership row to prevent concurrent mutations.
		target, err := q.GetProjectMemberForUpdate(ctx, db.GetProjectMemberForUpdateParams{
			ProjectID: projectID,
			UserID:    targetUserID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("user is not a member of this project")
			}
			return fmt.Errorf("get member for update: %w", err)
		}

		// Admin cannot modify an owner.
		if actor.Role == domain.RoleAdmin && target.Role == domain.RoleOwner {
			return domain.Forbidden("requires owner role for this action")
		}

		// No-op: role unchanged — return current state.
		if target.Role == newRole {
			result = dbconv.DBMemberToDomain(target)
			return nil
		}

		// Demoting an owner: check last-owner invariant.
		if target.Role == domain.RoleOwner && newRole != domain.RoleOwner {
			count, cErr := q.CountProjectOwnersForUpdate(ctx, projectID)
			if cErr != nil {
				return fmt.Errorf("count owners: %w", cErr)
			}
			if count <= 1 {
				return domain.Conflict("cannot demote the last project owner")
			}
		}

		updated, uErr := q.UpdateProjectMemberRole(ctx, db.UpdateProjectMemberRoleParams{
			ProjectID: projectID,
			UserID:    targetUserID,
			Role:      newRole,
		})
		if uErr != nil {
			return fmt.Errorf("update member role: %w", uErr)
		}
		result = dbconv.DBMemberToDomain(updated)
		return nil
	})
	if txErr != nil {
		if appErr, ok := txErr.(*domain.AppError); ok {
			return domain.ProjectMember{}, appErr
		}
		return domain.ProjectMember{}, domain.ErrInternal
	}
	return result, nil
}

// Add adds a user to a project with the given role. The actor must be admin+.
// Admins cannot assign the owner role — only owners can.
func (s *Service) Add(ctx context.Context, projectID, targetUserID pgtype.UUID, role string, actor *domain.ProjectMember) (*domain.ProjectMember, *domain.AppError) {
	// 0. Actor validation (defense-in-depth).
	if appErr := validateActor(actor, projectID); appErr != nil {
		return nil, appErr
	}

	// 1. Validate role.
	if !domain.RoleKnown(role) {
		return nil, domain.BadRequest("role must be one of: owner, admin, member")
	}

	// 2. Defense-in-depth: verify actor is at least admin.
	if !domain.RoleSufficient(actor.Role, domain.RoleAdmin) {
		return nil, domain.Forbidden("requires admin role to add members")
	}

	// 3. Admin cannot assign owner role.
	if actor.Role == domain.RoleAdmin && role == domain.RoleOwner {
		return nil, domain.Forbidden("requires owner role for this action")
	}

	// 4. Verify target user exists and is active.
	user, err := s.db.Queries.GetUserByID(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound("user not found")
		}
		return nil, domain.ErrInternal
	}
	if !user.IsActive {
		return nil, domain.NotFound("user not found")
	}

	// 5. Create membership — unique constraint prevents duplicates.
	actorID, err := dbconv.StringToPgUUID(actor.UserID)
	if err != nil {
		return nil, domain.ErrInternal
	}
	row, err := s.db.Queries.CreateProjectMember(ctx, db.CreateProjectMemberParams{
		ProjectID: projectID,
		UserID:    targetUserID,
		Role:      role,
		InvitedBy: actorID,
	})
	if err != nil {
		if postgres.IsUniqueViolation(err) {
			return nil, domain.Conflict("user is already a member of this project")
		}
		return nil, domain.ErrInternal
	}

	member := dbconv.DBMemberToDomain(row)
	return &member, nil
}

// Remove removes a member from a project, enforcing role hierarchy and
// ownership invariants. Self-removal (leaving) is allowed for any member
// unless they are the last owner.
//
// The entire read-check-delete sequence runs inside a single transaction with
// a FOR UPDATE lock on the target membership row to prevent TOCTOU races
// (e.g. a concurrent promotion to owner between the role check and the delete).
func (s *Service) Remove(ctx context.Context, projectID, targetUserID pgtype.UUID, actor *domain.ProjectMember) *domain.AppError {
	// 0. Actor validation (defense-in-depth).
	if appErr := validateActor(actor, projectID); appErr != nil {
		return appErr
	}

	isSelfRemoval := actor.UserID == dbconv.PgUUIDToString(targetUserID)

	// Non-self removal requires admin+.
	if !isSelfRemoval && !domain.RoleSufficient(actor.Role, domain.RoleAdmin) {
		return domain.Forbidden("requires admin role to remove other members")
	}

	txErr := s.db.WithTx(ctx, func(q *db.Queries) error {
		// Lock the target membership row.
		target, err := q.GetProjectMemberForUpdate(ctx, db.GetProjectMemberForUpdateParams{
			ProjectID: projectID,
			UserID:    targetUserID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.NotFound("user is not a member of this project")
			}
			return fmt.Errorf("get member for update: %w", err)
		}

		// Admin cannot remove an owner.
		if !isSelfRemoval && actor.Role == domain.RoleAdmin && target.Role == domain.RoleOwner {
			return domain.Forbidden("requires owner role for this action")
		}

		// Owner removal: check last-owner invariant.
		if target.Role == domain.RoleOwner {
			count, cErr := q.CountProjectOwnersForUpdate(ctx, projectID)
			if cErr != nil {
				return fmt.Errorf("count owners: %w", cErr)
			}
			if count <= 1 {
				return domain.Conflict("cannot remove the last project owner")
			}
		}

		if err := q.DeleteProjectMember(ctx, db.DeleteProjectMemberParams{
			ProjectID: projectID,
			UserID:    targetUserID,
		}); err != nil {
			return fmt.Errorf("delete member: %w", err)
		}
		return nil
	})
	if txErr != nil {
		if appErr, ok := txErr.(*domain.AppError); ok {
			return appErr
		}
		return domain.ErrInternal
	}
	return nil
}

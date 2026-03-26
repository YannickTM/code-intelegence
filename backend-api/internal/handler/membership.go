package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/membership"
	"myjungle/backend-api/internal/sse"
	"myjungle/backend-api/internal/storage/postgres"
	"myjungle/backend-api/internal/validate"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// redisPublishTimeout is the maximum time allowed for publishing an event to
// Redis. A short-lived background context is used instead of the request
// context so that the publish succeeds even if the client disconnects after
// the HTTP response has been written.
const redisPublishTimeout = 3 * time.Second

// MembershipHandler serves project membership endpoints.
type MembershipHandler struct {
	db         *postgres.DB
	membership *membership.Service
	hub        *sse.Hub            // nil-safe; no-op if nil
	publisher  *sse.EventPublisher // nil-safe; publishes to Redis for remote instances
	instanceID string              // identifies this API instance for dedup
}

// NewMembershipHandler creates a new MembershipHandler.
// instanceID is stamped onto outgoing events so that this instance's
// Subscriber can skip re-broadcasting events it already delivered locally.
func NewMembershipHandler(pdb *postgres.DB, hub *sse.Hub, pub *sse.EventPublisher, instanceID string) *MembershipHandler {
	return &MembershipHandler{
		db:         pdb,
		membership: membership.NewService(pdb),
		hub:        hub,
		publisher:  pub,
		instanceID: instanceID,
	}
}

func (h *MembershipHandler) ensureDB(w http.ResponseWriter) bool {
	if h.db == nil || h.db.Queries == nil || h.membership == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

// HandleList lists project members (GET /v1/projects/{projectID}/members).
func (h *MembershipHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	projectID, ok := h.parseUUIDParam(w, r, "projectID", "project_id")
	if !ok {
		return
	}

	members, err := h.membership.List(r.Context(), projectID)
	if err != nil {
		if appErr, ok := err.(*domain.AppError); ok {
			WriteAppError(w, appErr)
		} else {
			WriteAppError(w, domain.ErrInternal)
		}
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"items": members,
	})
}

// HandleUpdate changes a member's role (PATCH /v1/projects/{projectID}/members/{userID}).
func (h *MembershipHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	projectID, ok := h.parseUUIDParam(w, r, "projectID", "project_id")
	if !ok {
		return
	}
	targetUserID, ok := h.parseUUIDParam(w, r, "userID", "user_id")
	if !ok {
		return
	}

	actor, ok := auth.MembershipFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	role := strings.TrimSpace(body.Role)
	if role == "" {
		WriteAppError(w, domain.BadRequest("role must be one of: owner, admin, member"))
		return
	}

	updated, appErr := h.membership.UpdateRole(r.Context(), projectID, targetUserID, role, actor)
	if appErr != nil {
		WriteAppError(w, appErr)
		return
	}

	WriteJSON(w, http.StatusOK, updated)

	canonicalUserID := dbconv.PgUUIDToString(targetUserID)
	evt := sse.SSEEvent{
		Event:     "member:role_updated",
		ProjectID: dbconv.PgUUIDToString(projectID),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data: map[string]any{
			"user_id":       canonicalUserID,
			"role":          role,
			"actor_user_id": actor.UserID,
		},
		Origin: h.instanceID,
	}
	h.hub.PublishToUser(canonicalUserID, evt)
	if h.publisher != nil {
		pubCtx, pubCancel := context.WithTimeout(context.Background(), redisPublishTimeout)
		defer pubCancel()
		h.publisher.Publish(pubCtx, evt)
	}
}

// HandleAdd adds a member to a project (POST /v1/projects/{projectID}/members).
func (h *MembershipHandler) HandleAdd(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	projectID, ok := h.parseUUIDParam(w, r, "projectID", "project_id")
	if !ok {
		return
	}

	actor, ok := auth.MembershipFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	var body struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	// Validate user_id.
	userIDStr := strings.TrimSpace(body.UserID)
	errs := make(validate.Errors)
	validate.UUID(userIDStr, "user_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return
	}
	targetUserID, err := dbconv.StringToPgUUID(userIDStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Default role to member.
	role := strings.TrimSpace(body.Role)
	if role == "" {
		role = domain.RoleMember
	}

	member, appErr := h.membership.Add(r.Context(), projectID, targetUserID, role, actor)
	if appErr != nil {
		WriteAppError(w, appErr)
		return
	}

	WriteJSON(w, http.StatusCreated, member)

	projIDStr := dbconv.PgUUIDToString(projectID)
	canonicalUserID := dbconv.PgUUIDToString(targetUserID)

	// Update Hub's live membership so already-connected SSE clients for the
	// newly added user immediately start receiving events for this project.
	h.hub.AddProjectForUser(canonicalUserID, projIDStr)

	evt := sse.SSEEvent{
		Event:     "member:added",
		ProjectID: projIDStr,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data: map[string]any{
			"user_id":       canonicalUserID,
			"role":          role,
			"actor_user_id": actor.UserID,
		},
		Origin: h.instanceID,
	}
	h.hub.PublishToUser(canonicalUserID, evt)
	if h.publisher != nil {
		pubCtx, pubCancel := context.WithTimeout(context.Background(), redisPublishTimeout)
		defer pubCancel()
		h.publisher.Publish(pubCtx, evt)
	}
}

// HandleRemove removes a member from a project (DELETE /v1/projects/{projectID}/members/{userID}).
func (h *MembershipHandler) HandleRemove(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}

	projectID, ok := h.parseUUIDParam(w, r, "projectID", "project_id")
	if !ok {
		return
	}
	targetUserID, ok := h.parseUUIDParam(w, r, "userID", "user_id")
	if !ok {
		return
	}

	actor, ok := auth.MembershipFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	if appErr := h.membership.Remove(r.Context(), projectID, targetUserID, actor); appErr != nil {
		WriteAppError(w, appErr)
		return
	}

	w.WriteHeader(http.StatusNoContent)

	projIDStr := dbconv.PgUUIDToString(projectID)
	canonicalUserID := dbconv.PgUUIDToString(targetUserID)

	// Remove project from Hub's live membership so the removed user's
	// already-connected SSE clients stop receiving events for this project.
	h.hub.RemoveProjectForUser(canonicalUserID, projIDStr)

	evt := sse.SSEEvent{
		Event:     "member:removed",
		ProjectID: projIDStr,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data: map[string]any{
			"user_id":       canonicalUserID,
			"actor_user_id": actor.UserID,
		},
		Origin: h.instanceID,
	}
	h.hub.PublishToUser(canonicalUserID, evt)
	if h.publisher != nil {
		pubCtx, pubCancel := context.WithTimeout(context.Background(), redisPublishTimeout)
		defer pubCancel()
		h.publisher.Publish(pubCtx, evt)
	}
}

// parseUUIDParam extracts and validates a UUID URL parameter by chi param name
// (e.g. "projectID") and validation field name (e.g. "project_id").
func (h *MembershipHandler) parseUUIDParam(w http.ResponseWriter, r *http.Request, paramName, fieldName string) (pgtype.UUID, bool) {
	idStr := chi.URLParam(r, paramName)
	errs := make(validate.Errors)
	validate.UUID(idStr, fieldName, errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return pgtype.UUID{}, false
	}
	id, err := dbconv.StringToPgUUID(idStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return pgtype.UUID{}, false
	}
	return id, true
}

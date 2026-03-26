//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"myjungle/backend-api/internal/app"
	"myjungle/backend-api/internal/dbconv"

	db "myjungle/datastore/postgres/sqlc"
)

// addMember inserts a project member directly via the database.
// Returns the membership ID.
func addMember(t *testing.T, a *app.App, projectID, userID, role, invitedByUserID string) string {
	t.Helper()
	pid, err := dbconv.StringToPgUUID(projectID)
	if err != nil {
		t.Fatalf("addMember: parse projectID: %v", err)
	}
	uid, err := dbconv.StringToPgUUID(userID)
	if err != nil {
		t.Fatalf("addMember: parse userID: %v", err)
	}
	inv, err := dbconv.StringToPgUUID(invitedByUserID)
	if err != nil {
		t.Fatalf("addMember: parse invitedByUserID: %v", err)
	}
	m, err := a.DB.Queries.CreateProjectMember(context.Background(), db.CreateProjectMemberParams{
		ProjectID: pid,
		UserID:    uid,
		Role:      role,
		InvitedBy: inv,
	})
	if err != nil {
		t.Fatalf("addMember: %v", err)
	}
	return dbconv.PgUUIDToString(m.ID)
}

// getUserID extracts the user ID from a registerUser response.
// The response shape is {"user": {"id": "...", ...}}.
func getUserID(t *testing.T, m map[string]any) string {
	t.Helper()
	user := mustMap(t, m, "user")
	return mustString(t, user, "id")
}

func TestMembership_ListMembers(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	// Setup: alice creates a project.
	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Add bob as a member.
	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// List members as alice.
	w := doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s/members", projID), nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", body["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	// Verify user info is present with all expected fields.
	found := false
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("item is not map[string]any: %T", item)
		}
		if m["username"] == "alice" {
			found = true
			if m["role"] != "owner" {
				t.Errorf("alice role = %q, want owner", m["role"])
			}
			if _, ok := m["id"]; !ok {
				t.Error("alice missing id field")
			}
			if _, ok := m["project_id"]; !ok {
				t.Error("alice missing project_id field")
			}
			if _, ok := m["user_id"]; !ok {
				t.Error("alice missing user_id field")
			}
			if _, ok := m["created_at"]; !ok {
				t.Error("alice missing created_at field")
			}
			if _, ok := m["display_name"]; !ok {
				t.Error("alice missing display_name field")
			}
		}
	}
	if !found {
		t.Error("alice not found in member list")
	}
}

func TestMembership_OwnerPromotesMemberToAdmin(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// Owner promotes member → admin.
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		map[string]any{"role": "admin"},
		authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["role"] != "admin" {
		t.Errorf("role = %q, want admin", resp["role"])
	}
	if resp["user_id"] != bobID {
		t.Errorf("user_id = %v, want %s", resp["user_id"], bobID)
	}
	if resp["project_id"] != projID {
		t.Errorf("project_id = %v, want %s", resp["project_id"], projID)
	}
}

func TestMembership_OwnerPromotesAdminToOwner(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	// Owner promotes admin → owner.
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		map[string]any{"role": "owner"},
		authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["role"] != "owner" {
		t.Errorf("role = %q, want owner", resp["role"])
	}
	if resp["user_id"] != bobID {
		t.Errorf("user_id = %v, want %s", resp["user_id"], bobID)
	}
	if resp["project_id"] != projID {
		t.Errorf("project_id = %v, want %s", resp["project_id"], projID)
	}
}

func TestMembership_AdminCannotPromoteToOwner(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	// Register charlie as member.
	charlieResp := registerUser(t, a, "charlie")
	charlieID := getUserID(t, charlieResp)
	addMember(t, a, projID, charlieID, "member", aliceID)

	// Admin (bob) tries to promote charlie → owner — should fail.
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, charlieID),
		map[string]any{"role": "owner"},
		authHeader(bobToken))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestMembership_CannotChangeOwnRole(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Owner tries to change own role.
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, aliceID),
		map[string]any{"role": "admin"},
		authHeader(aliceToken))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestMembership_AdminCannotDemoteOwner(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	// Admin (bob) tries to demote owner (alice) → 403: admin can't modify owner.
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, aliceID),
		map[string]any{"role": "admin"},
		authHeader(bobToken))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestMembership_OwnerDemotesOwnerSucceedsWhenMultiple(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "owner", aliceID)

	// Alice (owner) demotes bob (owner) → admin. Two owners, so this succeeds.
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		map[string]any{"role": "admin"},
		authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["role"] != "admin" {
		t.Errorf("role = %q, want admin", resp["role"])
	}
	if resp["user_id"] != bobID {
		t.Errorf("user_id = %v, want %s", resp["user_id"], bobID)
	}
	if resp["project_id"] != projID {
		t.Errorf("project_id = %v, want %s", resp["project_id"], projID)
	}
}

func TestMembership_MemberPatchBlocked(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Add bob as second owner.
	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "owner", aliceID)

	// Bob demotes alice (2 owners → 1, ok).
	bobToken := loginUser(t, a, "bob")
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, aliceID),
		map[string]any{"role": "member"},
		authHeader(bobToken))
	if w.Code != http.StatusOK {
		t.Fatalf("demote alice: status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	// Alice (now member) tries to PATCH bob → 403: members cannot use PATCH at all (middleware blocks).
	aliceToken2 := loginUser(t, a, "alice")
	w = doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		map[string]any{"role": "admin"},
		authHeader(aliceToken2))
	if w.Code != http.StatusForbidden {
		t.Fatalf("member patch: status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestMembership_AdminDemotesAdminToMember(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	charlieResp := registerUser(t, a, "charlie")
	charlieID := getUserID(t, charlieResp)
	addMember(t, a, projID, charlieID, "admin", aliceID)

	// Admin bob demotes admin charlie → member.
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, charlieID),
		map[string]any{"role": "member"},
		authHeader(bobToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["role"] != "member" {
		t.Errorf("role = %q, want member", resp["role"])
	}
	if resp["user_id"] != charlieID {
		t.Errorf("user_id = %v, want %s", resp["user_id"], charlieID)
	}
	if resp["project_id"] != projID {
		t.Errorf("project_id = %v, want %s", resp["project_id"], projID)
	}
}

func TestMembership_OwnerRemovesAdmin(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	// Owner removes admin.
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestMembership_AdminCannotRemoveOwner(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	// Admin tries to remove owner.
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, aliceID),
		nil, authHeader(bobToken))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestMembership_CannotRemoveLastOwner(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Owner tries self-remove — should fail as last owner.
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, aliceID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestMembership_MemberSelfRemoval(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// Member bob removes self (leaves).
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		nil, authHeader(bobToken))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Verify bob is no longer in member list.
	w = doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s/members", projID), nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("list: status = %d (body=%s)", w.Code, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", body["items"])
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
}

func TestMembership_MemberCannotRemoveOthers(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	charlieResp := registerUser(t, a, "charlie")
	charlieID := getUserID(t, charlieResp)
	addMember(t, a, projID, charlieID, "member", aliceID)

	// Member bob tries to remove charlie — should fail.
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, charlieID),
		nil, authHeader(bobToken))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestMembership_UpdateTargetNotMember(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Use a random valid UUID as non-member userID.
	fakeUserID := "00000000-0000-0000-0000-000000000099"

	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, fakeUserID),
		map[string]any{"role": "admin"},
		authHeader(aliceToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestMembership_InvalidRole(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// Try to set an invalid role.
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		map[string]any{"role": "superadmin"},
		authHeader(aliceToken))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}

	// Try to set an empty role.
	w = doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		map[string]any{"role": ""},
		authHeader(aliceToken))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty role: status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestMembership_RemoveTargetNotMember(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	fakeUserID := "00000000-0000-0000-0000-000000000099"

	// Owner tries to remove non-member → 404.
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, fakeUserID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// TestMembership_OwnerDemotionRace verifies that concurrent owner demotions
// never leave a project with zero owners. Two owners each try to demote the
// other simultaneously, and the system must serialize those requests so that
// exactly one demotion succeeds and at least one owner remains.
//
// The loser can fail in one of two places:
//   - 409 if it reaches the transactional last-owner check after the other
//     request has already demoted its target.
//   - 403 if the other request demotes the caller first and the request then
//     fails RBAC in RequireProjectRole before entering the handler.
func TestMembership_OwnerDemotionRace(t *testing.T) {
	a := setupTestApp(t)

	const iterations = 10
	for i := 0; i < iterations; i++ {
		truncateAll(t, a)

		aliceResp := registerUser(t, a, "alice")
		aliceToken := loginUser(t, a, "alice")
		aliceID := getUserID(t, aliceResp)
		keyID := createSSHKey(t, a, "key1", aliceToken)
		proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
		projID := mustString(t, proj, "id")

		bobResp := registerUser(t, a, "bob")
		bobToken := loginUser(t, a, "bob")
		bobID := getUserID(t, bobResp)
		addMember(t, a, projID, bobID, "owner", aliceID)

		// Both owners try to demote the other concurrently.
		var wg sync.WaitGroup
		codes := make([]int, 2)
		start := make(chan struct{})

		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			w := doRequest(t, a, http.MethodPatch,
				fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
				map[string]any{"role": "member"},
				authHeader(aliceToken))
			codes[0] = w.Code
		}()
		go func() {
			defer wg.Done()
			<-start
			w := doRequest(t, a, http.MethodPatch,
				fmt.Sprintf("/v1/projects/%s/members/%s", projID, aliceID),
				map[string]any{"role": "member"},
				authHeader(bobToken))
			codes[1] = w.Code
		}()
		close(start)
		wg.Wait()

		successes := 0
		for _, code := range codes {
			if code == http.StatusOK {
				successes++
			} else if code != http.StatusConflict && code != http.StatusForbidden {
				t.Errorf("iteration %d: unexpected status %d (want 200, 403, or 409)", i, code)
			}
		}
		if successes != 1 {
			t.Fatalf("iteration %d: expected exactly one successful demotion, got %d (%v)", i, successes, codes)
		}

		// Verify exactly one owner remains after one demotion succeeds.
		pid, err := dbconv.StringToPgUUID(projID)
		if err != nil {
			t.Fatalf("iteration %d: parse projectID: %v", i, err)
		}
		count, err := a.DB.Queries.CountProjectOwners(context.Background(), pid)
		if err != nil {
			t.Fatalf("iteration %d: count owners: %v", i, err)
		}
		if count != 1 {
			t.Fatalf("iteration %d: owner count = %d, want = 1", i, count)
		}
	}
}

// --- Add member tests ---

func TestMembership_AddMember(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)

	// Owner adds bob as member.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projID),
		map[string]any{"user_id": bobID, "role": "member"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["user_id"] != bobID {
		t.Errorf("user_id = %v, want %s", resp["user_id"], bobID)
	}
	if resp["role"] != "member" {
		t.Errorf("role = %v, want member", resp["role"])
	}
	if resp["project_id"] != projID {
		t.Errorf("project_id = %v, want %s", resp["project_id"], projID)
	}

	// Verify bob appears in member list.
	w = doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s/members", projID), nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", body["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	_ = aliceID // used for setup
}

func TestMembership_AddMemberAsAdmin(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	charlieResp := registerUser(t, a, "charlie")
	charlieID := getUserID(t, charlieResp)

	// Admin bob adds charlie as member.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projID),
		map[string]any{"user_id": charlieID, "role": "member"},
		authHeader(bobToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestMembership_AddMemberDefaultRole(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)

	// POST with no role field → defaults to member.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projID),
		map[string]any{"user_id": bobID},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["role"] != "member" {
		t.Errorf("role = %v, want member", resp["role"])
	}
}

func TestMembership_AddMemberAdminCannotAssignOwner(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	charlieResp := registerUser(t, a, "charlie")
	charlieID := getUserID(t, charlieResp)

	// Admin bob tries to add charlie as owner → 403.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projID),
		map[string]any{"user_id": charlieID, "role": "owner"},
		authHeader(bobToken))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestMembership_AddMemberOwnerCanAssignOwner(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)

	// Owner adds bob as owner.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projID),
		map[string]any{"user_id": bobID, "role": "owner"},
		authHeader(aliceToken))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["role"] != "owner" {
		t.Errorf("role = %v, want owner", resp["role"])
	}
}

func TestMembership_AddMemberAlreadyExists(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	// Try to add bob again → 409.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projID),
		map[string]any{"user_id": bobID, "role": "member"},
		authHeader(aliceToken))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestMembership_AddMemberUserNotFound(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	fakeUserID := "00000000-0000-0000-0000-000000000099"

	// Try to add non-existent user → 404.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projID),
		map[string]any{"user_id": fakeUserID, "role": "member"},
		authHeader(aliceToken))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestMembership_AddMemberInvalidRole(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)

	// Invalid role.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projID),
		map[string]any{"user_id": bobID, "role": "superadmin"},
		authHeader(aliceToken))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestMembership_AddMemberMissingUserID(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Missing user_id → 422 validation error.
	w := doRequest(t, a, http.MethodPost,
		fmt.Sprintf("/v1/projects/%s/members", projID),
		map[string]any{"role": "member"},
		authHeader(aliceToken))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
}

// --- Additional coverage tests ---

func TestMembership_ListMembersFiltersInactiveUsers(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	// Add bob as member, then deactivate bob.
	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "member", aliceID)

	uid, err := dbconv.StringToPgUUID(bobID)
	if err != nil {
		t.Fatalf("parse bobID: %v", err)
	}
	if err := a.DB.Queries.DeactivateUser(context.Background(), uid); err != nil {
		t.Fatalf("deactivate bob: %v", err)
	}

	// List members — bob should be excluded.
	w := doRequest(t, a, http.MethodGet, fmt.Sprintf("/v1/projects/%s/members", projID), nil, authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", body["items"])
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (inactive user should be filtered)", len(items))
	}
	m, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item is not map[string]any: %T", items[0])
	}
	if m["username"] != "alice" {
		t.Errorf("expected alice, got username = %v", m["username"])
	}
}

func TestMembership_AdminRemovesAdmin(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobToken := loginUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	charlieResp := registerUser(t, a, "charlie")
	charlieID := getUserID(t, charlieResp)
	addMember(t, a, projID, charlieID, "admin", aliceID)

	// Admin bob removes admin charlie — should succeed.
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, charlieID),
		nil, authHeader(bobToken))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestMembership_OwnerRemovesOwnerWhenMultiple(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "owner", aliceID)

	// Alice (owner) removes bob (owner) — two owners, so this succeeds.
	w := doRequest(t, a, http.MethodDelete,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		nil, authHeader(aliceToken))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestMembership_UpdateRoleNoOp(t *testing.T) {
	a := setupTestApp(t)
	truncateAll(t, a)

	aliceResp := registerUser(t, a, "alice")
	aliceToken := loginUser(t, a, "alice")
	aliceID := getUserID(t, aliceResp)
	keyID := createSSHKey(t, a, "key1", aliceToken)
	proj := createProject(t, a, "proj1", "git@github.com:org/repo.git", keyID, aliceToken)
	projID := mustString(t, proj, "id")

	bobResp := registerUser(t, a, "bob")
	bobID := getUserID(t, bobResp)
	addMember(t, a, projID, bobID, "admin", aliceID)

	// Update bob's role to the same value — should return 200 with current state.
	w := doRequest(t, a, http.MethodPatch,
		fmt.Sprintf("/v1/projects/%s/members/%s", projID, bobID),
		map[string]any{"role": "admin"},
		authHeader(aliceToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	resp := decodeJSON(t, w)
	if resp["role"] != "admin" {
		t.Errorf("role = %v, want admin", resp["role"])
	}
	if resp["user_id"] != bobID {
		t.Errorf("user_id = %v, want %s", resp["user_id"], bobID)
	}
}

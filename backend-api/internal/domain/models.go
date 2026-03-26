package domain

import "time"

// User represents a platform user (Phase 1: username-based identity).
type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProjectMember represents a user's membership and role within a project.
type ProjectMember struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProjectMemberWithUser is a list-response projection of a membership row
// joined with user info (username, display_name, avatar_url).
// Unlike ProjectMember it omits updated_at (not needed in list views).
type ProjectMemberWithUser struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

// SSHKey represents a user's SSH key pair in the key library.
// The private key is never exposed via the API.
type SSHKey struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	PublicKey   string     `json:"public_key"`
	Fingerprint string     `json:"fingerprint"`
	KeyType     string     `json:"key_type"`
	IsActive    bool       `json:"is_active"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	RotatedAt   *time.Time `json:"rotated_at"`
}

// Project represents a code project managed by MyJungle.
type Project struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	RepoURL       string    `json:"repo_url"`
	DefaultBranch string    `json:"default_branch"`
	Status        string    `json:"status"`
	CreatedBy     string    `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SSHKeySummary is a lightweight view of an SSH key used when returning
// the key assigned to a project (no private key, no is_active flag).
type SSHKeySummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint"`
	PublicKey   string    `json:"public_key"`
	KeyType     string    `json:"key_type"`
	CreatedAt   time.Time `json:"created_at"`
}

// EmbeddingProviderConfig represents an embedding provider configuration.
type EmbeddingProviderConfig struct {
	ID                    string         `json:"id"`
	Name                  string         `json:"name"`
	Provider              string         `json:"provider"`
	EndpointURL           string         `json:"endpoint_url"`
	Model                 string         `json:"model"`
	Dimensions            int            `json:"dimensions"`
	MaxTokens             int            `json:"max_tokens"`
	Settings              map[string]any `json:"settings"`
	HasCredentials        bool           `json:"has_credentials"`
	IsActive              bool           `json:"is_active"`
	IsDefault             bool           `json:"is_default"`
	IsAvailableToProjects bool           `json:"is_available_to_projects"`
	ProjectID             string         `json:"project_id,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

// LLMProviderConfig represents an LLM provider configuration.
type LLMProviderConfig struct {
	ID                    string         `json:"id"`
	Name                  string         `json:"name"`
	Provider              string         `json:"provider"`
	EndpointURL           string         `json:"endpoint_url"`
	Model                 string         `json:"model,omitempty"`
	Settings              map[string]any `json:"settings"`
	HasCredentials        bool           `json:"has_credentials"`
	IsActive              bool           `json:"is_active"`
	IsDefault             bool           `json:"is_default"`
	IsAvailableToProjects bool           `json:"is_available_to_projects"`
	ProjectID             string         `json:"project_id,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

// ProjectProviderSetting describes a project's current selection mode for a capability.
type ProjectProviderSetting struct {
	Mode           string `json:"mode"`
	GlobalConfigID string `json:"global_config_id,omitempty"`
	Config         any    `json:"config"`
}

// ResolvedEmbeddingProviderSetting describes the effective embedding config.
type ResolvedEmbeddingProviderSetting struct {
	Source string                  `json:"source"`
	Config EmbeddingProviderConfig `json:"config"`
}

// ResolvedLLMProviderSetting describes the effective LLM config.
type ResolvedLLMProviderSetting struct {
	Source string            `json:"source"`
	Config LLMProviderConfig `json:"config"`
}

// API key type constants.
const (
	KeyTypeProject  = "project"
	KeyTypePersonal = "personal"
)

// API key role constants (subset of project roles — no "owner").
const (
	KeyRoleRead  = "read"
	KeyRoleWrite = "write"
)

// KeyRoleValid reports whether role is a recognised API key role.
func KeyRoleValid(role string) bool {
	return role == KeyRoleRead || role == KeyRoleWrite
}

// APIKeyInfo represents an API key returned by list and create endpoints.
// The plaintext key is never stored; it is returned exactly once on creation
// via a separate response field.
type APIKeyInfo struct {
	ID         string     `json:"id"`
	KeyType    string     `json:"key_type"`
	KeyPrefix  string     `json:"key_prefix"`
	Name       string     `json:"name"`
	Role       string     `json:"role"`
	IsActive   bool       `json:"is_active"`
	ProjectID  string     `json:"project_id,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedBy  string     `json:"created_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Project-scoped role constants.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Platform-level role constants.
const (
	PlatformRoleAdmin = "platform_admin"
)

// PlatformRoleKnown reports whether role is a recognised platform role constant.
func PlatformRoleKnown(role string) bool {
	return role == PlatformRoleAdmin
}

// RoleRank returns a numeric rank for role hierarchy comparison.
// Higher rank means more privilege: owner(3) > admin(2) > member(1).
// Unknown roles return 0.
func RoleRank(role string) int {
	switch role {
	case RoleOwner:
		return 3
	case RoleAdmin:
		return 2
	case RoleMember:
		return 1
	default:
		return 0
	}
}

// RoleKnown reports whether role is a recognised role constant.
func RoleKnown(role string) bool {
	return RoleRank(role) > 0
}

// RoleSufficient returns true if the user's role meets or exceeds the minimum
// required role. It fails closed: if minRole is unknown the result is always
// false, preventing accidental access grants from typos or invalid input.
func RoleSufficient(userRole, minRole string) bool {
	if !RoleKnown(minRole) {
		return false
	}
	return RoleRank(userRole) >= RoleRank(minRole)
}

// APIKeyIdentity represents a resolved API key caller (stored in request context).
type APIKeyIdentity struct {
	KeyHash string `json:"-"`        // SHA-256 hex digest (never exposed)
	KeyType string `json:"key_type"` // "project" or "personal"
	Role    string `json:"role"`     // "read" or "write"
}

// KeyRoleRank returns a numeric rank for API key role hierarchy.
// Higher rank means more privilege: write(2) > read(1).
// Unknown roles return 0.
func KeyRoleRank(role string) int {
	switch role {
	case KeyRoleWrite:
		return 2
	case KeyRoleRead:
		return 1
	default:
		return 0
	}
}

// KeyRoleSufficient returns true if the key's effective role meets or exceeds
// the minimum required key role. Fails closed: unknown minRole → false.
func KeyRoleSufficient(keyRole, minKeyRole string) bool {
	if KeyRoleRank(minKeyRole) == 0 {
		return false
	}
	return KeyRoleRank(keyRole) >= KeyRoleRank(minKeyRole)
}

// MembershipRoleToKeyRole maps a project membership minRole to the equivalent
// API key minRole. Returns ("", false) for roles that API keys cannot satisfy
// (e.g. owner-level operations).
//
//	member → read
//	admin  → write
//	owner  → (blocked)
func MembershipRoleToKeyRole(membershipMinRole string) (string, bool) {
	switch membershipMinRole {
	case RoleMember:
		return KeyRoleRead, true
	case RoleAdmin:
		return KeyRoleWrite, true
	default:
		// owner or unknown → API keys cannot access
		return "", false
	}
}

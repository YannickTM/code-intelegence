// Package validate provides reusable request validation helpers.
//
// Validators follow two return conventions:
//   - "sanitising" validators (Required, UUID, URL) return "" on failure,
//     allowing callers to use the cleaned value directly.
//   - "checking" validators (MinMax, OneOf, MaxLength) return the original
//     value on failure, only recording an error in the Errors map.
//
// In both cases, callers should check errs.HasErrors() before using results.
package validate

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"myjungle/backend-api/internal/domain"
)

// uuidRE matches a standard UUID (8-4-4-4-12 hex).
var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// scpRE matches SCP-style SSH URLs: git@host:org/repo or git@host:org/repo.git
var scpRE = regexp.MustCompile(`^git@[a-zA-Z0-9._-]+:[a-zA-Z0-9._-]+/[a-zA-Z0-9._/-]+(\.git)?$`)

// allowedRepoSchemes is the set of URL schemes accepted for repository URLs.
var allowedRepoSchemes = map[string]bool{
	"https": true,
	"ssh":   true,
}

// Errors collects per-field validation errors.
type Errors map[string]string

// Add registers a validation error for the given field.
func (e Errors) Add(field, message string) {
	e[field] = message
}

// HasErrors returns true if any validation errors have been recorded.
func (e Errors) HasErrors() bool {
	return len(e) > 0
}

// ToAppError converts the validation errors into a *domain.AppError
// with status 422 and the field map as details.
func (e Errors) ToAppError() *domain.AppError {
	return domain.ValidationError(map[string]string(e))
}

// Required checks that value is non-empty after trimming.
// On failure it adds an error to errs and returns "".
func Required(value, field string, errs Errors) string {
	v := strings.TrimSpace(value)
	if v == "" {
		errs.Add(field, field+" is required")
		return ""
	}
	return v
}

// UUID checks that value is a valid UUID string.
// On failure it adds an error to errs and returns "".
func UUID(value, field string, errs Errors) string {
	if !uuidRE.MatchString(value) {
		errs.Add(field, field+" must be a valid UUID")
		return ""
	}
	return value
}

// MinMax checks that value is between min and max inclusive.
// On failure it adds an error to errs and returns the original value.
func MinMax(value, min, max int, field string, errs Errors) int {
	if value < min || value > max {
		errs.Add(field, fmt.Sprintf("%s must be between %d and %d", field, min, max))
	}
	return value
}

// OneOf checks that value is one of the allowed values.
// On failure it adds an error to errs and returns the original value.
func OneOf(value string, allowed []string, field string, errs Errors) string {
	if len(allowed) == 0 {
		errs.Add(field, fmt.Sprintf("%s has no allowed values", field))
		return value
	}
	for _, a := range allowed {
		if value == a {
			return value
		}
	}
	errs.Add(field, fmt.Sprintf("%s must be one of: %s", field, strings.Join(allowed, ", ")))
	return value
}

// URL checks that value is a valid URL with a scheme and host.
// Empty values are accepted without error.
// On failure it adds an error to errs and returns "".
func URL(value, field string, errs Errors) string {
	if value == "" {
		return value
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		errs.Add(field, field+" must be a valid URL")
		return ""
	}
	return value
}

// RepoURL checks that value looks like a git repository URL.
// It accepts SCP-style SSH URLs (git@host:org/repo) validated by regex,
// and standard URLs with an allowed scheme (https, ssh) and a host.
// Both forms must include at least an org/repo path shape.
// Empty values are accepted without error.
// On failure it adds an error to errs and returns "".
func RepoURL(value, field string, errs Errors) string {
	if value == "" {
		return value
	}
	// Accept SCP-style SSH URLs: git@host:org/repo.git
	if strings.HasPrefix(value, "git@") {
		if !scpRE.MatchString(value) {
			errs.Add(field, field+" must be a valid git URL (git@host:org/repo)")
			return ""
		}
		return value
	}
	// Standard URL: must have an allowed scheme + host.
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || !allowedRepoSchemes[u.Scheme] {
		errs.Add(field, field+" must be a valid git URL (git@..., https://..., or ssh://...)")
		return ""
	}
	// Require at least org/repo path shape (both segments non-empty).
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		errs.Add(field, field+" must include a repository path (e.g. org/repo)")
		return ""
	}
	return value
}

// Email checks that value is a valid email address (non-empty, exactly one @,
// non-empty local and domain parts, no embedded whitespace).
// The value is normalized to lowercase and trimmed.
// On failure it adds an error to errs and returns "".
func Email(value, field string, errs Errors) string {
	v := strings.TrimSpace(value)
	if v == "" {
		errs.Add(field, field+" is required")
		return ""
	}
	if strings.ContainsAny(v, " \t\n\r") {
		errs.Add(field, field+" must be a valid email address")
		return ""
	}
	if strings.Count(v, "@") != 1 {
		errs.Add(field, field+" must be a valid email address")
		return ""
	}
	at := strings.Index(v, "@")
	if at <= 0 || at >= len(v)-1 {
		errs.Add(field, field+" must be a valid email address")
		return ""
	}
	return strings.ToLower(v)
}

// reservedUsernames contains usernames that are created by seed migrations
// and must not be claimed via normal registration.
var reservedUsernames = map[string]bool{
	"admin": true,
}

// ReservedUsername records a validation error if the given username is
// reserved for system use. Call after normalisation (lowercase + trim).
func ReservedUsername(username, field string, errs Errors) {
	if reservedUsernames[username] {
		errs[field] = field + " is reserved"
	}
}

// MaxLength checks that value does not exceed max characters (Unicode runes).
// On failure it adds an error to errs and returns the original value.
func MaxLength(value string, max int, field string, errs Errors) string {
	if utf8.RuneCountInString(value) > max {
		errs.Add(field, fmt.Sprintf("%s must not exceed %d characters", field, max))
	}
	return value
}

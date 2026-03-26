package validate

import (
	"testing"
)

func TestRequired_Valid(t *testing.T) {
	errs := Errors{}
	got := Required("hello", "name", errs)
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	if errs.HasErrors() {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestRequired_Trims(t *testing.T) {
	errs := Errors{}
	got := Required("  hello  ", "name", errs)
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	if errs.HasErrors() {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestRequired_Empty(t *testing.T) {
	errs := Errors{}
	got := Required("", "name", errs)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if !errs.HasErrors() {
		t.Error("expected error")
	}
	if errs["name"] != "name is required" {
		t.Errorf("error = %q, want %q", errs["name"], "name is required")
	}
}

func TestRequired_Whitespace(t *testing.T) {
	errs := Errors{}
	Required("   ", "name", errs)
	if !errs.HasErrors() {
		t.Error("expected error for whitespace-only value")
	}
}

func TestUUID_Valid(t *testing.T) {
	errs := Errors{}
	got := UUID("550e8400-e29b-41d4-a716-446655440000", "id", errs)
	if got != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("got %q, want UUID back", got)
	}
	if errs.HasErrors() {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestUUID_Invalid(t *testing.T) {
	tests := []string{"", "not-a-uuid", "550e8400-e29b-41d4-a716", "zze8400-e29b-41d4-a716-446655440000"}
	for _, v := range tests {
		errs := Errors{}
		got := UUID(v, "id", errs)
		if got != "" {
			t.Errorf("UUID(%q) = %q, want empty", v, got)
		}
		if !errs.HasErrors() {
			t.Errorf("UUID(%q) expected error", v)
		}
	}
}

func TestMinMax_InRange(t *testing.T) {
	errs := Errors{}
	got := MinMax(5, 1, 10, "count", errs)
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
	if errs.HasErrors() {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestMinMax_AtBounds(t *testing.T) {
	errs := Errors{}
	MinMax(1, 1, 10, "count", errs)
	if errs.HasErrors() {
		t.Error("min bound should be valid")
	}
	errs = Errors{}
	MinMax(10, 1, 10, "count", errs)
	if errs.HasErrors() {
		t.Error("max bound should be valid")
	}
}

func TestMinMax_OutOfRange(t *testing.T) {
	errs := Errors{}
	MinMax(0, 1, 10, "count", errs)
	if !errs.HasErrors() {
		t.Error("expected error for below min")
	}

	errs = Errors{}
	MinMax(11, 1, 10, "count", errs)
	if !errs.HasErrors() {
		t.Error("expected error for above max")
	}
}

func TestOneOf_Valid(t *testing.T) {
	errs := Errors{}
	got := OneOf("admin", []string{"admin", "member", "viewer"}, "role", errs)
	if got != "admin" {
		t.Errorf("got %q, want %q", got, "admin")
	}
	if errs.HasErrors() {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestOneOf_Invalid(t *testing.T) {
	errs := Errors{}
	OneOf("superadmin", []string{"admin", "member", "viewer"}, "role", errs)
	if !errs.HasErrors() {
		t.Error("expected error")
	}
	if msg := errs["role"]; msg == "" {
		t.Error("expected error message for role")
	}
}

func TestOneOf_EmptyAllowed(t *testing.T) {
	errs := Errors{}
	OneOf("anything", nil, "role", errs)
	if !errs.HasErrors() {
		t.Error("expected error for empty allowed slice")
	}
	if msg := errs["role"]; msg != "role has no allowed values" {
		t.Errorf("error = %q, want %q", msg, "role has no allowed values")
	}
}

func TestURL_Valid(t *testing.T) {
	errs := Errors{}
	got := URL("https://example.com/path", "endpoint", errs)
	if got != "https://example.com/path" {
		t.Errorf("got %q", got)
	}
	if errs.HasErrors() {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestURL_Empty(t *testing.T) {
	errs := Errors{}
	got := URL("", "endpoint", errs)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if errs.HasErrors() {
		t.Error("empty URL should not be an error")
	}
}

func TestURL_Invalid(t *testing.T) {
	tests := []string{"not-a-url", "ftp://", "://missing-scheme"}
	for _, v := range tests {
		errs := Errors{}
		URL(v, "endpoint", errs)
		if !errs.HasErrors() {
			t.Errorf("URL(%q) expected error", v)
		}
	}
}

func TestMaxLength_Valid(t *testing.T) {
	errs := Errors{}
	got := MaxLength("hello", 10, "name", errs)
	if got != "hello" {
		t.Errorf("got %q", got)
	}
	if errs.HasErrors() {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestMaxLength_Exceeded(t *testing.T) {
	errs := Errors{}
	MaxLength("this is too long", 5, "name", errs)
	if !errs.HasErrors() {
		t.Error("expected error")
	}
}

func TestMaxLength_Unicode(t *testing.T) {
	// "héllo" is 5 runes but 6 bytes (é = 2 bytes in UTF-8).
	errs := Errors{}
	got := MaxLength("héllo", 5, "name", errs)
	if got != "héllo" {
		t.Errorf("got %q", got)
	}
	if errs.HasErrors() {
		t.Error("5-rune string should pass max=5 (rune counting, not byte counting)")
	}
}

func TestErrors_Multiple(t *testing.T) {
	errs := Errors{}
	Required("", "name", errs)
	Required("", "email", errs)
	UUID("bad", "id", errs)

	if len(errs) != 3 {
		t.Errorf("len(errs) = %d, want 3", len(errs))
	}
}

func TestRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty accepted", "", false},
		{"scp org/repo.git", "git@github.com:org/repo.git", false},
		{"scp org/repo no .git", "git@github.com:org/repo", false},
		{"scp nested path", "git@github.com:org/sub/repo.git", false},
		{"scp no slash", "git@host:repo", true},
		{"scp no slash with .git", "git@host:repo.git", true},
		{"https org/repo.git", "https://github.com/org/repo.git", false},
		{"https org/repo", "https://github.com/org/repo", false},
		{"ssh org/repo", "ssh://git@github.com/org/repo", false},
		{"http rejected", "http://github.com/org/repo", true},
		{"git scheme rejected", "git://github.com/org/repo", true},
		{"https host only", "https://github.com", true},
		{"https single segment", "https://github.com/repo", true},
		{"ftp rejected", "ftp://example.com/org/repo.git", true},
		{"garbage", "not-a-url", true},
		// Regression: trailing slash must not satisfy org/repo shape.
		{"https trailing slash", "https://github.com/org/", true},
		{"https trailing slash no org", "https://github.com//", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Errors{}
			got := RepoURL(tt.value, "repo_url", errs)
			if tt.wantErr {
				if !errs.HasErrors() {
					t.Errorf("RepoURL(%q) expected error, got none", tt.value)
				}
				if got != "" {
					t.Errorf("RepoURL(%q) = %q, want empty on error", tt.value, got)
				}
			} else {
				if errs.HasErrors() {
					t.Errorf("RepoURL(%q) unexpected error: %v", tt.value, errs)
				}
				if got != tt.value {
					t.Errorf("RepoURL(%q) = %q, want %q", tt.value, got, tt.value)
				}
			}
		})
	}
}

func TestErrors_ToAppError(t *testing.T) {
	errs := Errors{}
	Required("", "name", errs)

	appErr := errs.ToAppError()
	if appErr.Status != 422 {
		t.Errorf("Status = %d, want 422", appErr.Status)
	}
	if appErr.Code != "validation_error" {
		t.Errorf("Code = %q, want %q", appErr.Code, "validation_error")
	}
	details, ok := appErr.Details.(map[string]string)
	if !ok {
		t.Fatalf("Details type = %T, want map[string]string", appErr.Details)
	}
	if details["name"] != "name is required" {
		t.Errorf("details[name] = %q", details["name"])
	}
}

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

const tokenBytes = 32 // 256 bits of entropy

// GenerateSessionToken generates a cryptographically random session token
// and its SHA-256 hash. The raw token is returned to the client; only the
// hash is stored in the database.
func GenerateSessionToken() (raw string, hash string, err error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	raw = hex.EncodeToString(b)
	hash = HashToken(raw)
	return raw, hash, nil
}

// HashToken returns the SHA-256 hex digest of a raw token string.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// HasBearerScheme returns true if the Authorization header uses the Bearer scheme,
// regardless of whether a token value follows. Matches "Bearer" exactly or
// "Bearer " followed by any token value.
func HasBearerScheme(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	if len(h) == 6 {
		return strings.EqualFold(h, "bearer")
	}
	return len(h) > 6 && strings.EqualFold(h[:7], "bearer ")
}

// ExtractBearerToken extracts the token from an "Authorization: Bearer <token>"
// header. Returns "" if the header is missing, malformed, or has an empty token.
func ExtractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// Package apikey provides API key generation, hashing, and CRUD operations.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"myjungle/backend-api/internal/domain"
)

// base62 alphabet for random key generation.
const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// randomChars is the number of random characters after the prefix.
const randomChars = 32

// storedPrefixLen is how many leading characters of the full key are stored
// in key_prefix for display/identification purposes.
const storedPrefixLen = 12

// prefixForType returns the key prefix string for the given key type.
func prefixForType(keyType string) (string, error) {
	switch keyType {
	case domain.KeyTypeProject:
		return "mj_proj_", nil
	case domain.KeyTypePersonal:
		return "mj_pers_", nil
	default:
		return "", fmt.Errorf("unknown key type: %q", keyType)
	}
}

// GenerateAPIKey creates a new API key of the given type.
// It returns:
//   - plaintext: the full key (shown once to the user, never stored)
//   - prefix:    first 12 chars for display (stored in DB)
//   - hash:      hex-encoded SHA-256 of the plaintext (stored in DB)
func GenerateAPIKey(keyType string) (plaintext, prefix, hash string, err error) {
	typePrefix, err := prefixForType(keyType)
	if err != nil {
		return "", "", "", err
	}

	// Generate random base62 characters.
	buf := make([]byte, randomChars)
	max := big.NewInt(int64(len(base62)))
	for i := range buf {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", "", "", fmt.Errorf("generate random: %w", err)
		}
		buf[i] = base62[n.Int64()]
	}

	plaintext = typePrefix + string(buf)
	prefix = plaintext[:storedPrefixLen]
	hash = HashKey(plaintext)
	return plaintext, prefix, hash, nil
}

// HashKey returns the hex-encoded SHA-256 hash of a plaintext API key.
func HashKey(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

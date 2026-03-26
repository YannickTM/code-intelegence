// Package sshkey provides SSH private key decryption for the worker.
// Compatible with backend-api/internal/sshkey/encrypt.go.
package sshkey

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"fmt"
)

// DeriveKey derives a 32-byte AES-256 key from the given secret string
// using SHA-256.
func DeriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// Decrypt decrypts ciphertext produced by AES-256-GCM encryption.
// Format: nonce (12 bytes) || sealed(ciphertext + tag).
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("sshkey: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("sshkey: new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("sshkey: ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("sshkey: decrypt: %w", err)
	}
	return plaintext, nil
}

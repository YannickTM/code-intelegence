package sshkey

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"testing"
)

// encrypt is a test helper matching backend-api's Encrypt function.
func encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func TestDeriveKey_Deterministic(t *testing.T) {
	k1 := DeriveKey("test-secret")
	k2 := DeriveKey("test-secret")
	if !bytes.Equal(k1, k2) {
		t.Error("DeriveKey is not deterministic")
	}
}

func TestDeriveKey_Length(t *testing.T) {
	k := DeriveKey("any-secret")
	if len(k) != 32 {
		t.Errorf("DeriveKey length = %d, want 32", len(k))
	}
}

func TestDeriveKey_DifferentSecrets(t *testing.T) {
	k1 := DeriveKey("secret-one")
	k2 := DeriveKey("secret-two")
	if bytes.Equal(k1, k2) {
		t.Error("different secrets should produce different keys")
	}
}

func TestDecrypt_Roundtrip(t *testing.T) {
	key := DeriveKey("roundtrip-secret-at-least-32-chars")
	plaintext := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nfake-key-data\n-----END OPENSSH PRIVATE KEY-----\n")

	ct, err := encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Errorf("Decrypt mismatch: got %q", pt)
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := DeriveKey("correct-key-secret-value-here!!")
	key2 := DeriveKey("wrong-key-secret-different-val!!")

	ct, err := encrypt([]byte("secret-data"), key1)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = Decrypt(ct, key2)
	if err == nil {
		t.Error("Decrypt with wrong key should fail")
	}
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	_, err := Decrypt([]byte("short"), DeriveKey("any-key"))
	if err == nil {
		t.Error("Decrypt with truncated ciphertext should fail")
	}
}

func TestDecrypt_EmptyCiphertext(t *testing.T) {
	_, err := Decrypt(nil, DeriveKey("any-key"))
	if err == nil {
		t.Error("Decrypt with empty ciphertext should fail")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	key := DeriveKey("tamper-test-secret-value-here!!")
	ct, err := encrypt([]byte("original"), key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Flip a byte in the ciphertext portion (after nonce)
	ct[len(ct)-1] ^= 0xff

	_, err = Decrypt(ct, key)
	if err == nil {
		t.Error("Decrypt with tampered ciphertext should fail")
	}
}

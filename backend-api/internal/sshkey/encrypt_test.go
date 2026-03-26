package sshkey

import (
	"bytes"
	"testing"
)

func TestDeriveKey_Deterministic(t *testing.T) {
	k1 := DeriveKey("test-secret")
	k2 := DeriveKey("test-secret")
	if !bytes.Equal(k1, k2) {
		t.Error("DeriveKey is not deterministic for the same input")
	}
}

func TestDeriveKey_Length(t *testing.T) {
	k := DeriveKey("any-secret")
	if len(k) != 32 {
		t.Errorf("DeriveKey length = %d, want 32", len(k))
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	key := DeriveKey("roundtrip-secret")
	plaintext := []byte("hello, this is a secret SSH key")

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(pt, plaintext) {
		t.Errorf("Decrypt result = %q, want %q", pt, plaintext)
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := DeriveKey("key-one")
	key2 := DeriveKey("key-two")

	ct, err := Encrypt([]byte("secret"), key1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = Decrypt(ct, key2)
	if err == nil {
		t.Error("Decrypt with wrong key should fail")
	}
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	_, err := Decrypt([]byte("short"), DeriveKey("key"))
	if err == nil {
		t.Error("Decrypt with truncated ciphertext should fail")
	}
}

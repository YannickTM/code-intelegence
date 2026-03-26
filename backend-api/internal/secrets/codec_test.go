package secrets

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := DeriveKey("provider-secret")
	plaintext := []byte(`{"api_key":"secret"}`)

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestDecryptShortCiphertext(t *testing.T) {
	key := DeriveKey("provider-secret")

	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
	if !strings.Contains(err.Error(), "ciphertext too short") {
		t.Fatalf("Decrypt() error = %v, want ciphertext too short", err)
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	keyA := DeriveKey("provider-secret-a")
	keyB := DeriveKey("provider-secret-b")
	plaintext := []byte(`{"api_key":"secret"}`)

	ciphertext, err := Encrypt(plaintext, keyA)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	got, err := Decrypt(ciphertext, keyB)
	if err == nil {
		t.Fatalf("Decrypt() with wrong key unexpectedly succeeded: %q", got)
	}
}

func TestEncryptDecryptEmptyPlaintext(t *testing.T) {
	key := DeriveKey("provider-secret")
	plaintext := []byte{}

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Decrypt() = %q, want empty plaintext", got)
	}
}

func TestDecryptTamperedCiphertextFails(t *testing.T) {
	key := DeriveKey("provider-secret")
	plaintext := []byte(`{"api_key":"secret"}`)

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if len(ciphertext) < 13 {
		t.Fatalf("Encrypt() produced ciphertext too short for tamper test: %d", len(ciphertext))
	}

	ciphertext[12] ^= 0xFF

	if _, err := Decrypt(ciphertext, key); err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}

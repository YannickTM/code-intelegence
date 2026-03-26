package sshkey

import (
	"errors"
	"strings"
	"testing"
)

func TestNewService_EmptySecret(t *testing.T) {
	_, err := NewService("")
	if err == nil {
		t.Fatal("expected error for empty secret, got nil")
	}
}

func TestNewService_ShortSecret(t *testing.T) {
	_, err := NewService("too-short")
	if err == nil {
		t.Fatal("expected error for short secret, got nil")
	}
}

func TestServiceCreate(t *testing.T) {
	svc, err := NewService("test-encryption-secret-long-enough!")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	pub, fingerprint, keyType, encPriv, err := svc.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Errorf("pub prefix = %q, want ssh-ed25519 prefix", pub)
	}
	if !strings.HasPrefix(fingerprint, "SHA256:") {
		t.Errorf("fingerprint prefix = %q, want SHA256: prefix", fingerprint)
	}
	if keyType != "ed25519" {
		t.Errorf("keyType = %q, want %q", keyType, "ed25519")
	}
	if len(encPriv) == 0 {
		t.Error("encrypted private key is empty")
	}

	// Verify the encrypted private key can be decrypted.
	decrypted, err := Decrypt(encPriv, svc.encKey)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !strings.Contains(string(decrypted), "PRIVATE KEY") {
		t.Error("decrypted private key does not contain PEM header")
	}
}

func TestServiceCreateFromPrivateKey(t *testing.T) {
	svc, err := NewService("test-encryption-secret-long-enough!")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Generate a key to use as "uploaded" input.
	orig, err := GenerateEd25519()
	if err != nil {
		t.Fatalf("GenerateEd25519: %v", err)
	}

	pub, fingerprint, keyType, encPriv, err := svc.CreateFromPrivateKey(orig.PrivateKey)
	if err != nil {
		t.Fatalf("CreateFromPrivateKey: %v", err)
	}

	if pub != orig.PublicKey {
		t.Errorf("pub = %q, want %q", pub, orig.PublicKey)
	}
	if fingerprint != orig.Fingerprint {
		t.Errorf("fingerprint = %q, want %q", fingerprint, orig.Fingerprint)
	}
	if keyType != "ed25519" {
		t.Errorf("keyType = %q, want %q", keyType, "ed25519")
	}
	if len(encPriv) == 0 {
		t.Error("encrypted private key is empty")
	}

	// Verify decryption roundtrip.
	decrypted, err := Decrypt(encPriv, svc.encKey)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(orig.PrivateKey) {
		t.Error("decrypted private key does not match original")
	}
}

func TestServiceCreateFromPrivateKey_InvalidPEM(t *testing.T) {
	svc, err := NewService("test-encryption-secret-long-enough!")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, _, _, _, err = svc.CreateFromPrivateKey([]byte("garbage"))
	if err == nil {
		t.Fatal("expected error for invalid PEM, got nil")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("error should wrap ErrInvalidKey, got %v", err)
	}
}

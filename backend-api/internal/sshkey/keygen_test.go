package sshkey

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateEd25519(t *testing.T) {
	kp, err := GenerateEd25519()
	if err != nil {
		t.Fatalf("GenerateEd25519: %v", err)
	}

	if !strings.HasPrefix(kp.PublicKey, "ssh-ed25519 ") {
		t.Errorf("PublicKey prefix = %q, want ssh-ed25519 prefix", kp.PublicKey)
	}
	if !strings.HasPrefix(kp.Fingerprint, "SHA256:") {
		t.Errorf("Fingerprint prefix = %q, want SHA256: prefix", kp.Fingerprint)
	}
	if !strings.Contains(string(kp.PrivateKey), "PRIVATE KEY") {
		t.Error("PrivateKey does not contain PRIVATE KEY PEM header")
	}
	if kp.KeyType != "ed25519" {
		t.Errorf("KeyType = %q, want %q", kp.KeyType, "ed25519")
	}
}

func TestGenerateEd25519_Uniqueness(t *testing.T) {
	kp1, err := GenerateEd25519()
	if err != nil {
		t.Fatalf("GenerateEd25519 (1): %v", err)
	}
	kp2, err := GenerateEd25519()
	if err != nil {
		t.Fatalf("GenerateEd25519 (2): %v", err)
	}

	if kp1.PublicKey == kp2.PublicKey {
		t.Error("two generated keys have the same public key")
	}
	if kp1.Fingerprint == kp2.Fingerprint {
		t.Error("two generated keys have the same fingerprint")
	}
}

func TestParsePrivateKey_Ed25519(t *testing.T) {
	orig, err := GenerateEd25519()
	if err != nil {
		t.Fatalf("GenerateEd25519: %v", err)
	}

	parsed, err := ParsePrivateKey(orig.PrivateKey)
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}

	if parsed.KeyType != "ed25519" {
		t.Errorf("KeyType = %q, want %q", parsed.KeyType, "ed25519")
	}
	if parsed.PublicKey != orig.PublicKey {
		t.Errorf("PublicKey mismatch:\n  got  = %q\n  want = %q", parsed.PublicKey, orig.PublicKey)
	}
	if parsed.Fingerprint != orig.Fingerprint {
		t.Errorf("Fingerprint mismatch: got %q, want %q", parsed.Fingerprint, orig.Fingerprint)
	}
}

func TestParsePrivateKey_RSA(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	})

	kp, err := ParsePrivateKey(pemBytes)
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}

	if kp.KeyType != "rsa" {
		t.Errorf("KeyType = %q, want %q", kp.KeyType, "rsa")
	}
	if !strings.HasPrefix(kp.PublicKey, "ssh-rsa ") {
		t.Errorf("PublicKey prefix = %q, want ssh-rsa prefix", kp.PublicKey)
	}
	if !strings.HasPrefix(kp.Fingerprint, "SHA256:") {
		t.Errorf("Fingerprint prefix = %q, want SHA256: prefix", kp.Fingerprint)
	}
}

func TestParsePrivateKey_ECDSA(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	})

	kp, err := ParsePrivateKey(pemBytes)
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}

	if kp.KeyType != "ecdsa" {
		t.Errorf("KeyType = %q, want %q", kp.KeyType, "ecdsa")
	}
	if !strings.HasPrefix(kp.PublicKey, "ecdsa-sha2-") {
		t.Errorf("PublicKey prefix = %q, want ecdsa-sha2- prefix", kp.PublicKey)
	}
	if !strings.HasPrefix(kp.Fingerprint, "SHA256:") {
		t.Errorf("Fingerprint prefix = %q, want SHA256: prefix", kp.Fingerprint)
	}
}

func TestParsePrivateKey_RejectSmallRSA(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	})

	_, err = ParsePrivateKey(pemBytes)
	if err == nil {
		t.Fatal("expected ParsePrivateKey to reject 1024-bit RSA key, got nil")
	}
	if !strings.Contains(err.Error(), "rsa key too small") {
		t.Errorf("error = %q, want message containing 'rsa key too small'", err.Error())
	}
}

func TestParsePrivateKey_RejectUnsupportedECDSACurve(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	})

	_, err = ParsePrivateKey(pemBytes)
	if err == nil {
		t.Fatal("expected ParsePrivateKey to reject P-224 ECDSA key, got nil")
	}
	if !strings.Contains(err.Error(), "ecdsa unsupported curve") {
		t.Errorf("error = %q, want message containing 'ecdsa unsupported curve'", err.Error())
	}
}

func TestParsePrivateKey_InvalidPEM(t *testing.T) {
	_, err := ParsePrivateKey([]byte("this is not a PEM key"))
	if err == nil {
		t.Fatal("expected error for invalid PEM, got nil")
	}
}

func TestParsePrivateKey_Passphrase(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	pemBlock, err := ssh.MarshalPrivateKeyWithPassphrase(privKey, "", []byte("secret123"))
	if err != nil {
		t.Fatalf("MarshalPrivateKeyWithPassphrase: %v", err)
	}
	pemBytes := pem.EncodeToMemory(pemBlock)

	_, err = ParsePrivateKey(pemBytes)
	if err == nil {
		t.Fatal("expected error for passphrase-protected key, got nil")
	}
	if !strings.Contains(err.Error(), "passphrase") {
		t.Errorf("error = %q, want message containing 'passphrase'", err.Error())
	}
	if !errors.Is(err, ErrPassphraseProtected) {
		t.Errorf("error should wrap ErrPassphraseProtected, got %v", err)
	}
}

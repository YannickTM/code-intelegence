// Package sshkey provides SSH key pair generation, parsing, and private key encryption.
package sshkey

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

const minRSABits = 2048

// KeyPair holds a generated or parsed SSH key pair.
type KeyPair struct {
	PublicKey   string // OpenSSH authorized_keys format
	PrivateKey  []byte // PEM-encoded private key (cleartext)
	Fingerprint string // SHA256:base64...
	KeyType     string // "ed25519", "rsa", "ecdsa"
}

// GenerateEd25519 generates a new Ed25519 SSH key pair.
func GenerateEd25519() (*KeyPair, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("sshkey: generate ed25519: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("sshkey: new public key: %w", err)
	}

	privPEM, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, fmt.Errorf("sshkey: marshal private key: %w", err)
	}

	fingerprint := ssh.FingerprintSHA256(sshPub)
	authorizedKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))

	return &KeyPair{
		PublicKey:   authorizedKey,
		PrivateKey:  pem.EncodeToMemory(privPEM),
		Fingerprint: fingerprint,
		KeyType:     "ed25519",
	}, nil
}

// ParsePrivateKey parses a PEM-encoded private key and derives the public key,
// fingerprint, and key type. Supports Ed25519, RSA, and ECDSA keys.
// Passphrase-protected keys are rejected with a clear error message.
func ParsePrivateKey(pemData []byte) (*KeyPair, error) {
	raw, err := ssh.ParseRawPrivateKey(pemData)
	if err != nil {
		var ppErr *ssh.PassphraseMissingError
		if errors.As(err, &ppErr) {
			return nil, fmt.Errorf("%w: please provide an unencrypted key", ErrPassphraseProtected)
		}
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	var keyType string
	var pubCrypto interface{}

	switch k := raw.(type) {
	case ed25519.PrivateKey:
		keyType = "ed25519"
		pubCrypto = k.Public()
	case *ed25519.PrivateKey:
		keyType = "ed25519"
		pubCrypto = k.Public()
	case *rsa.PrivateKey:
		if k.N.BitLen() < minRSABits {
			return nil, fmt.Errorf("rsa key too small: %d bits (minimum %d)", k.N.BitLen(), minRSABits)
		}
		keyType = "rsa"
		pubCrypto = k.Public()
	case *ecdsa.PrivateKey:
		switch k.Curve {
		case elliptic.P256(), elliptic.P384(), elliptic.P521():
			// allowed
		default:
			return nil, fmt.Errorf("ecdsa unsupported curve: %v", k.Curve.Params().Name)
		}
		keyType = "ecdsa"
		pubCrypto = k.Public()
	default:
		return nil, fmt.Errorf("unsupported key type: %T", raw)
	}

	sshPub, err := ssh.NewPublicKey(pubCrypto)
	if err != nil {
		return nil, fmt.Errorf("sshkey: derive public key: %w", err)
	}

	fingerprint := ssh.FingerprintSHA256(sshPub)
	authorizedKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))

	return &KeyPair{
		PublicKey:   authorizedKey,
		PrivateKey:  pemData,
		Fingerprint: fingerprint,
		KeyType:     keyType,
	}, nil
}

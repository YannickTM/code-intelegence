package sshkey

import (
	"errors"
	"fmt"
)

// ErrInvalidKey indicates a user-supplied private key could not be parsed.
var ErrInvalidKey = errors.New("invalid private key")

// ErrPassphraseProtected indicates the uploaded key is passphrase-protected.
var ErrPassphraseProtected = errors.New("passphrase-protected key")

// Service provides SSH key business logic.
type Service struct {
	encKey []byte // 32-byte AES-256 key derived from config secret
}

// NewService creates a Service from the raw encryption secret.
// Returns an error if the secret is less than 32 characters.
func NewService(encryptionSecret string) (*Service, error) {
	if len(encryptionSecret) < 32 {
		return nil, fmt.Errorf("sshkey: SSH_KEY_ENCRYPTION_SECRET must be at least 32 characters")
	}
	return &Service{
		encKey: DeriveKey(encryptionSecret),
	}, nil
}

// Create generates a new Ed25519 key pair and encrypts the private key.
// Returns the public key (OpenSSH format), fingerprint, key type, and
// encrypted private key bytes suitable for database storage.
func (s *Service) Create() (pub, fingerprint, keyType string, encryptedPrivKey []byte, err error) {
	kp, err := GenerateEd25519()
	if err != nil {
		return "", "", "", nil, err
	}
	encrypted, err := Encrypt(kp.PrivateKey, s.encKey)
	if err != nil {
		return "", "", "", nil, err
	}
	return kp.PublicKey, kp.Fingerprint, kp.KeyType, encrypted, nil
}

// CreateFromPrivateKey parses a PEM-encoded private key, derives the public
// key and fingerprint, and encrypts the private key for storage.
// Supports Ed25519, RSA, and ECDSA keys.
func (s *Service) CreateFromPrivateKey(pemData []byte) (pub, fingerprint, keyType string, encryptedPrivKey []byte, err error) {
	kp, err := ParsePrivateKey(pemData)
	if err != nil {
		if errors.Is(err, ErrPassphraseProtected) {
			return "", "", "", nil, err
		}
		return "", "", "", nil, fmt.Errorf("%w: %w", ErrInvalidKey, err)
	}
	encrypted, err := Encrypt(kp.PrivateKey, s.encKey)
	if err != nil {
		return "", "", "", nil, err
	}
	return kp.PublicKey, kp.Fingerprint, kp.KeyType, encrypted, nil
}

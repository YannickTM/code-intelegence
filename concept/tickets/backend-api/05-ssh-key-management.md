# 05 — SSH Key CRUD with Encryption

## Status
Done

## Goal
Implemented the SSH key library: per-user key management with Ed25519 generation, PEM private key upload (Ed25519, RSA, ECDSA), AES-256-GCM encryption at rest, project assignment with active/inactive tracking, key retirement with assignment protection, and key-projects listing.

## Depends On
01-foundation, 02-authentication

## Scope

### Key Generation and Parsing

`internal/sshkey/keygen.go` provides:
- `GenerateEd25519()` -- creates a new Ed25519 key pair using `crypto/ed25519`, returns public key in OpenSSH authorized_keys format, PEM-encoded private key, SHA256 fingerprint, and key type.
- `ParsePrivateKey(pemData)` -- accepts PEM-encoded private keys (Ed25519, RSA, ECDSA) via `ssh.ParseRawPrivateKey()`, derives public key, fingerprint, and key type. Uses type-switch on the parsed key. Passphrase-protected keys are detected via `ssh.PassphraseMissingError` and rejected with a clear error. Small RSA keys and unsupported ECDSA curves are rejected.

Fingerprint format matches `ssh-keygen -l` output: `SHA256:<base64>`.

### Private Key Encryption

`internal/sshkey/` (and later generalized into `internal/secrets/`) implements AES-256-GCM encryption:
- `EncryptPrivateKey(plaintext, secret)` -- derives a 256-bit key from SHA-256 of the secret, encrypts with AES-256-GCM, returns ciphertext as `[]byte` for the BYTEA column.
- `DecryptPrivateKey(ciphertext, secret)` -- reverses encryption, used by the worker for git operations.

Encryption is performed in Go (not pgcrypto) for more control and to avoid passing secrets in SQL.

### SSH Key Service

`internal/sshkey/service.go` provides:
- `NewService(encryptionSecret)` -- validates secret length, derives AES key
- `Create()` -- generates Ed25519 key pair and encrypts private key
- `CreateFromPrivateKey(pemData)` -- parses uploaded PEM, derives public key/fingerprint/type, encrypts private key

DB operations (Get, List, Retire, ListProjects) are handled directly in handlers via sqlc queries.

### Endpoints

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| `POST` | `/v1/ssh-keys` | user | Create key (generate or upload PEM) |
| `GET` | `/v1/ssh-keys` | user | List caller's keys |
| `GET` | `/v1/ssh-keys/{id}` | user | Get key detail (public key + fingerprint) |
| `GET` | `/v1/ssh-keys/{id}/projects` | user | List projects assigned to key |
| `POST` | `/v1/ssh-keys/{id}/retire` | user | Mark key inactive |

All SSH key endpoints are user-scoped: queries filter by `created_by = current user`. Attempting to access another user's key returns 404.

### Create Flow

`POST /v1/ssh-keys` with `{"name": "..."}` generates a new Ed25519 key pair. With `{"name": "...", "private_key": "-----BEGIN ..."}` uploads and parses an existing PEM private key. Response (201) returns `id`, `name`, `public_key`, `fingerprint`, `key_type`, `is_active`, `created_at`, `rotated_at`. Private key is never returned in any API response.

Name validation: non-empty, max 100 chars. Duplicate fingerprints rejected with 409.

### Retire Flow

`POST /v1/ssh-keys/{id}/retire` checks for active project assignments via `CountActiveAssignmentsByKey`. If assignments exist, returns 409 with count. If none, marks `is_active = FALSE` and sets `rotated_at = NOW()`.

### Project Assignment

Project-level SSH key operations (assign, reassign, generate-and-assign, remove) are handled by the project handler (see ticket 04). A single SSH key can be assigned to multiple projects. All project members can view the assigned public key; only owner/admin can modify assignments.

### Security

- Private key never appears in any response body or log
- Private key encrypted (AES-256-GCM) before database storage
- Encryption secret (`SSH_KEY_ENCRYPTION_SECRET`) required at startup, never logged
- `created_by` column serves as both audit trail and ownership filter

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/sshkey/keygen.go` | Ed25519 generation, PEM parsing, fingerprint derivation |
| `internal/sshkey/keygen_test.go` | 19 unit tests for keygen, parse, encrypt/decrypt |
| `internal/sshkey/service.go` | Create, CreateFromPrivateKey with encryption |
| `internal/handler/sshkey.go` | HandleCreate, HandleList, HandleGet, HandleListProjects, HandleRetire |
| `internal/secrets/` | AES-256-GCM encrypt/decrypt (generalized from sshkey) |
| `internal/app/routes.go` | SSH key routes under RequireUser group |
| `datastore/postgres/queries/ssh_keys.sql` | CreateSSHKey, GetSSHKey, ListSSHKeys, RetireSSHKey, ListProjectsBySSHKey, CountActiveAssignmentsByKey |
| `tests/integration/sshkey_test.go` | 16+ integration tests for full CRUD lifecycle |

## Acceptance Criteria
- [x] `POST /v1/ssh-keys` generates a real Ed25519 key pair
- [x] `POST /v1/ssh-keys` with `private_key` PEM uploads and parses the key (Ed25519, RSA, ECDSA)
- [x] Uploaded passphrase-protected keys rejected with 400
- [x] Duplicate fingerprints rejected with 409
- [x] Public key is in valid OpenSSH format
- [x] Private key is encrypted (AES-256-GCM) before database storage
- [x] `GET /v1/ssh-keys` returns list of keys (no private key data)
- [x] `GET /v1/ssh-keys/{id}` returns single key detail
- [x] `GET /v1/ssh-keys/{id}/projects` returns projects assigned to the key
- [x] `POST /v1/ssh-keys/{id}/retire` fails with 409 if key has active assignments
- [x] `POST /v1/ssh-keys/{id}/retire` succeeds and sets is_active=false, rotated_at=now
- [x] Private key never appears in any response body or log
- [x] All queries filter by created_by -- returns 404 for keys owned by other users
- [x] Name validation: non-empty, max 100 chars
- [x] 19 unit tests and 16+ integration tests pass

# Phase 2: External Authentication

## Goal

Add secure access control for remote and multi-user deployments without turning the core backend into an identity system.

## Current State

Phase 1 ships with **GitHub OAuth** via better-auth:

- GitHub is the sole OAuth provider for backoffice login
- better-auth manages sessions, token lifecycle, and user records
- User identity is derived from the GitHub profile (name, email, avatar)
- Session cookies are httpOnly and managed by better-auth middleware
- MCP API key authorization is separate and unchanged

This provides a solid authentication baseline for single-provider deployments. The scope below covers extending beyond GitHub.

## Remaining Scope

- Additional OIDC/OAuth2 providers beyond GitHub (Google, GitLab, etc.)
- Reverse proxy + forward auth pattern for enterprise gateways
- Enterprise SSO/SAML integration
- IdP group-to-role mapping (e.g. IdP group → `platform_admin`)
- Support for multiple simultaneous identity providers

## Scope Boundaries

- Applies to backoffice and backoffice-oriented backend routes
- Does not replace MCP API key authorization
- better-auth remains the session layer regardless of upstream identity provider

## Design Principles

- Prefer external identity providers over built-in password auth
- Keep authentication and authorization concerns separated:
  - Authentication (who are you?): external IdP / auth gateway / better-auth
  - Authorization (what can you access?): backend project/API key rules
- Avoid storing passwords in PostgreSQL for production deployments
- Make local development still possible without mandatory cloud identity dependencies

## Deployment Patterns

### Pattern A (Preferred): Reverse Proxy + Forward Auth

1. User accesses backoffice through reverse proxy
2. Proxy enforces login with OIDC provider (OAuth2 Authorization Code + PKCE)
3. Proxy forwards authenticated requests to backoffice/backend
4. Proxy injects trusted identity headers (for example `X-User-Id`, `X-User-Email`, `X-User-Groups`)
5. Backend validates trusted proxy source and consumes identity headers

Why preferred:

- Centralized auth policy
- Easy MFA/SSO integration
- Minimal auth complexity inside backend

### Pattern B: Direct JWT Validation in Backend

1. Client gets OIDC access token
2. Client sends bearer token to backend
3. Backend validates issuer, audience, signature, expiry, and claims

Use when:

- No reverse proxy identity layer is available
- Services are accessed directly

### Pattern C (Current): better-auth Direct OAuth

1. Backoffice redirects to GitHub (or future provider) via better-auth
2. better-auth handles the OAuth callback and creates a session
3. Session cookie is used for subsequent requests

This is the current Phase 1 pattern. Extending better-auth to support additional OAuth providers is the simplest path for adding Google, GitLab, etc.

## Backend Requirements

- Keep API key checks for MCP query routes exactly as-is
- better-auth session middleware handles backoffice routes (current behavior)
- For reverse proxy deployments, add middleware that accepts trusted identity headers from configured upstream proxy
- For direct JWT deployments, add middleware that validates OIDC JWT claims
- Define clear trust boundaries:
  - reject identity headers from untrusted direct clients
  - require TLS between proxy and backend in non-local deployments
- Emit user identity fields in audit logs (without sensitive token values)

## Data Model Guidance

better-auth already manages the `user`, `session`, and `account` tables. For external-auth deployments with reverse proxy or direct JWT:

- Optional: maintain lightweight `external_identities` table for audit and display mapping:
  - `issuer`
  - `subject`
  - `email`
  - `display_name`
  - `last_seen_at`

For additional OAuth providers via better-auth, the existing `account` table already supports multiple providers per user (GitHub + Google, etc.).

## Backoffice Requirements

- Handle 401/403 responses with a clear "authentication required" state
- better-auth manages session cookies (current behavior)
- Provide logout action that redirects to IdP logout when configured
- Support multiple login options on the login page when multiple providers are configured

## Rollout Plan

1. Phase 1 baseline: GitHub OAuth via better-auth (shipped)
2. Add additional OAuth providers to better-auth config (Google, GitLab)
3. Add optional reverse proxy + forward auth mode behind config flags
4. Add staging deployment with reverse proxy and OIDC integration
5. Validate:
   - login flow with multiple providers
   - logout flow
   - header trust boundary (reverse proxy mode)
   - audit logging
6. Make external auth the recommended mode for any non-local deployment

## Non-Goals

- Building a full internal IAM product
- Implementing enterprise RBAC matrices in Phase 2
- Replacing API keys for MCP agents

## Open Questions

- Which external providers should be first-class examples (Authentik, Keycloak, Okta)?
- Should group claims map to backoffice permissions in Phase 2 or Phase 3?
- Do we need per-project backoffice authorization immediately, or only authenticated admin access first?
- Should better-auth be configured with multiple OAuth providers simultaneously, or one at a time?

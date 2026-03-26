# Phase 2 — Project Invitation System

## Overview

Project invitations allow owners and admins to invite users to a project by GitHub identity (email or name). Invitations are token-based and resolve into `project_members` entries upon acceptance.

Phase 1 uses direct member assignment via `project_members` instead.

## Database Schema

```sql
CREATE TABLE project_invitations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  invited_email CITEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
  invited_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'declined', 'expired', 'cancelled')),
  token TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  resolved_at TIMESTAMPTZ
);

CREATE INDEX idx_pi_project ON project_invitations(project_id);
CREATE INDEX idx_pi_email_status ON project_invitations(invited_email, status);
CREATE UNIQUE INDEX idx_pi_active_unique
  ON project_invitations(project_id, invited_email)
  WHERE status = 'pending';
```

Notes:

- `invited_email` matches the email from the user's better-auth identity (sourced from GitHub OAuth profile)
- When a user logs in via GitHub OAuth, their email is resolved from their better-auth user record
- If additional OAuth providers are added later, the email remains the common matching key

## API Endpoints

- `GET /v1/projects/{id}/invitations` — list pending invitations (`admin+`)
- `POST /v1/projects/{id}/invitations` — create invitation link (`admin+`; only owners may invite owners)
- `DELETE /v1/projects/{id}/invitations/{inv_id}` — cancel invitation (`admin+`)
- `POST /v1/invitations/{token}/accept` — accept invite (logged-in user's email must match invited email)
- `POST /v1/invitations/{token}/decline` — decline invite (logged-in user's email must match invited email)

## Flow

1. Admin/owner creates an invitation for an email address with a role
2. Backend generates a unique token and returns a shareable link
3. Invited user opens the link and accepts (or declines) — they must be logged in via GitHub OAuth (or a future provider) and their email must match the invitation
4. On accept: backend creates a `project_members` row with the specified role and marks invitation as accepted
5. On decline: backend marks invitation as declined

## Role Constraints

- Only owners can create invitations with `role = 'owner'`
- Admins can invite up to `admin` level
- Members cannot create invitations
- One active (pending) invitation per project+email at a time (enforced by partial unique index)

## Backoffice UI

- Invitation management panel within project detail view
- Show pending invitations with expiry countdown
- Copy-to-clipboard for invitation links
- Cancel button for pending invitations

## Helper View

```sql
CREATE VIEW user_project_access AS
SELECT pm.user_id, pm.project_id, pm.role, u.name, u.email
FROM project_members pm
JOIN users u ON u.id = pm.user_id
WHERE u.is_active = TRUE;
```

This view simplifies access resolution queries across the application.

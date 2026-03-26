-- Seed the default admin user and grant platform_admin role.
--
-- Creates the built-in administrator account and assigns it the
-- platform_admin role so the platform can be managed immediately
-- after deployment. Idempotent via WHERE NOT EXISTS guards that
-- check both username and email before inserting.
--
-- Safety: The role grant uses a CTE that only matches the user row
-- if this migration actually inserted it (via RETURNING), so a
-- pre-existing 'admin' user will NOT be promoted.

-- 1. Create the admin user, returning the id only if newly inserted
WITH new_admin AS (
  INSERT INTO users (username, email, display_name, is_active)
  SELECT 'admin', 'admin@local.local', 'Administrator', TRUE
  WHERE NOT EXISTS (SELECT 1 FROM users WHERE username = 'admin')
    AND NOT EXISTS (SELECT 1 FROM users WHERE email = 'admin@local.local')
  RETURNING id
)
-- 2. Grant platform_admin role only to the newly created user
INSERT INTO user_platform_roles (user_id, role)
SELECT id, 'platform_admin'
FROM new_admin
ON CONFLICT (user_id, role) DO NOTHING;

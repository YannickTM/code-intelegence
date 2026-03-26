-- Revoke the seeded platform admin role and remove the admin user.
--
-- Safety: Only deletes the user if it matches the exact seeded values
-- (email = 'admin@local.local'). A pre-existing 'admin' user with a
-- different email is left untouched.

DELETE FROM user_platform_roles
WHERE role = 'platform_admin'
  AND user_id = (SELECT id FROM users WHERE username = 'admin' AND email = 'admin@local.local');

DELETE FROM users
WHERE username = 'admin'
  AND email = 'admin@local.local';

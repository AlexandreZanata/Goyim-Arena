-- Administrative assignment queries for the PostgreSQL platform adapter.
--
-- Assignments key on account identifiers only: email, frontend flags,
-- payment state and popularity never enter this surface, so authorization
-- can never be influenced by who pays or who is popular (P13-T02).

-- name: GetAdminRoleByAccount :one
SELECT account_id, role, granted_by, granted_at, revoked_at
FROM app.admin_roles
WHERE account_id = $1;

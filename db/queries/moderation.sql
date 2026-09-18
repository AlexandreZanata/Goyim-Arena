-- Administrative assignment queries for the PostgreSQL platform adapter.
--
-- Assignments key on account identifiers only: email, frontend flags,
-- payment state and popularity never enter this surface, so authorization
-- can never be influenced by who pays or who is popular (P13-T02).

-- name: GetAdminRoleByAccount :one
SELECT account_id, role, granted_by, granted_at, revoked_at
FROM app.admin_roles
WHERE account_id = $1;

-- Structured report queries for the PostgreSQL platform adapter (P13-T03).
--
-- Reports are restricted evidence: inserts carry reporter, target, reason
-- and bounded context; reads serve deduplication and the rate signal only.
-- No query mutates a target: volume never removes content automatically.

-- name: CreateModerationReport :one
INSERT INTO app.moderation_reports (reporter_id, target_type, target_arena_id, target_argument_id, target_account_id, reason, context)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, reporter_id, target_type, target_arena_id, target_argument_id, target_account_id, reason, context, created_at;

-- name: GetDuplicateModerationReport :one
SELECT id, reporter_id, target_type, target_arena_id, target_argument_id, target_account_id, reason, context, created_at
FROM app.moderation_reports
WHERE reporter_id = $1
  AND target_type = $2
  AND COALESCE(target_arena_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE($3, '00000000-0000-0000-0000-000000000000'::uuid)
  AND COALESCE(target_argument_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE($4, '00000000-0000-0000-0000-000000000000'::uuid)
  AND COALESCE(target_account_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE($5, '00000000-0000-0000-0000-000000000000'::uuid)
  AND reason = $6
  AND created_at >= $7
ORDER BY created_at ASC, id ASC
LIMIT 1;

-- name: CountRecentModerationReportsByReporter :one
SELECT count(*)::bigint AS recent_reports
FROM app.moderation_reports
WHERE reporter_id = $1
  AND created_at >= $2;

-- Target resolution for report filing (P13-T03). Each read returns the
-- owner and the lifecycle needed to distinguish unknown targets from
-- removed ones. Drafts stay reportable (self-report path); removed and
-- withdrawn targets deny.

-- name: GetArenaModerationTarget :one
SELECT id, creator_id, status
FROM app.arenas
WHERE id = $1;

-- name: GetArgumentModerationTarget :one
SELECT id, author_id, status
FROM app.arguments
WHERE id = $1;

-- name: GetAccountModerationTarget :one
SELECT id, status
FROM app.accounts
WHERE id = $1;

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

-- Sanction effects applied in the same transaction as the audit event
-- (P13-T05). Each statement is conditional: zero affected rows means the
-- target left the sanctionable state concurrently, and the adapter rolls
-- the whole decision back instead of recording a phantom sanction.

-- name: CloseArenaForModeration :one
UPDATE app.arenas
SET status = 'closed',
    version = version + 1
WHERE id = $1 AND status = 'published'
RETURNING id, status, version;

-- name: RemoveArgumentForModeration :one
UPDATE app.arguments
SET status = 'removed',
    updated_at = now()
WHERE id = $1 AND status IN ('published', 'withdrawn')
RETURNING id, status;

-- name: InvalidateArgumentAttributions :execrows
UPDATE app.persuasion_attributions
SET status = 'invalid',
    invalidated_at = now(),
    moderation_reason = $2,
    moderated_by = $3,
    moderated_at = now()
WHERE argument_id = $1 AND status = 'valid';

-- name: SuspendAccountForModeration :one
UPDATE app.accounts
SET status = 'suspended',
    updated_at = now()
WHERE id = $1 AND status = 'active'
RETURNING id, status;

-- Appeal reads and lifecycle (P13-T06). Exactly one appeal contests one
-- action: the UNIQUE constraint refuses the second contest, and the
-- adapter maps the violation to the duplicate sentinel instead of
-- surfacing storage detail.

-- name: GetModerationActionForAppeal :one
SELECT a.id, a.case_id, a.action_type, a.actor_id, a.rule_applied, a.created_at,
    c.target_type, c.target_arena_id, c.target_argument_id, c.target_account_id,
    COALESCE(ar.creator_id, ag.author_id, ac.id) AS target_owner_id
FROM app.moderation_actions a
JOIN app.moderation_cases c ON c.id = a.case_id
LEFT JOIN app.arenas ar ON ar.id = c.target_arena_id
LEFT JOIN app.arguments ag ON ag.id = c.target_argument_id
LEFT JOIN app.accounts ac ON ac.id = c.target_account_id
WHERE a.id = $1;

-- name: GetModerationAppealByAction :one
SELECT id, action_id, appellant_id, status, reviewer_id, decision_reason, created_at, decided_at
FROM app.moderation_appeals
WHERE action_id = $1;

-- name: GetModerationAppealByID :one
SELECT id, action_id, appellant_id, status, reviewer_id, decision_reason, created_at, decided_at
FROM app.moderation_appeals
WHERE id = $1;

-- name: InsertModerationAppeal :one
INSERT INTO app.moderation_appeals (action_id, appellant_id, context)
VALUES ($1, $2, $3)
RETURNING id, action_id, appellant_id, status, reviewer_id, decision_reason, created_at, decided_at;

-- name: ClaimModerationAppeal :one
UPDATE app.moderation_appeals
SET status = 'under_review'
WHERE id = $1 AND status = 'open'
RETURNING id, action_id, appellant_id, status, reviewer_id, decision_reason, created_at, decided_at;

-- name: DecideModerationAppeal :one
UPDATE app.moderation_appeals
SET status = $2, reviewer_id = $3, decision_reason = $4, decided_at = $5
WHERE id = $1 AND status = 'under_review'
RETURNING id, action_id, appellant_id, status, reviewer_id, decision_reason, created_at, decided_at;

-- Reversal effects restoring sanctioned projections from the original
-- action record (P13-T06). Each statement is conditional: zero affected
-- rows aborts the whole outcome instead of recording a phantom restore.
-- The original action row is never edited or deleted.

-- name: ReopenArenaForAppeal :one
UPDATE app.arenas
SET status = 'published',
    version = version + 1
WHERE id = $1 AND status = 'closed'
RETURNING id, status, version;

-- name: RestoreArgumentForAppeal :one
UPDATE app.arguments
SET status = 'published',
    updated_at = now()
WHERE id = $1 AND status = 'removed'
RETURNING id, status;

-- name: RestoreArgumentAttributions :execrows
UPDATE app.persuasion_attributions
SET status = 'valid',
    moderation_reason = $2,
    moderated_by = $3,
    moderated_at = now()
WHERE argument_id = $1 AND status = 'invalid';

-- name: ReactivateAccountForAppeal :one
UPDATE app.accounts
SET status = 'active',
    updated_at = now()
WHERE id = $1 AND status = 'suspended'
RETURNING id, status;

-- Triage queue reads (P13-T07). One keyset-paginated page of case routing,
-- newest first: target, lifecycle, priority, claim holder and instants.
-- Restricted evidence (reporter context, justifications, appeal contexts)
-- is never selected here. NULL after_* parameters select the first page;
-- the (created_at, id) tuple comparison never duplicates or skips rows. An
-- empty status filter lists every lifecycle.

-- name: ListModerationCasesPage :many
SELECT id, target_type, target_arena_id, target_argument_id, target_account_id,
    status, priority, created_at, claimed_by
FROM app.moderation_cases
WHERE (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
  AND (
      sqlc.arg(after_created_at)::timestamptz IS NULL
      OR (created_at, id) < (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- Session freshness for step-up evaluation (P13-T07). The age counts from
-- session creation as of the caller's instant; unknown sessions deny
-- distinctly instead of being treated as fresh.

-- name: GetSessionCreatedAt :one
SELECT created_at
FROM app.sessions
WHERE id = $1;

-- Review claim and decision queries (P13-T04). Claims serialize on the
-- row: one conditional update moves open (or expired-lease) cases under
-- the claimant with a fresh lease. Decisions record one immutable action
-- and move the case to decided while clearing the claim, atomically in
-- the adapter transaction.

-- name: GetModerationCaseByID :one
SELECT c.id, c.target_type, c.target_arena_id, c.target_argument_id, c.target_account_id,
    c.status, c.claimed_by, c.lease_expires_at,
    COALESCE(ar.creator_id, ag.author_id, ac.id) AS target_owner_id
FROM app.moderation_cases c
LEFT JOIN app.arenas ar ON ar.id = c.target_arena_id
LEFT JOIN app.arguments ag ON ag.id = c.target_argument_id
LEFT JOIN app.accounts ac ON ac.id = c.target_account_id
WHERE c.id = $1;

-- name: ClaimModerationCase :one
UPDATE app.moderation_cases
SET status = 'under_review', claimed_by = $2, claimed_at = $3, lease_expires_at = $4, updated_at = now()
WHERE id = $1
  AND (
    (status = 'open' AND claimed_by IS NULL)
    OR (status = 'under_review' AND lease_expires_at IS NOT NULL AND lease_expires_at <= $5)
  )
RETURNING id, target_type, target_arena_id, target_argument_id, target_account_id,
    status, claimed_by, lease_expires_at;

-- name: DecideModerationCase :one
UPDATE app.moderation_cases
SET status = 'decided', claimed_by = NULL, claimed_at = NULL, lease_expires_at = NULL, updated_at = now()
WHERE id = $1
  AND status = 'under_review'
  AND claimed_by = $2
  AND lease_expires_at IS NOT NULL AND lease_expires_at > $3
RETURNING id, target_type, target_arena_id, target_argument_id, target_account_id, status;

-- name: CreateModerationAction :one
INSERT INTO app.moderation_actions (case_id, action_type, actor_id, rule_applied, justification, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, case_id, action_type, actor_id, rule_applied, justification, expires_at, created_at;

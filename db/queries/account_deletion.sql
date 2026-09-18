-- Account deletion state machine (P14-T06, docs/PRIVACY.md §4/§5, BR §10).
-- The record statements only ever touch the deletion table and the account
-- row; the execution statements purge private rows and anonymize public
-- authorship while the billing, ledger, pass, moderation and audit schemas
-- stay untouched.

-- CreateDeletionRequest creates a new active request. The partial unique
-- index resolves concurrent requests: the loser inserts nothing and re-reads
-- the winner, so the cooldown never restarts. A canceled record may be
-- followed by a fresh one; the terminal history is retained.
-- name: CreateDeletionRequest :one
INSERT INTO app.account_deletion_requests (account_id, status, requested_at, updated_at)
VALUES (
    sqlc.arg(account_id)::uuid,
    'requested',
    sqlc.arg(requested_at)::timestamptz,
    sqlc.arg(requested_at)::timestamptz
)
ON CONFLICT (account_id) WHERE status = 'requested' DO NOTHING
RETURNING id, account_id, status, requested_at, executed_at, canceled_at;

-- GetDeletionRequestForAccount loads the owner-scoped deletion state: the
-- active request when one exists, otherwise the most recent terminal record.
-- name: GetDeletionRequestForAccount :one
SELECT id, account_id, status, requested_at, executed_at, canceled_at
FROM app.account_deletion_requests
WHERE account_id = sqlc.arg(account_id)::uuid
ORDER BY (status = 'requested') DESC, requested_at DESC, id DESC
LIMIT 1;

-- ListDueDeletionRequests returns active requests whose cooldown elapsed,
-- oldest first, so the workflow can execute them.
-- name: ListDueDeletionRequests :many
SELECT id, account_id, requested_at
FROM app.account_deletion_requests
WHERE status = 'requested'
  AND requested_at <= sqlc.arg(due_before)::timestamptz
ORDER BY requested_at, id;

-- CancelDeletionRequest cancels the active request inside its window; the
-- status guard makes the transition once-only and terminal records
-- immutable.
-- name: CancelDeletionRequest :one
UPDATE app.account_deletion_requests
SET status = 'canceled',
    canceled_at = sqlc.arg(canceled_at)::timestamptz,
    cancel_reason = sqlc.arg(reason)::text,
    updated_at = sqlc.arg(canceled_at)::timestamptz
WHERE account_id = sqlc.arg(account_id)::uuid
  AND status = 'requested'
RETURNING id, account_id, status, requested_at, executed_at, canceled_at;

-- MarkDeletionExecuted records the terminal executed state on the active
-- request. The cooldown is enforced in the statement itself (defense in
-- depth): a request whose window has not elapsed can never execute early.
-- name: MarkDeletionExecuted :execrows
UPDATE app.account_deletion_requests
SET status = 'executed',
    executed_at = sqlc.arg(executed_at)::timestamptz,
    updated_at = sqlc.arg(executed_at)::timestamptz
WHERE account_id = sqlc.arg(account_id)::uuid
  AND status = 'requested'
  AND requested_at <= sqlc.arg(due_before)::timestamptz;

-- AnonymizeDeletedAccount replaces the account's private identity with the
-- opaque placeholder: the stable identifier survives for referential
-- integrity, the email stops being personal data, and the account moves to
-- the terminal deleted state.
-- name: AnonymizeDeletedAccount :execrows
UPDATE app.accounts
SET email = sqlc.arg(placeholder_email)::text,
    status = 'deleted',
    email_verified_at = NULL,
    updated_at = sqlc.arg(executed_at)::timestamptz
WHERE id = sqlc.arg(account_id)::uuid;

-- DeleteDeletedAccountProfile removes the profile so public authorship
-- becomes unresolvable while published content keeps its stable author id.
-- name: DeleteDeletedAccountProfile :execrows
DELETE FROM app.profiles
WHERE account_id = sqlc.arg(account_id)::uuid;

-- DeleteDeletedAccountUsernameHistory removes the username audit trail.
-- name: DeleteDeletedAccountUsernameHistory :execrows
DELETE FROM app.username_history
WHERE account_id = sqlc.arg(account_id)::uuid;

-- DeleteDeletedAccountCredentials removes the password credential, so no
-- stored secret survives the deletion.
-- name: DeleteDeletedAccountCredentials :execrows
DELETE FROM app.password_credentials
WHERE account_id = sqlc.arg(account_id)::uuid;

-- DeleteDeletedAccountCommunicationPreferences removes the explicit opt-ins.
-- name: DeleteDeletedAccountCommunicationPreferences :execrows
DELETE FROM app.communication_preferences
WHERE account_id = sqlc.arg(account_id)::uuid;

-- DeleteDeletedAccountCommunicationPreferenceHistory removes the preference
-- audit trail.
-- name: DeleteDeletedAccountCommunicationPreferenceHistory :execrows
DELETE FROM app.communication_preference_history
WHERE account_id = sqlc.arg(account_id)::uuid;

-- DeleteDeletedAccountDraftRelations removes linkage rows that reference the
-- account's unpublished drafts: a relation is only meaningful while both
-- Arenas exist, and drafts do not survive the deletion.
-- name: DeleteDeletedAccountDraftRelations :execrows
DELETE FROM app.arena_relations
WHERE arena_id IN (
        SELECT id FROM app.arenas WHERE creator_id = sqlc.arg(account_id)::uuid AND status = 'draft'
    )
   OR related_arena_id IN (
        SELECT id FROM app.arenas WHERE creator_id = sqlc.arg(account_id)::uuid AND status = 'draft'
    );

-- DeleteDeletedAccountDrafts removes the account's unpublished drafts:
-- private content with no retention obligation. Published Arenas are
-- untouched and keep their stable author id.
-- name: DeleteDeletedAccountDrafts :execrows
DELETE FROM app.arenas
WHERE creator_id = sqlc.arg(account_id)::uuid
  AND status = 'draft';

-- DeleteDeletedAccountVerificationTokens removes every email verification
-- token of the account.
-- name: DeleteDeletedAccountVerificationTokens :execrows
DELETE FROM app.email_verification_tokens
WHERE account_id = sqlc.arg(account_id)::uuid;

-- DeleteDeletedAccountResetTokens removes every password reset token of the
-- account.
-- name: DeleteDeletedAccountResetTokens :execrows
DELETE FROM app.password_reset_tokens
WHERE account_id = sqlc.arg(account_id)::uuid;

-- DeleteDeletedAccountSessions removes every session, so no credential of
-- the deleted account can ever authenticate again.
-- name: DeleteDeletedAccountSessions :execrows
DELETE FROM app.sessions
WHERE account_id = sqlc.arg(account_id)::uuid;

-- RevokeDeletedAccountAdminRoles revokes any active administrative role of
-- the deleted account. The assignment row is retained as restricted audit
-- evidence; marking it revoked removes the latent privilege.
-- name: RevokeDeletedAccountAdminRoles :execrows
UPDATE app.admin_roles
SET revoked_at = sqlc.arg(revoked_at)::timestamptz
WHERE account_id = sqlc.arg(account_id)::uuid
  AND revoked_at IS NULL;

-- PurgeDeletedAccountExports expires the account's personal exports and
-- drops their documents: the private data copy never outlives the account.
-- The records survive as retention evidence, matching the export invariant.
-- name: PurgeDeletedAccountExports :execrows
UPDATE app.data_exports
SET status = 'expired',
    document = NULL,
    document_sha256 = NULL,
    updated_at = sqlc.arg(purged_at)::timestamptz
WHERE account_id = sqlc.arg(account_id)::uuid
  AND status IN ('requested', 'ready');

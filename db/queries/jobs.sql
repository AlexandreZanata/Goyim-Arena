-- Durable job queue (P15-T01). Enqueue is idempotent by caller-chosen key;
-- claim is a single statement that locks one due row with SKIP LOCKED, so
-- many workers never claim the same job; complete and fail are lease-guarded;
-- release recovers leases whose holder died without reporting back.

-- name: EnqueueJob :one
INSERT INTO app.jobs (
    type, version, parameters, idempotency_key, state, available_at, max_attempts, created_at, updated_at
)
VALUES (
    sqlc.arg(type)::text,
    sqlc.arg(version)::integer,
    sqlc.arg(parameters),
    sqlc.narg(idempotency_key)::text,
    'queued',
    sqlc.arg(available_at)::timestamptz,
    sqlc.arg(max_attempts)::integer,
    sqlc.arg(created_at)::timestamptz,
    sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING id, type, version, parameters, idempotency_key, state, available_at,
          attempts, max_attempts, lease_owner, leased_until,
          last_error_code, last_error_detail, created_at, updated_at;

-- name: GetJob :one
SELECT id, type, version, parameters, idempotency_key, state, available_at,
       attempts, max_attempts, lease_owner, leased_until,
       last_error_code, last_error_detail, created_at, updated_at
FROM app.jobs
WHERE id = sqlc.arg(id)::uuid;

-- name: GetJobByIdempotencyKey :one
SELECT id, type, version, parameters, idempotency_key, state, available_at,
       attempts, max_attempts, lease_owner, leased_until,
       last_error_code, last_error_detail, created_at, updated_at
FROM app.jobs
WHERE idempotency_key = sqlc.arg(idempotency_key)::text;

-- name: LeaseJob :one
-- Claim exactly one job that is either queued and due or leased and expired.
-- FOR UPDATE SKIP LOCKED lets concurrent workers take different rows without
-- queueing behind each other and without ever taking the same row twice.
WITH claimable AS (
    SELECT id
    FROM app.jobs
    WHERE (state = 'queued' AND available_at <= sqlc.arg(now)::timestamptz)
       OR (state = 'leased' AND leased_until <= sqlc.arg(now)::timestamptz)
    ORDER BY available_at ASC, id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE app.jobs AS j
SET state = 'leased',
    lease_owner = sqlc.arg(lease_owner)::text,
    leased_until = sqlc.arg(leased_until)::timestamptz,
    attempts = j.attempts + 1,
    updated_at = sqlc.arg(now)::timestamptz
FROM claimable
WHERE j.id = claimable.id
RETURNING j.id, j.type, j.version, j.parameters, j.idempotency_key, j.state,
          j.available_at, j.attempts, j.max_attempts, j.lease_owner,
          j.leased_until, j.last_error_code, j.last_error_detail,
          j.created_at, j.updated_at;

-- name: CompleteJob :one
-- A lease holder finishes its job. A stale holder (lease already recovered
-- and re-leased) matches nothing and cannot overwrite the new outcome.
UPDATE app.jobs AS j
SET state = 'succeeded',
    lease_owner = NULL,
    leased_until = NULL,
    updated_at = sqlc.arg(now)::timestamptz
WHERE j.id = sqlc.arg(id)::uuid
  AND j.state = 'leased'
  AND j.lease_owner = sqlc.arg(lease_owner)::text
RETURNING j.id, j.type, j.version, j.parameters, j.idempotency_key, j.state,
          j.available_at, j.attempts, j.max_attempts, j.lease_owner,
          j.leased_until, j.last_error_code, j.last_error_detail,
          j.created_at, j.updated_at;

-- name: FailJob :one
-- Record a redacted failure. The attempt was consumed at lease time, so the
-- budget check here decides between requeueing at retry_at and the terminal
-- dead state: a poison job leaves the queue instead of blocking it.
UPDATE app.jobs AS j
SET state = CASE WHEN j.attempts >= j.max_attempts THEN 'dead' ELSE 'queued' END,
    available_at = CASE
        WHEN j.attempts >= j.max_attempts THEN j.available_at
        ELSE sqlc.arg(retry_at)::timestamptz
    END,
    lease_owner = NULL,
    leased_until = NULL,
    last_error_code = sqlc.arg(error_code)::text,
    last_error_detail = sqlc.arg(error_detail)::text,
    updated_at = sqlc.arg(now)::timestamptz
WHERE j.id = sqlc.arg(id)::uuid
  AND j.state = 'leased'
  AND j.lease_owner = sqlc.arg(lease_owner)::text
RETURNING j.id, j.type, j.version, j.parameters, j.idempotency_key, j.state,
          j.available_at, j.attempts, j.max_attempts, j.lease_owner,
          j.leased_until, j.last_error_code, j.last_error_detail,
          j.created_at, j.updated_at;

-- name: ReleaseExpiredLeases :execrows
-- Recovery sweep: return every job whose lease expired to the queue. The
-- count tells the scheduler how much work was reclaimed.
UPDATE app.jobs
SET state = 'queued',
    lease_owner = NULL,
    leased_until = NULL,
    updated_at = sqlc.arg(now)::timestamptz
WHERE state = 'leased'
  AND leased_until <= sqlc.arg(now)::timestamptz;

-- name: CountJobsByState :many
SELECT state, count(*)::bigint AS total
FROM app.jobs
GROUP BY state
ORDER BY state;

-- name: GetQueueHealth :one
-- Operational health of the queue (P15-T06). One pass over the table: the
-- counts an operator reads and the instants the lag is measured from. It
-- selects no payload column, so the surface cannot leak one.
SELECT
    count(*) FILTER (WHERE state = 'queued')::bigint AS queued,
    count(*) FILTER (WHERE state = 'leased')::bigint AS leased,
    count(*) FILTER (WHERE state = 'succeeded')::bigint AS succeeded,
    count(*) FILTER (WHERE state = 'dead')::bigint AS dead,
    count(*) FILTER (
        WHERE state = 'queued' AND available_at <= sqlc.arg(now)::timestamptz
    )::bigint AS due_now,
    min(available_at) FILTER (
        WHERE state = 'queued' AND available_at <= sqlc.arg(now)::timestamptz
    )::timestamptz AS oldest_due_at,
    min(updated_at) FILTER (WHERE state = 'dead')::timestamptz AS oldest_dead_at,
    max(updated_at) FILTER (WHERE state = 'dead')::timestamptz AS newest_dead_at
FROM app.jobs;

-- name: ListDeadJobs :many
-- The dead rows an operator can act on, oldest first. The payload column is
-- deliberately absent from the projection: what the job carried is not part
-- of the operational surface.
SELECT id, type, version, state, attempts, max_attempts,
       last_error_code, last_error_detail, available_at, updated_at
FROM app.jobs
WHERE state = 'dead'
ORDER BY updated_at ASC, id ASC
LIMIT sqlc.arg(row_limit)::integer;

-- name: RetryDeadJob :one
-- An authorized operator returns one dead row to the queue (P15-T06). The
-- attempt budget is renewed because the operator is asserting the cause is
-- fixed: without that, a job whose budget is spent would be leased and
-- immediately recorded dead again. Only lifecycle columns change, which is
-- what the provenance trigger allows.
UPDATE app.jobs AS j
SET state = 'queued',
    attempts = 0,
    available_at = sqlc.arg(now)::timestamptz,
    lease_owner = NULL,
    leased_until = NULL,
    last_error_code = NULL,
    last_error_detail = NULL,
    updated_at = sqlc.arg(now)::timestamptz
WHERE j.id = sqlc.arg(id)::uuid
  AND j.state = 'dead'
RETURNING j.id, j.type, j.version, j.state, j.attempts, j.max_attempts,
          j.available_at, j.updated_at;

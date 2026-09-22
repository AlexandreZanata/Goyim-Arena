#!/usr/bin/env bash
#
# Verification of the migration lifecycle (P20-T03).
#
# Why the gate is a script and not only a Go command: the audit needs a
# PostgreSQL 18.4 it may create and drop databases in, and the phase forbids
# measuring SQLite — a gate that audited another engine's lock semantics would
# be answering a different question. So the script owns the engine: it pins the
# image by digest, waits for it, hands the DSN to `tools/migrationaudit` (which
# provisions and drops every database it uses) and tears the container down on
# every exit path.
#
# What one run owns, and therefore removes: one PostgreSQL container and, inside
# it, the databases named `arena_migaudit_<pid>_*` that the audit creates. The
# report is written into the working tree when ARENA_MIGRATION_AUDIT_REPORT is
# set, because the report is the deliverable and a report the gate can throw
# away is not evidence.
#
# It is fail-closed: a missing command, a Docker daemon that is not reachable, a
# database that never becomes ready, a rule broken, an advisory uncovered or a
# missing report all abort with a non-zero status.
#
# Requirements: Docker with a reachable daemon, Go.
#
# Environment:
#   ARENA_MIGRATION_AUDIT_IMAGE   PostgreSQL to run against (default the pinned
#                                 postgres:18.4 digest)
#   ARENA_MIGRATION_AUDIT_REPORT  where to write the Markdown report (default:
#                                 none; the run prints its summary either way)
#   ARENA_MIGRATION_AUDIT_PORT    host port to publish (default: an ephemeral
#                                 one, so two runs never collide)
set -euo pipefail

TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$TOOL_DIR/../.." && pwd)"
cd "$ROOT"

POSTGRES_IMAGE="${ARENA_MIGRATION_AUDIT_IMAGE:-postgres:18.4@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636}"
DB_PASSWORD="arena-local-dev"
RUN_ID="$$"
CONTAINER="arena-migration-audit-db-${RUN_ID}"
REPORT="${ARENA_MIGRATION_AUDIT_REPORT:-}"
PUBLISH="${ARENA_MIGRATION_AUDIT_PORT:-}"

log() { printf 'migration-audit: %s\n' "$*" >&2; }
fail() { printf 'migration-audit: %s\n' "$*" >&2; exit 1; }

cleanup() {
	docker rm --force "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		fail "$1 is required and was not found in PATH"
	fi
}
require_command docker
require_command go

if ! docker info >/dev/null 2>&1; then
	fail "the Docker daemon is not reachable"
fi

# ---------------------------------------------------------------------------
# 1. A throwaway PostgreSQL 18.4, pinned by digest. The audit creates and drops
#    its own databases inside it; nothing of this container outlives the run.
# ---------------------------------------------------------------------------
log "starting $POSTGRES_IMAGE"
if [ -n "$PUBLISH" ]; then
	docker run --detach \
		--name "$CONTAINER" \
		--env POSTGRES_USER=arena \
		--env POSTGRES_PASSWORD="$DB_PASSWORD" \
		--env POSTGRES_DB=arena \
		--env POSTGRES_INITDB_ARGS="--encoding=UTF8 --locale=C" \
		--publish "127.0.0.1:${PUBLISH}:5432" \
		"$POSTGRES_IMAGE" >/dev/null
	HOST_PORT="$PUBLISH"
else
	docker run --detach \
		--name "$CONTAINER" \
		--env POSTGRES_USER=arena \
		--env POSTGRES_PASSWORD="$DB_PASSWORD" \
		--env POSTGRES_DB=arena \
		--env POSTGRES_INITDB_ARGS="--encoding=UTF8 --locale=C" \
		--publish 127.0.0.1::5432 \
		"$POSTGRES_IMAGE" >/dev/null
	published="$(docker port "$CONTAINER" 5432/tcp | head -1)"
	[ -n "$published" ] || fail "the database published no port"
	HOST_PORT="${published##*:}"
fi

for attempt in $(seq 1 60); do
	if docker exec "$CONTAINER" pg_isready --username arena --dbname arena >/dev/null 2>&1; then
		break
	fi
	if [ "$attempt" = 60 ]; then
		docker logs "$CONTAINER" >&2 || true
		fail "the database never became ready"
	fi
	sleep 1
done

server_version="$(docker exec "$CONTAINER" psql --username arena --dbname arena --tuples-only --no-align --command 'SHOW server_version')"
case "$server_version" in
18.*) ;;
*) fail "the container runs PostgreSQL $server_version, and the plan requires 18 (the schema uses features of 18)" ;;
esac
log "PostgreSQL $server_version is ready on 127.0.0.1:$HOST_PORT"

# The audit connects as the container's own superuser: it creates and drops the
# databases it measures, and the runtime role it judges is one the history
# creates. A throwaway password in a container that exists for this run is not a
# credential of anything.
DSN="postgres://arena:${DB_PASSWORD}@127.0.0.1:${HOST_PORT}/arena?sslmode=disable"

# ---------------------------------------------------------------------------
# 2. The exercise. Exit status is the gate: a finding refuses, an advisory is
#    printed and recorded in the report.
# ---------------------------------------------------------------------------
arguments=(-admin-dsn "$DSN" -root "$ROOT")
if [ -n "$REPORT" ]; then
	arguments+=(-report "$REPORT")
fi

log "running the migration lifecycle audit"
if ! go run ./tools/migrationaudit "${arguments[@]}" >/dev/null; then
	fail "the audit refused: the findings above name what a migration promotion would break"
fi

# ---------------------------------------------------------------------------
# 3. The report is the deliverable of the phase, so a run told to write one must
#    have written something with the measurements in it.
# ---------------------------------------------------------------------------
if [ -n "$REPORT" ]; then
	[ -s "$REPORT" ] || fail "the report $REPORT is empty"
	for section in "## 1. Banco vazio" "## 2. Degrau" "## 3. Snapshot e upgrade" "## 4. Falha simulada" "## 5. Dataset" "## 6. Regras" "## 7. Ledgers"; do
		grep -qF "$section" "$REPORT" || fail "the report $REPORT has no $section section"
	done
	log "report written to $REPORT"
fi

log "ok — every snapshot lands on the fresh database, the failure rolls back, and the schema holds the grants the ledger declares"

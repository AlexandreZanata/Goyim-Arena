#!/usr/bin/env bash
#
# Verification of the deployment pipeline (P19-T07).
#
# Why this gate is a script and not a file audit: a deployment is a sequence
# whose failure point is the release gate, and what matters is what the pipeline
# *did* — whether a release that never became healthy was taken back out,
# whether the previous digest came back serving, whether the state file stayed
# where it was, whether the database was left alone. `deploy/deploy.sh` can be
# read to see that a probe is called; only running it can show that the refusal
# happens, that the rollback restores the previous version, and that none of it
# moves the schema.
#
# What one run owns, and therefore tears down on every exit path: a throwaway
# registry that holds the releases by digest, three release stand-ins built into
# scratch images, a Compose project with its own volumes, a throwaway
# certificate, and the operator's environment files.
#
# The releases are two: the application, built from this repository, and the
# stand-in of `tools/deployaudit/stub`. The stand-in is what makes the refusal
# measurable — see the file it lives in for what it is claimed to be.
#
# It is fail-closed: a missing command, an artifact that is not pinned by
# digest, a mutable tag accepted, a promotion that does not become ready, a
# promote that leaves another digest running, a page that stops answering, a
# rollback that does not restore the previous version, a state file that
# advances after a failed release, a schema rollback the runner should not have,
# or a deployment that proceeds without a database all abort with a non-zero
# status.
#
# Requirements: Docker with a reachable daemon and the Compose plugin, Go, curl,
# openssl.
#
# Environment:
#   ARENA_IMAGE   tag of the application image to promote (default
#                 goyim-arena:verify). Every scenario promotes the digest of
#                 that image through the registry, which is what production
#                 does.
set -euo pipefail

TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$TOOL_DIR/../.." && pwd)"
cd "$ROOT"

IMAGE="${ARENA_IMAGE:-goyim-arena:verify}"
COMPOSE_FILE="compose.production.yaml"
RUN_ID="$$"
PROJECT="arena-deployaudit-${RUN_ID}"
REGISTRY="arena-deployaudit-registry-${RUN_ID}"
SITE="arena.verify.invalid"
DB_PASSWORD="verify-database-password"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/arena-deployaudit-XXXXXX")"
ENV_FILE="$WORK_DIR/.env.production"
STATE_FILE="$WORK_DIR/deployed.state"
RELEASES_DIR="$WORK_DIR/releases"
# One applied row per migration file. goose records the creation of the schema
# itself as version 0, which is why every count below is taken over the
# versions that came from a file.
EXPECTED_MIGRATIONS="$(find internal/platform/dbmigrate/migrations -maxdepth 1 -name '*.sql' | wc -l | tr -d '[:space:]')"
[[ -n "$EXPECTED_MIGRATIONS" && "$EXPECTED_MIGRATIONS" != "0" ]] ||
	printf 'deploy-verify: warning: no migration files were found to count\n' >&2

log() { printf 'deploy-verify: %s\n' "$*" >&2; }
fail() { printf 'deploy-verify: %s\n' "$*" >&2; exit 1; }

# What a process said, quoted under the assertion that judged it. A gate that
# reports only its own verdict makes the reader run the whole drill again to
# find out why.
indent() { sed 's/^/    /' "$1" >&2; }

cleanup() {
	compose down --volumes --remove-orphans >/dev/null 2>&1 || true
	docker rm --force "$REGISTRY" >/dev/null 2>&1 || true
	for mode in not-ready ready page-missing; do
		docker image rm "arena-deployaudit-standin-${RUN_ID}-${mode}" >/dev/null 2>&1 || true
	done
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		fail "$1 is required and was not found in PATH"
	fi
}
require_command docker
require_command go
require_command curl
require_command openssl

compose() {
	docker compose --project-name "$PROJECT" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

mkdir -p "$RELEASES_DIR"

# --------------------------------------------------------------------------
# 1. The releases, promoted by digest through a registry
# --------------------------------------------------------------------------
# A deployment promotes an artifact someone can fetch, so the drill produces
# digests the way the pipeline does — by pushing to a registry and reading back
# what the registry recorded. A tag never appears in the pipeline's arguments
# below except where a scenario asserts that it is refused.
if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
	fail "the application image $IMAGE does not exist; run 'make image-build' first"
fi

docker run --detach --name "$REGISTRY" --publish 127.0.0.1::5000 \
	registry:2@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373 >/dev/null
REGISTRY_PORT="$(docker port "$REGISTRY" 5000/tcp | head -1 | sed 's/.*://')"
[[ -n "$REGISTRY_PORT" ]] || fail "the throwaway registry did not publish a port"
REGISTRY_REPOSITORY="127.0.0.1:${REGISTRY_PORT}/goyim-arena"

# register_release tags a local image into the registry, pushes it and prints
# the digest reference the registry recorded. Everything else in this script
# references releases by that string.
register_release() {
	local local_ref="$1" name="$2" digest
	docker tag "$local_ref" "${REGISTRY_REPOSITORY}:${name}"
	docker push "${REGISTRY_REPOSITORY}:${name}" >/dev/null
	digest="$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "${REGISTRY_REPOSITORY}:${name}" |
		grep -F "${REGISTRY_REPOSITORY}@sha256:" | head -1)"
	[[ -n "$digest" ]] || fail "the pushed release ${name} has no digest"
	printf '%s\n' "$digest"
}

APPLICATION_IMAGE="$(register_release "$IMAGE" application)"

# The stand-ins: one source, three images, the mode baked in at build time. A
# mode that arrived in the environment file instead would ride into the
# rollback, and the rollback would then restore the release that is *supposed*
# to be healthy while its configuration still says otherwise.
CGO_ENABLED=0 go build -o "$RELEASES_DIR/stand-in" ./tools/deployaudit/stub ||
	fail "the release stand-in did not build"

build_stand_in() {
	local mode="$1"
	local tag="arena-deployaudit-standin-${RUN_ID}-${mode}"
	cat >"$RELEASES_DIR/Dockerfile.stand-in" <<EOF
# Built by tools/deployaudit/verify.sh. Static, non-root and minimal for the
# same reason the application image is: the stand-in is promoted by the real
# compose document, under its read-only filesystem and dropped capabilities.
FROM scratch
ENV STAND_IN_MODE=${mode}
COPY stand-in /arena
USER 65532:65532
ENTRYPOINT ["/arena"]
EOF
	docker build --quiet --tag "$tag" --file "$RELEASES_DIR/Dockerfile.stand-in" "$RELEASES_DIR" >/dev/null ||
		fail "the stand-in image for mode ${mode} did not build"
	register_release "$tag" "stand-in-${mode}"
}

STAND_IN_READY="$(build_stand_in ready)"
STAND_IN_NOT_READY="$(build_stand_in not-ready)"
STAND_IN_PAGE_MISSING="$(build_stand_in page-missing)"
log "the releases are registered by digest: the application and three stand-ins"

# --------------------------------------------------------------------------
# 2. The throwaway stand-in for the operator's files
# --------------------------------------------------------------------------
# The certificate is generated here rather than borrowed, so the probe verifies
# the chain the ingress serves instead of skipping verification. The backup
# mounts are the database's own files (P19-T04); a bind mount whose host path
# does not exist is a *directory* Docker creates, which would litter the
# checkout, so the drill stands in for both.
openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
	-keyout "$WORK_DIR/origin.key" -out "$WORK_DIR/origin.crt" \
	-subj "/CN=${SITE}" -addext "subjectAltName=DNS:${SITE}" >/dev/null 2>&1 ||
	fail "openssl could not generate the throwaway origin certificate"

printf '#!/bin/sh\nexit 0\n' >"$WORK_DIR/backupctl"
chmod 0755 "$WORK_DIR/backupctl"
printf 'not a key, and nothing in this gate opens it\n' >"$WORK_DIR/backup.key"
chmod 0600 "$WORK_DIR/backup.key"

cat >"$ENV_FILE" <<EOF
COMPOSE_ARENA_IMAGE=${APPLICATION_IMAGE}
COMPOSE_ENV_FILE=${ENV_FILE}
COMPOSE_DB_ENV_FILE=${WORK_DIR}/.env.production.db
# The drill binds loopback with an ephemeral host port: it must not take the
# machine's 80/443, and it must not be reachable from anywhere else.
COMPOSE_INGRESS_BIND=127.0.0.1
COMPOSE_HTTP_PORT=0
COMPOSE_HTTPS_PORT=0
COMPOSE_SITE_ADDRESS=${SITE}
COMPOSE_TLS_CERT_FILE=${WORK_DIR}/origin.crt
COMPOSE_TLS_KEY_FILE=${WORK_DIR}/origin.key
COMPOSE_BACKUP_TOOL=${WORK_DIR}/backupctl
COMPOSE_BACKUP_KEY_FILE=${WORK_DIR}/backup.key
ARENA_ENV=production
ARENA_DATABASE_URL=postgres://arena:${DB_PASSWORD}@db:5432/arena?sslmode=disable
ARENA_CURSOR_SECRET=deploy-verify-cursor-secret-32-bytes
ARENA_RESEND_API_KEY=re_verify_not_a_real_key
ARENA_EMAIL_FROM=Arena <no-reply@arena.invalid>
ARENA_STRIPE_SECRET_KEY=sk_live_verify_not_a_real_key
ARENA_LOG_LEVEL=info
EOF
printf 'POSTGRES_PASSWORD=%s\n' "$DB_PASSWORD" >"${WORK_DIR}/.env.production.db"

# --------------------------------------------------------------------------
# 3. The shared vocabulary of the assertions
# --------------------------------------------------------------------------
# A state file that does not exist is "no recorded release", not an error: the
# guards are explicit because this script runs under `set -e`, where a failed
# read inside a command substitution aborts with a status and no explanation.
state_current() {
	[[ -f "$STATE_FILE" ]] || return 0
	sed -n 's/^current=//p' "$STATE_FILE" | head -1
}
state_previous() {
	[[ -f "$STATE_FILE" ]] || return 0
	sed -n 's/^previous=//p' "$STATE_FILE" | head -1
}

assert_state() {
	local current previous
	current="$(state_current)"
	previous="$(state_previous)"
	[[ "$current" == "$1" ]] || fail "the state file records current=${current:-none}, want ${1:-none}"
	[[ "$previous" == "$2" ]] || fail "the state file records previous=${previous:-none}, want ${2:-none}"
}

assert_running_application() {
	local container running
	container="$(compose ps --quiet app)"
	[[ -n "$container" ]] || fail "the application has no container to inspect"
	running="$(docker inspect --format '{{.Config.Image}}' "$container")"
	[[ "$running" == "$1" ]] || fail "the application is running ${running}, want $1"
}

assert_running_version_is_the_application() {
	local version
	version="$(running_version || true)"
	[[ -n "$version" ]] || fail "the running release does not report a version"
	case "$version" in
	*stand-in*)
		fail "the running release reports version '${version}', which belongs to a stand-in rather than to the application"
		;;
	esac
}

query_database() {
	local out
	out="$(compose exec -T db psql --username arena --dbname arena --no-align --tuples-only --command "$1")" ||
		fail "the database query failed: $1"
	printf '%s' "$out" | tr -d '[:space:]'
}

public_status() {
	curl --silent --output /dev/null --write-out '%{http_code}' --max-time 10 \
		--cacert "$WORK_DIR/origin.crt" --resolve "${SITE}:${HTTPS_PORT}:127.0.0.1" \
		"https://${SITE}:${HTTPS_PORT}$1" 2>/dev/null || true
}

public_body() {
	curl --silent --max-time 10 --cacert "$WORK_DIR/origin.crt" \
		--resolve "${SITE}:${HTTPS_PORT}:127.0.0.1" "https://${SITE}:${HTTPS_PORT}$1" 2>/dev/null || true
}

# Every post-condition about the public surface is waited for with a deadline,
# never sampled once: the ingress marks an upstream unhealthy when a probe stops
# answering 200 and re-marks it healthy on its own schedule, so a single fetch
# that happens to land in that window would measure the ingress's timer instead
# of the release.
wait_for_status() {
	local path="$1" want="$2" what="$3" deadline=$((SECONDS + 90)) status
	while ((SECONDS < deadline)); do
		status="$(public_status "$path")"
		[[ "$status" == "$want" ]] && return 0
		sleep 2
	done
	fail "${what}: ${path} answered ${status:-nothing}, want ${want}"
}

wait_for_page_containing() {
	local path="$1" needle="$2" what="$3" deadline=$((SECONDS + 90)) body
	while ((SECONDS < deadline)); do
		body="$(public_body "$path")"
		[[ "$body" == *"$needle"* ]] && return 0
		sleep 2
	done
	fail "${what}: ${path} never served a page containing '${needle}'"
}

wait_for_database() {
	local deadline=$((SECONDS + 120))
	while ((SECONDS < deadline)); do
		if compose exec -T db pg_isready --username arena --dbname arena >/dev/null 2>&1; then
			return 0
		fi
		sleep 2
	done
	fail "the database never came back after the restart"
}

# The version a running release reports about itself. It is how the drill tells
# the application from a stand-in without opening the container, and it is the
# string the logs of the pipeline carry.
running_version() {
	local out
	out="$(compose exec -T app /arena version 2>/dev/null)" || return 1
	printf '%s' "$out" | sed -n 's/^arena version //p' | head -1
}

# The migration rows the pipeline applied. goose records the creation of the
# schema itself as version 0, so the count that answers "were the migrations
# applied" is the one over the versions that came from a file.
applied_migrations() {
	query_database "select count(*) from app.schema_metadata where is_applied and version_id >= 1"
}

# --------------------------------------------------------------------------
# 4. The stack, and the pipeline's vocabulary
# --------------------------------------------------------------------------
# The database and the ingress come up first, and only those two: the release's
# processes are not started by hand anywhere in this drill. Every container that
# serves is created by `deploy`, which is how the migrate step is measured
# rather than assumed.
log "bringing up the database and the ingress"
compose up --detach --wait --wait-timeout 300 db caddy >/dev/null ||
	fail "the database and the ingress did not become ready"

HTTPS_PORT="$(compose port caddy 443 | head -1 | sed 's/.*://')"
[[ -n "$HTTPS_PORT" ]] || fail "the ingress published no port"
log "the ingress answers on 127.0.0.1:${HTTPS_PORT}"

DB_CONTAINER="$(compose ps --quiet db)"
[[ -n "$DB_CONTAINER" ]] || fail "the database has no container to inspect"

COMMON_ARGS=(
	--project "$PROJECT"
	--env-file "$ENV_FILE"
	--state-file "$STATE_FILE"
	--compose-file "$COMPOSE_FILE"
	--health-url "https://${SITE}:${HTTPS_PORT}"
	--health-path /health/ready
	--smoke-path /login
	--resolve "${SITE}:${HTTPS_PORT}:127.0.0.1"
	--cacert "$WORK_DIR/origin.crt"
	--yes
)

# The caller's arguments come last on purpose: `deploy.sh` takes the last value
# of a flag it is given twice, so a scenario that deliberately overrides one of
# the shared options — a state file that disagrees with the containers, a
# document that runs another artifact — is the one in effect.
pipeline() {
	local command="$1"
	shift
	"$ROOT/deploy/deploy.sh" "$command" "${COMMON_ARGS[@]}" "$@"
}

expect_promotion() {
	local description="$1"
	shift
	if ! pipeline "$@" >"$WORK_DIR/promotion.txt" 2>&1; then
		indent "$WORK_DIR/promotion.txt"
		fail "${description}: the pipeline failed where it had to succeed"
	fi
	log "promoted: ${description}"
}

expect_refusal() {
	local description="$1" expected="$2"
	shift 2
	if pipeline "$@" >"$WORK_DIR/refusal.txt" 2>&1; then
		indent "$WORK_DIR/refusal.txt"
		fail "${description}: the pipeline accepted it; it must refuse"
	fi
	if ! grep -qF "$expected" "$WORK_DIR/refusal.txt"; then
		indent "$WORK_DIR/refusal.txt"
		fail "${description}: the refusal did not say '${expected}'"
	fi
	log "refused as intended: ${description}"
}

# --------------------------------------------------------------------------
# 5. Preflight: what the pipeline refuses before it touches anything
# --------------------------------------------------------------------------
# Each of these is a refusal that must happen before the first state change, so
# they run before the first promotion and the state file is asserted to still
# not exist afterwards.
expect_refusal "a tag instead of a digest" "is not pinned by digest" \
	deploy --image "$IMAGE" --timeout 30
expect_refusal "a malformed digest" "malformed" \
	deploy --image "${REGISTRY_REPOSITORY}@sha256:nothex" --timeout 30

if "$ROOT/deploy/deploy.sh" deploy --image "$APPLICATION_IMAGE" \
	--env-file "$ENV_FILE" --project "$PROJECT" --state-file "$STATE_FILE" --yes \
	>"$WORK_DIR/refusal.txt" 2>&1; then
	fail "a deployment without a health probe was accepted; without a probe a promotion is a guess"
fi
grep -qF "health probe" "$WORK_DIR/refusal.txt" ||
	fail "the refusal for a missing --health-url did not name the health probe"
log "refused as intended: a deployment with nothing to prove health with"

# A document that runs its own artifact: the promoted digest never reaches the
# application, so the promotion would be a deployment of something else.
sed -E "s|^    image: [\$][{]COMPOSE_ARENA_IMAGE.*|    image: ${STAND_IN_READY}|" \
	"$COMPOSE_FILE" >"$WORK_DIR/pins-its-own-image.yaml"
if cmp -s "$COMPOSE_FILE" "$WORK_DIR/pins-its-own-image.yaml"; then
	fail "the document used to prove the artifact check could not be rewritten"
fi
if "$ROOT/deploy/deploy.sh" deploy --image "$APPLICATION_IMAGE" \
	--compose-file "$WORK_DIR/pins-its-own-image.yaml" \
	--env-file "$ENV_FILE" --project "$PROJECT" --state-file "$STATE_FILE" \
	--health-url "https://${SITE}:${HTTPS_PORT}" --yes >"$WORK_DIR/refusal.txt" 2>&1; then
	fail "a document that does not run the promoted digest was accepted"
fi
grep -qF "does not run" "$WORK_DIR/refusal.txt" ||
	fail "the refusal for a document that runs another artifact did not say so"
log "refused as intended: a document that would run a digest other than the promoted one"

# The migration runner has no down path, and this is the assertion that keeps
# that true: the pipeline's rollback is a rollback of the process, and a schema
# rollback appearing in the runner is a decision this drill would have to be
# told about rather than discover.
if compose run --rm --no-deps app migrate down >"$WORK_DIR/down.txt" 2>&1; then
	fail "the migration runner accepted a down subcommand; an automated schema rollback is what the pipeline does not do"
fi
grep -qF "unknown migrate subcommand" "$WORK_DIR/down.txt" ||
	fail "the runner refused 'migrate down' for a reason other than it not existing"
log "the migration runner has no down path; the pipeline can only move forward"

[[ ! -e "$STATE_FILE" ]] ||
	fail "a refused deployment created $STATE_FILE; a preflight must leave nothing behind"

# --------------------------------------------------------------------------
# 6. A deployment does not create the database it migrates
# --------------------------------------------------------------------------
log "stopping the database to prove a deployment does not start one"
compose stop db >/dev/null || fail "stopping the database failed"
expect_refusal "a deployment against a stopped database" "not running" \
	deploy --image "$APPLICATION_IMAGE" --timeout 30
if [[ -n "$(compose ps --status running --quiet db)" ]]; then
	fail "the refused deployment started the database"
fi
compose start db >/dev/null || fail "starting the database again failed"
wait_for_database
log "the database is back, and the refused deployment had not started it"

# --------------------------------------------------------------------------
# 7. The first release: migrate, promote, prove
# --------------------------------------------------------------------------
SCHEMA_BEFORE="$(query_database "select to_regclass('app.schema_metadata') is null")"
[[ "$SCHEMA_BEFORE" == "t" ]] ||
	fail "the schema already exists before the first deployment; the drill cannot attribute it to the pipeline"

expect_promotion "the application, migrating and serving for the first time" \
	deploy --image "$APPLICATION_IMAGE" --timeout 120

EXPECTED_MIGRATIONS="$(find internal/platform/dbmigrate/migrations -maxdepth 1 -name '*.sql' | wc -l | tr -d '[:space:]')"
APPLIED="$(applied_migrations)"
[[ "$APPLIED" == "$EXPECTED_MIGRATIONS" ]] ||
	fail "the pipeline applied ${APPLIED} migration(s), want ${EXPECTED_MIGRATIONS} (one per file in internal/platform/dbmigrate/migrations)"
log "the pipeline applied all ${APPLIED} migrations before the release served"

assert_state "$APPLICATION_IMAGE" ""
assert_running_application "$APPLICATION_IMAGE"
assert_running_version_is_the_application
APPLICATION_VERSION="$(running_version || true)"
wait_for_page_containing /login 'name="csrf_token"' "the first release serves its pages"
log "the application is serving (version ${APPLICATION_VERSION})"

# "Already deployed" is a claim about the release, not about the state file, so
# the pipeline proves it instead of trusting it.
expect_promotion "the same digest, already deployed" \
	deploy --image "$APPLICATION_IMAGE" --timeout 120
grep -qF "nothing to promote" "$WORK_DIR/promotion.txt" ||
	fail "re-deploying the recorded release did not report that there was nothing to promote"
assert_state "$APPLICATION_IMAGE" ""

# There is nothing to go back to, and the pipeline says so rather than promoting
# an arbitrary digest.
expect_refusal "a rollback with no previous release" "no previous digest" rollback

# --------------------------------------------------------------------------
# 8. The previous version: promote it, and roll the application back to it
# --------------------------------------------------------------------------
expect_promotion "a newer release, promoted by digest" \
	deploy --image "$STAND_IN_READY" --timeout 120
assert_state "$STAND_IN_READY" "$APPLICATION_IMAGE"
assert_running_application "$STAND_IN_READY"
[[ "$(running_version || true)" == *stand-in* ]] ||
	fail "the promoted release does not report the version of the artifact that was promoted"

expect_promotion "the previous release, restored by rollback" rollback --timeout 120
assert_state "$APPLICATION_IMAGE" "$STAND_IN_READY"
assert_running_application "$APPLICATION_IMAGE"
ROLLED_BACK_VERSION="$(running_version || true)"
[[ "$ROLLED_BACK_VERSION" == "$APPLICATION_VERSION" ]] ||
	fail "the rollback left version '${ROLLED_BACK_VERSION}' running, want the application's '${APPLICATION_VERSION}'"
wait_for_page_containing /login 'name="csrf_token"' "the rollback restored a serving previous version"
AFTER_ROLLBACK="$(applied_migrations)"
[[ "$AFTER_ROLLBACK" == "$EXPECTED_MIGRATIONS" ]] ||
	fail "the rollback moved the schema: ${AFTER_ROLLBACK} applied migration(s), want ${EXPECTED_MIGRATIONS} unchanged"
log "the rollback restored the previous version and left the schema where the expand step put it"

# A clean pair again, so the two failure scenarios below start from a release
# that is healthy and has a previous release to fall back to.
expect_promotion "the newer release again, so the failures have something to fall back to" \
	deploy --image "$STAND_IN_READY" --timeout 120
assert_state "$STAND_IN_READY" "$APPLICATION_IMAGE"
cp "$STATE_FILE" "$WORK_DIR/state.before-failures"

# --------------------------------------------------------------------------
# 9. A release that never becomes ready is not promoted
# --------------------------------------------------------------------------
if pipeline deploy --image "$STAND_IN_NOT_READY" --timeout 30 >"$WORK_DIR/failure.txt" 2>&1; then
	fail "a release that never answers the readiness probe was promoted"
fi
for expected in "was not promoted" "health probe never answered 200" "previous digest was restored"; do
	if ! grep -qF "$expected" "$WORK_DIR/failure.txt"; then
		indent "$WORK_DIR/failure.txt"
		fail "the failed promotion did not report '${expected}'"
	fi
done
if ! cmp -s "$WORK_DIR/state.before-failures" "$STATE_FILE"; then
	indent "$STATE_FILE"
	fail "the state file changed after a release that was not promoted"
fi
assert_state "$STAND_IN_READY" "$APPLICATION_IMAGE"
assert_running_application "$STAND_IN_READY"
wait_for_status /health/ready 200 "after a refused promotion the surface must still be ready"
wait_for_status /login 200 "after a refused promotion the surface must still serve"
log "a release that never became ready was refused and the previous digest came back serving"

# --------------------------------------------------------------------------
# 10. A release that is ready and has lost a page is not promoted either
# --------------------------------------------------------------------------
if pipeline deploy --image "$STAND_IN_PAGE_MISSING" --timeout 30 >"$WORK_DIR/failure.txt" 2>&1; then
	fail "a release whose pages do not answer was promoted"
fi
if ! grep -qF "smoke probe answered 404" "$WORK_DIR/failure.txt"; then
	indent "$WORK_DIR/failure.txt"
	fail "the failed promotion did not name the smoke probe and its status"
fi
if ! cmp -s "$WORK_DIR/state.before-failures" "$STATE_FILE"; then
	indent "$STATE_FILE"
	fail "the state file changed after a release that failed its smoke probe"
fi
assert_running_application "$STAND_IN_READY"
wait_for_status /health/ready 200 "after a refused promotion the surface must still be ready"
log "a release that answered readiness and lost a page was refused, and readiness alone was not enough"

# --------------------------------------------------------------------------
# 11. A state file that disagrees with the containers is refused
# --------------------------------------------------------------------------
# The state file is what a rollback reads, so a file that describes a release
# nobody is running would make the rollback target a guess. It is refused, and
# refused *before* the migrate step.
printf '# written by the drill, deliberately wrong\ncurrent=%s\nprevious=%s\n' \
	"$STAND_IN_PAGE_MISSING" "$APPLICATION_IMAGE" >"$WORK_DIR/drifted.state"
if pipeline deploy --image "$APPLICATION_IMAGE" --state-file "$WORK_DIR/drifted.state" --timeout 30 \
	>"$WORK_DIR/refusal.txt" 2>&1; then
	fail "a state file that disagrees with the running container was accepted"
fi
if ! grep -qF "reconcile the state" "$WORK_DIR/refusal.txt"; then
	indent "$WORK_DIR/refusal.txt"
	fail "the refusal for a drifted state file did not ask for a reconciliation"
fi
assert_running_application "$STAND_IN_READY"
log "a state file that disagrees with the containers is refused instead of becoming a rollback target"

# --------------------------------------------------------------------------
# 12. What the deployments did not touch, and what the state says
# --------------------------------------------------------------------------
# A deployment replaces the processes that run the release. The database keeps
# its container across all of the above, which is the difference between
# promoting an artifact and rebuilding a stack.
[[ "$(compose ps --quiet db)" == "$DB_CONTAINER" ]] ||
	fail "a deployment replaced the database container"
log "the database container was never replaced"

STATUS_OUTPUT="$(pipeline status)" || fail "status failed"
printf '%s\n' "$STATUS_OUTPUT" >&2
printf '%s\n' "$STATUS_OUTPUT" | grep -qF "running:  ${STAND_IN_READY}" ||
	fail "status does not report the running release as the recorded one"
printf '%s\n' "$STATUS_OUTPUT" | grep -qF "previous: ${APPLICATION_IMAGE}" ||
	fail "status does not report the rollback target"
log "status reports the recorded release and the rollback target"

log "ok — digest-pinned promotions migrated and proved, a failed release refused by readiness and by smoke, the previous version restored, the schema unmoved, and the database untouched"

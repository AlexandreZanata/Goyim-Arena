#!/usr/bin/env bash
#
# Verification of the production Compose topology (P19-T02).
#
# Why the gate is a script and not only a file audit: `docker compose config`
# says what Compose *would* create, and a topology is only real once it runs.
# This gate closes the difference — it renders the document, audits it, brings
# the stack up, applies migrations with the application's own image, drives a
# registration through the public ingress, and then restarts and recreates the
# stack to prove the data outlives both.
#
# What one run owns, and therefore tears down on every exit path: a local
# registry that holds the artifact by digest, a Compose project with its own
# volumes, a throwaway certificate and the environment files that stand in for
# the operator's.
#
# It is fail-closed: a missing command, a document that does not render, a rule
# broken, a stack that never becomes healthy, a page that is not served, a port
# that should not be published, a limit that was not applied, or data that does
# not survive a restart all abort with a non-zero status.
#
# Requirements: Docker with a reachable daemon and the Compose plugin, Go,
# curl, openssl.
#
# Environment:
#   ARENA_IMAGE   tag of the application image to promote (default
#                 goyim-arena:verify). It is pushed to a throwaway registry and
#                 the stack runs the digest, which is what production does.
set -euo pipefail

TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$TOOL_DIR/../.." && pwd)"
cd "$ROOT"

IMAGE="${ARENA_IMAGE:-goyim-arena:verify}"
COMPOSE_FILE="compose.production.yaml"
RUN_ID="$$"
PROJECT="arena-compose-verify-${RUN_ID}"
REGISTRY="arena-compose-verify-registry-${RUN_ID}"
SITE="arena.verify.invalid"
DB_PASSWORD="verify-database-password"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/arena-compose-XXXXXX")"

log() { printf 'compose-verify: %s\n' "$*" >&2; }
fail() { printf 'compose-verify: %s\n' "$*" >&2; exit 1; }

cleanup() {
	compose down --volumes --remove-orphans >/dev/null 2>&1 || true
	docker rm --force "$REGISTRY" >/dev/null 2>&1 || true
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
	docker compose --project-name "$PROJECT" --env-file "$WORK_DIR/.env.production" -f "$COMPOSE_FILE" "$@"
}

# ---------------------------------------------------------------------------
# 1. The artifact, promoted by digest through a registry
# ---------------------------------------------------------------------------
# The stack references the application by digest, so the gate has to produce
# one. Pushing to a throwaway registry is how the real pipeline produces it
# (docs/DEPLOYMENT.md §5.2), and it is also what proves the reference in the
# file is an artifact someone can fetch rather than a tag on this machine.
if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
	fail "the application image $IMAGE does not exist; run 'make image-build' first"
fi

docker run --detach --name "$REGISTRY" --publish 127.0.0.1::5000 \
	registry:2@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373 >/dev/null
REGISTRY_PORT="$(docker port "$REGISTRY" 5000/tcp | head -1 | sed 's/.*://')"
[[ -n "$REGISTRY_PORT" ]] || fail "the throwaway registry did not publish a port"

REGISTRY_REPOSITORY="127.0.0.1:${REGISTRY_PORT}/goyim-arena"
docker tag "$IMAGE" "${REGISTRY_REPOSITORY}:verify"
docker push "${REGISTRY_REPOSITORY}:verify" >/dev/null
APPLICATION_IMAGE="$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "${REGISTRY_REPOSITORY}:verify" |
	grep -F "${REGISTRY_REPOSITORY}@sha256:" | head -1)"
[[ -n "$APPLICATION_IMAGE" ]] || fail "the pushed image has no digest to promote"
log "promoting ${APPLICATION_IMAGE#127.0.0.1:${REGISTRY_PORT}/}"

# ---------------------------------------------------------------------------
# 2. The throwaway stand-in for the operator's secrets
# ---------------------------------------------------------------------------
# Two files, because the roles are two: the application reads the ARENA_*
# runtime configuration, the database reads its own password, and neither reads
# the other's. The certificate is generated here rather than borrowed, so the
# smoke can verify the chain Caddy serves instead of disabling verification.
openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
	-keyout "$WORK_DIR/origin.key" -out "$WORK_DIR/origin.crt" \
	-subj "/CN=${SITE}" -addext "subjectAltName=DNS:${SITE}" >/dev/null 2>&1 ||
	fail "openssl could not generate the throwaway origin certificate"

# The database's backup mounts are the operator's files (P19-T04): the tool the
# scripts call and the sealing key. The gate stands in for both, because a bind
# mount whose host path does not exist is a *directory* Docker creates — which
# would both litter the checkout and hand the archive a path that is not the
# tool. The tool here is a placeholder: this gate is about the topology, and the
# pipeline that uses it is proved by `make backup-verify`.
printf '#!/bin/sh\nexit 0\n' >"$WORK_DIR/backupctl"
chmod 0755 "$WORK_DIR/backupctl"
printf 'not a key, and nothing in this gate opens it\n' >"$WORK_DIR/backup.key"
chmod 0600 "$WORK_DIR/backup.key"

cat >"$WORK_DIR/.env.production" <<EOF
COMPOSE_ARENA_IMAGE=${APPLICATION_IMAGE}
COMPOSE_ENV_FILE=${WORK_DIR}/.env.production
COMPOSE_DB_ENV_FILE=${WORK_DIR}/.env.production.db
# The gate binds loopback with an ephemeral host port: it must not take the
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
ARENA_CURSOR_SECRET=compose-verify-cursor-secret-32-bytes
ARENA_RESEND_API_KEY=re_verify_not_a_real_key
ARENA_EMAIL_FROM=Arena <no-reply@arena.invalid>
ARENA_STRIPE_SECRET_KEY=sk_live_verify_not_a_real_key
ARENA_LOG_LEVEL=info
EOF
printf 'POSTGRES_PASSWORD=%s\n' "$DB_PASSWORD" >"$WORK_DIR/.env.production.db"

# ---------------------------------------------------------------------------
# 3. The rendered document and its audit
# ---------------------------------------------------------------------------
# `config` is both the validity check of the file and the input of the audit.
# Its output is piped straight into the tool and never printed: by then every
# environment file has been inlined, so the document carries the operator's
# credentials, and a gate that echoes them is a leak with a green tick.
compose config --quiet || fail "docker compose config refused the topology"
compose config --format json >"$WORK_DIR/rendered.json" || fail "the topology could not be rendered"
go run ./tools/composeaudit \
	-file "$COMPOSE_FILE" \
	-document "$WORK_DIR/rendered.json" \
	-application-image "$APPLICATION_IMAGE"

# ---------------------------------------------------------------------------
# 4. The stack
# ---------------------------------------------------------------------------
log "bringing the stack up"
compose up --detach --wait --wait-timeout 300 >/dev/null ||
	fail "the stack did not become ready; see 'docker compose -p $PROJECT logs'"

# The host ports are ephemeral, so they are read back after every state change
# instead of remembered: a binding belongs to the container that was created,
# and a restart or a recreate may hand out another one.
resolve_ports() {
	HTTPS_PORT="$(compose port caddy 443 | head -1 | sed 's/.*://')"
	HTTP_PORT="$(compose port caddy 80 | head -1 | sed 's/.*://')"
	[[ -n "$HTTPS_PORT" && -n "$HTTP_PORT" ]] || fail "the ingress published no port"
}

resolve_ports
log "the ingress answers on 127.0.0.1:${HTTPS_PORT} (https) and 127.0.0.1:${HTTP_PORT} (http)"

# Migrations are the release's own step, with the application's own image
# (docs/DEPLOYMENT.md §5.5): the stack does not migrate itself on boot.
log "applying migrations with the application image"
compose run --rm app migrate up >/dev/null || fail "the migration step failed"

ingress() {
	curl --silent --show-error --fail-with-body --max-time 10 \
		--cacert "$WORK_DIR/origin.crt" --resolve "${SITE}:${HTTPS_PORT}:127.0.0.1" \
		"$@"
}

wait_for_page() {
	local path="$1" deadline=$((SECONDS + 90)) status
	while ((SECONDS < deadline)); do
		status="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 10 \
			--cacert "$WORK_DIR/origin.crt" --resolve "${SITE}:${HTTPS_PORT}:127.0.0.1" \
			"https://${SITE}:${HTTPS_PORT}${path}" || true)"
		[[ "$status" == "200" ]] && return 0
		sleep 2
	done
	fail "https://${SITE}:${HTTPS_PORT}${path} never answered 200 through the ingress (last status: ${status:-none})"
}

# The chain is verified against the certificate the operator's file provides:
# a Caddy serving its own self-signed default would fail here, which is the
# point of mounting a certificate at all.
wait_for_page /health/live
wait_for_page /health/ready
log "the ingress serves the application over the mounted certificate"

# The HTTP side is the automatic redirect of the same site, and the redirect is
# the reason publishing 80 is not decoration.
HTTP_STATUS="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 10 \
	"http://127.0.0.1:${HTTP_PORT}/health/live" || true)"
case "$HTTP_STATUS" in
308 | 301 | 302) ;;
*) fail "plain HTTP answered ${HTTP_STATUS:-nothing}, want a redirect to the site address" ;;
esac
log "plain HTTP redirects to the site address (${HTTP_STATUS})"

# ---------------------------------------------------------------------------
# 5. A registration, end to end through the ingress
# ---------------------------------------------------------------------------
# This is the claim that ties this task to P19-T02A: the ingress reaches the
# application, the application reaches the database, and the message it must
# not lose becomes durable work in the queue the worker consumes.
EMAIL="compose-verify@example.test"
PASSWORD="compose verify password"
COOKIE_JAR="$WORK_DIR/cookies.txt"

register_page() {
	ingress --cookie-jar "$COOKIE_JAR" --cookie "$COOKIE_JAR" \
		"https://${SITE}:${HTTPS_PORT}/register"
}

document="$(register_page)" || fail "GET /register through the ingress failed"
CSRF="$(printf '%s' "$document" | grep -o 'name="csrf_token" value="[^"]*"' | head -1 | sed 's/.*value="//; s/"$//')"
[[ -n "$CSRF" ]] || fail "the registration page carries no csrf_token field"

REGISTER_STATUS="$(ingress --cookie-jar "$COOKIE_JAR" --cookie "$COOKIE_JAR" \
	--output "$WORK_DIR/register.html" --write-out '%{http_code}' \
	--data-urlencode "email=${EMAIL}" \
	--data-urlencode "password=${PASSWORD}" \
	--data-urlencode "csrf_token=${CSRF}" \
	"https://${SITE}:${HTTPS_PORT}/register" || true)"
[[ "$REGISTER_STATUS" == "200" ]] || fail "POST /register answered ${REGISTER_STATUS}, want 200"
log "a registration through the ingress created the account and queued its message"

query_database() {
	compose exec -T db psql --username arena --dbname arena --no-align --tuples-only --command "$1" |
		tr -d '[:space:]'
}

ACCOUNTS="$(query_database "select count(*) from app.accounts where email = '${EMAIL}'")"
[[ "$ACCOUNTS" == "1" ]] || fail "the database holds ${ACCOUNTS:-no} account(s) for the registration, want exactly one"

JOBS="$(query_database "select count(*) from app.jobs where type = 'email_delivery'")"
[[ "$JOBS" == "1" ]] || fail "the queue holds ${JOBS:-no} email_delivery job(s), want exactly the one the registration queued"

# The worker is a separate process, and the proof that it runs is that the job
# stops being untouched work: it is leased, attempted, and left with a recorded
# outcome. That is what this step asserts, and it is all it asserts — a worker
# that could not open a socket to the provider records an attempt too, so the
# counter says "the queue is consumed", never "the provider answered".
#
# The recorded failure is therefore read back and printed instead of being
# turned into a verdict. Against a reachable provider the placeholder
# credential is rejected and the detail is the rejection; without egress the
# detail is the transport failure. Requiring the former would make this gate
# depend on a third party's uptime, which is the failure mode the rest of the
# gate avoids by never calling anything it does not own.
DEADLINE=$((SECONDS + 120))
ATTEMPTS="0"
while ((SECONDS < DEADLINE)); do
	ATTEMPTS="$(query_database "select attempts from app.jobs where type = 'email_delivery'")"
	[[ "$ATTEMPTS" != "" && "$ATTEMPTS" != "0" ]] && break
	sleep 3
done
[[ "$ATTEMPTS" != "" && "$ATTEMPTS" != "0" ]] ||
	fail "the worker never attempted the queued delivery; the queue would grow forever"
log "the worker leased and attempted the queued delivery (attempt ${ATTEMPTS})"

FAILURE_CODE="$(query_database "select coalesce(last_error_code, '(none)') from app.jobs where type = 'email_delivery'")"
FAILURE_DETAIL="$(compose exec -T db psql --username arena --dbname arena --no-align --tuples-only \
	--command "select coalesce(last_error_detail, '(none)') from app.jobs where type = 'email_delivery'" | tr -s '[:space:]' ' ' | sed 's/^ //; s/ $//')"
log "the recorded outcome is ${FAILURE_CODE}; the process observed: ${FAILURE_DETAIL}"
case "$FAILURE_DETAIL" in
"" | "(none)")
	log "the worker recorded no detail for the attempt"
	;;
*"no such host"* | *"dial tcp"* | *refused* | *"network is unreachable"* | *"i/o timeout"*)
	log "the provider was not reached from this host: the attempt is recorded, but this run does not prove the worker's route out"
	;;
*)
	log "the provider answered and refused the placeholder credential, which is this run's proof that the worker can reach it"
	;;
esac

# ---------------------------------------------------------------------------
# 6. What is published, and what was applied
# ---------------------------------------------------------------------------
# A port is a grant, so it is inspected rather than assumed: the container that
# is supposed to have none is checked for none, and the one that has them is
# checked for exactly the ingress's own HTTP and HTTPS.
container_of() { compose ps --quiet "$1"; }

# `docker port` lists the bindings that actually exist on the host, and says
# nothing about the ports an image merely declares, which is exactly the
# distinction being inspected: 5432 is exposed by the PostgreSQL image and must
# still not be reachable, and the ingress's admin endpoint is exposed and must
# not be published either.
check_no_published_port() {
	local service="$1" container bindings
	container="$(container_of "$service")"
	[[ -n "$container" ]] || fail "service $service has no container to inspect"
	bindings="$(docker port "$container")"
	if [[ -n "$bindings" ]]; then
		fail "service $service publishes ${bindings//$'\n'/, }; only the ingress may be reachable from outside"
	fi
}

check_no_published_port db
check_no_published_port app
check_no_published_port worker
log "the database, the application and the worker publish nothing"

INGRESS_BINDINGS="$(docker port "$(container_of caddy)")"
for expected in "80/tcp" "443/tcp"; do
	printf '%s' "$INGRESS_BINDINGS" | grep -q "^${expected} " ||
		fail "the ingress does not publish ${expected}: ${INGRESS_BINDINGS//$'\n'/, }"
done
PUBLISHED_COUNT="$(printf '%s' "$INGRESS_BINDINGS" | grep -c .)"
[[ "$PUBLISHED_COUNT" == "2" ]] ||
	fail "the ingress publishes ${PUBLISHED_COUNT} binding(s), and only its HTTP and HTTPS ports are publishable: ${INGRESS_BINDINGS//$'\n'/, }"
log "the ingress publishes exactly its HTTP and HTTPS ports"

# Limits and the read-only filesystem are declared in the file; the container
# is where they become true. A Compose implementation that ignored the deploy
# block would leave these at zero, and the numbers are the whole point of
# docs/DEPLOYMENT.md §2.
for service in db app worker caddy; do
	container="$(container_of "$service")"
	memory="$(docker inspect --format '{{.HostConfig.Memory}}' "$container")"
	cpus="$(docker inspect --format '{{.HostConfig.NanoCpus}}' "$container")"
	[[ "$memory" != "0" ]] || fail "service $service runs with no memory limit applied"
	[[ "$cpus" != "0" ]] || fail "service $service runs with no CPU limit applied"
done
for service in app worker; do
	container="$(container_of "$service")"
	readonly_rootfs="$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$container")"
	[[ "$readonly_rootfs" == "true" ]] || fail "service $service does not run with a read-only root filesystem"
done
log "every service runs with its declared CPU, memory and filesystem limits"

# ---------------------------------------------------------------------------
# 7. A restart, and a full recreate, without loss
# ---------------------------------------------------------------------------
log "restarting the application, the worker and the ingress"
compose restart app worker caddy >/dev/null || fail "the restart failed"
resolve_ports
log "after the restart the ingress answers on 127.0.0.1:${HTTPS_PORT} (https)"
wait_for_page /health/live

AFTER_RESTART="$(query_database "select count(*) from app.accounts where email = '${EMAIL}'")"
[[ "$AFTER_RESTART" == "1" ]] ||
	fail "the account did not survive the restart (${AFTER_RESTART:-none} row(s))"

# A recreate is the stronger claim: `down` without `--volumes` removes the
# containers and keeps the data, which is the difference between a restart and
# a deployment.
log "recreating the stack from the same volumes"
compose down --remove-orphans >/dev/null || fail "tearing the stack down failed"
compose up --detach --wait --wait-timeout 300 >/dev/null || fail "the stack did not come back"
resolve_ports
wait_for_page /health/live

AFTER_RECREATE_ACCOUNTS="$(query_database "select count(*) from app.accounts where email = '${EMAIL}'")"
[[ "$AFTER_RECREATE_ACCOUNTS" == "1" ]] ||
	fail "the account did not survive the recreate (${AFTER_RECREATE_ACCOUNTS:-none} row(s))"
AFTER_RECREATE_JOBS="$(query_database "select count(*) from app.jobs where type = 'email_delivery'")"
[[ "$AFTER_RECREATE_JOBS" == "1" ]] ||
	fail "the queued work did not survive the recreate (${AFTER_RECREATE_JOBS:-none} job(s))"
log "the account and its queued work survived a restart and a recreate"

log "ok — digest-pinned stack, one public surface, migrations applied by the image, a registration delivered to the queue, and no data lost across a restart"

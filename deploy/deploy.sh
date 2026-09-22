#!/usr/bin/env bash
#
# deploy.sh — promote one application image by digest, migrate forward, prove
# health, and be able to go back (P19-T07).
#
# Why this is a script and not a paragraph of instructions: a deployment is a
# sequence with a failure point, and the point is the health check. "Deploy and
# hope" is not a process; "if the new release does not become healthy, put the
# previous one back and say so" is. That behaviour only exists if something runs
# it, so it lives here.
#
# The four rules the pipeline enforces:
#
#   1. The artifact is a DIGEST, never a tag. A tag is mutable, so a tag is a
#      promise the registry can break after the machine that promoted it has
#      looked away. The reference must carry `@sha256:` and the image must have
#      that digest.
#   2. Migrations are FORWARD ONLY, and they run before the new binary serves.
#      The migration runner has no down path at all (internal/platform/dbmigrate
#      exposes Status/Up/CurrentVersion and nothing else), and this pipeline
#      only ever calls `migrate up`. A rollback is a rollback of the *process*:
#      the schema stays where the expand step left it, which is why the expand
#      step must be backward compatible with the release it replaces
#      (docs/DEPLOYMENT.md §5).
#   3. A release that does not become healthy is not a release. After the
#      promote, three things must hold: the readiness probe answers 200 at the
#      public surface, every smoke path answers 200, and the container that is
#      running reports the digest that was promoted. Any of the three failing
#      rolls the application back to the previous digest and reports the
#      release as not promoted. The third is not ceremony: a promote that
#      silently left the old container in place would pass a health check the
#      *old* release answered, and the state file would then record a release
#      no process is running.
#   4. Nothing is assumed about the operator's environment. The compose file,
#      the env file, the project name, the health URL and the state file are all
#      arguments; the state file records what is running so a rollback has a
#      target that does not depend on anyone's memory.
#
# What the script never does: echo a secret (it never reads the env file
# itself), publish a port, tear down a volume, or touch the database beyond
# `migrate up`.
set -euo pipefail

COMPOSE_FILE="compose.production.yaml"
ENV_FILE=""
PROJECT=""
STATE_FILE=""
HEALTH_URL=""
HEALTH_PATH="/health/ready"
SMOKE_PATHS=()
CURL_ARGS=()
PROMOTION_FAILURE=""
TIMEOUT=180
IMAGE=""
ASSUME_YES="no"

usage() {
	cat <<'USAGE'
usage: deploy.sh <command> [options]

Commands:
  deploy --image REF     promote REF (an image pinned by digest), migrate
                         forward, promote the processes and prove health
  rollback               promote the digest recorded as previous and prove
                         health (the process only; never a down migration)
  status                 print the recorded current/previous digests

Options:
  --compose-file F   compose document (default: compose.production.yaml)
  --env-file F       operator environment file (required for deploy/rollback)
  --project P        compose project name (default: the directory name)
  --state-file F     where the deployed digests are recorded
                     (default: <env-file>.deployed)
  --health-url U     base URL of the public surface, e.g. https://arena.example
                     (required for deploy/rollback: without it there is no
                     health to prove)
  --health-path P    readiness path (default: /health/ready)
  --smoke-path P     page fetched once the release is ready, expected to answer
                     200 (default: /login; repeatable, and a release that loses
                     a page is a release that lost a page)
  --resolve H:P:IP   passed to curl as --resolve (a loopback exercise)
  --cacert F         passed to curl as --cacert (a self-signed exercise)
  --timeout SECONDS  how long health may take (default: 180)
  --yes              do not ask for confirmation
  -h | --help        this text

The command exits non-zero when the promotion did not happen — including when
it rolled back after a failed health check.
USAGE
}

log() { printf 'deploy: %s\n' "$*" >&2; }
fail() { printf 'deploy: %s\n' "$*" >&2; exit 1; }

command="${1:-}"
[[ -n "$command" ]] || { usage >&2; exit 2; }
shift

while (($# > 0)); do
	case "$1" in
	--compose-file) COMPOSE_FILE="$2"; shift 2 ;;
	--env-file) ENV_FILE="$2"; shift 2 ;;
	--project) PROJECT="$2"; shift 2 ;;
	--state-file) STATE_FILE="$2"; shift 2 ;;
	--health-url) HEALTH_URL="$2"; shift 2 ;;
	--health-path) HEALTH_PATH="$2"; shift 2 ;;
	--smoke-path) SMOKE_PATHS+=("$2"); shift 2 ;;
	--resolve) CURL_ARGS+=(--resolve "$2"); shift 2 ;;
	--cacert) CURL_ARGS+=(--cacert "$2"); shift 2 ;;
	--timeout) TIMEOUT="$2"; shift 2 ;;
	--image) IMAGE="$2"; shift 2 ;;
	--yes) ASSUME_YES="yes"; shift ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage >&2
		fail "unknown argument $1"
		;;
	esac
done

case "$command" in
status) ;;
deploy | rollback)
	[[ -n "$ENV_FILE" ]] || fail "--env-file is required for $command (the operator's environment)"
	[[ -n "$HEALTH_URL" ]] || fail "--health-url is required for $command: without a health probe a promotion is a guess"
	[[ -f "$ENV_FILE" ]] || fail "the environment file $ENV_FILE does not exist"
	;;
*)
	usage >&2
	fail "unknown command $command"
	;;
esac

# A release with no page to fetch is a release whose service was never
# observed: the readiness probe says the process will take work, the smoke says
# it answers a visitor. One default, and it is the sign-in page because that is
# the surface every visitor of a private arena passes through.
if ((${#SMOKE_PATHS[@]} == 0)); then
	SMOKE_PATHS=("/login")
fi

[[ -f "$COMPOSE_FILE" ]] || fail "the compose file $COMPOSE_FILE does not exist"
if [[ -z "$STATE_FILE" ]]; then
	[[ -n "$ENV_FILE" ]] || fail "--state-file is required when there is no --env-file"
	STATE_FILE="${ENV_FILE}.deployed"
fi
if [[ -z "$PROJECT" ]]; then
	PROJECT="$(basename "$(pwd)")"
fi

require_command() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required and was not found in PATH"
}
require_command docker
require_command curl

# `status` runs without an environment file (it only reads the local state and
# the containers), so the flag is passed when there is one: `--env-file ""` is
# an empty pathname, not an absence.
compose() {
	if [[ -n "$ENV_FILE" ]]; then
		docker compose --project-name "$PROJECT" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
	else
		docker compose --project-name "$PROJECT" -f "$COMPOSE_FILE" "$@"
	fi
}

# ---------------------------------------------------------------------------
# The state file: what is running, and what to go back to
# ---------------------------------------------------------------------------
# Two lines, no secrets, and no dependency outside the file itself. It is the
# operator's file, kept beside the environment file, never in Git.
#
# A state file that does not exist yet is not an error: it is the first
# deployment, and "no recorded release" is the value. The missing-file guard is
# explicit because these run under `set -e`, where a failed read would abort with
# an exit status and no explanation of what was being read.
state_current() {
	[[ -f "$STATE_FILE" ]] || return 0
	sed -n 's/^current=//p' "$STATE_FILE" | head -1
}
state_previous() {
	[[ -f "$STATE_FILE" ]] || return 0
	sed -n 's/^previous=//p' "$STATE_FILE" | head -1
}

write_state() {
	local current="$1" previous="$2"
	umask 077
	{
		printf '# written by deploy.sh; the operator owns this file and it is never in Git\n'
		printf 'current=%s\n' "$current"
		printf 'previous=%s\n' "$previous"
	} >"$STATE_FILE"
}

# ---------------------------------------------------------------------------
# Preflight: refuse before touching anything
# ---------------------------------------------------------------------------
# Every refusal here happens before the first state change, so a failed
# preflight leaves the running release exactly as it was.
require_digest_ref() {
	local ref="$1"
	[[ -n "$ref" ]] || fail "an image reference is required"
	case "$ref" in
	*"@sha256:"*)
		local digest="${ref##*@}"
		[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] ||
			fail "the digest of ${ref%%@*} is malformed; expected sha256 followed by 64 hex characters"
		;;
	*)
		fail "the reference '${ref}' is not pinned by digest; a tag is mutable and is refused (want <repository>@sha256:<digest>)"
		;;
	esac
}

# The image must exist and its own RepoDigests must contain the reference:
# "the registry has it" is what makes the promotion reproducible on another
# host, and checking it here is the difference between deploying and hoping.
require_image_pulled() {
	local ref="$1"
	if ! docker image inspect "$ref" >/dev/null 2>&1; then
		log "pulling $(printf '%s' "$ref" | sed 's/@sha256:.*//')"
		docker pull "$ref" >/dev/null || fail "the image $ref could not be pulled"
	fi
	docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$ref" |
		grep -Fxq "$ref" || fail "the image $ref does not report that digest"
}

# The compose document is the operator's; this checks that it will actually run
# the promised artifact and that it is valid, without printing it (by now the
# env file has been inlined, so the rendered document carries credentials).
require_compose_runs_the_artifact() {
	local ref="$1" images
	compose config --quiet || fail "docker compose refused $COMPOSE_FILE"

	# Rendered the way the promote renders it: the image is a variable of the
	# document, and the override is exactly what the run steps apply. The check
	# is that the document is valid under that override and that the promoted
	# digest is one of the images it would run.
	images="$(COMPOSE_ARENA_IMAGE="$ref" compose config --images)" ||
		fail "the compose document could not be rendered with $ref"
	printf '%s\n' "$images" | grep -Fxq "$ref" ||
		fail "the compose document does not run $ref; the application image is the COMPOSE_ARENA_IMAGE variable and the promoted digest must reach it"
}

# The database must already be up: the expand step migrates a database that
# exists, and a pipeline that starts a database on the way to production is a
# pipeline that can create an empty one.
require_database_ready() {
	compose ps --status running --quiet db | grep -q . ||
		fail "the database service is not running; a deployment migrates an existing database, it does not create one"
	compose exec -T db pg_isready --username arena --dbname arena >/dev/null 2>&1 ||
		fail "the database is not accepting connections"
}

# ---------------------------------------------------------------------------
# The steps
# ---------------------------------------------------------------------------
# Expand, then promote. `migrate up` is the only migration this script knows:
# there is no down path to forget to avoid.
migrate_forward() {
	local ref="$1"
	log "applying forward migrations with the incoming image"
	COMPOSE_ARENA_IMAGE="$ref" compose run --rm --no-deps app migrate up >/dev/null ||
		fail "the expand migration failed; nothing was promoted"
}

# Promote replaces the two processes that run the release. `--no-deps` is
# deliberate: the database is already running and is not part of a release.
promote_processes() {
	local ref="$1"
	log "promoting the application and the worker to the incoming digest"
	COMPOSE_ARENA_IMAGE="$ref" compose up --detach --no-deps app worker >/dev/null ||
		fail "docker compose could not recreate the application"
}

# One path, one status, through the public surface. Nothing here treats a
# redirect as success: a promotion that answers 30x at the readiness path is a
# promotion whose readiness nobody checked, so 200 is the expectation and 200
# is the only value that passes.
probe_status() {
	curl --silent --output /dev/null --write-out '%{http_code}' --max-time 10 \
		"${CURL_ARGS[@]}" "${HEALTH_URL}$1" 2>/dev/null || true
}

# The health probe is the release gate: it runs against the public surface, and
# it is the only thing that decides whether the promotion stands.
wait_healthy() {
	local deadline=$((SECONDS + TIMEOUT)) status
	while ((SECONDS < deadline)); do
		status="$(probe_status "$HEALTH_PATH")"
		[[ "$status" == "200" ]] && return 0
		sleep 2
	done
	PROMOTION_FAILURE="the health probe never answered 200 at ${HEALTH_URL}${HEALTH_PATH} (last status: ${status:-none})"
	return 1
}

# Ready is not the same claim as serving. A release can report itself ready and
# still have lost a page, and that release would be recorded here as promoted if
# nothing fetched one.
smoke_paths() {
	local path status
	for path in "${SMOKE_PATHS[@]}"; do
		status="$(probe_status "$path")"
		if [[ "$status" != "200" ]]; then
			PROMOTION_FAILURE="the smoke probe answered ${status:-nothing} at ${HEALTH_URL}${path}, want 200"
			return 1
		fi
	done
	return 0
}

# What a promotion is, stated once and in one place: the release is ready, it
# serves its pages, and the running container is the digest that was promoted.
# The three are one claim, so they have one function — `deploy` and `rollback`
# call it, and neither can decide to skip a part of it.
prove_promotion() {
	local ref="$1" running
	PROMOTION_FAILURE=""
	wait_healthy || return 1
	smoke_paths || return 1
	running="$(running_application_image || true)"
	if [[ "$running" != "$ref" ]]; then
		PROMOTION_FAILURE="the running container reports ${running:-no image}, not the promoted $ref"
		return 1
	fi
	return 0
}

# What is actually running, as the container reports it. The state file is the
# operator's bookkeeping; this is the fact it is supposed to describe, and the
# two are compared so a stale state file cannot silently become a wrong
# rollback target.
running_application_image() {
	local container
	container="$(compose ps --quiet app)" || return 1
	[[ -n "$container" ]] || return 1
	docker inspect --format '{{.Config.Image}}' "$container"
}

running_application_version() {
	compose exec -T app /arena version 2>/dev/null | sed -n 's/^arena version //p' | head -1
}

# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------
confirm() {
	[[ "$ASSUME_YES" == "yes" ]] && return 0
	[[ -t 0 ]] || fail "refusing to $command without a terminal; pass --yes to state the intent explicitly"
	printf 'deploy: %s %s? [y/N] ' "$command" "${1:-}" >&2
	local answer
	read -r answer
	case "$answer" in
	y | Y | yes | YES) return 0 ;;
	*) fail "aborted by the operator" ;;
	esac
}

cmd_deploy() {
	[[ -n "$IMAGE" ]] || fail "deploy requires --image REF"
	require_digest_ref "$IMAGE"
	confirm "$IMAGE"

	local current previous
	current="$(state_current)"
	previous="$(state_previous)"

	require_image_pulled "$IMAGE"
	require_compose_runs_the_artifact "$IMAGE"
	require_database_ready

	if [[ "$current" == "$IMAGE" ]]; then
		log "the recorded release is already $IMAGE; nothing to promote"
		# "Already deployed" is a claim about the release, so it is proved like
		# any other instead of being asserted from the state file: a state file
		# can be older than the containers it describes.
		prove_promotion "$IMAGE" ||
			fail "the recorded release $IMAGE does not prove itself: ${PROMOTION_FAILURE}"
		return 0
	fi

	# The state file must describe the running container, or the rollback
	# target it holds is a guess.
	local running
	running="$(running_application_image || true)"
	if [[ -n "$current" && -n "$running" && "$current" != "$running" ]]; then
		fail "the state file says $current but the application is running $running; reconcile the state before deploying"
	fi
	if [[ -z "$current" && -n "$running" ]]; then
		log "adopting the running container as the current release"
		current="$running"
	fi

	migrate_forward "$IMAGE"
	promote_processes "$IMAGE"

	if ! prove_promotion "$IMAGE"; then
		# The reason is kept before the rollback, because proving the rollback
		# overwrites it with a reason of its own.
		local reason="$PROMOTION_FAILURE"
		if [[ -n "$current" ]]; then
			log "the incoming release was not promoted: ${reason}"
			log "restoring the previous digest $current"
			promote_processes "$current" || fail "the automatic rollback could not promote $current; the surface may be down and needs an operator"
			prove_promotion "$current" || fail "the rollback promoted $current but ${PROMOTION_FAILURE}; this is an incident, not a deployment"
			# The schema is where the failed release left it: forward only.
			write_state "$current" "$previous"
			fail "the release $IMAGE was not promoted: ${reason}. The previous digest was restored"
		fi
		fail "the release $IMAGE was not promoted: ${reason}. There is no previous digest to restore"
	fi

	local version
	version="$(running_application_version || true)"
	log "the release is live: ready, serving its pages, and running the promoted digest${version:+ (${version})}"
	write_state "$IMAGE" "$current"
	log "promoted ${IMAGE}"
}

cmd_rollback() {
	local current previous
	current="$(state_current)"
	previous="$(state_previous)"
	[[ -n "$previous" ]] || fail "there is no previous digest recorded in $STATE_FILE; nothing to roll back to"
	[[ "$previous" != "$current" ]] || fail "the previous digest is the current digest; nothing to roll back to"
	confirm "$previous"

	require_image_pulled "$previous"
	require_compose_runs_the_artifact "$previous"
	require_database_ready

	# A rollback moves the processes, never the schema: the expand step that
	# ran with the newer release stays applied, which is why release N-1 must
	# still run against it (docs/DEPLOYMENT.md §5).
	log "rolling the application and the worker back to the previous digest"
	promote_processes "$previous"
	prove_promotion "$previous" || fail "the rollback promoted $previous but ${PROMOTION_FAILURE}; the surface needs an operator"
	write_state "$previous" "$current"
	log "rolled back to ${previous}"
}

cmd_status() {
	local current previous
	current="$(state_current)"
	previous="$(state_previous)"
	printf 'project:  %s\n' "$PROJECT"
	printf 'current:  %s\n' "${current:-unknown}"
	printf 'previous: %s\n' "${previous:-none}"
	local running
	running="$(running_application_image || true)"
	printf 'running:  %s\n' "${running:-none}"
	printf 'state:    %s\n' "$STATE_FILE"
}

case "$command" in
deploy) cmd_deploy ;;
rollback) cmd_rollback ;;
status) cmd_status ;;
esac

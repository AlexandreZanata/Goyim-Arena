#!/usr/bin/env bash
#
# Harness of the browser journeys (P18-T07).
#
# Why the composition is here and not in `playwright.config.js`: the journeys
# drive the delivered process, and a runner that started its own server would
# hide exactly the thing under test — the binary as it boots, against a real
# PostgreSQL, serving the build it references. This script composes all of that
# and names it in the environment; the runner only drives the pages.
#
# What one run owns, and therefore tears down at the end:
#
#   - a disposable database, created through `tools/e2e/database` and dropped
#     on every exit path, so a journey never sees the leftovers of the one
#     before it and never leaves anything behind;
#   - a directory the local email sink writes to, which is how the account
#     journey reads the code the message carries although the message is sent
#     by another process;
#   - the server itself, which is killed and waited for, so the next run can
#     bind the same address.
#
# It is fail-closed: a missing command, a database that cannot be provisioned,
# a server that never reports itself ready or a red journey all abort with a
# non-zero status, and the teardown happens anyway.
#
# Requirements: Go, Node and npm (the runner is pinned in package.json), a
# reachable PostgreSQL named by ARENA_DATABASE_URL, and a frontend build
# (`make build-web`, which `make test-e2e` depends on).
#
# Any argument is passed through to `npx playwright test`, so a single journey
# can be run with, for example: tools/e2e/harness.sh specs/account.spec.js
set -euo pipefail

HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HARNESS_DIR/../.." && pwd)"

log() { printf 'e2e: %s\n' "$*" >&2; }
fail() { printf 'e2e: %s\n' "$*" >&2; exit 1; }

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		fail "$1 is required and was not found in PATH"
	fi
}
require_command go
require_command node
require_command npm
require_command curl

if [ -z "${ARENA_DATABASE_URL:-}" ]; then
	fail "ARENA_DATABASE_URL is required: it names the administrative DSN the disposable database of the run is created beside (for example postgres://arena:arena-local-dev@127.0.0.1:54329/arena?sslmode=disable)"
fi

ASSETS_DIR="${ARENA_E2E_ASSETS_DIR:-${ARENA_ASSETS_DIR:-$ROOT/web/dist}}"
if [ ! -f "$ASSETS_DIR/manifest.json" ]; then
	fail "no build was found at $ASSETS_DIR (manifest.json is missing): run 'make build-web' or point ARENA_E2E_ASSETS_DIR at an existing build"
fi

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/arena-e2e-XXXXXX")"
BIN_DIR="$WORK_DIR/bin"
SINK_DIR="$WORK_DIR/email-sink"
SERVER_LOG="$WORK_DIR/server.log"
SERVER_PID=""
RUN_ID="$(node -e 'process.stdout.write(require("node:crypto").randomBytes(4).toString("hex"))')"
DB_NAME="arena_e2e_${RUN_ID}"

mkdir -p "$BIN_DIR" "$SINK_DIR"

# cleanup removes everything one run owns. It runs on every exit path, including
# a red journey, and it never fails the shell: a teardown that could not finish
# reports itself and lets the original status stand.
cleanup() {
	if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	if [ -x "$BIN_DIR/e2e-database" ]; then
		if ! "$BIN_DIR/e2e-database" drop --dsn "$ARENA_DATABASE_URL" --name "$DB_NAME" >/dev/null 2>&1; then
			log "the disposable database $DB_NAME could not be removed; drop it by hand"
		fi
	fi
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

# The port is asked of the kernel and then released: an address that is free at
# the moment the server binds it is what the run needs, and a fixed port would
# collide with a second check-out or a running development server.
if [ -n "${ARENA_E2E_PORT:-}" ]; then
	PORT="$ARENA_E2E_PORT"
else
	PORT="$(node -e '
		const net = require("node:net");
		const probe = net.createServer();
		probe.listen(0, "127.0.0.1", () => {
			const { port } = probe.address();
			probe.close(() => process.stdout.write(String(port)));
		});
	')"
fi
BASE_URL="http://127.0.0.1:$PORT"

# The pseudo-locale of the layout gate (P18-T10) is registered only in a build
# that asks for it, and this is that build: the tag adds a derived catalog, so
# the journeys can drive the pages with elongated, accented text while the
# delivered binary keeps the locale out of its allowlist (proved by
# `go test ./internal/i18n/...`, which runs without the tag in `make verify`).
PSEUDO_LOCALE="qps-Ploc"

log "building the server with the pseudo-locale, the seed and the database tooling (run $RUN_ID)"
(cd "$ROOT" && go build -tags pseudolocale -o "$BIN_DIR/arena" ./cmd/arena)
(cd "$ROOT" && go build -o "$BIN_DIR/e2e-seed" ./tools/e2e/seed)
(cd "$ROOT" && go build -o "$BIN_DIR/e2e-database" ./tools/e2e/database)

log "provisioning the disposable database $DB_NAME"
DATABASE_DOCUMENT="$("$BIN_DIR/e2e-database" create --dsn "$ARENA_DATABASE_URL" --name "$DB_NAME")"
DATABASE_DSN="$(node -e 'process.stdout.write(JSON.parse(process.argv[1]).dsn)' "$DATABASE_DOCUMENT")"

# The signing secret of the pagination cursors is generated per run: it is a
# development-only value, and a secret that is committed or shared between runs
# is exactly what the environment rule forbids.
CURSOR_SECRET="$(node -e 'process.stdout.write(require("node:crypto").randomBytes(48).toString("hex"))')"

# Synthetic identities of the run: every address carries the run identifier, so
# a message left in a sink directory can never be confused with another run's,
# and nothing here is a real person.
PARTICIPANT_EMAIL="e2e-participant-${RUN_ID}@example.test"
PARTNER_EMAIL="e2e-partner-${RUN_ID}@example.test"
CREATOR_EMAIL="e2e-creator-${RUN_ID}@example.test"
PASSWORD="correct horse battery staple"
ARENA_SLUG="e2e-arena-${RUN_ID}"

# scrubbed runs one command with an environment this script composed: the
# configuration of the process and the seed is strict about unknown ARENA_*
# variables (a typo is refused instead of ignored), and inheriting whatever the
# caller exported would make a run depend on the shell it was started from — the
# harness's own knobs (ARENA_E2E_*) are not configuration of the application.
scrubbed() {
	env -i \
		PATH="$PATH" \
		HOME="${HOME:-$WORK_DIR}" \
		TMPDIR="${TMPDIR:-/tmp}" \
		"$@"
}

log "seeding two confirmed accounts with INK and one published Arena"
scrubbed ARENA_ENV=test ARENA_DATABASE_URL="$DATABASE_DSN" "$BIN_DIR/e2e-seed" account \
	--email "$PARTICIPANT_EMAIL" --password "$PASSWORD" --ink 1000 >/dev/null
scrubbed ARENA_ENV=test ARENA_DATABASE_URL="$DATABASE_DSN" "$BIN_DIR/e2e-seed" account \
	--email "$PARTNER_EMAIL" --password "$PASSWORD" --ink 1000 >/dev/null
scrubbed ARENA_ENV=test ARENA_DATABASE_URL="$DATABASE_DSN" "$BIN_DIR/e2e-seed" arena \
	--slug "$ARENA_SLUG" --creator-email "$CREATOR_EMAIL" >/dev/null

log "starting the server on $BASE_URL"
scrubbed ARENA_ENV=test \
	ARENA_ADDR="127.0.0.1:$PORT" \
	ARENA_DATABASE_URL="$DATABASE_DSN" \
	ARENA_ASSETS_DIR="$ASSETS_DIR" \
	ARENA_EMAIL_SINK_DIR="$SINK_DIR" \
	ARENA_CURSOR_SECRET="$CURSOR_SECRET" \
	ARENA_LOG_LEVEL="${ARENA_E2E_LOG_LEVEL:-info}" \
	"$BIN_DIR/arena" server >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

ready=0
for _ in $(seq 1 150); do
	if ! kill -0 "$SERVER_PID" 2>/dev/null; then
		tail -n 40 "$SERVER_LOG" >&2
		fail "the server exited before it reported itself ready"
	fi
	if curl --silent --fail --max-time 2 "$BASE_URL/health/ready" >/dev/null; then
		ready=1
		break
	fi
	sleep 0.2
done
if [ "$ready" -ne 1 ]; then
	tail -n 40 "$SERVER_LOG" >&2
	fail "the server did not become ready at $BASE_URL/health/ready within 30s"
fi

# The tagged build must really serve the pseudo-locale: if it did not, every
# assertion of the layout gate would compare the default locale against itself
# and the run would be green for the wrong reason.
pseudo_page="$(curl --silent --max-time 5 -H "Accept-Language: $PSEUDO_LOCALE" "$BASE_URL/login")"
case "$pseudo_page" in
*"⟦"*) ;;
*)
	fail "the build with the pseudolocale tag did not serve the pseudo-locale at /login: the layout gate cannot run"
	;;
esac

cd "$HARNESS_DIR"

if [ ! -d node_modules ]; then
	log "installing the pinned runner"
	npm ci --no-audit --no-fund
fi

# The browser build the pinned runner expects. It is a no-op when it is already
# in the cache, and it is never installed with system dependencies: that is a
# machine-level change, and it belongs to whoever prepares the machine.
log "ensuring the chromium build of the pinned runner is installed"
npx playwright install chromium

log "running the journeys"
export ARENA_E2E_BASE_URL="$BASE_URL"
export ARENA_E2E_PSEUDO_LOCALE="$PSEUDO_LOCALE"
export ARENA_EMAIL_SINK_DIR="$SINK_DIR"
export ARENA_E2E_RUN_ID="$RUN_ID"
export ARENA_E2E_ARENA_SLUG="$ARENA_SLUG"
export ARENA_E2E_PARTICIPANT_EMAIL="$PARTICIPANT_EMAIL"
export ARENA_E2E_PARTICIPANT_PASSWORD="$PASSWORD"
export ARENA_E2E_PARTNER_EMAIL="$PARTNER_EMAIL"
export ARENA_E2E_PARTNER_PASSWORD="$PASSWORD"

status=0
npx playwright test "$@" || status=$?

if [ "$status" -ne 0 ]; then
	log "the journeys failed; the last lines the server logged:"
	tail -n 40 "$SERVER_LOG" >&2
fi
exit "$status"

#!/usr/bin/env bash
#
# The gate of the internationalization audit (P20-T09), invoked by
# `make i18n-audit` inside `make verify`.
#
# It does two things, in this order and for different reasons:
#
#   1. it executes the areas of the audit that need no browser, with the same
#      commands the register records, and refuses a red one. The commands are
#      the delivered gates, so this is the same execution the rest of `verify`
#      performs — the Go test cache makes the repeat nearly free, and what the
#      repetition buys is that "the area was executed" is decided here and not
#      remembered in a document;
#   2. it judges the register against the tree: the numbers it claims, the areas
#      it names, the findings it records and the human review it carries.
#
# The browser journeys are the one area this script does not execute. They need
# a Chromium build and a disposable PostgreSQL, which `tools/e2e/harness.sh`
# composes and CI runs as its own job (`make test-e2e`); the audit proves they
# carry the locale dimension by measuring the module the suite reads it from,
# and the register records the execution with its date.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

log() { printf 'i18n-audit: %s\n' "$*" >&2; }
fail() { printf 'i18n-audit: %s\n' "$*" >&2; exit 1; }

run() {
	local label="$1"
	shift
	log "$label"
	"$@" >&2 || fail "$label answered non-zero"
}

run "catalog coverage (generate-check)" make generate-check
run "pseudo-locale build (pseudolocale tag)" go test -tags pseudolocale ./internal/i18n/...
run "email snapshots (renderer)" go test ./internal/notifications/adapters/renderer/...
run "emails (notifications)" go test ./internal/notifications/...
run "problem details (httperror)" go test ./internal/platform/httperror/...
run "seo and cache (arenas html)" go test ./internal/arenas/adapters/html/...
run "money, plural and time (web)" make test-web
run "hardcoded text (audit-i18n)" make audit-i18n

log "judging the register against the tree"
go run ./tools/i18nrelease -check -root . -document docs/I18N_AUDIT.md >&2 ||
	fail "the register and the tree disagree: fix the cause, never the register"

log "ok"

#!/usr/bin/env bash
#
# Isolation gate of the browser tooling (P18-T07).
#
# The claim this gate defends: Playwright is test tooling and nothing of it is
# delivered. It is a claim about absence, so it can only be checked by looking
# at the artefacts a user would receive — the frontend package, the build the
# pages reference and the Go binary — and by looking at what the delivered code
# is allowed to mention.
#
# Five questions, each with the artefact it interrogates:
#
#   1. Does the frontend package depend on anything but the pinned TypeScript
#      compiler?      (web/package.json)
#   2. Does the delivered dependency graph mention the runner at all?
#                   (web/package-lock.json, go.mod, go.sum)
#   3. Does delivered code reference the tooling or the runner?
#                   (web/src, web/tests, internal/, cmd/)
#   4. Does the build the pages reference carry any of it, by name or by
#      content?      (web/dist)
#   5. Does the binary link the tooling, and is the runner pinned to an exact
#      version that its own lock file agrees with?  (go list -deps, tools/e2e)
#
# It fails closed: a missing artefact is a failure, not a skip, because an
# inspection that cannot see the thing it is inspecting proves nothing.
set -euo pipefail

TOOLING_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$TOOLING_DIR/../.." && pwd)"

fail() {
	printf 'isolation-check: %s\n' "$*" >&2
	exit 1
}
ok() { printf 'isolation-check: %s\n' "$*"; }

containing() {
	# containing <pattern> <path...> prints the files that match, and nothing
	# when none does. Binary files are skipped: the question is what the
	# delivered artefacts say, and a byte that happens to spell a name inside
	# a font is not a dependency.
	grep -rl --binary-files=without-match -- "$1" "${@:2}" 2>/dev/null || true
}

DELIVERED_SOURCES=()
for candidate in "$ROOT/web/src" "$ROOT/web/tests" "$ROOT/internal" "$ROOT/cmd"; do
	[ -d "$candidate" ] && DELIVERED_SOURCES+=("$candidate")
done

importing() {
	# importing <module-fragment> prints the delivered files whose *quoted*
	# module specifiers reach that fragment — a Go import path or a
	# JavaScript import.
	#
	# Quoted, and not merely mentioned, on purpose: a comment that names the
	# directory is documentation, while a specifier is what makes the tooling
	# load-bearing. The rule of the phase is that the tooling is never
	# imported by delivered code, and this is the shape of an import.
	local fragment="$1"
	local pattern
	for pattern in "\"[^\"]*${fragment}" "'[^']*${fragment}"; do
		grep -rl --binary-files=without-match -E -- "$pattern" "${DELIVERED_SOURCES[@]}" 2>/dev/null || true
	done
}

# 1. The frontend package has exactly one dependency, and it is the compiler.
FRONTEND_PACKAGES="$(node -e '
	const fs = require("node:fs");
	const manifest = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
	const names = [
		...Object.keys(manifest.dependencies ?? {}),
		...Object.keys(manifest.devDependencies ?? {}),
	];
	process.stdout.write(names.sort().join(" "));
' "$ROOT/web/package.json")"
if [ "$FRONTEND_PACKAGES" != "typescript" ]; then
	fail "web/package.json must depend on nothing but typescript, and it declares: ${FRONTEND_PACKAGES:-nothing}"
fi
ok "the frontend package depends only on the compiler"

# 2. Nothing in the delivered dependency graph mentions the runner.
GRAPH_FILES=()
for candidate in "$ROOT/web/package-lock.json" "$ROOT/go.mod" "$ROOT/go.sum"; do
	[ -f "$candidate" ] && GRAPH_FILES+=("$candidate")
done
if [ "${#GRAPH_FILES[@]}" -eq 0 ]; then
	fail "no dependency manifest was found to inspect"
fi
MENTIONED_IN_GRAPH="$(containing playwright "${GRAPH_FILES[@]}")"
if [ -n "$MENTIONED_IN_GRAPH" ]; then
	fail "the delivered dependency graph mentions the runner: $MENTIONED_IN_GRAPH"
fi
ok "the delivered dependency graph is free of the runner"

# 3. Delivered code never imports the tooling. The tooling is a consumer of
# the repository, never a part of it: a specifier that reaches `tools/e2e`
# from the server or from the frontend would make the test tooling
# load-bearing in what is delivered.
if [ "${#DELIVERED_SOURCES[@]}" -eq 0 ]; then
	fail "no delivered source directory was found to inspect"
fi
IMPORTING_RUNNER="$(importing '@playwright')"
if [ -n "$IMPORTING_RUNNER" ]; then
	fail "delivered source imports the runner: $IMPORTING_RUNNER"
fi
IMPORTING_TOOLING="$(importing 'tools/e2e')"
if [ -n "$IMPORTING_TOOLING" ]; then
	fail "delivered source imports the browser tooling: $IMPORTING_TOOLING"
fi
ok "delivered source does not import the tooling or the runner"

# 4. The build the pages reference carries none of it.
if [ ! -f "$ROOT/web/dist/manifest.json" ]; then
	fail "there is no build to inspect at web/dist: run 'make build-web' first (make test-e2e does it)"
fi
BY_NAME="$(find "$ROOT/web/dist" -iname '*playwright*' -print)"
if [ -n "$BY_NAME" ]; then
	fail "the delivered build carries a file named after the runner: $BY_NAME"
fi
BY_CONTENT="$(containing playwright "$ROOT/web/dist")"
if [ -n "$BY_CONTENT" ]; then
	fail "the delivered build mentions the runner: $BY_CONTENT"
fi
ok "the delivered build is free of the runner"

# 5. The delivered binary does not link the tooling, and the runner is pinned.
MODULE="$(awk '/^module /{print $2; exit}' "$ROOT/go.mod")"
if [ -z "$MODULE" ]; then
	fail "the module path could not be read from go.mod"
fi
if (cd "$ROOT" && go list -deps ./cmd/arena) | grep -q -- "$MODULE/tools/e2e"; then
	fail "the delivered binary links the browser tooling"
fi
ok "the delivered binary does not link the browser tooling"

PINNED="$(node -e '
	const fs = require("node:fs");
	const [manifestPath, lockPath] = process.argv.slice(1);
	const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
	const lock = JSON.parse(fs.readFileSync(lockPath, "utf8"));

	if (manifest.private !== true) {
		throw new Error("tools/e2e/package.json must be private: it is never published and never installed as a dependency");
	}

	const pinned = manifest.devDependencies?.["@playwright/test"];
	if (pinned === undefined) {
		throw new Error("tools/e2e/package.json must declare @playwright/test");
	}
	// An exact version, never a range: a runner that resolves to whatever is
	// newest is a runner whose behaviour changes without a reviewed commit.
	if (!/^\d+\.\d+\.\d+$/.test(pinned)) {
		throw new Error(`@playwright/test must be pinned to an exact version, and it is ${pinned}`);
	}

	const lockedRoot = lock.packages?.[""]?.devDependencies?.["@playwright/test"];
	const lockedPackage = lock.packages?.["node_modules/@playwright/test"]?.version;
	if (lockedRoot !== pinned || lockedPackage !== pinned) {
		throw new Error(`tools/e2e/package-lock.json disagrees with the pinned ${pinned} (root ${lockedRoot}, package ${lockedPackage})`);
	}

	process.stdout.write(pinned);
' "$TOOLING_DIR/package.json" "$TOOLING_DIR/package-lock.json")"
ok "the runner is pinned to $PINNED and its lock file agrees"

ok "ok"

#!/usr/bin/env bash
#
# Verification of the production image (P19-T01).
#
# Why the composition lives here and not in the Makefile: the gate is not "the
# build succeeded". It is a claim about a container — that it starts with a
# read-only filesystem, runs as a non-root user, applies its own embedded
# migrations, serves a page and the hashed asset that page references, and
# carries no toolchain, cache, source or credential. Each of those is a
# question asked of the running thing, and a Makefile recipe that only built an
# image would answer none of them.
#
# What one run owns, and therefore tears down on every exit path: a Docker
# network, a throwaway PostgreSQL, the application container, and the work
# directory that holds the artifacts handed to `tools/imageaudit`.
#
# It is fail-closed: a missing command, an image that does not build, a
# database that never becomes ready, a container that never answers, a page or
# asset that is not served, or a broken audit rule all abort with a non-zero
# status. The image itself is kept, because the scan of `make image-scan` and
# the eyes of a person both want to look at it afterwards.
#
# Requirements: Docker with a reachable daemon, Go, curl.
#
# Environment:
#   ARENA_IMAGE            tag the image is built and audited under
#                          (default goyim-arena:verify)
#   ARENA_POSTGRES_IMAGE   PostgreSQL the smoke runs against
#                          (default postgres:18.4, the image compose.yaml uses)
set -euo pipefail

TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$TOOL_DIR/../.." && pwd)"

IMAGE="${ARENA_IMAGE:-goyim-arena:verify}"
POSTGRES_IMAGE="${ARENA_POSTGRES_IMAGE:-postgres:18.4}"
RUN_ID="$$"
CONTAINER="goyim-arena-image-verify-${RUN_ID}"
DATABASE="goyim-arena-image-verify-db-${RUN_ID}"
NETWORK="goyim-arena-image-verify-net-${RUN_ID}"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/arena-image-XXXXXX")"

log() { printf 'image-verify: %s\n' "$*" >&2; }
fail() { printf 'image-verify: %s\n' "$*" >&2; exit 1; }

cleanup() {
	docker rm --force "$CONTAINER" >/dev/null 2>&1 || true
	docker rm --force "$DATABASE" >/dev/null 2>&1 || true
	docker network rm "$NETWORK" >/dev/null 2>&1 || true
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		fail "$1 is required and was not found in PATH"
	fi
}
require_command docker
require_command curl
require_command go

if ! docker info >/dev/null 2>&1; then
	fail "the Docker daemon is not reachable"
fi

# ---------------------------------------------------------------------------
# 1. Build. The recipe pins every base by digest, so this resolves the same
#    bytes on any machine that runs it.
# ---------------------------------------------------------------------------
log "building $IMAGE from $ROOT/Dockerfile"
docker build --file "$ROOT/Dockerfile" --tag "$IMAGE" "$ROOT" >&2

image_id="$(docker image inspect --format '{{.Id}}' "$IMAGE")"
log "built $image_id"

# ---------------------------------------------------------------------------
# 2. A throwaway PostgreSQL. The image is expected to apply its own migrations,
#    so the smoke gives it an empty database and asks it to.
# ---------------------------------------------------------------------------
docker network create "$NETWORK" >/dev/null
log "starting $POSTGRES_IMAGE on $NETWORK"
docker run --detach \
	--name "$DATABASE" \
	--network "$NETWORK" \
	--env POSTGRES_USER=arena \
	--env POSTGRES_PASSWORD=arena-local-dev \
	--env POSTGRES_DB=arena \
	--env POSTGRES_INITDB_ARGS="--encoding=UTF8 --locale=C" \
	"$POSTGRES_IMAGE" >/dev/null

for attempt in $(seq 1 60); do
	if docker exec "$DATABASE" pg_isready --username arena --dbname arena >/dev/null 2>&1; then
		break
	fi
	if [ "$attempt" = 60 ]; then
		docker logs "$DATABASE" >&2 || true
		fail "the database never became ready"
	fi
	sleep 1
done

# The DSN is the development one compose.yaml documents; it is a container on a
# network that exists for the length of this run, never a real credential.
DSN="postgres://arena:arena-local-dev@${DATABASE}:5432/arena?sslmode=disable"

# ---------------------------------------------------------------------------
# 3. The migrations of the image itself. `arena migrate up` runs the sources
#    embedded in the binary, so this also proves the image is self-sufficient:
#    nothing outside the container has to be mounted for the schema to exist.
# ---------------------------------------------------------------------------
log "applying the migrations embedded in the image"
docker run --rm \
	--network "$NETWORK" \
	--env ARENA_ENV=development \
	--env "ARENA_DATABASE_URL=$DSN" \
	"$IMAGE" migrate up >&2

# ---------------------------------------------------------------------------
# 4. The container under test: read-only, non-root, on the network, publishing
#    a loopback port only. No cursor secret, so the participation journey is
#    not mounted — this smoke is about the image, not about every journey.
# ---------------------------------------------------------------------------
log "starting $CONTAINER read-only"
docker run --detach \
	--name "$CONTAINER" \
	--network "$NETWORK" \
	--read-only \
	--env ARENA_ENV=development \
	--env "ARENA_DATABASE_URL=$DSN" \
	--publish 127.0.0.1::8080 \
	"$IMAGE" >/dev/null

published="$(docker port "$CONTAINER" 8080/tcp | head -1)"
[ -n "$published" ] || fail "the container published no port"
base="http://127.0.0.1:${published##*:}"

readonly_rootfs="$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$CONTAINER")"
[ "$readonly_rootfs" = "true" ] || fail "the container did not start with a read-only filesystem (ReadonlyRootfs=$readonly_rootfs)"

# ---------------------------------------------------------------------------
# 5. Smoke. A page and an asset, not just the liveness probe: the image bakes a
#    build in, and an image whose build is missing serves a page that loads
#    nothing.
# ---------------------------------------------------------------------------
fetch_status() {
	curl --silent --output "$WORK_DIR/body" --write-out '%{http_code}' --max-time 5 "$1" || true
}

health=""
for attempt in $(seq 1 30); do
	health="$(fetch_status "$base/health/live")"
	if [ "$health" = "200" ] && grep -q '"status":"live"' "$WORK_DIR/body"; then
		break
	fi
	if [ "$attempt" = 30 ]; then
		docker logs "$CONTAINER" >&2 || true
		fail "the container never answered /health/live (last status $health)"
	fi
	sleep 1
done
log "the container answers /health/live with 200 live"

login_status="$(fetch_status "$base/login")"
[ "$login_status" = "200" ] || fail "/login answered $login_status, want 200"
login_type="$(curl --silent --output /dev/null --write-out '%{content_type}' --max-time 5 "$base/login")"
case "$login_type" in
text/html*) ;;
*) fail "/login answered $login_type, want text/html" ;;
esac
grep -q 'lang="' "$WORK_DIR/body" || fail "the page served by the image declares no language"
log "the container serves /login as $login_type"

# One address of the build the image ships, read from the manifest inside the
# container's own filesystem — never from the repository's working copy, which
# would prove nothing about what is in the image.
docker export "$CONTAINER" >"$WORK_DIR/rootfs.tar"
asset_path="$(tar -xOf "$WORK_DIR/rootfs.tar" web/dist/manifest.json | tr ',' '\n' | grep -o '/assets/[^"]*' | head -1)"
[ -n "$asset_path" ] || fail "the manifest inside the image declares no asset to request"

asset_status="$(fetch_status "$base$asset_path")"
[ "$asset_status" = "200" ] || fail "$asset_path answered $asset_status, want 200"
asset_type="$(curl --silent --output /dev/null --write-out '%{content_type}' --max-time 5 "$base$asset_path")"
case "$asset_type" in
text/javascript* | text/css*) ;;
*) fail "$asset_path answered $asset_type, want a stylesheet or an ES module" ;;
esac
log "the container serves $asset_path as $asset_type"

undeclared_status="$(fetch_status "$base/assets/pages/undeclared-000000000000.js")"
[ "$undeclared_status" = "404" ] || fail "an address the manifest never declared answered $undeclared_status, want 404"

# ---------------------------------------------------------------------------
# 6. The audit: the recipe, the build context and the artifact. The config and
#    the filesystem come from the container that was just smoke-tested, so the
#    verdict is about the thing that ran.
# ---------------------------------------------------------------------------
docker inspect "$CONTAINER" >"$WORK_DIR/config.json"
log "auditing the recipe and the artifact"
(cd "$ROOT" && go run ./tools/imageaudit \
	-dockerfile "$ROOT/Dockerfile" \
	-dockerignore "$ROOT/.dockerignore" \
	-config "$WORK_DIR/config.json" \
	-rootfs "$WORK_DIR/rootfs.tar")

log "ok — the image builds from pinned bases and serves a page read-only as a non-root user"

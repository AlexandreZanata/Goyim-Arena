#!/usr/bin/env bash
#
# Verification of the Caddy origin configuration (P19-T03).
#
# Why the gate is a script and not only a file audit: the questions this file
# answers are questions about what happens on the wire. Which headers arrive at
# the application, which ones leave to the visitor, what a compressed response
# does to its validator, what a visitor sees when the application never
# answered, and whether a handshake with the wrong name is refused — none of
# them can be read out of a configuration file. The audit (`tools/caddyaudit`,
# run as a unit test inside `make verify`) reads the file and refuses a drifted
# rule; this script runs the real Caddy image the topology pins and asserts the
# behaviour the rules are about.
#
# What one run owns, and therefore tears down on every exit path: a throwaway
# certificate, a stub upstream built into a scratch image, a network of its own,
# and the containers that serve them.
#
# It is fail-closed: a missing command, a file that does not validate, a file
# that is not the file `caddy fmt` would write, a header that arrives changed, a
# response that names the software — on the proxied path or on the error
# response this process writes itself — a compression that silently reuses the
# validator of another representation, an admin path that answers through the
# site, or an error response without the policy all abort with a non-zero
# status.
#
# Requirements: Docker with a reachable daemon, Go, curl, openssl.
#
# Environment:
#   ARENA_CADDY_IMAGE   the Caddy image to run. Default: the digest pinned by
#                       compose.production.yaml, read from the file itself, so
#                       that the gate cannot pass on a version the deployment
#                       does not use.
set -euo pipefail

TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$TOOL_DIR/../.." && pwd)"
cd "$ROOT"

COMPOSE_FILE="compose.production.yaml"
CADDYFILE="deploy/caddy/Caddyfile"
SITE="arena.verify.invalid"
FORGED_IP="203.0.113.9"
RUN_ID="$$"
PROJECT="arena-caddyaudit-${RUN_ID}"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/arena-caddyaudit-XXXXXX")"

log() { printf 'caddy-verify: %s\n' "$*" >&2; }
fail() { printf 'caddy-verify: %s\n' "$*" >&2; exit 1; }

cleanup() {
	docker rm --force "${PROJECT}-caddy" "${PROJECT}-stub" >/dev/null 2>&1 || true
	docker network rm "${PROJECT}-net" >/dev/null 2>&1 || true
	docker image rm "${PROJECT}-stub" >/dev/null 2>&1 || true
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

# ---------------------------------------------------------------------------
# 1. The image the deployment pins, read from the file that pins it
# ---------------------------------------------------------------------------
# The gate runs the same digest the topology runs. A version installed on this
# machine would answer for a Caddy the deployment never starts, and the whole
# point of a pinned digest is that the answer cannot drift from the artifact.
if [[ -n "${ARENA_CADDY_IMAGE:-}" ]]; then
	CADDY_IMAGE="$ARENA_CADDY_IMAGE"
else
	CADDY_IMAGE="$(sed -nE 's/^[[:space:]]*image:[[:space:]]+(caddy:[^[:space:]]+).*/\1/p' "$COMPOSE_FILE" | head -1)"
	[[ -n "$CADDY_IMAGE" ]] || fail "compose.production.yaml pins no Caddy image"
fi
log "running $CADDY_IMAGE"

docker run --rm --entrypoint caddy "$CADDY_IMAGE" version >/dev/null ||
	fail "the pinned Caddy image is not available locally (docker pull it, or set ARENA_CADDY_IMAGE)"

# ---------------------------------------------------------------------------
# 2. A throwaway certificate, and the fixtures
# ---------------------------------------------------------------------------
openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
	-keyout "$WORK_DIR/origin.key" -out "$WORK_DIR/origin.crt" \
	-subj "/CN=${SITE}" -addext "subjectAltName=DNS:${SITE}" >/dev/null 2>&1 ||
	fail "openssl could not generate the throwaway origin certificate"

# The committed file, validated and formatted exactly as caddy would write it.
docker run --rm --entrypoint caddy \
	-v "$ROOT/$CADDYFILE:/etc/caddy/Caddyfile:ro" \
	-v "$WORK_DIR/origin.crt:/run/secrets/origin_certificate:ro" \
	-v "$WORK_DIR/origin.key:/run/secrets/origin_key:ro" \
	"$CADDY_IMAGE" validate --config /etc/caddy/Caddyfile >"$WORK_DIR/validate.log" 2>&1 ||
	{ cat "$WORK_DIR/validate.log" >&2; fail "caddy validate refused the committed file"; }
log "caddy validate: Valid configuration"

docker run --rm --entrypoint caddy \
	-v "$ROOT/$CADDYFILE:/etc/caddy/Caddyfile:ro" \
	"$CADDY_IMAGE" fmt /etc/caddy/Caddyfile >"$WORK_DIR/formatted" 2>/dev/null ||
	fail "caddy fmt refused the committed file"
if ! diff -q "$WORK_DIR/formatted" "$ROOT/$CADDYFILE" >/dev/null; then
	diff "$WORK_DIR/formatted" "$ROOT/$CADDYFILE" >&2 || true
	fail "the committed file is not the file caddy fmt writes; the formatter of a format is part of the format"
fi
log "caddy fmt: the committed file is already canonical"

# The positive control: the same file with the trust list replaced by every
# address. It exists so that "the forged header was ignored" is not vacuous —
# without it, a mechanism that was never wired would pass every assertion below.
#
# The substitution is asserted rather than assumed: a fixture that silently kept
# the original line would turn the control into a second copy of the file, and
# the control passing would then mean nothing at all.
sed -E 's|^([[:space:]]*)trusted_proxies static.*$|\1trusted_proxies static 0.0.0.0/0 ::/0|' \
	"$ROOT/$CADDYFILE" >"$WORK_DIR/Caddyfile.trusting"
[[ "$(grep -c 'trusted_proxies static 0.0.0.0/0 ::/0' "$WORK_DIR/Caddyfile.trusting")" == "1" ]] ||
	fail "the positive control did not replace the trust list, so it would measure nothing"
[[ "$(diff "$ROOT/$CADDYFILE" "$WORK_DIR/Caddyfile.trusting" | grep -c '^[<>]')" == "2" ]] ||
	fail "the positive control differs from the committed file in more than one line, so it is not the same file"
docker run --rm --entrypoint caddy \
	-v "$WORK_DIR/Caddyfile.trusting:/etc/caddy/Caddyfile:ro" \
	-v "$WORK_DIR/origin.crt:/run/secrets/origin_certificate:ro" \
	-v "$WORK_DIR/origin.key:/run/secrets/origin_key:ro" \
	"$CADDY_IMAGE" validate --config /etc/caddy/Caddyfile >/dev/null 2>&1 ||
	fail "the positive control does not validate, so it measures nothing"

# ---------------------------------------------------------------------------
# 3. The stub upstream, on a network of its own
# ---------------------------------------------------------------------------
# The stub runs in a container rather than on this host so that nothing in the
# run is published to a network outside it: the gate's only published ports are
# the ingress's own two, bound to loopback.
CGO_ENABLED=0 go build -o "$WORK_DIR/stub" ./tools/caddyaudit/stub ||
	fail "the stub upstream did not build"
cat >"$WORK_DIR/Dockerfile.stub" <<'EOF'
FROM scratch
COPY stub /stub
ENTRYPOINT ["/stub"]
EOF
docker build --quiet --tag "${PROJECT}-stub" --file "$WORK_DIR/Dockerfile.stub" "$WORK_DIR" >/dev/null ||
	fail "the stub image did not build"
docker network create "${PROJECT}-net" >/dev/null || fail "the throwaway network was not created"

start_stub() {
	docker run --detach --name "${PROJECT}-stub" --network "${PROJECT}-net" \
		--network-alias app "${PROJECT}-stub" --addr 0.0.0.0:8080 >/dev/null
}

# start_ingress <Caddyfile> — brings the ingress up with an ephemeral loopback
# port for each of its two listeners.
start_ingress() {
	local config="$1"
	docker run --detach --name "${PROJECT}-caddy" --network "${PROJECT}-net" \
		--publish 127.0.0.1::443 --publish 127.0.0.1::80 \
		--env "COMPOSE_SITE_ADDRESS=${SITE}" \
		--env "COMPOSE_APP_UPSTREAM=app:8080" \
		--mount "type=bind,src=${config},dst=/etc/caddy/Caddyfile,readonly" \
		--mount "type=bind,src=${WORK_DIR}/origin.crt,dst=/run/secrets/origin_certificate,readonly" \
		--mount "type=bind,src=${WORK_DIR}/origin.key,dst=/run/secrets/origin_key,readonly" \
		"$CADDY_IMAGE" >/dev/null

	# Readiness is asked of the process itself, over the admin endpoint on its
	# own loopback, and not of the published port: the published port is what
	# the assertions below are about, and a probe through it would consume the
	# first request that should have been measured.
	local deadline=$((SECONDS + 60))
	while ((SECONDS < deadline)); do
		if docker exec "${PROJECT}-caddy" wget --quiet --spider http://127.0.0.1:2019/config/ 2>/dev/null; then
			HTTPS_PORT="$(docker port "${PROJECT}-caddy" 443/tcp | head -1 | sed 's/.*://')"
			HTTP_PORT="$(docker port "${PROJECT}-caddy" 80/tcp | head -1 | sed 's/.*://')"
			[[ -n "$HTTPS_PORT" && -n "$HTTP_PORT" ]] || fail "the ingress published no port"
			return 0
		fi
		sleep 1
	done
	docker logs "${PROJECT}-caddy" >&2 || true
	fail "the ingress never answered on its admin endpoint"
}

# ingress <args...> — an HTTPS request to the site, with the certificate
# verified against the one the operator's file provides. Verification is not
# disabled: a Caddy serving its own self-signed default would fail here, which
# is the point of mounting a certificate at all.
ingress() {
	curl --silent --show-error --max-time 15 \
		--cacert "$WORK_DIR/origin.crt" --resolve "${SITE}:${HTTPS_PORT}:127.0.0.1" "$@"
}

# header_of <file> <name> — one response header of the last request. The value
# is stripped of its carriage return here rather than in every caller: a
# comparison against a value that ends with a control character fails for a
# reason nobody would guess from the failure message.
header_of() {
	local file="$1" name="$2"
	sed -nE "s/^${name}:[[:space:]]*(.*)$/\1/Ip" "$file" | tr -d '\r' | head -1
}

expect_equals() {
	local what="$1" want="$2" got="$3"
	[[ "$want" == "$got" ]] || fail "${what}: got ${got:-<nothing>}, want ${want}"
}

start_stub
start_ingress "$ROOT/$CADDYFILE"
log "the ingress answers on 127.0.0.1:${HTTPS_PORT} (https) and 127.0.0.1:${HTTP_PORT} (http)"

# ---------------------------------------------------------------------------
# 4. What passes through: the application's own decisions, unaltered
# ---------------------------------------------------------------------------
# The document is private and the asset is immutable, and both are the
# application's decision. The gate asserts the values byte for byte, so an edge
# that "helpfully" added a cache directive is caught by the difference rather
# than by an opinion about what the directive should have been.
ingress --dump-header "$WORK_DIR/document.headers" --output /dev/null "https://${SITE}:${HTTPS_PORT}/document" ||
	fail "the document was not served through the ingress"
expect_equals "the document's Cache-Control" "no-store" "$(header_of "$WORK_DIR/document.headers" 'cache-control')"
expect_equals "the document's ETag" '"document-1"' "$(header_of "$WORK_DIR/document.headers" 'etag')"

ingress --dump-header "$WORK_DIR/asset.headers" --output /dev/null "https://${SITE}:${HTTPS_PORT}/assets/hashed-1b2f5c0361dd.js" ||
	fail "the hashed asset was not served through the ingress"
expect_equals "the asset's Cache-Control" "public, max-age=31536000, immutable" "$(header_of "$WORK_DIR/asset.headers" 'cache-control')"

# The upstream in this run names itself, so what is asserted here is that the
# edge does not pass a name through — not merely that Caddy keeps its own name
# to itself.
if grep -qiE '^server:' "$WORK_DIR/document.headers"; then
	fail "the response names the software that served it: $(grep -iE '^server:' "$WORK_DIR/document.headers"); the upstream names itself, so this is the edge forwarding a name, not failing to add one"
fi
log "the application's cache decisions arrive unchanged, and the upstream's own name does not survive the edge"

# ---------------------------------------------------------------------------
# 5. Compression, and what it does to a validator
# ---------------------------------------------------------------------------
# Two requests for the same address with different requests for encoding. The
# assertions are that the compressed response says it is compressed, that it
# carries a validator of its own, and that the identity response keeps the
# validator the application chose: reusing one validator for two
# representations is the defect that makes a cache serve the wrong bytes.
ingress --header 'Accept-Encoding: gzip' --dump-header "$WORK_DIR/gzip.headers" \
	--output "$WORK_DIR/gzip.body" "https://${SITE}:${HTTPS_PORT}/compressible" ||
	fail "the compressible document was not served (gzip)"
expect_equals "the compressed response's Content-Encoding" "gzip" "$(header_of "$WORK_DIR/gzip.headers" 'content-encoding')"
case "$(header_of "$WORK_DIR/gzip.headers" 'vary')" in
*[Aa]ccept-[Ee]ncoding*) ;;
*) fail "the compressed response does not vary on Accept-Encoding, so a cache may hand it to a client that asked for identity" ;;
esac
GZIP_ETAG="$(header_of "$WORK_DIR/gzip.headers" 'etag')"
[[ -n "$GZIP_ETAG" ]] || fail "the compressed response carries no validator"

ingress --header 'Accept-Encoding: identity' --dump-header "$WORK_DIR/identity.headers" \
	--output "$WORK_DIR/identity.body" "https://${SITE}:${HTTPS_PORT}/compressible" ||
	fail "the compressible document was not served (identity)"
IDENTITY_ETAG="$(header_of "$WORK_DIR/identity.headers" 'etag')"
expect_equals "the identity response's ETag" '"compressible-1"' "$IDENTITY_ETAG"
[[ -z "$(header_of "$WORK_DIR/identity.headers" 'content-encoding')" ]] ||
	fail "a client that asked for identity was sent $(header_of "$WORK_DIR/identity.headers" 'content-encoding')"
[[ "$GZIP_ETAG" != "$IDENTITY_ETAG" ]] ||
	fail "both representations share the validator ${IDENTITY_ETAG}, so a cache cannot tell them apart"
log "compression is on, the compressed response varies on encoding and carries its own validator (${GZIP_ETAG})"

# ---------------------------------------------------------------------------
# 6. Who is believed about a visitor's address
# ---------------------------------------------------------------------------
# The request below forges all three forwarding headers a client could send.
# The peer is not Cloudflare, so none of them is evidence: the chain the
# application receives must be the address this process observed, and the two
# single-value headers must not arrive at all.
ingress --header "CF-Connecting-IP: ${FORGED_IP}" --header "X-Forwarded-For: ${FORGED_IP}" \
	--header "X-Real-IP: ${FORGED_IP}" --output "$WORK_DIR/echo.txt" \
	"https://${SITE}:${HTTPS_PORT}/echo" >/dev/null || fail "the echo endpoint was not served"
echo_field() { sed -nE "s/^$1=(.*)\$/\1/p" "$WORK_DIR/echo.txt" | head -1; }

FORWARDED="$(echo_field X-Forwarded-For)"
[[ -n "$FORWARDED" ]] || fail "the application was not told any client address at all"
[[ "$FORWARDED" != *"${FORGED_IP}"* ]] ||
	fail "the client's own X-Forwarded-For reached the application (${FORWARDED}); a chain a client pads is not evidence"
expect_equals "the client's CF-Connecting-IP upstream" "" "$(echo_field CF-Connecting-IP)"
expect_equals "the client's X-Real-IP upstream" "" "$(echo_field X-Real-IP)"
expect_equals "the observed scheme upstream" "https" "$(echo_field X-Forwarded-Proto)"
expect_equals "the observed host upstream" "${SITE}:${HTTPS_PORT}" "$(echo_field X-Forwarded-Host)"
log "the peer's own address (${FORWARDED}) is what the application was told; the forged headers did not survive"

# The positive control: the same request, against a file whose only difference
# is that every address is trusted. The forged claim now arrives — which is what
# makes the refusal above a decision rather than a mechanism that was never
# wired — while the forged X-Forwarded-For still does not, because the headers
# consulted were narrowed to the one the CDN writes.
docker rm --force "${PROJECT}-caddy" >/dev/null 2>&1 || true
start_ingress "$WORK_DIR/Caddyfile.trusting"
ingress --header "CF-Connecting-IP: ${FORGED_IP}" --header "X-Forwarded-For: ${FORGED_IP}" \
	--output "$WORK_DIR/echo-trusted.txt" \
	"https://${SITE}:${HTTPS_PORT}/echo" >/dev/null || fail "the echo endpoint was not served by the positive control"
TRUSTED_FORWARDED="$(sed -nE 's/^X-Forwarded-For=(.*)$/\1/p' "$WORK_DIR/echo-trusted.txt" | head -1)"
expect_equals "the trusted peer's claim upstream" "$FORGED_IP" "$TRUSTED_FORWARDED"
TRUSTED_CHAIN="$(sed -nE 's/^X-Forwarded-For=(.*)$/\1/p' "$WORK_DIR/echo-trusted.txt" | head -1)"
[[ "$TRUSTED_CHAIN" != *","* ]] ||
	fail "the trusted peer's claim arrived as a chain (${TRUSTED_CHAIN}); the header consulted is a single value, not a chain"
log "with the peer trusted, the claim is believed (${TRUSTED_FORWARDED}) — so the refusal above is the trust decision"

# ---------------------------------------------------------------------------
# 7. The admin surface, and the handshake with the wrong name
# ---------------------------------------------------------------------------
# The admin endpoint is alive (section 3 waited on it) and it is not reachable
# through the site: a request for its path arrives at the application like any
# other unknown path.
docker rm --force "${PROJECT}-caddy" >/dev/null 2>&1 || true
start_ingress "$ROOT/$CADDYFILE"
expect_equals "the admin path through the site" "404" \
	"$(ingress --output /dev/null --write-out '%{http_code}' "https://${SITE}:${HTTPS_PORT}/config/")"
ingress --output "$WORK_DIR/config.txt" "https://${SITE}:${HTTPS_PORT}/config/" >/dev/null
grep -q "stub: not found" "$WORK_DIR/config.txt" ||
	fail "the admin path did not reach the application, which is where an unknown path belongs"

PUBLISHED="$(docker port "${PROJECT}-caddy" | cut -d' ' -f1 | sort -u | tr '\n' ' ')"
expect_equals "the ingress's published container ports" "443/tcp 80/tcp " "$PUBLISHED"
log "the admin endpoint is reachable from inside the container only, and the site routes no path to it"

# A handshake whose name is not the one this origin answers for is refused
# rather than answered with whatever certificate comes first.
#
# The distinction the assertion makes is the one that matters: curl's code 60
# means a handshake completed and the certificate did not match the name — a
# served certificate, which is exactly what must not happen — while 35 and 56
# mean the handshake or the connection ended without one.
set +e
curl --silent --max-time 10 --cacert "$WORK_DIR/origin.crt" \
	--resolve "elsewhere.invalid:${HTTPS_PORT}:127.0.0.1" \
	"https://elsewhere.invalid:${HTTPS_PORT}/health/live" >/dev/null 2>&1
SNI_CODE="$?"
set -e
case "$SNI_CODE" in
0) fail "a handshake for elsewhere.invalid was served: the certificate binding is a convention, not a fact" ;;
60) fail "a handshake for elsewhere.invalid completed and was answered with a certificate for another name" ;;
esac

if ! ingress --output /dev/null "https://${SITE}:${HTTPS_PORT}/health/live"; then
	fail "the site itself stopped answering, which would make the refusal above meaningless"
fi
log "a handshake for another name ends without one (curl ${SNI_CODE}) while the site's own name is served"

# The redirect from plain HTTP is what makes a visitor who types the name land
# on TLS, and it survived the hardening.
REDIRECT_STATUS="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 10 \
	--header "Host: ${SITE}" "http://127.0.0.1:${HTTP_PORT}/health/live" || true)"
case "$REDIRECT_STATUS" in
308 | 301 | 302) ;;
*) fail "plain HTTP answered ${REDIRECT_STATUS:-nothing}, want a redirect to the site address" ;;
esac
log "plain HTTP still redirects to the site address (${REDIRECT_STATUS})"

# ---------------------------------------------------------------------------
# 8. What the process emits when the application never answered
# ---------------------------------------------------------------------------
# The upstream is stopped, so the proxy's own error response is what a visitor
# would receive during an outage. The static audit compares its values against
# the policy the application delivers; this half asserts the block is wired at
# all — a rule about a value cannot see a response that never happens.
docker stop "${PROJECT}-stub" >/dev/null
ERROR_STATUS="$(ingress --dump-header "$WORK_DIR/error.headers" --output "$WORK_DIR/error.body" \
	--write-out '%{http_code}' "https://${SITE}:${HTTPS_PORT}/document" || true)"
expect_equals "the error response's status" "502" "$ERROR_STATUS"
[[ -s "$WORK_DIR/error.headers" ]] || fail "the request against a stopped upstream produced no response at all"
for field in content-security-policy x-content-type-options referrer-policy permissions-policy strict-transport-security cache-control; do
	value="$(header_of "$WORK_DIR/error.headers" "$field")"
	[[ -n "$value" ]] || fail "the error response carries no ${field}: the one class of response the application cannot cover is the one left without a policy"
done
expect_equals "the error response's Cache-Control" "no-store" "$(header_of "$WORK_DIR/error.headers" 'cache-control')"
# The strip on the proxied path is a directive of that path, and this response
# never reaches it: the name that must not appear here is the edge's own, which
# is why the rule for it is a second rule rather than the same one.
NAMED_ON_ERROR="$(header_of "$WORK_DIR/error.headers" 'server')"
if [[ -n "$NAMED_ON_ERROR" ]]; then
	fail "the outage response names the software that served it (${NAMED_ON_ERROR}): the strip on the proxied path never sees a response this process wrote"
fi
log "an outage produces a response with the policy, a no-store and no server name, instead of a bare status line"

log "ok — the pinned Caddy validates, the file is canonical, the application's cache and validator decisions pass through unchanged, compression carries its own validator, only the CDN's claim about a visitor is believed, the admin surface is unreachable through the site, another name's handshake is refused, and an outage still answers with the policy"

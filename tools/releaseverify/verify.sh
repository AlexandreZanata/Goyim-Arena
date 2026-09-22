#!/usr/bin/env bash
# Reproducible release verification of the backend (P20-T07).
#
# This is the exercise the phase names, in the order it names it: a clean
# checkout of the current commit, the dependencies installed **from the
# lockfiles only**, the dependencies brought up, `make verify`, the image build
# and a smoke — every command passing twice — and then the state of the
# repository and of the release gate. It writes what it measured as JSON and
# renders docs/RELEASE_CHECKLIST.md from it (`releaseverify render`), so the
# document cannot claim a number the run did not produce.
#
# It fails closed: the first red run aborts before anything is written, so a
# failed verification never leaves a document that reads like a passed one. The
# release gate is the one command whose refusal is *expected* — it is about the
# owner's decisions, not about the code — and its verdict is recorded instead of
# aborting the run.
#
# Requirements: git, Go, Node with npm, Docker with a reachable daemon.
#
# Environment:
#   ARENA_RELEASE_CHECKLIST      where to write the checklist (default:
#                                docs/RELEASE_CHECKLIST.md)
#   IMAGE                        the image tag to build and smoke (default:
#                                goyim-arena:release-verify)
#   ARENA_RELEASE_VERIFY_IMAGE   PostgreSQL to run against (default the pinned
#                                postgres:18.4 digest)
set -euo pipefail

TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$TOOL_DIR/../.." && pwd)"
cd "$ROOT"

POSTGRES_IMAGE="${ARENA_RELEASE_VERIFY_IMAGE:-postgres:18.4@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636}"
DB_PASSWORD="arena-local-dev"
RUN_ID="$$"
CONTAINER="arena-release-verify-db-${RUN_ID}"
IMAGE="${IMAGE:-goyim-arena:release-verify}"
OUT="${ARENA_RELEASE_CHECKLIST:-$ROOT/docs/RELEASE_CHECKLIST.md}"

# git creates the worktree itself, so the temporary directory holds the parent
# and the checkout is a child: an existing directory is not a target it accepts.
SANDBOX="$(mktemp -d -t arena-release-verify-XXXXXX)"
WORK="$SANDBOX/checkout"
FACTS_FILE="$SANDBOX/facts.json"
LOG="$SANDBOX/verify.log"
TOOLS_BIN="$SANDBOX/bin"
mkdir -p "$TOOLS_BIN"
export PATH="$TOOLS_BIN:$PATH"

COMMANDS=""
RED=""

FAILED=0

log() { printf 'release-verify: %s\n' "$*" >&2; }
fail() {
	FAILED=1
	printf 'release-verify: %s\n' "$*" >&2
	exit 1
}

# A failed run keeps what it measured: the logs of every attempt and the facts
# as far as they were assembled. Deleting the evidence of a red run is how the
# next person spends an hour reproducing it.
cleanup() {
	docker rm --force "$CONTAINER" >/dev/null 2>&1 || true
	git -C "$ROOT" worktree remove --force "$WORK" >/dev/null 2>&1 || true
	git -C "$ROOT" worktree prune >/dev/null 2>&1 || true
	if [ "$FAILED" = "0" ]; then
		rm -rf "$SANDBOX"
	else
		printf 'release-verify: the run failed; every attempt log is kept in %s\n' "$SANDBOX" >&2
	fi
}
trap cleanup EXIT

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		fail "$1 is required and was not found in PATH"
	fi
}
require_command git
require_command go
require_command npm
require_command docker
require_command timeout || true

if ! docker info >/dev/null 2>&1; then
	fail "the Docker daemon is not reachable"
fi

# ---------------------------------------------------------------------------
# 1. A clean checkout of the current commit. The verification does not run in
#    the working checkout: a tree that has been used holds build output, modules
#    and generated artifacts, and "passes on my machine" is exactly what this
#    task exists to replace. `git worktree` gives the commit without touching
#    anything the operator has.
# ---------------------------------------------------------------------------
COMMIT="$(git rev-parse HEAD)"
BRANCH="$(git rev-parse --abbrev-ref HEAD)"
GENERATED="docs/RELEASE_CHECKLIST.md"

if [ -n "$(git -C "$ROOT" ls-files .local)" ]; then
	fail "the commit tracks files of the local directory, and the local plan never ships"
fi

log "creating a clean checkout of $COMMIT"
git worktree add --detach "$WORK" "$COMMIT" >/dev/null || fail "could not create the clean checkout"

if [ -n "$(git -C "$WORK" status --porcelain)" ]; then
	git -C "$WORK" status --short >&2
	fail "the clean checkout is not clean, which can only mean the commit contains what it should not"
fi
CLEAN=true
LOCAL_TRACKED="$(git -C "$WORK" ls-files .local | wc -l | tr -d ' ')"
if [ "$LOCAL_TRACKED" != "0" ]; then
	fail "the commit tracks $LOCAL_TRACKED file(s) of the local directory"
fi

# The instrument cannot be inside the commit it verifies: this task adds the
# tooling, so the tree that gets committed is the commit **plus these files**,
# and the run verifies that tree instead of the one before it. Every copied file
# is recorded with its digest, and anything outside the tooling, the evidence and
# the Makefile is refused: a production file smuggled in here would make the
# whole verification a statement about a tree nobody reviewed.
OVERLAY=""
while IFS= read -r line; do
	[ -n "$line" ] || continue
	path="${line:3}"
	should_skip=false
	case "$path" in
	"$GENERATED") should_skip=true ;;
	Makefile | tools/releaseverify/* | docs/*) ;;
	*) should_skip=true ;;
	esac
	if [ "$should_skip" = true ]; then
		if [ "$path" != "$GENERATED" ]; then
			echo "release-verify: $path is not the tooling, the evidence or the Makefile: the commit is the product code" >&2
			fail "the run refuses to copy $path over the commit"
		fi
		continue
	fi
	[ -f "$ROOT/$path" ] || continue
	mkdir -p "$WORK/$(dirname "$path")"
	cp "$ROOT/$path" "$WORK/$path"
	digest="sha256:$(sha256sum "$ROOT/$path" | awk '{print $1}')"
	OVERLAY="${OVERLAY}${OVERLAY:+;;}${path}|${digest}"
done < <(git -C "$ROOT" status --porcelain --untracked-files=all)
if [ -n "$OVERLAY" ]; then
	CHECKOUT_KIND="git worktree limpo no commit, com sobreposição declarada dos arquivos desta tarefa copiados por cima"
else
	CHECKOUT_KIND="git worktree limpo no commit, criado por tools/releaseverify/verify.sh e descartado no fim"
fi
log "overlay: ${OVERLAY:-none}"

# ---------------------------------------------------------------------------
# 2. Measuring. Every command the phase names runs **twice**. A red run is
#    recorded and aborts the run at the end of the measurements: a document is
#    only written when every command passed, so a failed verification can never
#    leave behind something that reads like a passed one.
# ---------------------------------------------------------------------------
measure() { # key, command, note
	local key="$1" command="$2" note="$3"
	local runs="" seconds code started finished duration
	for attempt in 1 2; do
		: >"$LOG.${key}.${attempt}"
		started="$(date +%s.%N)"
		set +e
		(cd "$WORK" && eval "$command") >>"$LOG.${key}.${attempt}" 2>&1
		code=$?
		set -e
		finished="$(date +%s.%N)"
		duration="$(awk -v a="$started" -v b="$finished" 'BEGIN { printf "%.1f", b - a }')"
		runs="${runs}${runs:+,}${duration}:${code}"
		if [ "$code" -eq 0 ]; then
			log "$key: run $attempt ok in ${duration}s"
		else
			log "$key: run $attempt answered exit $code — $LOG.${key}.${attempt}"
			# The failing part of a suite is rarely at the end of its output: the
			# summary line names the package, and the package's own failure is
			# where it says what broke.
			grep -E -B2 -A12 '^--- FAIL|^FAIL|not ok' "$LOG.${key}.${attempt}" | head -60 >&2 || tail -25 "$LOG.${key}.${attempt}" >&2 || true
			RED="${RED}${RED:+, }${key}/run-${attempt}"
		fi
	done
	COMMANDS="${COMMANDS}${COMMANDS:+;;}${key}|${command}|${runs}|${note}"
}

# The pinned sqlc, installed into a private bin so the run cannot depend on
# whichever version happens to be in the operator's PATH.
measure "sqlc-toolchain" \
	"GOBIN='$TOOLS_BIN' go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0" \
	"Instala o sqlc na versão que o repositório fixa, fora do PATH do operador."

# Dependencies, from the lockfiles only. `go mod download` resolves against
# go.sum; `npm ci` refuses to write a lockfile and fails when it disagrees with
# package.json.
measure "go-modules" "go mod download" \
	"Baixa os módulos exatamente nas versões que go.sum fixa."
measure "web-modules" "npm ci --prefix web" \
	"Instala o frontend a partir de web/package-lock.json."
measure "e2e-modules" "npm ci --prefix tools/e2e" \
	"Instala as jornadas de navegador a partir de tools/e2e/package-lock.json."

# ---------------------------------------------------------------------------
# 3. The dependencies up. The integration harnesses look for PostgreSQL at
#    127.0.0.1:54329 and nowhere else — it is the door `compose.yaml` publishes
#    for development and the door the CI job declares as a service — so this is
#    the port the verification uses, and it uses whatever answers there. When
#    nothing does, it brings up the pinned image on that exact port, exactly as
#    the CI service is declared. The password is the local development one that
#    compose.yaml publishes; it is a credential of nothing.
# ---------------------------------------------------------------------------
DB_PORT=54329
export PGPASSWORD="$DB_PASSWORD"
DATABASE_OWNED=false
DATABASE_PROVENANCE="usou o PostgreSQL que já respondia na porta"

if (exec 3<>/dev/tcp/127.0.0.1/$DB_PORT) 2>/dev/null; then
	exec 3<&- 3>&-
	log "something already answers on 127.0.0.1:$DB_PORT: using it"
else
	log "starting $POSTGRES_IMAGE on 127.0.0.1:$DB_PORT"
	docker run --detach \
		--name "$CONTAINER" \
		--env POSTGRES_USER=arena \
		--env POSTGRES_PASSWORD="$DB_PASSWORD" \
		--env POSTGRES_DB=arena \
		--env POSTGRES_INITDB_ARGS="--encoding=UTF8 --locale=C" \
		--env POSTGRES_HOST_AUTH_METHOD=scram-sha-256 \
		--publish "127.0.0.1:${DB_PORT}:5432" \
		"$POSTGRES_IMAGE" >/dev/null
	DATABASE_OWNED=true
	DATABASE_PROVENANCE="subiu um contêiner da imagem fixada por digest e o removeu no fim"
fi

# Every query goes through the pinned image, so the client and the server are
# the versions the plan names whether the server was already there or not.
db_query() {
	docker run --rm --network host --env PGPASSWORD --env PGHOST=127.0.0.1 --env PGPORT="$DB_PORT" \
		--env PGUSER=arena --env PGDATABASE=arena "$POSTGRES_IMAGE" \
		psql --tuples-only --no-align --command "$1"
}
DB_COMMAND="docker run --rm --network host --env PGPASSWORD --env PGHOST=127.0.0.1 --env PGPORT=$DB_PORT --env PGUSER=arena --env PGDATABASE=arena $POSTGRES_IMAGE psql --tuples-only --no-align --command 'SHOW server_version'"

for attempt in $(seq 1 60); do
	if db_query 'SELECT 1' >/dev/null 2>&1; then
		break
	fi
	if [ "$attempt" = 60 ]; then
		docker logs "$CONTAINER" >&2 2>/dev/null || true
		fail "the database on 127.0.0.1:$DB_PORT never answered"
	fi
	sleep 1
done

POSTGRES_VERSION="$(db_query 'SHOW server_version')"
case "$POSTGRES_VERSION" in
18.*) ;;
*) fail "the server on 127.0.0.1:$DB_PORT runs PostgreSQL $POSTGRES_VERSION, and the plan requires 18" ;;
esac

# The harnesses name their databases after the test and a counter that restarts
# with every process, so a run interrupted halfway leaves names the next run
# collides with — and the collision reads like a broken test instead of a broken
# environment. A leftover is named as what it is.
LEFTOVER="$(db_query "SELECT count(*) FROM pg_database WHERE datname LIKE 'arena_test%'")"
if [ "$LEFTOVER" != "0" ]; then
	db_query "SELECT datname FROM pg_database WHERE datname LIKE 'arena_test%'" >&2
	fail "the server holds $LEFTOVER leftover test database(s): a previous run was interrupted. Drop them and run again"
fi

DSN="postgres://arena:${DB_PASSWORD}@127.0.0.1:${DB_PORT}/arena?sslmode=disable"
log "PostgreSQL $POSTGRES_VERSION is ready on 127.0.0.1:$DB_PORT ($DATABASE_PROVENANCE)"

measure "database" "$DB_COMMAND" \
	"Os harnesses de integração procuram o PostgreSQL em 127.0.0.1:54329 — a porta que o compose.yaml publica e que o CI declara como serviço —, e este run $DATABASE_PROVENANCE."

# ---------------------------------------------------------------------------
# 4. The aggregate gate, the image and the smoke, twice each.
# ---------------------------------------------------------------------------
measure "verify" "ARENA_DATABASE_URL='$DSN' make verify" \
	"O gate de integração completo, o mesmo que o CI roda no job foundation."

measure "image-build" "IMAGE='$IMAGE' make image-build" \
	"Constrói a imagem de produção a partir do Dockerfile."

measure "image-smoke" "ARENA_POSTGRES_IMAGE='$POSTGRES_IMAGE' IMAGE='$IMAGE' make image-verify" \
	"Sobe a imagem com filesystem somente leitura contra um PostgreSQL descartável, prova que ela aplica as próprias migrations, serve uma página e o asset com hash que ela referencia."

measure "fsck" "git fsck --no-progress" \
	"git fsck na árvore limpa."

# ---------------------------------------------------------------------------
# 5. What the run left behind, and the toolchain it used.
# ---------------------------------------------------------------------------
if [ -n "$RED" ]; then
	fail "the verification is not green: $RED — nothing was written"
fi

if [ "$(git rev-parse HEAD)" != "$COMMIT" ]; then
	fail "HEAD moved during the run: the document would describe a commit that is no longer the tip"
fi

FSCK_OUTPUT="$(git fsck --no-progress 2>&1 || true)"
FSCK_ERRORS="$(printf '%s\n' "$FSCK_OUTPUT" | grep -cE 'error|missing|corrupt|broken' || true)"
if [ "$FSCK_ERRORS" != "0" ]; then
	printf '%s\n' "$FSCK_OUTPUT" >&2
	fail "git fsck reported $FSCK_ERRORS error(s)"
fi

IMAGE_INFO="$(docker image inspect "$IMAGE" --format '{{.Id}}|{{if .RepoDigests}}{{index .RepoDigests 0}}{{end}}|{{.Size}}')"
IMAGE_ID="${IMAGE_INFO%%|*}"
IMAGE_REST="${IMAGE_INFO#*|}"
IMAGE_SIZE="${IMAGE_REST##*|}"
IMAGE_REPO_DIGEST="${IMAGE_REST%|*}"
if [ -n "$IMAGE_REPO_DIGEST" ]; then
	IMAGE_DIGEST="${IMAGE_REPO_DIGEST#*@}"
else
	# Nothing pushed this image, so there is no manifest digest: the config
	# digest is what identifies the artifact, and the document says so.
	IMAGE_DIGEST="$IMAGE_ID"
fi

TRACKED_FILES="$(git -C "$ROOT" ls-files | wc -l | tr -d ' ')"
LOCKFILES=""
for lock in go.sum web/package-lock.json tools/e2e/package-lock.json; do
	digest="$(sha256sum "$WORK/$lock" | awk '{print $1}')"
	case "$lock" in
	go.sum) installs="go mod download" ;;
	web/package-lock.json) installs="npm ci --prefix web" ;;
	*) installs="npm ci --prefix tools/e2e" ;;
	esac
	LOCKFILES="${LOCKFILES}${LOCKFILES:+;;}${lock}|${digest}|${installs}"
done

NODE_VERSION="$(node --version)"
NPM_VERSION="$(npm --version)"
GO_VERSION="$(go version | awk '{print $3}')"
DOCKER_VERSION="$(docker version --format '{{.Client.Version}}/{{.Server.Version}}' 2>/dev/null || echo unknown)"
SQLC_VERSION="$(sqlc version 2>/dev/null || echo unknown)"
OS_NAME="$(uname -sr)"
ARCH_NAME="$(uname -m)"
CPU_COUNT="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 0)"
VERIFIED_ON="$(date +%F)"

# ---------------------------------------------------------------------------
# 6. The release gate. Its refusal is a fact about decisions, so it is recorded
#    rather than treated as a failed verification — and both runs have to agree,
#    because a gate that answers differently twice is a gate nobody can trust.
# ---------------------------------------------------------------------------
GOV_COMMAND="make release-gate"
set +e
(cd "$ROOT" && make release-gate) >"$WORK/release-gate.1.log" 2>&1
GOV_CODE_1=$?
(cd "$ROOT" && make release-gate) >"$WORK/release-gate.2.log" 2>&1
GOV_CODE_2=$?
set -e
if [ "$GOV_CODE_1" != "$GOV_CODE_2" ]; then
	cat "$WORK/release-gate.1.log" "$WORK/release-gate.2.log" >&2 || true
	fail "the release gate answered exit $GOV_CODE_1 and then $GOV_CODE_2"
fi
GOV_BLOCKED=false
if [ "$GOV_CODE_1" != "0" ]; then
	GOV_BLOCKED=true
fi
GOV_OPEN="$(sed -n 's/^governanceaudit: BLOQUEIO \([a-z-]*\).*/\1/p' "$WORK/release-gate.1.log" | paste -sd';' -)"
GOV_PENDING="$(sed -n 's/^governanceaudit: pending (\([a-z-]*\)):.*/\1/p' "$WORK/release-gate.1.log" | paste -sd';' -)"
log "release gate: blocked=$GOV_BLOCKED open=[$GOV_OPEN] pending=[$GOV_PENDING]"

# ---------------------------------------------------------------------------
# 7. The document, rendered from the measurements by the tool of the verified
#    commit. Writing it is the only thing this run changes in the working
#    checkout, which is what the "clean tree" part of the phase means.
# ---------------------------------------------------------------------------
LIMITS="Limitações do make verify: o alvo cobre o que não exige ambiente próprio, e os gates que exigem k6, navegador, trivy ou daemon Docker (test-e2e, test-load-smoke, image-scan, caddy-verify, compose-verify, backup-verify, deploy-verify, migration-audit, disaster-drill e vuln) não rodaram aqui — cada um tem o próprio alvo e a própria evidência versionada."
LIMITS="${LIMITS};;A verificação mede a máquina desta execução ($OS_NAME/$ARCH_NAME, $CPU_COUNT processador(es), Node $NODE_VERSION, Docker $DOCKER_VERSION): dois commits verificados em máquinas diferentes são dois fatos diferentes, e este documento registra qual máquina mediu."
LIMITS="${LIMITS};;A imagem não foi publicada num registry, então o digest registrado é o da configuração local (docker image inspect .Id), não um digest de manifesto de registry."
LIMITS="${LIMITS};;O portão de release continua vermelho enquanto a decisão do titular estiver em aberto: o commit verificado não é uma release autorizada, e nenhuma verificação técnica muda isso."
LIMITS="${LIMITS};;O dataset dos testes de integração é criado pelos próprios harnesses: esta verificação mede correção, não volume, latência nem capacidade — isso é tests/load e o exercício da P20-T05."
LIMITS="${LIMITS};;O documento entra no commit seguinte ao verificado: a árvore verificada é a do commit registrado, e o único arquivo que muda depois dela é este."
NEXT="Fechar as duas decisões humanas do titular e rodar ./.local/git-flow.sh finish, que marca o PR #62 como pronto e só mergeia com o CI verify verde."
NEXT="${NEXT};;P20-T08 (handoff) e P20-T09 (auditoria final de i18n) fecham a fase; este checklist é re-rodado depois do último commit da fase."

# What the tree will hold when the run is done: the document, plus the files of
# the overlay it named. Anything else means the verification changed something it
# cannot account for, and it is refused before the document is written.
OVERLAY_DIRTY=""
if [ -n "$OVERLAY" ]; then
	while IFS= read -r entry; do
		[ -n "$entry" ] || continue
		OVERLAY_DIRTY="$OVERLAY_DIRTY,$(printf '%s' "$entry" | cut -d'|' -f1)"
	done < <(printf '%s\n' "$OVERLAY" | sed 's/;;/\n/g')
fi
OVERLAY_DIRTY="$(printf '%s' "$OVERLAY_DIRTY" | sed 's/^,//' | tr ',' '\n' | sort | paste -sd',' -)"
EXPECTED_DIRTY="$(printf '%s,%s' "$GENERATED" "$OVERLAY_DIRTY" | tr ',' '\n' | sort | paste -sd',' -)"
# Before this run writes anything, the tree holds the overlay and nothing else,
# plus the document of an earlier run when there was one — it is this run's own
# artifact and it is about to be replaced.
BEFORE="$( { git -C "$ROOT" status --porcelain --untracked-files=all | awk '{print $NF}' | grep -v "^${GENERATED}\$" || true; } | sort | paste -sd',' -)"
if [ "$BEFORE" != "$OVERLAY_DIRTY" ]; then
	git -C "$ROOT" status --short >&2
	fail "the repository holds [$BEFORE] and the run can account for [$OVERLAY_DIRTY]"
fi

log "rendering $OUT"
(cd "$WORK" && RV_VERSION=1 RV_VERIFIED_ON="$VERIFIED_ON" RV_COMMIT="$COMMIT" RV_BRANCH="$BRANCH" \
	RV_CHECKOUT_KIND="$CHECKOUT_KIND" \
	RV_CHECKOUT_CLEAN="$CLEAN" RV_LOCAL_TRACKED="$LOCAL_TRACKED" RV_GENERATED="$GENERATED" \
	RV_OVERLAY="$OVERLAY" RV_DIRTY="$EXPECTED_DIRTY" \
	RV_OS="$OS_NAME" RV_ARCH="$ARCH_NAME" RV_CPUS="$CPU_COUNT" RV_GO="$GO_VERSION" RV_NODE="$NODE_VERSION" \
	RV_NPM="$NPM_VERSION" RV_DOCKER="$DOCKER_VERSION" RV_SQLC="$SQLC_VERSION" RV_POSTGRES="$POSTGRES_VERSION" \
	RV_LOCKFILES="$LOCKFILES" RV_COMMANDS="$COMMANDS" \
	RV_IMAGE_REF="$IMAGE" RV_IMAGE_ID="$IMAGE_ID" RV_IMAGE_DIGEST="$IMAGE_DIGEST" RV_IMAGE_SIZE="$IMAGE_SIZE" \
	RV_IMAGE_SMOKE="make image-verify" \
	RV_IMAGE_SMOKE_DETAIL="a imagem sobe com filesystem somente leitura contra um PostgreSQL descartável, aplica as próprias migrations, serve uma página e o asset com hash que ela referencia" \
	RV_IMAGE_SMOKE_OK=true \
	RV_TRACKED="$TRACKED_FILES" RV_FSCK="ok" RV_FSCK_ERRORS="$FSCK_ERRORS" \
	RV_GOV_COMMAND="$GOV_COMMAND" RV_GOV_BLOCKED="$GOV_BLOCKED" RV_GOV_OPEN="$GOV_OPEN" \
	RV_GOV_PENDING="$GOV_PENDING" RV_LIMITS="$LIMITS" RV_NEXT="$NEXT" \
	python3 - "$FACTS_FILE" <<'PY'
import json, os, sys

def parts(name, separator=";;"):
    value = os.environ.get(name, "")
    return [item for item in value.split(separator) if item != ""]

def lockfiles():
    entries = []
    for raw in parts("RV_LOCKFILES"):
        path, digest, installs = raw.split("|")
        entries.append({"path": path, "sha256": digest, "installs": installs,
                        "note": "Dependência instalada a partir deste lockfile, nunca por versão flutuante."})
    return entries

def commands():
    entries = []
    for raw in parts("RV_COMMANDS"):
        key, command, runs, note = raw.split("|")
        entries.append({
            "key": key,
            "command": command,
            "runs": [{"seconds": float(seconds), "exit_code": int(code)}
                     for seconds, code in (run.split(":") for run in runs.split(","))],
            "note": note,
        })
    return entries

facts = {
    "version": int(os.environ["RV_VERSION"]),
    "verified_on": os.environ["RV_VERIFIED_ON"],
    "commit": os.environ["RV_COMMIT"],
    "branch": os.environ["RV_BRANCH"],
    "checkout": {
        "kind": os.environ["RV_CHECKOUT_KIND"],
        "clean": os.environ["RV_CHECKOUT_CLEAN"] == "true",
        "local_tracked_files": int(os.environ["RV_LOCAL_TRACKED"]),
        "generated": os.environ["RV_GENERATED"],
        "dirty": [item for item in os.environ["RV_DIRTY"].split(",") if item != ""],
        "overlay": [
            {"path": path, "sha256": digest}
            for path, digest in (entry.split("|") for entry in parts("RV_OVERLAY"))
        ],
    },
    "host": {
        "os": os.environ["RV_OS"], "arch": os.environ["RV_ARCH"], "cpus": int(os.environ["RV_CPUS"]),
        "go": os.environ["RV_GO"], "node": os.environ["RV_NODE"], "npm": os.environ["RV_NPM"],
        "docker": os.environ["RV_DOCKER"], "sqlc": os.environ["RV_SQLC"],
        "postgres": os.environ["RV_POSTGRES"],
    },
    "lockfiles": lockfiles(),
    "commands": commands(),
    "image": {
        "reference": os.environ["RV_IMAGE_REF"], "id": os.environ["RV_IMAGE_ID"],
        "digest": os.environ["RV_IMAGE_DIGEST"], "size_bytes": int(os.environ["RV_IMAGE_SIZE"]),
        "smoke": os.environ["RV_IMAGE_SMOKE"], "smoke_detail": os.environ["RV_IMAGE_SMOKE_DETAIL"],
        "smoke_ok": os.environ["RV_IMAGE_SMOKE_OK"] == "true",
    },
    "repository": {
        "tracked_files": int(os.environ["RV_TRACKED"]), "fsck": os.environ["RV_FSCK"],
        "fsck_errors": int(os.environ["RV_FSCK_ERRORS"]),
        "main_behind_origin": os.environ.get("RV_MAIN_BEHIND", "false") == "true",
    },
    "governance": {
        "command": os.environ["RV_GOV_COMMAND"], "blocked": os.environ["RV_GOV_BLOCKED"] == "true",
        "open": parts("RV_GOV_OPEN", ";"), "pending": parts("RV_GOV_PENDING", ";"),
        "note": "O portão das decisões humanas do lançamento (make release-gate): vermelho aqui não é defeito de código, é decisão em aberto.",
    },
    "limits": parts("RV_LIMITS"),
    "next": parts("RV_NEXT"),
}
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    json.dump(facts, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
PY
) || fail "the facts could not be assembled"

(cd "$WORK" && go run ./tools/releaseverify render -facts "$FACTS_FILE" -out "$OUT") \
	|| fail "the checklist could not be rendered"

# The document is the run's only addition, and the run is not green until the
# document passes the tool's own rules.
DIRTY="$(git -C "$ROOT" status --porcelain --untracked-files=all | awk '{print $NF}' | sort | paste -sd',' -)"
if [ "$DIRTY" != "$EXPECTED_DIRTY" ]; then
	fail "the run left the repository holding [$DIRTY] and can account for [$EXPECTED_DIRTY]"
fi

(cd "$WORK" && go run ./tools/releaseverify check -root "$ROOT" -file "$GENERATED") \
	|| fail "the checklist this run produced is refused by the tool's own rules"

log "wrote $OUT"
printf 'release-verify: %s\n' "$OUT" >&2

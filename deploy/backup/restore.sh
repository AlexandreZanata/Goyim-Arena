#!/usr/bin/env bash
#
# restore.sh — restore a base backup into an empty directory and recover to a
# point in time (P19-T04).
#
# This is the script both an incident and the monthly exercise run, which is the
# only arrangement in which the exercise proves something: a restore path that
# exists solely for drills is a restore path that has never been used when it
# matters.
#
# The order of the steps is the order of the guarantees:
#
#   1. the destination must be empty — restoring into a directory with a cluster
#      in it is a merge, and a merge of two data directories is a corrupt one
#      that no checksum catches;
#   2. the sealed tar is downloaded *as the store holds it* and its checksum is
#      compared with the manifest: what the backup said it uploaded is what
#      arrived;
#   3. it is unsealed, which authenticates every record — an object edited
#      between the two steps fails here rather than after recovery;
#   4. it is extracted, and the recovery configuration is written;
#   5. the server is started (only when asked) and, when asked, waited for until
#      it has promoted: a restore that starts a server in recovery has restored
#      something nobody can write to yet.
#
# What it never does: overwrite a destination, guess a target, or start a server
# that keeps accepting connections in recovery.
set -euo pipefail

BACKUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
. "$BACKUP_DIR/common.sh"

pgdata=""
base=""
target_time=""
target_lsn=""
target_action="promote"
start_server="no"
wait_promotion="no"
timeout_seconds="${BACKUP_RESTORE_TIMEOUT:-900}"
port="${BACKUP_RESTORE_PORT:-}"
# The role used to ask the server whether it is still recovering. It defaults to
# the image's own superuser name and is configurable because the cluster was
# created with POSTGRES_USER, which need not be `postgres`: a restore that
# cannot ask its question would be a restore that reports hope.
superuser="${PGUSER:-postgres}"

usage() {
	cat >&2 <<'USAGE'
usage: restore.sh --pgdata <dir> [--base <name>|--latest] [--target-time <timestamptz>]
                  [--target-lsn <lsn>] [--target-action promote|pause|shutdown]
                  [--start] [--wait-promotion] [--port <n>] [--timeout <seconds>]

  --pgdata           the directory to restore into; it must be empty or absent
  --base             the base backup name to restore (see `backupctl list base/`)
  --latest           restore the newest complete base backup
  --target-time      recover to this point in time (default: the end of the archive)
  --target-lsn       recover to this log sequence number
  --target-action    what PostgreSQL does when the target is reached (default promote)
  --start            start the server after preparing the directory
  --wait-promotion   wait until the server has left recovery (implies --start)
  --superuser        the role asked whether recovery finished (default $PGUSER or postgres)
USAGE
}

while (( $# > 0 )); do
	case "$1" in
	--pgdata)
		shift
		(( $# > 0 )) || backup_fail "--pgdata takes a path"
		pgdata="$1"
		;;
	--base)
		shift
		(( $# > 0 )) || backup_fail "--base takes a name"
		base="$1"
		;;
	--latest)
		base=""
		latest="yes"
		;;
	--target-time)
		shift
		(( $# > 0 )) || backup_fail "--target-time takes a timestamp"
		target_time="$1"
		;;
	--target-lsn)
		shift
		(( $# > 0 )) || backup_fail "--target-lsn takes a log sequence number"
		target_lsn="$1"
		;;
	--target-action)
		shift
		(( $# > 0 )) || backup_fail "--target-action takes a value"
		target_action="$1"
		;;
	--port)
		shift
		(( $# > 0 )) || backup_fail "--port takes a number"
		port="$1"
		;;
	--superuser)
		shift
		(( $# > 0 )) || backup_fail "--superuser takes a role name"
		superuser="$1"
		;;
	--timeout)
		shift
		(( $# > 0 )) || backup_fail "--timeout takes a number of seconds"
		timeout_seconds="$1"
		;;
	--start)
		start_server="yes"
		;;
	--wait-promotion)
		start_server="yes"
		wait_promotion="yes"
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage
		backup_fail "unknown argument $1"
		;;
	esac
	shift
done

[[ -n "$pgdata" ]] || {
	usage
	backup_fail "--pgdata is required"
}
case "$target_action" in
promote | pause | shutdown) ;;
*) backup_fail "--target-action must be promote, pause or shutdown" ;;
esac

backup_require_tool
backup_require_env BACKUP_S3_ENDPOINT BACKUP_S3_BUCKET BACKUP_S3_ACCESS_KEY BACKUP_S3_SECRET_KEY BACKUP_ENCRYPTION_KEY_FILE
backup_require_command tar

# 1. An empty destination, or none at all. The check is on the directory's
# contents rather than on its existence, because an empty directory left by an
# earlier attempt is a legitimate destination and a half-restored one is not.
if [[ -e "$pgdata" ]]; then
	[[ -d "$pgdata" ]] || backup_fail "$pgdata exists and is not a directory"
	if [[ -n "$(ls -A "$pgdata" 2>/dev/null)" ]]; then
		backup_fail "$pgdata is not empty: restoring into a directory that holds a cluster would produce a merge, not a restore"
	fi
else
	mkdir -p "$pgdata" || backup_fail "could not create $pgdata"
fi
# The mode is the server's condition, not a preference: PostgreSQL refuses to
# start on a data directory that is not 0700 or 0750, and an operator who made
# the directory with `mkdir` gets 0755 from their umask. The directory is empty
# (checked above) and is about to become a cluster, so it is prepared here rather
# than left for the server to reject with a message about permissions.
chmod 0700 "$pgdata" || backup_fail "could not set the mode of $pgdata"

workdir="$(backup_workdir)"
trap 'rm -rf "$workdir"' EXIT

# 2. Which base backup. `--latest` asks the tool, so the selection rule (the
# newest *complete* backup, meaning one with a manifest) lives in one place
# instead of in a shell glob that a half-uploaded object would defeat.
if [[ -z "$base" ]]; then
	base="$("$BACKUP_BIN" latest-base)" || backup_fail "no complete base backup is stored; there is nothing to restore"
fi
[[ -n "$base" ]] || backup_fail "no complete base backup is stored; there is nothing to restore"
backup_log "restoring base backup $base"

manifest_plain="$workdir/manifest.json"
"$BACKUP_BIN" get "base/${base}.json.enc" "$manifest_plain" ||
	backup_fail "the manifest of $base could not be read; the backup is incomplete"

expected_bytes="$(backup_json_number "$manifest_plain" bytes)"
expected_sha="$(backup_json_field "$manifest_plain" sha256_sealed)"
start_lsn="$(backup_json_field "$manifest_plain" start_lsn)"
[[ -n "$expected_sha" ]] || backup_fail "the manifest of $base quotes no checksum, so nothing about it can be verified"

# 3. Download as the store holds it, and compare with what the manifest says was
# uploaded. This is the step that distinguishes "the object arrived" from "the
# object arrived unedited": the unsealing that follows authenticates the bytes
# it is given, and it cannot tell whether they are the bytes that were sent.
sealed="$workdir/base.tar.gz.enc"
"$BACKUP_BIN" get "base/${base}.tar.gz.enc" "$sealed" --raw ||
	backup_fail "the base backup $base could not be downloaded"
actual_sha="$(sha256sum "$sealed" | cut -d' ' -f1)"
if [[ "$actual_sha" != "$expected_sha" ]]; then
	backup_fail "the downloaded base backup hashes to $actual_sha and the manifest records $expected_sha: the object in the store is not the object this backup uploaded"
fi
if [[ -n "$expected_bytes" ]]; then
	actual_bytes="$(wc -c <"$sealed" | tr -d ' ')"
	[[ "$actual_bytes" == "$expected_bytes" ]] ||
		backup_fail "the downloaded base backup is $actual_bytes bytes and the manifest records $expected_bytes"
fi
backup_log "the sealed base backup matches the manifest (sha256 $actual_sha)"

# 4. Unseal (every record authenticates) and extract.
plain="$workdir/base.tar.gz"
"$BACKUP_BIN" unseal "$sealed" "$plain" || backup_fail "the base backup did not unseal cleanly"
tar -xzf "$plain" -C "$pgdata" || backup_fail "the base backup tar could not be extracted into $pgdata"

# 5. The recovery configuration. It is written to postgresql.auto.conf, which is
# the file PostgreSQL itself maintains for settings that must not be edited by
# hand, and a recovery.signal is what makes a PostgreSQL 12+ server perform
# recovery instead of refusing to start on a data directory it would otherwise
# treat as a crashed primary.
auto_conf="$pgdata/postgresql.auto.conf"
{
	printf "restore_command = '%s %%f %%p'\n" "$BACKUP_DIR/restore-command.sh"
	printf "recovery_target_action = '%s'\n" "$target_action"
	if [[ -n "$target_time" ]]; then
		printf "recovery_target_time = '%s'\n" "$target_time"
		printf 'recovery_target_inclusive = on\n'
	fi
	if [[ -n "$target_lsn" ]]; then
		printf "recovery_target_lsn = '%s'\n" "$target_lsn"
		printf 'recovery_target_inclusive = on\n'
	fi
} >>"$auto_conf" || backup_fail "could not write the recovery configuration"
touch "$pgdata/recovery.signal" || backup_fail "could not write recovery.signal"

target_description="the end of the archive"
[[ -n "$target_time" ]] && target_description="time $target_time (inclusive)"
[[ -n "$target_lsn" ]] && target_description="lsn $target_lsn (inclusive)"
backup_log "prepared $pgdata to recover to $target_description, starting from $start_lsn"

if [[ "$start_server" != "yes" ]]; then
	backup_log "not starting the server: run postgres -D $pgdata when you are ready"
	exit 0
fi

# 6. Start, and optionally wait. The wait is what makes the completion of a
# recovery an *observation* rather than a hope: a server that is still in
# recovery has restored a database nobody can write to, and reporting success
# there would be reporting the wrong thing.
if [[ "$(id -u)" == "0" ]]; then
	backup_require_command chown
	chown -R postgres:postgres "$pgdata" || backup_fail "could not hand $pgdata to the postgres user"
	start_command=(gosu postgres postgres -D "$pgdata")
	[[ -n "$port" ]] && start_command+=(-c "port=$port")
else
	start_command=(postgres -D "$pgdata")
	[[ -n "$port" ]] && start_command+=(-c "port=$port")
fi

log_file="${BACKUP_RESTORE_LOG:-$workdir/postgres.log}"
backup_log "starting ${start_command[*]} (log: $log_file)"
"${start_command[@]}" >"$log_file" 2>&1 &
server_pid=$!

if [[ "$wait_promotion" != "yes" ]]; then
	backup_log "server started with pid $server_pid; the log is at $log_file"
	exit 0
fi

# postgres_in_recovery prints the server's own answer to "am I still
# recovering", or nothing at all while it is not answering yet. The question is
# asked of the server rather than inferred from the log, because a log line is a
# message and this needs a fact.
#
# The host is deliberately not passed: the server listens for local clients
# wherever its configuration puts the socket (in this image,
# /var/run/postgresql), and a client that named the data directory instead would
# be asking at an address nothing is listening on — which is a completed
# recovery reported as one that never finished.
postgres_in_recovery() {
	local port="$1"
	local port_argument=()
	[[ -n "$port" ]] && port_argument=(-p "$port")
	psql --no-psqlrc --tuples-only --no-align -U "$superuser" "${port_argument[@]}" \
		-c 'SELECT pg_is_in_recovery()' 2>/dev/null | head -1
}

deadline=$((SECONDS + timeout_seconds))
while (( SECONDS < deadline )); do
	if ! kill -0 "$server_pid" 2>/dev/null; then
		tail -20 "$log_file" >&2 || true
		backup_fail "the server exited before it finished recovering; the log is at $log_file"
	fi
	in_recovery="$(postgres_in_recovery "$port" || true)"
	if [[ "$in_recovery" == "f" ]]; then
		backup_log "recovery finished and the server is accepting writes"
		printf 'RESTORE_BASE=%s\nRESTORE_TARGET=%s\nRESTORE_SERVER_PID=%s\n' "$base" "$target_description" "$server_pid"
		exit 0
	fi
	sleep 1
done

tail -20 "$log_file" >&2 || true
backup_fail "the server was still in recovery after ${timeout_seconds}s; the log is at $log_file"

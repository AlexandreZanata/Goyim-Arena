#!/usr/bin/env bash
#
# Shared library for the PostgreSQL backup scripts (P19-T04).
#
# Sourced, never executed. It holds the four things every one of these scripts
# would otherwise repeat, and would repeat slightly differently:
#
#   logging      one prefix, on stderr, because PostgreSQL captures the output
#                of archive_command into the server log and a message without a
#                prefix is a message nobody can attribute;
#   validation   the variables the scripts need, all of them named at once: a
#                deployment that has to be fixed twice for the same reason is a
#                deployment that will be fixed twice;
#   temporaries  a directory that is removed on every exit path, so a sealed
#                object never survives a failed run;
#   the lock     so that two base backups cannot run at the same time and the
#                retention cannot delete a backup that is being uploaded.
#
# Nothing here holds a credential, and nothing here prints one: the object store
# is configured through the environment (BACKUP_S3_*) and the scripts only ever
# pass it on to `backupctl`, which reads it from the same environment.

set -euo pipefail

BACKUP_BIN="${BACKUP_BIN:-/opt/backup-tool/backupctl}"
BACKUP_TOOL="${BACKUP_TOOL:-$(basename "$BACKUP_BIN")}"

backup_log() {
	printf 'backup: %s\n' "$*" >&2
}

backup_fail() {
	printf 'backup: %s\n' "$*" >&2
	exit 1
}

backup_require_command() {
	local name="$1"
	command -v "$name" >/dev/null 2>&1 || backup_fail "$name is required and was not found in PATH"
}

# backup_require_env <name>... — refuses to run without every variable the
# operation needs, naming all of them. The check is a refusal rather than a
# default, because the default of "no endpoint configured" is a backup that
# silently goes nowhere.
backup_require_env() {
	local missing=()
	for name in "$@"; do
		if [[ -z "${!name:-}" ]]; then
			missing+=("$name")
		fi
	done
	if (( ${#missing[@]} > 0 )); then
		backup_fail "these variables are required and are not set: ${missing[*]}"
	fi
}

# backup_require_tool checks that the object-store tool is where this script
# expects it, and says where that is: a script that fails with "command not
# found" sends an operator looking for the wrong thing.
backup_require_tool() {
	if [[ ! -x "$BACKUP_BIN" ]]; then
		backup_fail "$BACKUP_BIN is not executable; the deployment mounts the backup tool there (BACKUP_BIN overrides the path)"
	fi
}

# backup_workdir draws a private temporary directory and prints it. It
# deliberately does *not* schedule the removal itself: every caller draws it in a
# command substitution, and the EXIT trap of a subshell runs when that subshell
# ends — which is immediately after the assignment, so a trap installed here
# would delete the directory before the caller wrote anything into it. The caller
# holds the path and schedules the removal in its own shell:
#
#     workdir="$(backup_workdir)"
#     trap 'rm -rf "$workdir"' EXIT
#
# A script that already owns an EXIT trap can append there instead of replacing
# it, which is why the removal is the caller's line and not the helper's.
backup_workdir() {
	local directory
	directory="$(mktemp -d "${BACKUP_TMPDIR:-/tmp}/backup-XXXXXXXX")" || backup_fail "could not create a temporary directory"
	chmod 0700 "$directory"
	printf '%s' "$directory"
}

# backup_lock <name> runs the rest of the script under an exclusive lock, so that
# two invocations cannot interleave: a base backup and a retention run at the
# same moment is how a backup is deleted while it is being uploaded.
backup_lock() {
	local name="$1"
	local directory="${BACKUP_LOCK_DIR:-/var/lock}"
	if [[ ! -d "$directory" ]]; then
		directory="${BACKUP_TMPDIR:-/tmp}"
	fi
	local lock="$directory/arena-backup-$name.lock"
	if command -v flock >/dev/null 2>&1; then
		exec 9>"$lock"
		if ! flock --nonblock 9; then
			backup_fail "another $name run holds $lock; refusing to run two at once"
		fi
		return 0
	fi
	# Without flock the scripts still work; the lock is a safety property, not
	# a correctness one, and a host without flock is a host whose schedule the
	# operator controls anyway. It is announced rather than assumed.
	backup_log "flock is not available: running without the $name lock"
}

# backup_segment_name validates the shape PostgreSQL uses for a WAL file. The
# archive and the retention policy both compare these names as strings, which is
# only meaningful when every name has the same shape.
# backup_validate_segment accepts every file name PostgreSQL hands to
# archive_command, which is more than the segment: the server also archives the
# backup history file it writes when a base backup finishes
# (<segment>.<offset>.backup), the timeline history after a switch
# (<timeline>.history) and, with archive_mode=always, a .partial segment. A
# validator that accepted only the segments would make the archive command fail
# on the history files — and because a checkpoint waits for the archive, that
# failure is not a missing object but a server that stops completing checkpoints.
backup_validate_segment() {
	local name="$1"
	if [[ ! "$name" =~ ^([0-9A-F]{24}|[0-9A-F]{24}\.partial|[0-9A-F]{24}\.[0-9A-F]{8}\.backup|[0-9A-F]{8}\.history)$ ]]; then
		backup_fail "$name is not a name PostgreSQL archives (a 24-digit WAL segment, a segment with .partial, a <segment>.<offset>.backup history file, or a <timeline>.history file)"
	fi
}

# backup_json_field reads one string field of a manifest. The manifests this
# pipeline writes have a flat, fixed shape, and the alternative — a JSON parser
# in a container whose only tools are the ones PostgreSQL ships — would be a
# parser somebody has to trust.
backup_json_field() {
	local file="$1" field="$2"
	sed -nE "s/.*\"$field\"[[:space:]]*:[[:space:]]*\"([^\"]*)\".*/\1/p" "$file" | head -1
}

backup_json_number() {
	local file="$1" field="$2"
	sed -nE "s/.*\"$field\"[[:space:]]*:[[:space:]]*([0-9]+).*/\1/p" "$file" | head -1
}

#!/usr/bin/env bash
#
# restore-command.sh <file> <path> — PostgreSQL's restore_command (P19-T04).
#
# Called by a server in recovery for every WAL file it needs, with the file name
# and the destination path. The exit status is the whole interface: zero means
# the file is now at the destination, anything else means it is not available,
# which is how PostgreSQL distinguishes "not archived yet" from "archived and
# broken".
#
# The wrapper exists so that the recovery configuration names one file inside
# the deployment instead of embedding a tool invocation, and so that the
# translation from "file name" to "object name" lives in exactly one place.
set -euo pipefail

BACKUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
. "$BACKUP_DIR/common.sh"

if (( $# != 2 )); then
	backup_fail "usage: restore-command.sh <file> <path> (PostgreSQL calls it with %f %p)"
fi

requested="$1"
destination="$2"

backup_require_tool
backup_require_env BACKUP_S3_ENDPOINT BACKUP_S3_BUCKET BACKUP_S3_ACCESS_KEY BACKUP_S3_SECRET_KEY BACKUP_ENCRYPTION_KEY_FILE

# Only segments are archived. A timeline history file or a partial segment is
# reported as unavailable — with a message, because a recovery that silently
# gives up on a file it was asked for is a recovery nobody can diagnose.
if [[ ! "$requested" =~ ^[0-9A-F]{24}$ ]]; then
	backup_log "$requested is not a WAL segment name; the archive holds segments only"
	exit 1
fi

if ! "$BACKUP_BIN" wal-fetch "$requested" "$destination"; then
	backup_log "$requested is not in the archive"
	exit 1
fi

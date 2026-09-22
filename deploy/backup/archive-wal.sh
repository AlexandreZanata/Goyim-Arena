#!/usr/bin/env bash
#
# archive-wal.sh <path> <file> — PostgreSQL's archive_command (P19-T04).
#
# PostgreSQL calls this once per completed WAL segment, with the segment's
# absolute path and its file name, and it reads the *exit status*: zero means
# the segment is archived and the server may recycle it, anything else means it
# is not and the server retries. Everything about this script follows from that
# contract:
#
#   * it is fast, because the segment cannot be recycled while it runs and the
#     server's write path waits on it;
#   * it is idempotent, because after a crash the server re-archives a segment
#     whose upload had already succeeded — the store holding the same segment
#     name is the archive already done, and that is success;
#   * it never writes a partial object, because S3 writes are atomic: the
#     segment is sealed into a temporary file first and uploaded in one request;
#   * it says what went wrong on stderr, because PostgreSQL puts that in the
#     server log and reports it in pg_stat_archiver, which is the alert surface
#     for a broken archive (docs/DEPLOYMENT.md §7/§8).
#
# What it deliberately does not do: retry. PostgreSQL already retries, with its
# own interval, and a second retry loop inside the command would make the
# server's own accounting of failures wrong.
set -euo pipefail

BACKUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
. "$BACKUP_DIR/common.sh"

if (( $# != 2 )); then
	backup_fail "usage: archive-wal.sh <path> <file> (PostgreSQL calls it with %p %f)"
fi

segment_path="$1"
segment_name="$2"

backup_validate_segment "$segment_name"
[[ -f "$segment_path" ]] || backup_fail "the segment $segment_path does not exist"
[[ -s "$segment_path" ]] || backup_fail "the segment $segment_path is empty, and PostgreSQL never archives an empty segment: refusing covers a caller that is not PostgreSQL"

backup_require_tool
backup_require_env BACKUP_S3_ENDPOINT BACKUP_S3_BUCKET BACKUP_S3_ACCESS_KEY BACKUP_S3_SECRET_KEY BACKUP_ENCRYPTION_KEY_FILE

# The object name carries the segment name and nothing else: a WAL segment whose
# name and content disagree is not something a restore can detect, so the name
# is the only address the archive and the restore_command have to agree on.
if ! "$BACKUP_BIN" put "wal/${segment_name}.enc" "$segment_path" --if-absent; then
	backup_fail "archiving ${segment_name} failed; PostgreSQL will retry and pg_stat_archiver will report the failure"
fi

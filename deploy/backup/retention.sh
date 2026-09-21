#!/usr/bin/env bash
#
# retention.sh [--apply] — what the archive keeps, and what it lets go
# (P19-T04).
#
# The policy is two-sided and lives in one place (the tool's `prune`): age
# decides, and a floor decides when age would remove too much. This script adds
# the two things a shell owns — the schedule's parameters and the decision to
# actually delete anything.
#
# It is a dry run unless `--apply` is given. That default is the whole point:
# retention is the only operation in this pipeline that destroys data, and a
# default that destroys is a default that destroys on a typo. The same ordering
# is why the WAL floor is derived from the oldest base backup that *stays*: WAL
# older than that restores nothing that is still stored, and WAL newer than it
# may be the difference between a restore and a gap.
set -euo pipefail

BACKUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
. "$BACKUP_DIR/common.sh"

apply="no"
while (( $# > 0 )); do
	case "$1" in
	--apply) apply="yes" ;;
	-h | --help)
		printf 'usage: retention.sh [--apply]   (BACKUP_KEEP_DAYS, BACKUP_KEEP_MIN)\n' >&2
		exit 0
		;;
	*) backup_fail "unknown argument $1 (usage: retention.sh [--apply])" ;;
	esac
	shift
done

keep_days="${BACKUP_KEEP_DAYS:-30}"
keep_min="${BACKUP_KEEP_MIN:-7}"
case "$keep_days" in
'' | *[!0-9]*) backup_fail "BACKUP_KEEP_DAYS must be a number of days, got $keep_days" ;;
esac
case "$keep_min" in
'' | *[!0-9]*) backup_fail "BACKUP_KEEP_MIN must be a number of backups, got $keep_min" ;;
esac
(( keep_min > 0 )) || backup_fail "BACKUP_KEEP_MIN of 0 would let retention remove the last backup of a deployment that stopped backing up"

backup_lock retention
backup_require_tool
backup_require_env BACKUP_S3_ENDPOINT BACKUP_S3_BUCKET BACKUP_S3_ACCESS_KEY BACKUP_S3_SECRET_KEY BACKUP_ENCRYPTION_KEY_FILE

mode="reporting only"
[[ "$apply" == "yes" ]] && mode="applying"
backup_log "retention: keeping $keep_days days and at least $keep_min base backups, $mode"

arguments=(--keep-days "$keep_days" --keep-min "$keep_min")
[[ "$apply" == "yes" ]] && arguments+=(--apply)

# The WAL prefix is pruned after the base prefix, and the order is deliberate:
# the floor is computed from the manifests that survive this run, so the base
# decision has to be made first. (The tool re-reads the manifests, so a crash
# between the two leaves WAL that is still usable — never a base backup without
# its WAL.)
"$BACKUP_BIN" prune --prefix base/ "${arguments[@]}" || backup_fail "pruning base backups failed"
"$BACKUP_BIN" prune --prefix wal/ "${arguments[@]}" || backup_fail "pruning WAL failed"

backup_log "retention finished ($mode)"

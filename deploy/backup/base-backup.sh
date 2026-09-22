#!/usr/bin/env bash
#
# base-backup.sh — one base backup, sealed into the object store (P19-T04).
#
# The base backup is what gives the WAL archive a starting point: without it a
# WAL segment restores nothing, and with it alone nothing after the backup's
# start point can be replayed. The two objects therefore belong together, and
# the way this script keeps them together is the *order* of the uploads:
#
#   1. the sealed tar of the data directory;
#   2. the manifest, which quotes the sealed tar's checksum and the WAL location
#      the backup starts at.
#
# The manifest is uploaded last, and its presence is what marks the backup as
# complete. A run killed between the two leaves an object no retention policy
# counts as a backup and no restore path selects — which is the failure that
# wants to be obvious rather than the one that wants to be half-working.
#
# The WAL the backup needs is *not* bundled: `pg_basebackup -X none` requires
# the running server to archive it (archive_mode=on, `archive-wal.sh`), so the
# base backup is exactly as good as the archive, and a gap in the archive is
# visible here rather than hidden inside a tarball.
set -euo pipefail

BACKUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
. "$BACKUP_DIR/common.sh"

label="${BACKUP_LABEL:-}"
while (( $# > 0 )); do
	case "$1" in
	--label)
		shift
		(( $# > 0 )) || backup_fail "--label takes a value"
		label="$1"
		;;
	-h | --help)
		printf 'usage: base-backup.sh [--label <label>]\n' >&2
		exit 0
		;;
	*)
		backup_fail "unknown argument $1 (usage: base-backup.sh [--label <label>])"
		;;
	esac
	shift
done

backup_lock base-backup
backup_require_tool
backup_require_env BACKUP_S3_ENDPOINT BACKUP_S3_BUCKET BACKUP_S3_ACCESS_KEY BACKUP_S3_SECRET_KEY BACKUP_ENCRYPTION_KEY_FILE
backup_require_command pg_basebackup
backup_require_command tar

# The label doubles as the object name, so it must be safe in a key and sortable
# in a listing. A label given by the operator is checked rather than trusted:
# a name that needed escaping would be a name the retention policy reads wrong.
if [[ -z "$label" ]]; then
	label="$(date -u +%Y-%m-%dT%H-%M-%SZ)"
fi
case "$label" in
*[!0-9A-Za-zTZ:._-]* | "") backup_fail "--label $label is not usable as an object name" ;;
esac

workdir="$(backup_workdir)"
trap 'rm -rf "$workdir"' EXIT
backup_log "taking a base backup labelled $label"

# -Ft -z: one tar.gz, so one sealed object and one checksum. -X none: the WAL
# comes from the archive, which is the arrangement this pipeline is built on.
# --checkpoint=fast: the checkpoint is forced instead of waited for, because the
# alternative is a backup that takes as long as the checkpoint interval.
pg_basebackup -D "$workdir/base" -Ft -z -X none -c fast -l "$label" >&2 ||
	backup_fail "pg_basebackup failed; no object was uploaded"

tarball="$workdir/base/base.tar.gz"
[[ -f "$tarball" ]] || backup_fail "pg_basebackup produced no $tarball"

# The backup label inside the tar is where the figures come from: PostgreSQL
# wrote them, and a value this script computed itself would be a second opinion
# about the same thing.
label_file="$workdir/backup_label"
tar -xzOf "$tarball" backup_label >"$label_file" 2>/dev/null ||
	backup_fail "the base backup carries no backup_label, so its start point is unknown"

start_lsn="$(sed -nE 's/^START WAL LOCATION:[[:space:]]*([0-9A-F]+\/[0-9A-F]+).*/\1/p' "$label_file" | head -1)"
start_time="$(sed -nE 's/^START TIME:[[:space:]]*(.*)$/\1/p' "$label_file" | head -1)"
timeline="$(sed -nE 's/^START TIMELINE:[[:space:]]*([0-9]+).*/\1/p' "$label_file" | head -1)"
[[ -n "$start_lsn" ]] || backup_fail "the backup_label quotes no start WAL location, so the archive floor cannot be derived"
[[ -n "$timeline" ]] || timeline=1

record="$workdir/record.json"
backup_log "uploading the sealed base backup ($(du -h "$tarball" | cut -f1))"
"$BACKUP_BIN" put "base/${label}.tar.gz.enc" "$tarball" --record "$record" ||
	backup_fail "uploading the base backup failed; nothing was written for the manifest, so this run is not a backup"

sealed_bytes="$(backup_json_number "$record" bytes)"
sealed_sha="$(backup_json_field "$record" sha256)"
[[ -n "$sealed_bytes" && -n "$sealed_sha" ]] || backup_fail "the upload reported no checksum, and a manifest without one verifies nothing"

manifest="$workdir/manifest.json"
# The seal is re-done inside `backupctl put`, which is why the checksum cannot be
# computed here: sealing the same bytes twice produces two different objects.
printf '{"name":"%s","label":"%s","created_at":"%s","start_lsn":"%s","start_time":"%s","timeline":%s,"bytes":%s,"sha256_sealed":"%s"}\n' \
	"$label" "$label" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$start_lsn" "$start_time" "$timeline" "$sealed_bytes" "$sealed_sha" >"$manifest"

"$BACKUP_BIN" put "base/${label}.json.enc" "$manifest" ||
	backup_fail "uploading the manifest failed: the base backup is in the store but incomplete, and no restore will select it"

backup_log "base backup $label is complete (starts at $start_lsn, ${sealed_bytes} sealed bytes)"

#!/usr/bin/env bash
# ============================================================================
# backup_db.sh — logical backup of the PropVest database (Phase 3.5)
#
# Uses pg_dump custom format (-Fc): compressed, and supports pg_restore into a
# DIFFERENT database name, which is what the restore drill needs.
#
# Usage:
#   scripts/backup_db.sh                 # -> backups/propvest_YYYYmmdd_HHMMSS.dump
#   BACKUP_DIR=/elsewhere scripts/backup_db.sh
#
# Exit codes: 0 = dump written, non-zero = failed (the caller should alert).
# ============================================================================
set -euo pipefail

cd "$(dirname "$0")/.."   # repo root, so .env and internal/ are where we expect

# DATABASE_URL drives everything so this works unchanged in staging/prod.
DATABASE_URL="${DATABASE_URL:-$(grep -E '^DATABASE_URL=' .env | head -1 | cut -d= -f2-)}"
if [ -z "$DATABASE_URL" ]; then
  echo "ERROR: DATABASE_URL not set and not found in .env" >&2
  exit 1
fi

BACKUP_DIR="${BACKUP_DIR:-backups}"
STAMP="$(date +%Y%m%d_%H%M%S)"
OUT="$BACKUP_DIR/propvest_${STAMP}.dump"

mkdir -p "$BACKUP_DIR"

echo "Backing up to $OUT ..."
pg_dump --no-owner --no-privileges -Fc -d "$DATABASE_URL" -f "$OUT"

# Keep the newest 14 dumps (two weeks at daily cadence); older ones rotate out.
ls -1t "$BACKUP_DIR"/propvest_*.dump 2>/dev/null | tail -n +15 | while read -r old; do
  echo "Rotating out old backup: $old"
  rm -f "$old"
done

# Always refresh a stable name for the drill / quick restore.
cp -f "$OUT" "$BACKUP_DIR/propvest_latest.dump"

echo "OK: $OUT ($(du -h "$OUT" | cut -f1))"

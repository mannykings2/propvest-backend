#!/usr/bin/env bash
# ============================================================================
# restore_drill.sh — "can we actually restore?" (Phase 3.5)
#
# Restores a backup into a SCRATCH database (propvest_restore_drill), then
# compares table list + row counts against the source database. Running this
# on a schedule is the difference between having backups and having restores.
#
# Usage:
#   scripts/restore_drill.sh                # drill with the newest backup
#   scripts/restore_drill.sh path/to.dump   # drill with a specific backup
#
# Only the scratch database (propvest_restore_drill) is ever dropped/created.
# Exit codes: 0 = restore verified, non-zero = DRILL FAILED (alert on this).
# ============================================================================
set -euo pipefail

cd "$(dirname "$0")/.."

DATABASE_URL="${DATABASE_URL:-$(grep -E '^DATABASE_URL=' .env | head -1 | cut -d= -f2-)}"
if [ -z "$DATABASE_URL" ]; then
  echo "ERROR: DATABASE_URL not set and not found in .env" >&2
  exit 1
fi

DUMP="${1:-$(ls -1t backups/propvest_*.dump 2>/dev/null | head -1)}"
if [ -z "$DUMP" ] || [ ! -f "$DUMP" ]; then
  echo "ERROR: no backup dump found (run scripts/backup_db.sh first)" >&2
  exit 1
fi

DRILL_DB="propvest_restore_drill"
SRC_DB=$(echo "$DATABASE_URL" | sed -E 's|.*/([^?]+)(\?.*)?|\1|')
# Swap only the database-name segment (last path component before any query),
# keeping host/port/user/query intact — done in bash because a sed with '|' as
# both delimiter and alternation char silently mangles the pattern.
BASE_NO_QUERY="${DATABASE_URL%%\?*}"          # url without ?query
URL_QUERY="${DATABASE_URL#"$BASE_NO_QUERY"}"   # "?query" or empty
DRILL_URL="${BASE_NO_QUERY%/*}/${DRILL_DB}${URL_QUERY}"

echo "== Restore drill =="
echo "   dump:    $DUMP"
echo "   source:  $SRC_DB"
echo "   scratch: $DRILL_DB (dropped + recreated each run)"

echo "[1/4] recreating scratch database ..."
# Separate statements: psql -c with multiple statements runs them in one
# implicit transaction, and DROP DATABASE cannot run inside one.
psql -d "$DATABASE_URL" -v ON_ERROR_STOP=1 -qc "DROP DATABASE IF EXISTS $DRILL_DB WITH (FORCE);"
psql -d "$DATABASE_URL" -v ON_ERROR_STOP=1 -qc "CREATE DATABASE $DRILL_DB;"

echo "[2/4] restoring dump into scratch ..."
# pg_restore exits non-zero on some benign errors, so capture stderr and judge
# it instead of trusting the exit code — but do NOT swallow real failures.
restore_log="$(mktemp)"
pg_restore --no-owner --no-privileges -d "$DRILL_URL" "$DUMP" 2>"$restore_log" || true
# Known-benign: pg_dump 18 emits `SET transaction_timeout = 0`, which servers
# older than 17 don't understand. Everything else is a genuine restore error.
real_errors="$(grep -v 'transaction_timeout' "$restore_log" \
  | grep -E '^pg_restore: error|^pg_restore: fatal|ERROR:' || true)"
if [ -n "$real_errors" ]; then
  echo "FAIL: pg_restore reported non-benign errors:" >&2
  echo "$real_errors" >&2
  rm -f "$restore_log"
  exit 1
fi
rm -f "$restore_log"
echo "   restore completed (only known-benign errors, if any)"

echo "[3/4] comparing table list ..."
src_tables=$(psql -d "$DATABASE_URL" -tAc "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY 1")
drill_tables=$(psql -d "$DRILL_URL" -tAc "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY 1")
if [ "$src_tables" != "$drill_tables" ]; then
  echo "FAIL: table list differs between $SRC_DB and $DRILL_DB" >&2
  diff <(echo "$src_tables") <(echo "$drill_tables") >&2 || true
  exit 1
fi
table_count=$(echo "$src_tables" | grep -c . || true)
echo "   $table_count tables match"

echo "[4/4] comparing row counts per table ..."
mismatch=0
while IFS= read -r t; do
  t=${t%$'\r'}   # Windows psql emits CRLF; the bare CR would break quoting
  [ -z "$t" ] && continue
  src_n=$(psql -d "$DATABASE_URL" -tAc "SELECT count(*) FROM \"$t\"")
  drill_n=$(psql -d "$DRILL_URL" -tAc "SELECT count(*) FROM \"$t\"")
  if [ "$src_n" != "$drill_n" ]; then
    echo "FAIL: $t — source=$src_n restored=$drill_n" >&2
    mismatch=1
  fi
done <<< "$src_tables"
[ "$mismatch" -ne 0 ] && exit 1

echo "PASS: restored $DUMP into $DRILL_DB — $table_count tables, all row counts identical."
echo "Drill result: the backup is genuinely restorable."

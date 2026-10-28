# Scripts

This directory contains helper scripts for testing, development and operations.

## Credit Wallet Script

**File:** `credit_wallet.sql`

Manually credit a user's wallet for testing withdrawal functionality.

### Quick Usage

```bash
# 1. Connect to your database
psql -h localhost -p 5435 -U propvest -d propvest

# 2. Find your user ID
SELECT id, email FROM users WHERE email = 'your-email@example.com';

# 3. Edit credit_wallet.sql and replace 'YOUR-USER-ID-HERE' with your actual UUID

# 4. Run the script
\i scripts/credit_wallet.sql

# Or run directly:
psql -h localhost -p 5435 -U propvest -d propvest -f scripts/credit_wallet.sql
```

### What It Does

- Credits ₦100,000.00 (10,000,000 kobo) to the specified wallet
- Creates a transaction record for audit trail
- Shows before/after balance confirmation

### Alternative: Use PowerShell

```powershell
# Get your user ID first
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT id, email FROM users WHERE email = 'test@example.com';"

# Then run the credit script (after editing the user ID)
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -f scripts/credit_wallet.sql
```

## Database backup (Phase 3.5)

**Files:** `backup_db.sh`, `restore_drill.sh`

```bash
# Take a logical backup (pg_dump custom format, compressed)
bash scripts/backup_db.sh
#   -> backups/propvest_YYYYmmdd_HHMMSS.dump (+ backups/propvest_latest.dump)

# Prove the backup is actually restorable (table list + row counts)
bash scripts/restore_drill.sh          # drills the newest backup
DUMP=backups/propvest_20261025_1200.dump bash scripts/restore_drill.sh
```

The drill restores into a **scratch database** (`propvest_restore_drill`,
recreated every run) and compares the table list and every table's row count
against the source. It fails loudly on any mismatch or any non-benign
`pg_restore` error, so a green run means the backup is genuinely restorable —
not merely written.

**RPO / RTO (defaults to tune per environment):**

| | |
|---|---|
| RPO (max data loss) | = backup interval. Schedule `backup_db.sh` at least daily; for ≤1 h loss, run it hourly. |
| RTO (time to recover) | Backup (seconds for this DB size) + `restore_drill.sh` runtime (~seconds locally). Restoring into the real DB = drill time + app restart. |

Restore into the real database by pointing `pg_restore` at it instead of the
scratch DB (stop the API/worker first):

```bash
pg_restore --no-owner --no-privileges -d "$DATABASE_URL" backups/propvest_latest.dump
```

> Note: dumps taken with `pg_dump` 18 restore fine on older servers; the only
> tolerated warning is `SET transaction_timeout` (a PG17+ GUC). The drill
> filters exactly that and fails on every other error.

## Important Notes

- **TEST ENVIRONMENT ONLY** - Never use this in production
- Default amount is ₦100,000.00 - edit the `v_amount_kobo` variable to change
- The script is wrapped in a transaction block - if anything fails, nothing is changed
- All transactions are logged in `wallet_transactions` table

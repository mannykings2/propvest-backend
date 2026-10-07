# Testing Scripts

This directory contains helper scripts for testing and development.

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

## Important Notes

- **TEST ENVIRONMENT ONLY** - Never use this in production
- Default amount is ₦100,000.00 - edit the `v_amount_kobo` variable to change
- The script is wrapped in a transaction block - if anything fails, nothing is changed
- All transactions are logged in `wallet_transactions` table

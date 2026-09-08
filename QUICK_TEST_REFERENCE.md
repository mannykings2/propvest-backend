# 🚀 Quick Test Reference Card

## One-Time Setup

```powershell
# 1. Configure Paystack webhook
# Go to: https://dashboard.paystack.com/settings/webhooks
# Add: https://YOUR_NGROK_URL/api/v1/webhooks/paystack/transfer
# Events: transfer.success, transfer.failed, transfer.reversed

# 2. Enable Paystack transfers
# Go to: https://dashboard.paystack.com/settings/transfers
# Click "Enable Transfers" and set Transfer PIN

# 3. Build executables
go build -o api.exe ./cmd/api
go build -o worker.exe ./cmd/worker
```

---

## Start Services (4 Terminals)

### Terminal 1: API Server
```powershell
.\api.exe
# Runs on http://localhost:8081
```

### Terminal 2: Ngrok Tunnel
```powershell
ngrok http 8081
# Copy the HTTPS URL for Paystack webhook config
```

### Terminal 3: Background Worker
```powershell
.\worker.exe
# Processes withdrawals and reconciles pending transfers
```

### Terminal 4: Testing Commands
```powershell
# Set your access token
$TOKEN = "eyJhbGciOiJIUzI1NiIs..."

# Ready to test!
```

---

## Quick Test Flow

### 1. Check Wallet Balance
```powershell
curl -X GET http://localhost:8081/api/v1/wallet `
  -H "Authorization: Bearer $TOKEN"
```

### 2. Initiate Withdrawal
```powershell
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount_kobo": 5000000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058",
    "bank_name": "GTBank"
  }'
```

### 3. Check Transaction Status
```powershell
curl -X GET http://localhost:8081/api/v1/wallet/transactions `
  -H "Authorization: Bearer $TOKEN"
```

### 4. Verify Final Balance
```powershell
curl -X GET http://localhost:8081/api/v1/wallet `
  -H "Authorization: Bearer $TOKEN"
```

---

## Expected Results

### After Initiation
```json
{
  "main_balance": 10000000,    // ₦100,000 (unchanged)
  "locked_balance": 5000000,   // ₦50,000 (LOCKED)
  "available": 5000000         // ₦50,000 (spendable)
}
```

### After Completion (via webhook)
```json
{
  "main_balance": 5000000,     // ₦50,000 (DEBITED)
  "locked_balance": 0,         // ₦0 (CLEARED)
  "available": 5000000         // ₦50,000 (spendable)
}
```

---

## Database Quick Checks

### Connect to PostgreSQL
```powershell
docker exec -it propvest-postgres psql -U propvest -d propvest
```

### Check Wallet Balance
```sql
SELECT 
  main_balance/100.0 as main_naira,
  locked_balance/100.0 as locked_naira,
  (main_balance - locked_balance)/100.0 as available_naira
FROM wallets 
WHERE user_id = (SELECT id FROM users WHERE email = 'testuser@example.com');
```

### Check Transaction Status
```sql
SELECT 
  reference,
  amount/100.0 as amount_naira,
  status,
  external_reference,
  created_at,
  updated_at
FROM wallet_transactions
WHERE type = 'withdrawal'
ORDER BY created_at DESC
LIMIT 5;
```

### Find Pending Withdrawals
```sql
SELECT 
  reference,
  amount/100.0 as amount_naira,
  status,
  AGE(NOW(), created_at) as pending_duration
FROM wallet_transactions
WHERE type = 'withdrawal' AND status = 'pending'
ORDER BY created_at DESC;
```

### Exit PostgreSQL
```sql
\q
```

---

## What to Watch

### Terminal 1 (API Logs)
```
✅ POST /api/v1/wallet/withdraw 200 OK
✅ POST /api/v1/webhooks/paystack/transfer 200 OK
✅ finalization complete status=completed
```

### Terminal 2 (Ngrok Logs)
```
✅ POST /api/v1/webhooks/paystack/transfer  200 OK
```

### Terminal 3 (Worker Logs)
```
✅ 🏦 processing withdrawal reference=WD-ABC123
✅ transfer initiated transfer_code=TRF_xyz status=pending
✅ 🔄 running reconciliation check...
```

---

## Common Test Scenarios

### Test 1: Happy Path
```powershell
# Initiate ₦50,000 withdrawal
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"amount_kobo": 5000000, "account_number": "0123456789", "account_name": "Test User", "bank_code": "058", "bank_name": "GTBank"}'

# Wait 1-5 minutes for webhook
# Check final balance
curl -X GET http://localhost:8081/api/v1/wallet -H "Authorization: Bearer $TOKEN"
```

### Test 2: Insufficient Balance
```powershell
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"amount_kobo": 99999999, "account_number": "0123456789", "account_name": "Test User", "bank_code": "058", "bank_name": "GTBank"}'

# Expected: 422 Unprocessable Entity
# Error: "insufficient balance"
```

### Test 3: Below Minimum
```powershell
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"amount_kobo": 10000, "account_number": "0123456789", "account_name": "Test User", "bank_code": "058", "bank_name": "GTBank"}'

# Expected: 422 Unprocessable Entity
# Error: "amount is below minimum withdrawal of ₦500.00"
```

---

## Troubleshooting Quick Fixes

### Webhook Not Received
```powershell
# 1. Check ngrok is running and forwarding
# Terminal 2 should show active tunnel

# 2. Verify Paystack webhook URL
# Dashboard → Webhooks → Must use HTTPS ngrok URL

# 3. Manually trigger webhook
cd tools
go run trigger_transfer_webhook.go --reference WD-ABC123 --event success
```

### Worker Not Processing
```powershell
# 1. Check worker is running
# Terminal 3 should show: ✓ withdrawal processor started

# 2. If RabbitMQ not available
# Reconciliation will process in 10 minutes
# Or restart worker to trigger immediate reconciliation
```

### Locked Balance Not Clearing
```powershell
# Check transaction status
curl -X GET http://localhost:8081/api/v1/wallet/transactions `
  -H "Authorization: Bearer $TOKEN" | jq '.data.transactions[] | select(.type=="withdrawal")'

# If status is "pending" after 10 minutes:
# - Check Paystack dashboard for transfer status
# - Check worker logs for errors
# - Restart worker to trigger reconciliation
```

---

## Credit Test Wallet (Quick)

```powershell
docker exec -it propvest-postgres psql -U propvest -d propvest
```

```sql
-- Credit ₦100,000 (10,000,000 kobo)
UPDATE wallets 
SET main_balance = 10000000 
WHERE user_id = (SELECT id FROM users WHERE email = 'testuser@example.com');

-- Verify
SELECT main_balance/100.0 as balance_naira FROM wallets 
WHERE user_id = (SELECT id FROM users WHERE email = 'testuser@example.com');

\q
```

---

## Nigerian Bank Codes (Common)

```
044 - Access Bank
057 - Zenith Bank
058 - GTBank
011 - First Bank
033 - United Bank for Africa (UBA)
032 - Union Bank
214 - First City Monument Bank (FCMB)
221 - Stanbic IBTC Bank
082 - Keystone Bank
076 - Polaris Bank
```

---

## Test Amount Examples

```
Minimum:  50,000 kobo  = ₦500
Small:    500,000 kobo = ₦5,000
Medium:   5,000,000 kobo = ₦50,000
Large:    10,000,000 kobo = ₦100,000
Maximum:  10,000,000 kobo = ₦100,000 (default limit)
```

---

## Success Checklist

After each test, verify:

- [ ] Withdrawal initiated (status 200)
- [ ] `locked_balance` increased
- [ ] Worker processed transfer
- [ ] Webhook received (check ngrok/API logs)
- [ ] `main_balance` debited
- [ ] `locked_balance` cleared
- [ ] Transaction status = "completed"
- [ ] Paystack dashboard shows success

---

## Useful Links

- **Paystack Dashboard**: https://dashboard.paystack.com
- **Transfers**: https://dashboard.paystack.com/transfers
- **Webhooks**: https://dashboard.paystack.com/settings/webhooks
- **API Docs**: https://paystack.com/docs/api/transfer
- **Bank List**: https://paystack.com/docs/transfers/single-transfers/#supported-banks

---

## Emergency Commands

### Stop Everything
```powershell
# Stop API (Ctrl+C in Terminal 1)
# Stop ngrok (Ctrl+C in Terminal 2)
# Stop worker (Ctrl+C in Terminal 3)
# Stop database
docker-compose down
```

### Reset Wallet Balance
```sql
-- Clear locked balance
UPDATE wallets SET locked_balance = 0;

-- Reset to ₦100,000
UPDATE wallets SET main_balance = 10000000 WHERE user_id = ...;
```

### Clear Test Transactions
```sql
-- Delete test withdrawals (⚠️ USE CAREFULLY)
DELETE FROM wallet_transactions WHERE type = 'withdrawal';
```

---

**Pro Tip:** Keep all 4 terminals visible side-by-side to monitor the entire flow in real-time!

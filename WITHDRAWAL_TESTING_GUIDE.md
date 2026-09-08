# 🧪 Withdrawal System Testing Guide

## Prerequisites Checklist

Before testing, ensure you have:

- ✅ PostgreSQL running (port 5435)
- ✅ Ngrok installed and configured
- ✅ Paystack test account with API keys
- ✅ Test bank account details ready
- ✅ API and Worker executables built
- ⚠️ RabbitMQ (optional - system works without it)

---

## Step-by-Step Testing Workflow

### Phase 1: Setup & Preparation

#### 1.1 Start Database
```powershell
# Start PostgreSQL
docker-compose up -d postgres

# Verify it's running
docker ps
```

#### 1.2 (Optional) Start RabbitMQ
```powershell
# Start RabbitMQ for queue-based processing
docker-compose up -d rabbitmq

# Access management UI (optional)
# http://localhost:15672
# Username: guest
# Password: guest
```

#### 1.3 Configure Paystack Webhook URL

**Important:** You need to configure Paystack to send transfer webhooks to your ngrok URL.

1. **Start ngrok:**
```powershell
ngrok http 8081
```

2. **Copy your ngrok URL:**
```
Forwarding: https://abc123.ngrok.io -> http://localhost:8081
```

3. **Configure Paystack Webhook:**
   - Go to: https://dashboard.paystack.com/settings/webhooks
   - Add webhook URL: `https://abc123.ngrok.io/api/v1/webhooks/paystack/transfer`
   - Select events:
     - ✅ `transfer.success`
     - ✅ `transfer.failed`
     - ✅ `transfer.reversed`
   - Save configuration

**Note:** Paystack will send a test event to verify the webhook. Your API must be running to receive it.

#### 1.4 Enable Transfers in Paystack

**Critical Step:** Paystack transfers are disabled by default!

1. Go to: https://dashboard.paystack.com/settings/transfers
2. Click **"Enable Transfers"**
3. Set up your **Transfer PIN** (you'll need this for manual transfers)
4. Review and accept terms

#### 1.5 Build Executables
```powershell
# Build API
go build -o api.exe ./cmd/api

# Build Worker
go build -o worker.exe ./cmd/worker
```

---

### Phase 2: Start Services

#### 2.1 Terminal 1: Start API Server
```powershell
# Run API
.\api.exe
```

**Expected output:**
```
starting PropVest API env=development
✓ database connected
✓ migrations up-to-date
✓ payment provider initialized (provider=paystack)
✓ RabbitMQ connected (or warning if not running)
listening on :8081
```

#### 2.2 Terminal 2: Start Ngrok
```powershell
# Tunnel to API server
ngrok http 8081
```

**Keep this terminal open!** Copy the HTTPS URL for Paystack configuration.

#### 2.3 Terminal 3: Start Worker
```powershell
# Run background worker
.\worker.exe
```

**Expected output:**
```
🚀 PropVest Background Worker starting...
✓ database connected
✓ payment provider initialized (provider=paystack)
✓ message queue connected
✓ withdrawal processor started
✓ reconciliation worker started
✅ Worker is running. Press Ctrl+C to stop.
```

**If RabbitMQ is not running:**
```
⚠️ RabbitMQ not available; worker will only run reconciliation
✓ reconciliation worker started
```

---

### Phase 3: Setup Test User & Wallet

#### 3.1 Register Test User

**Option A: Use existing tool**
```powershell
cd tools
go run register_test_user.go
```

**Option B: Manual API call**
```powershell
curl -X POST http://localhost:8081/api/v1/auth/register `
  -H "Content-Type: application/json" `
  -d '{
    "email": "testuser@example.com",
    "password": "Test1234!",
    "first_name": "Test",
    "last_name": "User",
    "phone": "+2348012345678"
  }'
```

#### 3.2 Verify Email (Development Mode)

**Check database for verification token:**
```powershell
# Connect to PostgreSQL
docker exec -it propvest-postgres psql -U propvest -d propvest

# Get verification token
SELECT token FROM verification_tokens WHERE email = 'testuser@example.com' ORDER BY created_at DESC LIMIT 1;

# Exit PostgreSQL
\q
```

**Verify email:**
```powershell
curl -X GET "http://localhost:8081/api/v1/auth/verify-email?token=<TOKEN_FROM_DB>"
```

#### 3.3 Login and Get Access Token
```powershell
curl -X POST http://localhost:8081/api/v1/auth/login `
  -H "Content-Type: application/json" `
  -d '{
    "email": "testuser@example.com",
    "password": "Test1234!"
  }'
```

**Save the access_token from response:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",  // ← COPY THIS
    "refresh_token": "...",
    "user": {...}
  }
}
```

**For convenience, set as environment variable:**
```powershell
$TOKEN = "eyJhbGciOiJIUzI1NiIs..."
```

#### 3.4 Credit Test Wallet

**Option A: Using database (quick for testing)**
```powershell
docker exec -it propvest-postgres psql -U propvest -d propvest
```

```sql
-- Credit wallet with ₦100,000 (10,000,000 kobo)
UPDATE wallets 
SET main_balance = 10000000 
WHERE user_id = (SELECT id FROM users WHERE email = 'testuser@example.com');

-- Verify balance
SELECT main_balance, locked_balance, main_balance - locked_balance as available 
FROM wallets 
WHERE user_id = (SELECT id FROM users WHERE email = 'testuser@example.com');
```

**Option B: Using deposit flow (more realistic)**
```powershell
# Initiate deposit
curl -X POST http://localhost:8081/api/v1/wallet/deposit `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"amount_kobo": 10000000}'

# Then simulate webhook to credit wallet
# (See tools/trigger_webhook.go)
```

#### 3.5 Verify Wallet Balance
```powershell
curl -X GET http://localhost:8081/api/v1/wallet `
  -H "Authorization: Bearer $TOKEN"
```

**Expected response:**
```json
{
  "success": true,
  "data": {
    "main_balance": 10000000,
    "locked_balance": 0,
    "earnings_balance": 0,
    "currency": "NGN"
  }
}
```

---

### Phase 4: Test Withdrawal Workflow

#### 4.1 Get Nigerian Bank Account (for testing)

**Paystack Test Bank Accounts:**
- Account Number: `0123456789` (test account)
- Bank Code: `058` (GTBank)
- Account Name: Any name (will be verified)

**Real test account (if you have one):**
- Use your actual bank account details
- Paystack test mode won't send real money
- Account name must match bank records

#### 4.2 Initiate Withdrawal

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

**Expected response:**
```json
{
  "success": true,
  "message": "Withdrawal initiated successfully",
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "withdrawal",
    "amount": 5000000,
    "balance_before": 10000000,
    "balance_after": 10000000,  // Unchanged (locked pattern)
    "reference": "WD-ABC123XYZ",
    "description": "Withdrawal to GTBank (0123456789)",
    "status": "pending",
    "created_at": "2026-09-06T12:00:00Z"
  }
}
```

**Save the reference for tracking:**
```powershell
$WITHDRAWAL_REF = "WD-ABC123XYZ"
```

#### 4.3 Verify Locked Balance

**Check wallet immediately:**
```powershell
curl -X GET http://localhost:8081/api/v1/wallet `
  -H "Authorization: Bearer $TOKEN"
```

**Expected:**
```json
{
  "main_balance": 10000000,     // Unchanged
  "locked_balance": 5000000,    // ← LOCKED
  "available": 5000000          // Only ₦50,000 spendable
}
```

**Verify in database:**
```sql
SELECT 
  main_balance, 
  locked_balance, 
  main_balance - locked_balance as available_balance
FROM wallets 
WHERE user_id = (SELECT id FROM users WHERE email = 'testuser@example.com');
```

**Check transaction:**
```sql
SELECT id, type, amount, status, reference, created_at
FROM wallet_transactions
WHERE reference = 'WD-ABC123XYZ';
```

Expected: `status = 'pending'`

#### 4.4 Monitor Worker Logs

**Terminal 3 (Worker) should show:**
```
🏦 processing withdrawal transaction_id=... amount=5000000 reference=WD-ABC123XYZ
resolving bank account account_number=0123456789 bank_code=058
account verified account_name="TEST USER" bank_name="GTBank Plc"
transfer initiated transfer_code=TRF_xyz789 status=pending
```

**If RabbitMQ is NOT running:**
- Withdrawal won't be processed immediately
- Reconciliation worker will pick it up in 5 minutes
- Or webhook will trigger finalization when Paystack completes

---

### Phase 5: Monitor Transfer Status

#### 5.1 Check Transaction Status
```powershell
curl -X GET http://localhost:8081/api/v1/wallet/transactions `
  -H "Authorization: Bearer $TOKEN"
```

**Look for your withdrawal:**
```json
{
  "transactions": [
    {
      "id": "...",
      "type": "withdrawal",
      "amount": 5000000,
      "status": "pending",  // or "completed" if webhook arrived
      "reference": "WD-ABC123XYZ",
      "created_at": "..."
    }
  ]
}
```

#### 5.2 Check Paystack Dashboard

1. Go to: https://dashboard.paystack.com/transfers
2. Find your transfer (search by reference or amount)
3. Check status: `pending`, `success`, or `failed`

#### 5.3 Wait for Webhook

**What happens:**
1. Paystack processes the transfer (1-5 minutes in test mode)
2. Paystack sends webhook to your ngrok URL
3. Your API receives and processes webhook
4. Wallet is finalized (debited or reversed)

**Monitor API logs (Terminal 1):**
```
[INFO] POST /api/v1/webhooks/paystack/transfer
[INFO] webhook signature verified
[INFO] processing transfer webhook event=transfer.success reference=WD-ABC123XYZ
[INFO] finalizing withdrawal transaction_id=... success=true
[INFO] withdrawal finalized status=completed
```

#### 5.4 Verify Final State

**Check wallet after webhook:**
```powershell
curl -X GET http://localhost:8081/api/v1/wallet `
  -H "Authorization: Bearer $TOKEN"
```

**Expected (SUCCESS):**
```json
{
  "main_balance": 5000000,      // Debited ✓
  "locked_balance": 0,          // Cleared ✓
  "available": 5000000          // ₦50,000 available
}
```

**Check transaction:**
```powershell
curl -X GET http://localhost:8081/api/v1/wallet/transactions `
  -H "Authorization: Bearer $TOKEN"
```

**Expected:**
```json
{
  "id": "...",
  "status": "completed",  // ← Changed from "pending"
  "external_reference": "TRF_xyz789"  // Paystack transfer code
}
```

**Verify in database:**
```sql
SELECT 
  reference,
  status,
  external_reference,
  balance_before,
  balance_after,
  created_at,
  updated_at
FROM wallet_transactions
WHERE reference = 'WD-ABC123XYZ';
```

Expected:
- `status = 'completed'`
- `external_reference = 'TRF_xyz789'`
- `updated_at > created_at`

---

### Phase 6: Test Failure Scenarios

#### 6.1 Test Insufficient Balance

```powershell
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount_kobo": 99999999,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058",
    "bank_name": "GTBank"
  }'
```

**Expected response:**
```json
{
  "success": false,
  "error": "insufficient balance: requested ₦999,999.99 but only ₦50,000.00 available",
  "code": "insufficient_funds"
}
```

#### 6.2 Test Below Minimum Amount

```powershell
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount_kobo": 10000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058",
    "bank_name": "GTBank"
  }'
```

**Expected response:**
```json
{
  "success": false,
  "error": "amount is below minimum withdrawal of ₦500.00",
  "code": "invalid_amount"
}
```

#### 6.3 Test Invalid Bank Account

```powershell
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount_kobo": 5000000,
    "account_number": "0000000000",
    "account_name": "Invalid Account",
    "bank_code": "058",
    "bank_name": "GTBank"
  }'
```

**Expected response:**
```json
{
  "success": false,
  "error": "Invalid bank account details",
  "code": "invalid_bank_account"
}
```

#### 6.4 Test Account Name Mismatch

```powershell
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount_kobo": 5000000,
    "account_number": "0123456789",
    "account_name": "Wrong Name",
    "bank_code": "058",
    "bank_name": "GTBank"
  }'
```

**Expected response:**
```json
{
  "success": false,
  "error": "account name mismatch: you entered 'Wrong Name' but bank records show 'Test User'",
  "code": "account_name_mismatch"
}
```

---

### Phase 7: Test Reconciliation Worker

#### 7.1 Simulate Missed Webhook

**Scenario:** Webhook doesn't arrive, but transfer completes on Paystack.

1. **Create a withdrawal** (as in Phase 4.2)
2. **Stop the API server** (to prevent webhook from being received)
3. **Wait for transfer to complete on Paystack** (check dashboard)
4. **Restart API server**
5. **Wait 5 minutes** (or restart worker to trigger immediate reconciliation)

**Worker logs should show:**
```
🔄 running reconciliation check...
found pending withdrawals count=1
verifying withdrawal status transaction_id=... reference=WD-ABC123XYZ
transfer status verified status=success
reconciling successful transfer transaction_id=...
✓ reconciliation complete
```

#### 7.2 Monitor Reconciliation Cycles

**Worker runs reconciliation every 5 minutes:**
```
🔄 running reconciliation check...
✓ no pending withdrawals to reconcile
```

**To force immediate reconciliation:**
- Restart worker (runs on startup)
- Or wait for next 5-minute cycle

---

### Phase 8: Test Edge Cases

#### 8.1 Test Duplicate Withdrawal Prevention

Try to initiate withdrawal while funds are locked:

1. **Initiate first withdrawal** (₦50,000)
2. **Immediately try second withdrawal** (₦50,000)

**Expected:** Second request should fail with insufficient funds (locked funds can't be spent).

#### 8.2 Test Webhook Idempotency

**Simulate duplicate webhook:**
```powershell
cd tools
go run trigger_transfer_webhook.go --reference WD-ABC123XYZ --event success
```

Run it twice. Expected: Both return 200 OK, but wallet only updated once.

#### 8.3 Test Concurrent Withdrawals

**Not recommended in test mode** (can lock all funds), but architecture supports it:
- Multiple withdrawals can be initiated simultaneously
- Each locks its own portion of funds
- All are independent and idempotent

---

## Common Issues & Solutions

### Issue 1: Webhook Not Received

**Symptoms:**
- Withdrawal stays "pending" forever
- No logs in API terminal about webhook
- Paystack dashboard shows "delivered" but you see nothing

**Solutions:**
1. **Check ngrok is running:**
   ```powershell
   # In ngrok terminal, you should see:
   POST /api/v1/webhooks/paystack/transfer  200 OK
   ```

2. **Verify Paystack webhook URL:**
   - Must be `https://YOUR_NGROK_URL/api/v1/webhooks/paystack/transfer`
   - Must use HTTPS (not HTTP)

3. **Check API logs for signature errors:**
   ```
   [ERROR] invalid webhook signature
   ```
   If you see this, your Paystack secret key might be wrong.

4. **Manually trigger webhook:**
   ```powershell
   cd tools
   go run trigger_transfer_webhook.go --reference WD-ABC123XYZ --event success
   ```

### Issue 2: Worker Not Processing

**Symptoms:**
- Withdrawal stays "pending"
- No worker logs about processing
- RabbitMQ queue has messages

**Solutions:**
1. **Check worker is running:**
   ```powershell
   # Should see:
   ✓ withdrawal processor started
   ```

2. **Check RabbitMQ connection:**
   ```powershell
   # If you see:
   ⚠️ RabbitMQ not available
   
   # Start RabbitMQ:
   docker-compose up -d rabbitmq
   
   # Restart worker
   ```

3. **Wait for reconciliation:**
   - Even without RabbitMQ, reconciliation will process after 10 minutes

### Issue 3: Account Name Always Mismatches

**Cause:** Bank returns name in different format than you entered.

**Solution:** Use fuzzy matching (already implemented):
- "JOHN DOE" matches "John Doe" ✓
- "John D. Doe" matches "John Doe" ✓

If still failing, check what bank actually returns:
```sql
-- Check transaction metadata
SELECT metadata->>'account_name' as bank_name
FROM wallet_transactions
WHERE reference = 'WD-ABC123XYZ';
```

### Issue 4: Transfer Fails on Paystack

**Common reasons:**
1. **Insufficient Paystack balance** (test mode still requires test balance)
2. **Transfers not enabled** (check dashboard settings)
3. **Invalid bank account**
4. **Transfer limits exceeded**

**Check Paystack dashboard for exact error:**
https://dashboard.paystack.com/transfers

---

## Testing Checklist

Use this checklist to verify everything works:

### Basic Flow
- [ ] User can initiate withdrawal
- [ ] Funds are locked immediately
- [ ] Transaction status is "pending"
- [ ] Worker picks up job
- [ ] Paystack transfer initiated
- [ ] Webhook received
- [ ] Wallet debited
- [ ] Locked funds cleared
- [ ] Transaction marked "completed"

### Error Handling
- [ ] Insufficient balance rejected
- [ ] Below minimum amount rejected
- [ ] Invalid account rejected
- [ ] Account name mismatch caught
- [ ] Failed transfer reverses funds

### Reliability
- [ ] Duplicate webhooks handled gracefully
- [ ] Reconciliation catches missed webhooks
- [ ] System works without RabbitMQ (degraded mode)
- [ ] Worker can be restarted safely

### Database Integrity
- [ ] `locked_balance` never exceeds `main_balance`
- [ ] `locked_balance` always non-negative
- [ ] Transaction records are immutable
- [ ] Balance changes are atomic

---

## Quick Test Commands

**Full test in one go:**
```powershell
# 1. Start services
docker-compose up -d postgres
.\api.exe  # Terminal 1
ngrok http 8081  # Terminal 2
.\worker.exe  # Terminal 3

# 2. Setup (in Terminal 4)
$TOKEN = "YOUR_ACCESS_TOKEN_HERE"

# 3. Check balance
curl -X GET http://localhost:8081/api/v1/wallet -H "Authorization: Bearer $TOKEN"

# 4. Initiate withdrawal
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"amount_kobo": 5000000, "account_number": "0123456789", "account_name": "Test User", "bank_code": "058", "bank_name": "GTBank"}'

# 5. Monitor status
curl -X GET http://localhost:8081/api/v1/wallet/transactions -H "Authorization: Bearer $TOKEN"
```

---

## Success Indicators

You know everything is working when you see:

✅ **API logs:**
```
POST /api/v1/wallet/withdraw 200 OK
POST /api/v1/webhooks/paystack/transfer 200 OK
finalization complete status=completed
```

✅ **Worker logs:**
```
🏦 processing withdrawal
transfer initiated transfer_code=TRF_xyz
```

✅ **Database state:**
```sql
-- Before: main=100k, locked=0, available=100k
-- After:  main=50k,  locked=0, available=50k
```

✅ **Paystack dashboard:**
- Transfer status: "success"
- Amount matches your request
- Reference matches your withdrawal

---

## Next Steps

After successful testing:

1. **Test with real bank account** (still in test mode)
2. **Test various amounts and scenarios**
3. **Load test** (multiple concurrent withdrawals)
4. **Monitor logs and metrics**
5. **Document any issues found**
6. **Prepare for production deployment**

---

**Need Help?**
- Check API logs: Terminal 1
- Check Worker logs: Terminal 3
- Check ngrok logs: Terminal 2
- Check Paystack dashboard: https://dashboard.paystack.com
- Check database: `docker exec -it propvest-postgres psql -U propvest -d propvest`

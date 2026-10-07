# Withdrawal Fix & Complete Testing Guide

## 🔴 Problem Identified

Your withdrawal got stuck because:

1. ✅ API successfully created the withdrawal transaction
2. ❌ RabbitMQ was NOT configured in `.env`
3. ❌ Message was NOT queued to RabbitMQ
4. ❌ Worker never processed it (no `InitiateTransfer` call to Paystack)
5. ❌ No `transfer_code` saved
6. ❌ Reconciliation worker skips transactions without `transfer_code`

**Root Cause:** Missing `RABBITMQ_URL` in your `.env` file!

---

## ✅ Solution Applied

I've added this line to your `.env`:

```env
RABBITMQ_URL=amqp://propvest:password@localhost:5672/
```

This matches the RabbitMQ credentials in `docker-compose.yml`.

---

## 🚀 Complete Testing Steps

### Step 1: Restart Everything

```powershell
# Stop all services
docker-compose down

# Start services again
docker-compose up -d

# Verify all running
docker ps
```

You should see 3 containers:
- `propvest_postgres`
- `propvest_redis`
- `propvest_rabbitmq`

---

### Step 2: Start API Server

```powershell
# Terminal 1: API Server (watch logs here!)
go run cmd/api/main.go
```

You should see:
```
[INFO] Starting PropVest API server on port 8081
[INFO] Connected to database
[INFO] Connected to Redis
[INFO] Message queue enabled: true  ← IMPORTANT! Should be true
[INFO] Payment provider: paystack
```

**🚨 If you see `Message queue enabled: false`, the RABBITMQ_URL is not configured correctly!**

---

### Step 3: Start Worker

```powershell
# Terminal 2: Worker (watch logs here too!)
go run cmd/worker/main.go
```

You should see:
```
[INFO] PropVest Background Worker starting...
[INFO] Connected to database
[INFO] Payment provider: paystack
[INFO] Message queue enabled: true
[INFO] 🚀 Withdrawal processor started
[INFO] 🔄 Reconciliation worker started (check every 5m)
```

**🚨 Both terminals must be running for withdrawals to work!**

---

### Step 4: Clean Up Old Pending Withdrawal

Your old withdrawal is stuck without a transfer code. Let's clean it up:

```powershell
# Check the stuck withdrawal
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT id, reference, amount/100.0 as amount_ngn, status, external_reference FROM wallet_transactions WHERE reference = 'WD-8721CB30-F52';"

# Option A: Mark as failed and unlock funds
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "UPDATE wallet_transactions SET status = 'failed' WHERE reference = 'WD-8721CB30-F52';"

$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "UPDATE wallets SET main_balance = main_balance + locked_balance, locked_balance = 0 WHERE locked_balance > 0;"

# Option B: Delete it entirely
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "DELETE FROM wallet_transactions WHERE reference = 'WD-8721CB30-F52';"

$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "UPDATE wallets SET main_balance = main_balance + locked_balance, locked_balance = 0 WHERE locked_balance > 0;"
```

---

### Step 5: Verify Wallet Balance

```powershell
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT u.email, w.main_balance/100.0 as available_ngn, w.locked_balance/100.0 as locked_ngn FROM wallets w JOIN users u ON u.id = w.user_id;"
```

Make sure you have available balance (not locked).

---

### Step 6: Test New Withdrawal

**POST** `http://localhost:8081/api/v1/wallet/withdraw`

**Headers:**
```
Authorization: Bearer YOUR_JWT_TOKEN
Content-Type: application/json
```

**Body:**
```json
{
  "amount": 50000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "John Doe"
}
```

---

### Step 7: Watch the Logs!

**Terminal 1 (API):**
```
[INFO] Withdrawal request received user_id=xxx amount=50000
[INFO] Resolving bank account account_number=0123456789 bank_code=058
[INFO] Account verified account_name="John Doe" bank_name="Guaranty Trust Bank"
[INFO] Withdrawal initiated reference=WD-ABC123 amount=50000
[INFO] Withdrawal queued to RabbitMQ queue=withdrawal.process  ← NEW! Should see this
```

**Terminal 2 (Worker):**
```
[INFO] 🏦 processing withdrawal transaction_id=xxx amount=50000 reference=WD-ABC123
[INFO] Transfer initiated transaction_id=xxx transfer_code=TRF_xxx status=pending
[INFO] Transfer pending, will be finalized by webhook or reconciliation
```

**🎉 Success! The withdrawal is now being processed!**

---

## 📊 Expected Flow with Fix

### Before (Broken):
```
User → API → Database (pending)
                  ↓
               [STUCK - No queue]
                  ↓
            Reconciliation skips (no transfer_code)
```

### After (Fixed):
```
User → API → Database (pending) → RabbitMQ Queue
                                       ↓
                                    Worker
                                       ↓
                              Paystack Transfer API
                                       ↓
                           Saves transfer_code
                                       ↓
                        Webhook/Reconciliation finalizes
                                       ↓
                             Status: completed
```

---

## 🔍 Verify Everything is Working

### Check 1: Message Queue Connected
```powershell
# API logs should show:
[INFO] Message queue enabled: true
```

### Check 2: Worker is Consuming
```powershell
# Worker logs should show:
[INFO] 🚀 Withdrawal processor started
```

### Check 3: RabbitMQ Management UI
Open http://localhost:15672
- Username: `propvest`
- Password: `password`

You should see:
- Queues: `withdrawal.process`, `withdrawal.process.dlq`
- Messages being consumed

### Check 4: Database Has Transfer Code
```powershell
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT reference, status, external_reference FROM wallet_transactions WHERE type='withdrawal' ORDER BY created_at DESC LIMIT 1;"
```

`external_reference` should have a value like `TRF_xxxxx` (this is the transfer_code from Paystack)

---

## 🎭 Test Different Scenarios

### Scenario 1: Successful Withdrawal
```json
{
  "amount": 50000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "Test User"
}
```

**Expected:**
- API: Withdrawal queued ✅
- Worker: Transfer initiated ✅
- Worker: Transfer completed ✅ (if using mock provider)
- Database: Status = "completed" ✅

### Scenario 2: Insufficient Balance
Try to withdraw more than available:
```json
{
  "amount": 9999999999,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "Test User"
}
```

**Expected:** 422 error "Insufficient funds"

### Scenario 3: Test Mock Provider Patterns

If using `PAYMENT_PROVIDER=mock` in `.env`, you can test specific scenarios:

**Success Pattern:**
```json
{
  "amount": 50000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "Test User"
}
```
Reference generated will be like `WD-SUCCESS123` → completes immediately

**Failure Pattern:**
Edit your test to generate a reference with "FAIL" in it.

---

## 🐛 Troubleshooting

### Issue 1: "Message queue enabled: false"

**Cause:** RABBITMQ_URL not loaded or incorrect

**Fix:**
```powershell
# Check .env has this line:
cat .env | Select-String "RABBITMQ"

# Should show:
RABBITMQ_URL=amqp://propvest:password@localhost:5672/

# If not there, add it and restart API
```

### Issue 2: Worker not picking up messages

**Cause:** Worker not running or crashed

**Fix:**
```powershell
# Check worker is running
# Terminal 2 should show:
[INFO] 🚀 Withdrawal processor started

# If not, restart:
go run cmd/worker/main.go
```

### Issue 3: "Transfer initiation failed"

**Cause:** Paystack API key invalid or network issue

**Check logs for exact error:**
```
[ERROR] transfer initiation failed error="..."
```

**Fix:**
- Verify PAYSTACK_SECRET_KEY in `.env`
- Test Paystack API manually:
```powershell
curl -H "Authorization: Bearer sk_test_YOUR_TEST_SECRET_KEY" https://api.paystack.co/bank
```

### Issue 4: RabbitMQ connection refused

**Cause:** RabbitMQ not running

**Fix:**
```powershell
docker-compose up -d rabbitmq
docker logs propvest_rabbitmq
```

---

## 📋 Complete Checklist

Before testing, ensure:

- [ ] `.env` has `RABBITMQ_URL=amqp://propvest:password@localhost:5672/`
- [ ] Docker services running: `docker ps` shows 3 containers
- [ ] API running in Terminal 1
- [ ] Worker running in Terminal 2
- [ ] API logs show "Message queue enabled: true"
- [ ] Worker logs show "Withdrawal processor started"
- [ ] Old stuck withdrawal cleaned up
- [ ] Wallet has available balance (not locked)
- [ ] Valid JWT token (login if expired)

Then test:

- [ ] Make withdrawal request
- [ ] API logs show "Withdrawal queued to RabbitMQ"
- [ ] Worker logs show "processing withdrawal"
- [ ] Worker logs show "Transfer initiated"
- [ ] Database shows transfer_code saved
- [ ] Status updates to "completed" (or "pending" if async)

---

## 🎓 Understanding The Architecture

### Components:

1. **API Server (Terminal 1)**
   - Receives withdrawal requests
   - Validates and locks funds
   - Queues message to RabbitMQ
   - Returns immediately to user

2. **RabbitMQ (Docker)**
   - Message queue broker
   - Stores withdrawal jobs
   - Ensures reliable message delivery

3. **Worker (Terminal 2)**
   - Consumes messages from RabbitMQ
   - Calls Paystack Transfer API
   - Saves transfer_code
   - Updates transaction status

4. **Reconciliation (Worker, every 5 min)**
   - Finds old pending transactions
   - Verifies status with Paystack
   - Finalizes completed/failed transfers

### Why This Design?

✅ **Async Processing:** API responds immediately, actual transfer happens in background  
✅ **Reliability:** RabbitMQ persists messages, no data loss  
✅ **Retry Logic:** Worker retries failed transfers  
✅ **Recovery:** Reconciliation catches missed webhooks  

---

## 🚀 You're All Set!

Now test your withdrawal and watch both terminals. You should see:

**Terminal 1 (API):**
```
✅ Withdrawal initiated
✅ Withdrawal queued to RabbitMQ
```

**Terminal 2 (Worker):**
```
✅ 🏦 processing withdrawal
✅ Transfer initiated
✅ Transfer completed
```

Happy testing! 🎉


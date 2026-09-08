# 🌐 Testing Withdrawals with Ngrok & Paystack Webhooks

## Overview

This guide shows you how to test the **complete end-to-end withdrawal flow** with:
- Real Paystack webhook delivery
- Ngrok tunnel for local development
- Automatic finalization via webhooks

---

## 🎯 The Goal

We want to see:
1. ✅ Initiate withdrawal in Postman
2. ✅ Worker processes it via Paystack
3. ✅ Paystack sends real webhook to your ngrok URL
4. ✅ Webhook automatically finalizes the withdrawal
5. ✅ Balance updated in database

---

## 📋 Prerequisites

- [ ] API server running (`.\api.exe`)
- [ ] Worker running (`.\worker.exe`)
- [ ] Ngrok installed
- [ ] Paystack test account
- [ ] PostgreSQL running

---

## Part 1: Setup Ngrok Tunnel

### Step 1.1: Start Ngrok

**Open a dedicated terminal for ngrok:**

```powershell
ngrok http 8081
```

**You'll see output like:**
```
ngrok

Session Status                online
Account                       your@email.com
Version                       3.x.x
Region                        United States (us)
Latency                       -
Web Interface                 http://127.0.0.1:4040
Forwarding                    https://abc123xyz.ngrok.io -> http://localhost:8081

Connections                   ttl     opn     rt1     rt5     p50     p90
                              0       0       0.00    0.00    0.00    0.00
```

**🎯 IMPORTANT: Copy this URL:**
```
https://abc123xyz.ngrok.io
```

⚠️ **Keep this terminal open!** If you close it, the tunnel dies.

### Step 1.2: Verify Ngrok is Working

**In Postman or browser, test:**
```
https://abc123xyz.ngrok.io/api/v1/health
```

**Expected response:**
```json
{
  "status": "healthy",
  "database": "connected"
}
```

✅ **If you see this, ngrok is working!**

### Step 1.3: Access Ngrok Dashboard (Optional)

**Open in browser:**
```
http://127.0.0.1:4040
```

This shows:
- All HTTP requests passing through tunnel
- Request/response details
- Replay requests
- Very useful for debugging!

---

## Part 2: Configure Paystack Webhook

### Step 2.1: Login to Paystack Dashboard

1. Go to: https://dashboard.paystack.com
2. Login with your test account
3. Ensure you're in **Test Mode** (toggle at top)

### Step 2.2: Navigate to Webhooks Settings

**URL:** https://dashboard.paystack.com/settings/webhooks

Or:
1. Click **Settings** (left sidebar)
2. Click **Webhooks**

### Step 2.3: Add Your Webhook URL

**Click "Add Webhook URL" button**

**Enter this URL:**
```
https://YOUR_NGROK_URL/api/v1/webhooks/paystack/transfer
```

**Example:**
```
https://abc123xyz.ngrok.io/api/v1/webhooks/paystack/transfer
```

⚠️ **CRITICAL:**
- Must be HTTPS (ngrok provides this)
- Must end with `/api/v1/webhooks/paystack/transfer`
- Use YOUR actual ngrok URL (changes each restart)

**Select Events:**
- ✅ `transfer.success`
- ✅ `transfer.failed`
- ✅ `transfer.reversed`

**Click "Save"**

### Step 2.4: Test Webhook Delivery

Paystack will send a test event immediately.

**In your API terminal, you should see:**
```
POST /api/v1/webhooks/paystack/transfer 200 OK
```

**In ngrok terminal/dashboard, you should see:**
```
POST /api/v1/webhooks/paystack/transfer  200 OK
```

✅ **If you see this, webhook delivery is working!**

❌ **If you see errors:**
- Check API is running
- Verify ngrok URL is correct
- Check API logs for errors

---

## Part 3: Enable Paystack Transfers

### Step 3.1: Navigate to Transfers Settings

**URL:** https://dashboard.paystack.com/settings/transfers

Or:
1. Click **Settings** (left sidebar)
2. Click **Transfers**

### Step 3.2: Enable Transfers

**Click "Enable Transfers" button**

**Set up Transfer PIN:**
1. Enter a 4-digit PIN (e.g., 1234)
2. Confirm PIN
3. Accept terms and conditions

⚠️ **IMPORTANT:** Transfers are disabled by default, even in test mode!

### Step 3.3: Verify Transfer Settings

**Check:**
- ✅ "Transfers" toggle is ON
- ✅ Transfer PIN is set
- ✅ Auto-approval is enabled (for test mode)

---

## Part 4: Complete End-to-End Test

Now let's test the full flow with real webhook delivery!

### Step 4.1: Start All Services

**Terminal 1: API Server**
```powershell
.\api.exe
```

**Terminal 2: Ngrok Tunnel**
```powershell
ngrok http 8081
```

**Terminal 3: Background Worker**
```powershell
.\worker.exe
```

**Terminal 4: Ngrok Dashboard (Optional)**
```
Open browser: http://127.0.0.1:4040
```

### Step 4.2: Login in Postman

**Request:** Login  
**Expected:** Access token saved automatically

### Step 4.3: Check Initial Balance

**Request:** Get Wallet Balance

**Expected Response:**
```json
{
  "main_balance": 10000000,    // ₦100,000
  "locked_balance": 0,
  "currency": "NGN"
}
```

### Step 4.4: Initiate Withdrawal

**Request:** Initiate Withdrawal

**Body:**
```json
{
  "amount_kobo": 5000000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Expected Response:**
```json
{
  "success": true,
  "message": "Withdrawal initiated successfully",
  "data": {
    "reference": "WD-ABC123XYZ",
    "status": "pending",
    "amount": 5000000
  }
}
```

**📋 Copy the reference:** `WD-ABC123XYZ`

### Step 4.5: Verify Locked Balance

**Request:** Get Wallet Balance (immediately)

**Expected Response:**
```json
{
  "main_balance": 10000000,    // ₦100,000 (unchanged)
  "locked_balance": 5000000,   // ₦50,000 (LOCKED!) ⭐
}
```

✅ **Perfect! Funds are locked.**

### Step 4.6: Watch Worker Process

**In Worker Terminal (Terminal 3), you should see:**
```
🏦 processing withdrawal reference=WD-ABC123XYZ amount=5000000
resolving bank account account_number=0123456789 bank_code=058
account verified account_name="TEST USER"
initiating transfer to Paystack
transfer initiated transfer_code=TRF_xyz789 status=pending
```

### Step 4.7: Check Paystack Dashboard

**Go to:** https://dashboard.paystack.com/transfers

**You should see:**
- Your transfer listed
- Amount: ₦50,000
- Status: "Processing" or "Pending"
- Reference: WD-ABC123XYZ

### Step 4.8: Wait for Webhook (1-5 minutes)

**What happens:**
1. Paystack processes the transfer
2. Transfer completes (in test mode: instant or few minutes)
3. Paystack sends webhook to your ngrok URL
4. Your API receives and processes it
5. Wallet is finalized

**Watch these terminals:**

**API Terminal (Terminal 1):**
```
[INFO] POST /api/v1/webhooks/paystack/transfer
[INFO] webhook signature verified ✓
[INFO] processing transfer event=transfer.success reference=WD-ABC123XYZ
[INFO] finalizing withdrawal transaction_id=... success=true
[INFO] withdrawal finalized status=completed
```

**Ngrok Dashboard (Terminal 4 / Browser):**
```
POST /api/v1/webhooks/paystack/transfer
Status: 200 OK
Body: {"success": true, ...}
```

**Worker Terminal (Terminal 3):**
```
(May show reconciliation checks every 5 minutes)
```

### Step 4.9: Verify Transaction Status

**Request:** Get Transactions

**Expected Response:**
```json
{
  "transactions": [
    {
      "reference": "WD-ABC123XYZ",
      "type": "withdrawal",
      "amount": 5000000,
      "status": "completed",    // ⭐ Changed from "pending"!
      "external_reference": "TRF_xyz789"
    }
  ]
}
```

### Step 4.10: Verify Final Balance

**Request:** Get Wallet Balance

**Expected Response:**
```json
{
  "main_balance": 5000000,     // ₦50,000 (DEBITED!) ⭐
  "locked_balance": 0,         // Cleared! ⭐
}
```

**🎉 SUCCESS! Complete end-to-end flow worked!**

---

## Part 5: Debugging Webhook Issues

### Issue 1: Webhook Not Received

**Symptoms:**
- Withdrawal stays "pending" forever
- No logs in API terminal about webhook
- Ngrok shows no POST to `/webhooks/paystack/transfer`

**Debug Steps:**

**1. Check Paystack Dashboard**
- Go to: https://dashboard.paystack.com/webhooks-log
- Find your transfer event
- Check delivery status

**Status: "Failed" or "Error"**
→ Paystack couldn't reach your webhook

**Possible causes:**
- Ngrok tunnel died (restart ngrok)
- Wrong webhook URL in Paystack settings
- API server crashed (check Terminal 1)

**Status: "Delivered"**
→ Webhook was sent successfully

**Check:**
- API logs for errors
- Ngrok dashboard for the request
- Response code (should be 200)

**2. Check Ngrok Tunnel**

**In ngrok terminal:**
```
Connections    ttl     opn
               5       1      ← Should have connections
```

**In ngrok dashboard (http://127.0.0.1:4040):**
- Look for POST requests
- Check response codes
- Replay failed requests

**3. Check API Logs**

Look for these errors:

**"Invalid signature"**
→ Secret key mismatch
```
Check .env file: PAYSTACK_SECRET_KEY
Should match your Paystack test key
```

**"Transaction not found"**
→ Reference mismatch
```
Verify reference in webhook matches your withdrawal
```

**4. Manually Trigger Webhook**

Test your endpoint directly:

```powershell
cd tools
go run trigger_transfer_webhook.go --reference WD-ABC123XYZ --event success
```

This bypasses Paystack and sends webhook directly to your API.

---

### Issue 2: Signature Verification Fails

**Error in API logs:**
```
[ERROR] invalid webhook signature
```

**Causes:**
1. Wrong secret key in .env
2. Webhook from unknown source
3. Body was modified

**Solution:**

**Check your secret key:**
```powershell
# View current key
type .env | findstr PAYSTACK_SECRET_KEY

# Get correct key from Paystack dashboard
# Settings → API Keys & Webhooks → Secret Key (test mode)
```

**Update .env:**
```
PAYSTACK_SECRET_KEY=sk_test_YOUR_ACTUAL_TEST_KEY_HERE
```

**Restart API server:**
```powershell
# Stop API (Ctrl+C)
.\api.exe
```

---

### Issue 3: Webhook Received but Not Processing

**Symptoms:**
- Webhook arrives (see in ngrok/API logs)
- Returns 200 OK
- But balance doesn't update

**Debug Steps:**

**1. Check API Response**

In ngrok dashboard, look at response body:
```json
{
  "success": true,
  "message": "Webhook processed successfully"
}
```

If different, check for error message.

**2. Check Transaction Status**

```sql
-- Connect to database
docker exec -it propvest-postgres psql -U propvest -d propvest

-- Check transaction
SELECT reference, status, external_reference, updated_at
FROM wallet_transactions
WHERE reference = 'WD-ABC123XYZ';

-- Expected: status='completed', external_reference='TRF_xyz'
```

**3. Check Wallet Balance**

```sql
SELECT main_balance, locked_balance
FROM wallets
WHERE user_id = (SELECT id FROM users WHERE email = 'testuser@example.com');
```

**4. Check for Idempotency**

Transaction might already be finalized:
```sql
SELECT status, updated_at, created_at
FROM wallet_transactions
WHERE reference = 'WD-ABC123XYZ';
```

If `updated_at > created_at`, it was already processed.

---

### Issue 4: Ngrok URL Changes

**Problem:**
- You restart ngrok
- URL changes from `abc123.ngrok.io` to `def456.ngrok.io`
- Paystack still sends to old URL

**Solution:**

**1. Update Paystack webhook URL:**
- Go to: https://dashboard.paystack.com/settings/webhooks
- Delete old webhook URL
- Add new ngrok URL
- Save

**2. Use ngrok with fixed subdomain (paid feature):**
```powershell
ngrok http 8081 --subdomain=propvest-test
# Always: https://propvest-test.ngrok.io
```

**3. Use ngrok config for custom domain:**
See: `SETUP_NGROK.md`

---

## Part 6: Testing Different Scenarios

### Scenario 1: Successful Transfer

**Setup:**
- Valid bank account
- Sufficient balance
- Correct details

**Expected:**
- Transfer succeeds
- Webhook: `transfer.success`
- Balance debited
- Locked funds cleared

### Scenario 2: Failed Transfer

**Setup:**
- Invalid account number
- Or use test failure pattern

**Test with invalid account:**
```json
{
  "amount_kobo": 5000000,
  "account_number": "0000000000",
  "account_name": "Test",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Expected:**
- Initiation might fail immediately
- Or webhook: `transfer.failed`
- Balance NOT debited
- Locked funds returned

### Scenario 3: Webhook Delay

**What if webhook arrives late?**

**Test:**
1. Initiate withdrawal
2. Stop API server (webhook can't be received)
3. Wait for transfer to complete on Paystack
4. Start API server
5. Webhook will retry (Paystack retries for 3 days)

OR reconciliation worker will catch it after 10 minutes:

**Worker logs:**
```
🔄 running reconciliation check...
found pending withdrawals count=1
verifying withdrawal status
transfer status verified status=success
reconciling successful transfer
✓ reconciliation complete
```

---

## Part 7: Monitoring & Observability

### Real-Time Monitoring

**Terminal Setup:**
```
┌─────────────────┬─────────────────┐
│  Terminal 1     │  Terminal 2     │
│  API Logs       │  Ngrok Tunnel   │
├─────────────────┼─────────────────┤
│  Terminal 3     │  Terminal 4     │
│  Worker Logs    │  Ngrok Dash     │
└─────────────────┴─────────────────┘
```

**Watch for:**
- Terminal 1: Webhook received
- Terminal 2: Connection status
- Terminal 3: Processing logs
- Terminal 4: HTTP traffic (http://127.0.0.1:4040)

### Paystack Dashboard Monitoring

**Check:**
1. **Transfers:** https://dashboard.paystack.com/transfers
   - Transfer status
   - Amount
   - Recipient

2. **Webhook Logs:** https://dashboard.paystack.com/webhooks-log
   - Delivery status
   - Response codes
   - Retry attempts

3. **API Logs:** https://dashboard.paystack.com/logs
   - All API calls
   - Errors
   - Rate limits

### Database Monitoring

```sql
-- Real-time withdrawal status
SELECT 
  reference,
  status,
  amount/100.0 as amount_naira,
  AGE(NOW(), created_at) as age
FROM wallet_transactions
WHERE type = 'withdrawal' AND created_at > NOW() - INTERVAL '1 hour'
ORDER BY created_at DESC;

-- Locked balance overview
SELECT 
  COUNT(*) as wallets_with_locked_funds,
  SUM(locked_balance)/100.0 as total_locked_naira
FROM wallets
WHERE locked_balance > 0;
```

---

## ✅ Success Checklist

Complete end-to-end test is successful when you see:

- [ ] Ngrok tunnel running (green status)
- [ ] Paystack webhook configured with ngrok URL
- [ ] Withdrawal initiated (status: pending)
- [ ] Locked balance increased immediately
- [ ] Worker processed transfer
- [ ] Webhook received via ngrok
- [ ] API logged webhook processing
- [ ] Transaction status changed to completed
- [ ] Main balance debited
- [ ] Locked balance cleared to 0
- [ ] Paystack dashboard shows successful transfer

---

## 🎓 Pro Tips

### Tip 1: Keep Ngrok Running

```powershell
# Run in dedicated terminal
# Don't close this terminal!
ngrok http 8081
```

### Tip 2: Use Ngrok Dashboard

```
http://127.0.0.1:4040
```

Features:
- See all requests in real-time
- Inspect request/response bodies
- Replay requests
- Debug webhook issues

### Tip 3: Test Webhook Signature

```powershell
# Manual webhook with correct signature
cd tools
go run trigger_transfer_webhook.go --reference WD-ABC123 --event success
```

### Tip 4: Monitor Multiple Withdrawals

```sql
-- Watch all pending withdrawals
SELECT reference, status, created_at
FROM wallet_transactions
WHERE type = 'withdrawal' AND status = 'pending'
ORDER BY created_at DESC;
```

### Tip 5: Save Ngrok Sessions

```powershell
# Save important sessions
ngrok http 8081 --log=stdout > ngrok.log
```

---

## 🚀 Quick Test Command

**Complete test in one go:**

```powershell
# Terminal 1
.\api.exe

# Terminal 2
ngrok http 8081

# Terminal 3
.\worker.exe

# Then in Postman:
# 1. Login
# 2. Initiate Withdrawal
# 3. Wait 1-5 minutes
# 4. Check balance
# ✅ Done!
```

---

**You're now ready to test the complete withdrawal flow with real Paystack webhooks! 🎉**

For questions or issues, check:
- API logs (Terminal 1)
- Ngrok dashboard (http://127.0.0.1:4040)
- Paystack webhook logs (dashboard.paystack.com/webhooks-log)

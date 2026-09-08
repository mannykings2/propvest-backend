# ✅ Webhook Testing Checklist

## Pre-Flight Checklist

Before starting, ensure:

- [ ] PostgreSQL running: `docker ps | findstr postgres`
- [ ] API built: `go build -o api.exe ./cmd/api`
- [ ] Worker built: `go build -o worker.exe ./cmd/worker`
- [ ] Ngrok installed: `ngrok version`
- [ ] Postman collection imported
- [ ] Test wallet has balance (₦100,000+)

---

## Step 1: Start Services ⚙️

### 1.1 Start API Server
- [ ] Open Terminal 1
- [ ] Run: `.\api.exe`
- [ ] Wait for: `listening on :8081`
- [ ] Status: ✅ API Running

### 1.2 Start Ngrok Tunnel
- [ ] Open Terminal 2
- [ ] Run: `ngrok http 8081`
- [ ] Copy HTTPS URL (e.g., `https://abc123.ngrok.io`)
- [ ] Test URL: Visit `https://YOUR_NGROK_URL/api/v1/health`
- [ ] Status: ✅ Ngrok Running

### 1.3 Start Worker
- [ ] Open Terminal 3
- [ ] Run: `.\worker.exe`
- [ ] Wait for: `✅ Worker is running`
- [ ] Status: ✅ Worker Running

### 1.4 Open Ngrok Dashboard (Optional)
- [ ] Open browser: `http://127.0.0.1:4040`
- [ ] Status: ✅ Dashboard Accessible

---

## Step 2: Configure Paystack 🔐

### 2.1 Setup Webhook URL
- [ ] Go to: https://dashboard.paystack.com/settings/webhooks
- [ ] Click: "Add Webhook URL"
- [ ] Enter: `https://YOUR_NGROK_URL/api/v1/webhooks/paystack/transfer`
- [ ] Select events:
  - [ ] `transfer.success`
  - [ ] `transfer.failed`
  - [ ] `transfer.reversed`
- [ ] Click: "Save"
- [ ] Verify: Test webhook sent
- [ ] Check Terminal 1: Should show `POST /api/v1/webhooks/paystack/transfer 200`
- [ ] Status: ✅ Webhook Configured

### 2.2 Enable Transfers
- [ ] Go to: https://dashboard.paystack.com/settings/transfers
- [ ] Click: "Enable Transfers"
- [ ] Set Transfer PIN (e.g., 1234)
- [ ] Confirm PIN
- [ ] Accept terms
- [ ] Status: ✅ Transfers Enabled

---

## Step 3: Test in Postman 📮

### 3.1 Login
- [ ] Open Postman
- [ ] Collection: "PropVest Withdrawal Testing"
- [ ] Request: "1. Authentication" → "Login"
- [ ] Click: "Send"
- [ ] Verify: Response 200 OK
- [ ] Check: `access_token` saved automatically
- [ ] Status: ✅ Logged In

### 3.2 Check Initial Balance
- [ ] Request: "2. Wallet Operations" → "Get Wallet Balance"
- [ ] Click: "Send"
- [ ] Note: `main_balance` (should be ~10000000 = ₦100,000)
- [ ] Note: `locked_balance` (should be 0)
- [ ] Status: ✅ Balance Checked
- [ ] **Write down:** Main = ₦_______ Locked = ₦_______

### 3.3 Initiate Withdrawal
- [ ] Request: "3. Withdrawal Flow" → "Initiate Withdrawal"
- [ ] Review body:
  ```json
  {
    "amount_kobo": 5000000,       // ₦50,000
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058",
    "bank_name": "GTBank"
  }
  ```
- [ ] Click: "Send"
- [ ] Verify: Response 200 OK
- [ ] Verify: `status: "pending"`
- [ ] Copy: `reference` value (e.g., WD-ABC123)
- [ ] Status: ✅ Withdrawal Initiated
- [ ] **Write down:** Reference = __________________

### 3.4 Verify Locked Balance
- [ ] Request: "Get Wallet Balance" (repeat 3.2)
- [ ] Click: "Send"
- [ ] Verify: `main_balance` = 10000000 (UNCHANGED)
- [ ] Verify: `locked_balance` = 5000000 (₦50,000 LOCKED! ⭐)
- [ ] Status: ✅ Funds Locked
- [ ] **Calculation:** Available = Main - Locked = ₦_______

---

## Step 4: Monitor Processing 👀

### 4.1 Check Worker Logs (Terminal 3)
- [ ] Look for: `🏦 processing withdrawal`
- [ ] Look for: `account verified`
- [ ] Look for: `transfer initiated`
- [ ] Look for: `transfer_code=TRF_xyz`
- [ ] Status: ✅ Worker Processed

### 4.2 Check Paystack Dashboard
- [ ] Go to: https://dashboard.paystack.com/transfers
- [ ] Find: Your transfer (₦50,000)
- [ ] Verify: Reference matches (WD-ABC123)
- [ ] Note: Status (Processing/Pending)
- [ ] Status: ✅ Transfer Visible

### 4.3 Watch for Webhook (1-5 minutes)

**In Terminal 1 (API), watch for:**
- [ ] `POST /api/v1/webhooks/paystack/transfer`
- [ ] `webhook signature verified`
- [ ] `processing transfer event=transfer.success`
- [ ] `finalizing withdrawal`
- [ ] `withdrawal finalized status=completed`
- [ ] Status: ✅ Webhook Received

**In Terminal 2 (Ngrok) or Dashboard:**
- [ ] `POST /api/v1/webhooks/paystack/transfer`
- [ ] Status: `200 OK`
- [ ] Status: ✅ Webhook Delivered

**Time waited:** _____ minutes

---

## Step 5: Verify Completion ✨

### 5.1 Check Transaction Status
- [ ] Request: "Get Transactions"
- [ ] Click: "Send"
- [ ] Find: Your withdrawal (WD-ABC123)
- [ ] Verify: `status: "completed"` (changed from "pending"! ⭐)
- [ ] Verify: `external_reference: "TRF_xyz"` (Paystack code)
- [ ] Status: ✅ Transaction Completed

### 5.2 Check Final Balance
- [ ] Request: "Get Wallet Balance"
- [ ] Click: "Send"
- [ ] Verify: `main_balance` = 5000000 (₦50,000 - DEBITED! ⭐)
- [ ] Verify: `locked_balance` = 0 (CLEARED! ⭐)
- [ ] Status: ✅ Balance Updated
- [ ] **Final balance:** ₦_______

### 5.3 Verify in Database
- [ ] Open: `docker exec -it propvest-postgres psql -U propvest -d propvest`
- [ ] Run:
  ```sql
  SELECT reference, status, main_balance, locked_balance
  FROM wallet_transactions wt
  JOIN wallets w ON w.id = wt.wallet_id
  WHERE reference = 'WD-ABC123';
  ```
- [ ] Verify: `status = 'completed'`
- [ ] Verify: `locked_balance = 0`
- [ ] Exit: `\q`
- [ ] Status: ✅ Database Verified

---

## Step 6: Test Edge Cases 🧪

### 6.1 Test Insufficient Balance
- [ ] Request: "4. Error Scenarios" → "Test Insufficient Balance"
- [ ] Click: "Send"
- [ ] Verify: Response 422
- [ ] Verify: Error mentions "insufficient balance"
- [ ] Status: ✅ Validation Works

### 6.2 Test Below Minimum
- [ ] Request: "Test Below Minimum"
- [ ] Click: "Send"
- [ ] Verify: Response 422
- [ ] Verify: Error mentions "minimum withdrawal"
- [ ] Status: ✅ Minimum Check Works

### 6.3 Test Invalid Account
- [ ] Request: "Test Invalid Account"
- [ ] Click: "Send"
- [ ] Verify: Response 400 or 422
- [ ] Verify: Error mentions invalid account
- [ ] Status: ✅ Account Validation Works

---

## Step 7: Test Reconciliation (Optional) 🔄

### 7.1 Test Missed Webhook Scenario
- [ ] Initiate another withdrawal
- [ ] **Stop API server** (Ctrl+C in Terminal 1)
- [ ] Wait 30 seconds
- [ ] **Restart API server** (`.\api.exe`)
- [ ] Wait 10 minutes (or restart worker for immediate check)
- [ ] Worker logs should show: `🔄 running reconciliation check`
- [ ] Verify: Transaction completes via reconciliation
- [ ] Status: ✅ Reconciliation Works

---

## Final Verification ✅

### All Systems Green?
- [ ] API server running without errors
- [ ] Worker running without errors
- [ ] Ngrok tunnel active
- [ ] Withdrawal completed successfully
- [ ] Balance updated correctly
- [ ] Webhook received and processed
- [ ] No locked funds remaining
- [ ] All validations working

### Success Indicators
- [x] Withdrawal: `pending` → `completed`
- [x] Main balance: ₦100,000 → ₦50,000
- [x] Locked balance: ₦50,000 → ₦0
- [x] Webhook delivered: 200 OK
- [x] Paystack dashboard shows success

---

## Troubleshooting Checklist 🔧

### If Webhook Not Received:

- [ ] Check ngrok tunnel is running
- [ ] Verify ngrok URL in Paystack settings
- [ ] Check API logs for errors
- [ ] Visit ngrok dashboard: http://127.0.0.1:4040
- [ ] Check Paystack webhook logs: https://dashboard.paystack.com/webhooks-log
- [ ] Try manual webhook: `go run tools/trigger_transfer_webhook.go --reference WD-ABC123 --event success`

### If Balance Not Updating:

- [ ] Check transaction status in database
- [ ] Check API response for errors
- [ ] Verify wallet_transactions table updated
- [ ] Check for locked funds in wallets table
- [ ] Restart worker and wait 5 minutes

### If Validation Not Working:

- [ ] Check request body format
- [ ] Verify required fields present
- [ ] Check data types (numbers not strings)
- [ ] Review API error response

---

## Performance Notes 📊

**Expected Timings:**
- Withdrawal initiation: < 1 second
- Worker picks up job: < 5 seconds (with RabbitMQ)
- Paystack processes: 1-5 minutes (test mode)
- Webhook delivery: < 1 second after completion
- Balance update: < 1 second after webhook

**Total time:** 1-5 minutes from initiation to completion

---

## Test Results 📝

**Date:** _______________  
**Tester:** _______________  

**Results:**
- [ ] ✅ All tests passed
- [ ] ⚠️ Partial success (note issues below)
- [ ] ❌ Failed (see errors below)

**Issues Found:**
```
_______________________________________
_______________________________________
_______________________________________
```

**Notes:**
```
_______________________________________
_______________________________________
_______________________________________
```

---

**Test Complete! 🎉**

If all checkboxes are ticked, your withdrawal system is working perfectly!

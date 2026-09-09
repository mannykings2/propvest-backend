# Paystack Withdrawal Integration Guide

## Overview

This guide explains how to test real withdrawals using Paystack's Transfer API in test mode. You'll be able to send actual test transfers to Nigerian bank accounts.

---

## Prerequisites

### 1. Paystack Account Setup

✅ You already have:
```env
PAYSTACK_SECRET_KEY=sk_test_bb30d89975e64d0787923e064e0af2df55478154
PAYSTACK_PUBLIC_KEY=pk_test_b1e892d2d025c78b50982d0100489f4dd14e8e24
PAYMENT_PROVIDER=paystack
```

### 2. Enable Transfers in Paystack Dashboard

**⚠️ CRITICAL: You MUST enable transfers before testing!**

1. Go to https://dashboard.paystack.com/
2. Navigate to **Settings** → **Preferences**
3. Scroll to **Transfers**
4. Click **Enable Transfers**
5. Set a **Transfer PIN** (4-digit PIN for authorizing transfers)
6. Save settings

**Without this, all transfer attempts will fail with "Transfers not enabled"**

### 3. Fund Your Paystack Test Balance

Paystack test mode simulates real money but doesn't actually move funds.

**Option A: Request Test Balance**
1. Go to https://dashboard.paystack.com/
2. Click **Get Help** → **Contact Support**
3. Request: "Please add test balance for withdrawal testing"

**Option B: Simulate Deposits**
You can "fund" your test account by doing test deposits first.

---

## How Paystack Transfers Work

```
┌────────────────────────────────────────────────────────┐
│ Step 1: Create Transfer Recipient                     │
├────────────────────────────────────────────────────────┤
│ POST /transferrecipient                                │
│ {                                                      │
│   "type": "nuban",                                     │
│   "name": "John Doe",                                  │
│   "account_number": "0123456789",                      │
│   "bank_code": "058"                                   │
│ }                                                      │
│ → Returns: recipient_code (RCP_xxxxx)                 │
└────────────────────────────────────────────────────────┘
           ↓
┌────────────────────────────────────────────────────────┐
│ Step 2: Initiate Transfer                             │
├────────────────────────────────────────────────────────┤
│ POST /transfer                                         │
│ {                                                      │
│   "source": "balance",                                 │
│   "amount": 50000,                                     │
│   "recipient": "RCP_xxxxx",                            │
│   "reference": "WD-ABC123"                             │
│ }                                                      │
│ → Returns: transfer_code (TRF_xxxxx)                  │
│ → Status: "pending" (being processed)                 │
└────────────────────────────────────────────────────────┘
           ↓
┌────────────────────────────────────────────────────────┐
│ Step 3: Paystack Processes Transfer (1-5 minutes)     │
├────────────────────────────────────────────────────────┤
│ - Debits your Paystack balance                         │
│ - Sends money via NIBSS to recipient bank             │
│ - Updates transfer status                              │
│ - Sends webhook notification                           │
└────────────────────────────────────────────────────────┘
           ↓
┌────────────────────────────────────────────────────────┐
│ Step 4: Verify Transfer                               │
├────────────────────────────────────────────────────────┤
│ GET /transfer/verify/TRF_xxxxx                         │
│ → Returns: status ("success", "failed", "pending")    │
└────────────────────────────────────────────────────────┘
```

---

## Testing Withdrawals

### Test Scenario 1: Resolve Bank Account

Before initiating withdrawal, verify the account exists:

```bash
# This happens automatically in the withdrawal flow
# But you can test it directly:

curl -X GET "https://api.paystack.co/bank/resolve?account_number=0123456789&bank_code=058" \
  -H "Authorization: Bearer sk_test_bb30d89975e64d0787923e064e0af2df55478154"
```

**Successful Response:**
```json
{
  "status": true,
  "message": "Account number resolved",
  "data": {
    "account_number": "0123456789",
    "account_name": "JOHN DOE",
    "bank_id": 9
  }
}
```

**Failed Response:**
```json
{
  "status": false,
  "message": "Could not resolve account name. Check parameters or try again."
}
```

### Test Scenario 2: End-to-End Withdrawal

#### Step 1: Check Current Balance

```bash
GET http://localhost:8081/api/v1/wallet
Authorization: Bearer <your_jwt_token>
```

**Response:**
```json
{
  "data": {
    "main_balance": 100000,
    "locked_balance": 0
  }
}
```

#### Step 2: Initiate Withdrawal

```bash
POST http://localhost:8081/api/v1/wallet/withdraw
Authorization: Bearer <your_jwt_token>
Content-Type: application/json

{
  "amount": 50000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "John Doe"
}
```

**What Happens Behind the Scenes:**
1. ✅ Backend calls `ResolveAccountNumber()` → Verifies account with NIBSS
2. ✅ Locks ₦50,000 in wallet (`locked_balance = 50000`)
3. ✅ Creates pending transaction
4. ✅ Queues withdrawal job for worker
5. ✅ Returns pending status to user

**Response:**
```json
{
  "data": {
    "id": "uuid",
    "reference": "WD-ABC123XYZ",
    "status": "pending",
    "amount": 50000
  }
}
```

#### Step 3: Worker Processes Transfer

The worker (or reconciler) automatically:
1. Picks up withdrawal from queue
2. Creates Paystack transfer recipient
3. Initiates transfer via Paystack API
4. Saves transfer_code for tracking

#### Step 4: Wait for Paystack Processing

In **test mode**: Usually instant or 1-2 minutes  
In **live mode**: 1-5 minutes typically

#### Step 5: Paystack Webhook Arrives (Optional)

Paystack sends webhook to your endpoint:
```
POST /api/v1/webhooks/paystack/transfer
X-Paystack-Signature: <signature>

{
  "event": "transfer.success",
  "data": {
    "reference": "WD-ABC123XYZ",
    "status": "success",
    "transfer_code": "TRF_xxxxx"
  }
}
```

#### Step 6: Check Final Status

```bash
GET http://localhost:8081/api/v1/wallet/transactions?reference=WD-ABC123XYZ
```

**Success Response:**
```json
{
  "data": {
    "reference": "WD-ABC123XYZ",
    "status": "completed",
    "amount": 50000,
    "balance_before": 100000,
    "balance_after": 50000
  }
}
```

#### Step 7: Verify Balance

```bash
GET http://localhost:8081/api/v1/wallet
```

**Response:**
```json
{
  "data": {
    "main_balance": 50000,
    "locked_balance": 0
  }
}
```

✅ **Withdrawal Complete!** Funds debited from wallet and sent to bank account.

---

## Paystack Test Bank Accounts

Paystack provides test bank accounts for testing:

| Bank | Code | Test Account | Expected Result |
|------|------|--------------|-----------------|
| GTBank | 058 | 0123456789 | Success |
| Access Bank | 044 | 0690000031 | Success |
| Zenith Bank | 057 | 1234567890 | Success |

Use these for testing without needing real bank accounts.

---

## Common Errors & Solutions

### Error: "Transfers not enabled"

**Solution:**
1. Go to Paystack Dashboard
2. Settings → Preferences → Enable Transfers
3. Set Transfer PIN

### Error: "Insufficient balance"

**Solution:**
1. Check Paystack Dashboard → Balance
2. Request test balance from Paystack support
3. Or do test deposits first to fund balance

### Error: "Could not resolve account name"

**Causes:**
- Invalid account number
- Invalid bank code
- Bank API temporarily down

**Solution:**
- Verify account number is correct (10 digits)
- Use valid bank code (see list above)
- Try again later if bank API is down

### Error: "Recipient creation failed"

**Causes:**
- Account already exists as recipient
- Invalid account details

**Solution:**
Check Paystack Dashboard → Transfers → Recipients to see if it already exists

### Error: "Transfer initiation failed: Unauthorized"

**Causes:**
- Wrong API key
- API key doesn't have transfer permissions

**Solution:**
- Verify `PAYSTACK_SECRET_KEY` in `.env`
- Ensure you're using secret key (starts with `sk_`)
- Not public key (starts with `pk_`)

---

## Monitoring Transfers

### Check Transfer Status in Paystack Dashboard

1. Go to https://dashboard.paystack.com/
2. Navigate to **Transfers** → **All Transfers**
3. Find your transfer by reference
4. View status, recipient, amount, etc.

### Check Transfer via API

```bash
curl -X GET "https://api.paystack.co/transfer/verify/TRF_xxxxx" \
  -H "Authorization: Bearer sk_test_bb30d89975e64d0787923e064e0af2df55478154"
```

### Check Your Application Logs

```bash
# Watch logs for transfer processing
tail -f logs/worker.log | grep "withdrawal"

# Or if using console output
go run cmd/worker/main.go
```

---

## Webhook Setup for Real-Time Updates

For production, you'll want Paystack to notify you immediately when transfers complete.

### 1. Setup Ngrok (for localhost testing)

```bash
ngrok http 8081
```

**Output:**
```
Forwarding https://abc123.ngrok.io -> http://localhost:8081
```

### 2. Configure Webhook in Paystack

1. Go to https://dashboard.paystack.com/
2. Settings → API Keys & Webhooks
3. Add webhook URL: `https://abc123.ngrok.io/api/v1/webhooks/paystack/transfer`
4. Save

### 3. Test Webhook

Paystack will now send real-time notifications when transfers complete.

**Webhook events:**
- `transfer.success` - Transfer completed successfully
- `transfer.failed` - Transfer failed
- `transfer.reversed` - Transfer was reversed

---

## Testing Checklist

- [ ] Enable Transfers in Paystack Dashboard
- [ ] Set Transfer PIN
- [ ] Fund test balance (or request from support)
- [ ] Test account resolution with valid account
- [ ] Test account resolution with invalid account
- [ ] Initiate withdrawal (small amount first, e.g., ₦100)
- [ ] Check wallet locked_balance increases
- [ ] Wait for worker to process
- [ ] Check Paystack Dashboard for transfer
- [ ] Verify transfer status via API
- [ ] Confirm wallet debited after success
- [ ] Test failure scenario (invalid account)
- [ ] Verify automatic reversal on failure
- [ ] Setup webhooks with ngrok
- [ ] Test real-time webhook delivery

---

## Production Checklist

Before going live with real money:

- [ ] Switch to live API keys (`sk_live_...`)
- [ ] Test with small real amounts first (₦100)
- [ ] Setup proper webhook URL (not ngrok)
- [ ] Implement webhook signature verification
- [ ] Setup monitoring and alerts
- [ ] Configure withdrawal limits
- [ ] Implement 2FA for large withdrawals
- [ ] Setup reconciliation worker
- [ ] Test failure scenarios thoroughly
- [ ] Document support procedures
- [ ] Train support team on handling stuck withdrawals

---

## Paystack API Limits

**Test Mode:**
- Unlimited API calls
- Simulated transfers (no real money)
- Instant or near-instant processing

**Live Mode:**
- Rate limits apply (check Paystack docs)
- Real money transfers
- 1-5 minute processing time
- Bank operating hours apply

---

## Support

### Paystack Support
- Email: support@paystack.com
- Phone: +234 1 888 5551
- Dashboard: Help button

### PropVest Backend Issues
1. Check logs: `tail -f logs/app.log`
2. Check database: See `TESTING_WITHDRAWALS.md` for SQL queries
3. Review `WITHDRAWAL_ARCHITECTURE.md` for architecture details

---

## Next Steps

1. ✅ Test account resolution
2. ✅ Test single withdrawal
3. ✅ Test failure handling
4. ✅ Setup webhooks
5. → Implement service layer (Step 6)
6. → Add webhook handler (Step 7)
7. → Enhance worker (Step 8)
8. → End-to-end testing (Step 9)

---

## Quick Reference

**Paystack API Base URL:**
```
Test: https://api.paystack.co
Live: https://api.paystack.co
```

**Headers:**
```
Authorization: Bearer sk_test_YOUR_SECRET_KEY
Content-Type: application/json
```

**Key Endpoints:**
```
GET  /bank/resolve                 - Verify account
POST /transferrecipient            - Create recipient
POST /transfer                     - Initiate transfer
GET  /transfer/verify/:reference   - Check status
```

**Status Values:**
- `pending` - Being processed
- `success` - Completed successfully
- `failed` - Transfer failed
- `reversed` - Transfer was reversed

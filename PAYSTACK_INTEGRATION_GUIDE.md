# 🚀 Paystack Integration - Complete Setup & Testing Guide

**Status:** ✅ Paystack provider already implemented!  
**Your Task:** Configure and test  
**Time Required:** 15-20 minutes

---

## 📋 What You Need to Know

**Good News:** The Paystack integration is already fully implemented in `internal/payments/payments.go`! You just need to:
1. Get your webhook secret from Paystack dashboard
2. Update `.env` file
3. Test the real payment flow

---

## Step 1: Get Paystack Webhook Secret

### Why Do We Need This?
The webhook secret is used to verify that webhook requests actually come from Paystack and not from an attacker. Without it, someone could send fake "payment successful" webhooks and credit wallets fraudulently.

### How to Get It:

1. **Go to Paystack Dashboard:**
   ```
   https://dashboard.paystack.com/#/settings/developer
   ```

2. **Find "Webhook" Section:**
   - Scroll down to the "Webhook" section
   - You'll see "Secret Hash" or "Webhook Secret"

3. **Copy the Secret:**
   - Click to reveal/copy the webhook secret
   - It looks like: `sk_test_xxxxxxxxxxxxx` or a long hash

4. **Update Your `.env` File:**
   Open `c:\Users\USER\go_projects\propvest-backend\.env` and replace:
   ```env
   PAYSTACK_WEBHOOK_SECRET=YOUR_WEBHOOK_SECRET_FROM_DASHBOARD
   ```
   
   With your actual secret:
   ```env
   PAYSTACK_WEBHOOK_SECRET=your_actual_secret_here
   ```

---

## Step 2: Verify Your Configuration

Your `.env` should have these Paystack settings:

```env
# Payment Configuration (Paystack)
PAYMENT_PROVIDER=paystack
PAYSTACK_SECRET_KEY=sk_test_YOUR_TEST_SECRET_KEY
PAYSTACK_PUBLIC_KEY=pk_test_YOUR_TEST_PUBLIC_KEY
PAYSTACK_WEBHOOK_SECRET=your_actual_webhook_secret
```

### Configuration Explained:

- **`PAYMENT_PROVIDER=paystack`** - Tells the system to use Paystack (not mock)
- **`PAYSTACK_SECRET_KEY`** - Used for server-side API calls (initialize & verify payments)
- **`PAYSTACK_PUBLIC_KEY`** - Optional, for frontend integration
- **`PAYSTACK_WEBHOOK_SECRET`** - Used to verify webhook signatures (security!)

---

## Step 3: Understand How Paystack Integration Works

### Architecture Overview:

```
User Request
    ↓
WalletHandler (internal/handlers/wallet.go)
    ↓
WalletService (internal/services/wallet_service.go)
    ↓
PaymentProvider Interface (internal/payments/provider.go)
    ↓
PaystackProvider (internal/payments/payments.go) ✅ ALREADY IMPLEMENTED
    ↓
Paystack API (https://api.paystack.co)
```

### What's Already Implemented:

✅ **Provider Interface** (`internal/payments/provider.go`)
- Defines the contract all providers must follow
- `InitializeDeposit()` - Start payment
- `VerifyTransaction()` - Verify payment status
- `VerifyWebhookSignature()` - Validate webhooks

✅ **Paystack Provider** (`internal/payments/payments.go`)
- Full Paystack API integration
- HMAC-SHA512 webhook signature verification
- Proper error handling
- 15-second HTTP timeout

✅ **Provider Factory** (`New()` function)
- Automatically selects provider based on config
- Falls back to mock if Paystack keys missing

---

## Step 4: How Real Paystack Flow Works

### Deposit Flow (Step-by-Step):

**1. User Initiates Deposit:**
```
POST /api/v1/wallet/deposit
{
  "amount_kobo": 150000
}
```

**2. Backend Calls Paystack:**
```
PaystackProvider.InitializeDeposit()
   ↓
POST https://api.paystack.co/transaction/initialize
Authorization: Bearer sk_test_...
{
  "email": "user@example.com",
  "amount": 150000,
  "reference": "DEP-abc123",
  "callback_url": "http://localhost:8080/api/v1/wallet/deposit/callback"
}
```

**3. Paystack Responds:**
```json
{
  "status": true,
  "data": {
    "authorization_url": "https://checkout.paystack.com/xyz123",
    "access_code": "xyz123",
    "reference": "DEP-abc123"
  }
}
```

**4. User Redirected to Paystack:**
- Frontend redirects user to `authorization_url`
- User enters card details on Paystack's secure page
- User completes payment

**5. Paystack Sends Webhook:**
```
POST http://localhost:8080/api/v1/webhooks/payment
X-Paystack-Signature: hmac_sha512_signature
{
  "event": "charge.success",
  "data": {
    "reference": "DEP-abc123",
    "amount": 150000,
    "status": "success",
    "customer": {
      "email": "user@example.com"
    }
  }
}
```

**6. Backend Verifies:**
```
1. Verify webhook signature (VerifyWebhookSignature)
2. Call Paystack API to verify transaction (VerifyTransaction)
3. Check amount matches
4. Credit wallet (idempotent)
```

---

## Step 5: Testing with Real Paystack

### Prerequisites:
1. ✅ Paystack test keys in `.env`
2. ✅ Webhook secret configured
3. ✅ Database running (`docker compose up -d`)
4. ✅ Backend running (`make run`)
5. ✅ User registered and logged in

### Test Flow:

#### **Test 1: Initiate Real Deposit**

**Request:**
```http
POST http://localhost:8080/api/v1/wallet/deposit
Authorization: Bearer YOUR_ACCESS_TOKEN
Content-Type: application/json

{
  "amount_kobo": 150000
}
```

**Expected Response:**
```json
{
  "success": true,
  "data": {
    "authorization_url": "https://checkout.paystack.com/xyz123",
    "access_code": "xyz123",
    "reference": "DEP-abc123-uuid",
    "amount_kobo": 150000,
    "amount_formatted": "₦1,500.00"
  }
}
```

**What to Do:**
1. Copy the `authorization_url`
2. Open it in your browser
3. You'll see Paystack's payment page

---

#### **Test 2: Complete Payment on Paystack**

**On Paystack Payment Page:**

1. **Use Test Card Details:**
   ```
   Card Number: 4084084084084081
   Expiry: Any future date (e.g., 12/25)
   CVV: Any 3 digits (e.g., 123)
   PIN: 0000 (Paystack test card PIN)
   OTP: 123456 (Paystack test OTP)
   ```

2. **Click "Pay ₦1,500"**

3. **Enter PIN when prompted:** `0000`

4. **Enter OTP when prompted:** `123456`

5. **Payment Success!**
   - You'll see "Payment Successful" page
   - Paystack will automatically send webhook to your backend

---

#### **Test 3: Verify Wallet Credited**

**Request:**
```http
GET http://localhost:8080/api/v1/wallet
Authorization: Bearer YOUR_ACCESS_TOKEN
```

**Expected Response:**
```json
{
  "success": true,
  "data": {
    "main_balance": 150000,
    "main_balance_formatted": "₦1,500.00",
    "earnings_balance": 0,
    "earnings_balance_formatted": "₦0.00",
    "currency": "NGN"
  }
}
```

✅ **Success!** Wallet balance should show ₦1,500

---

#### **Test 4: Check Transaction History**

**Request:**
```http
GET http://localhost:8080/api/v1/wallet/transactions
Authorization: Bearer YOUR_ACCESS_TOKEN
```

**Expected Response:**
```json
{
  "success": true,
  "data": {
    "transactions": [
      {
        "id": "uuid",
        "reference": "DEP-abc123-uuid",
        "type": "deposit",
        "amount": 150000,
        "amount_formatted": "₦1,500.00",
        "status": "completed",
        "balance_before": 0,
        "balance_after": 150000,
        "description": "Deposit via Paystack",
        "created_at": "2026-08-05T..."
      }
    ],
    "total": 1,
    "page": 1
  }
}
```

---

## Step 6: Understanding Webhook Security

### Why Signature Verification Matters:

**Without Verification:**
```
Attacker sends fake webhook:
POST /webhooks/payment
{
  "reference": "VICTIM-123",
  "status": "success",
  "amount": 1000000000
}

Backend credits ₦10,000,000 to victim's wallet
Attacker steals money from victim
Company loses money!
```

**With Verification:**
```
1. Attacker sends fake webhook
2. Backend computes signature: HMAC(secret, body)
3. Signature doesn't match (attacker doesn't know secret)
4. Backend rejects webhook
5. No money credited
6. Attack prevented!
```

### How It Works:

```go
// Paystack computes signature
signature = HMAC-SHA512(webhook_secret, request_body)

// Paystack sends
Headers: X-Paystack-Signature: {signature}
Body: {...}

// Backend verifies
expected = HMAC-SHA512(our_webhook_secret, request_body)
if signature == expected {
    // Authentic webhook from Paystack
    process()
} else {
    // Fake webhook, reject!
    return 400
}
```

---

## Step 7: Testing Webhook Locally (Optional)

Paystack can't send webhooks to `localhost` because your computer isn't accessible from the internet. For local testing, you have two options:

### Option A: Simulate Webhook Manually (Recommended for Development)

1. **Initiate deposit** → get reference
2. **Complete payment** on Paystack page
3. **Manually call verify endpoint:**

```http
GET http://localhost:8080/api/v1/wallet/verify/DEP-abc123
Authorization: Bearer YOUR_ACCESS_TOKEN
```

This forces backend to verify with Paystack and credit wallet.

### Option B: Use Ngrok for Real Webhooks (Advanced)

1. **Install ngrok:**
   ```bash
   choco install ngrok
   ```

2. **Expose localhost:**
   ```bash
   ngrok http 8080
   ```

3. **Get public URL:**
   ```
   https://abc123.ngrok.io
   ```

4. **Update Paystack webhook URL:**
   - Go to Paystack Dashboard → Settings → Webhooks
   - Set webhook URL: `https://abc123.ngrok.io/api/v1/webhooks/payment`
   - Save

5. **Test payment:**
   - Paystack will send real webhooks to ngrok
   - Ngrok forwards to your localhost:8080
   - Webhook handler processes it

---

## Step 8: Paystack Test Cards

Paystack provides test cards for different scenarios:

### Success Card (Recommended for Testing):
```
Card: 4084084084084081
Expiry: 12/25
CVV: 123
PIN: 0000
OTP: 123456
```

### Failed Card (Test Error Handling):
```
Card: 4084084084084081
Expiry: 12/25
CVV: 123
PIN: 1111 (Use wrong PIN to trigger failure)
```

### Declined Card:
```
Card: 5060666666666666666
Expiry: 12/25
CVV: 123
(This card always gets declined)
```

More test cards: https://paystack.com/docs/payments/test-payments/

---

## Step 9: Verify Logs

When testing, check backend logs for confirmation:

```
2026/08/05 12:00:00 payment provider: Paystack
2026/08/05 12:01:00 Initializing deposit for user@example.com: ₦1,500
2026/08/05 12:02:00 Webhook received: signature valid
2026/08/05 12:02:00 Verifying payment with Paystack: DEP-abc123
2026/08/05 12:02:00 Payment verified: success, amount: 150000
2026/08/05 12:02:00 Crediting wallet: user_id=..., amount=150000
2026/08/05 12:02:00 Wallet credited successfully
```

---

## Step 10: Common Issues & Solutions

### Issue 1: "payment provider: mock" in logs

**Problem:** Backend is still using mock provider  
**Solution:**
1. Check `.env` has `PAYMENT_PROVIDER=paystack`
2. Check `PAYSTACK_SECRET_KEY` is set and starts with `sk_test_`
3. Restart backend: `Ctrl+C` then `make run`

---

### Issue 2: "Invalid signature" error

**Problem:** Webhook signature verification fails  
**Solution:**
1. Get webhook secret from Paystack dashboard
2. Update `PAYSTACK_WEBHOOK_SECRET` in `.env`
3. Restart backend

---

### Issue 3: "Paystack API returned 401"

**Problem:** Invalid API keys  
**Solution:**
1. Verify `PAYSTACK_SECRET_KEY` in `.env`
2. Check key starts with `sk_test_` (test mode)
3. Get fresh keys from https://dashboard.paystack.com/#/settings/developers

---

### Issue 4: Wallet not credited after payment

**Problem:** Webhook didn't arrive or failed  
**Solutions:**
1. Check backend logs for webhook errors
2. Manually verify payment:
   ```http
   GET /api/v1/wallet/verify/{reference}
   ```
3. Check Paystack dashboard for webhook delivery status

---

## ✅ Testing Checklist

- [ ] Webhook secret configured in `.env`
- [ ] Backend shows "payment provider: Paystack" on startup
- [ ] Can initiate deposit (get Paystack checkout URL)
- [ ] Can complete payment on Paystack page with test card
- [ ] Wallet balance increases after payment
- [ ] Transaction appears in history
- [ ] Can withdraw funds (balance decreases)
- [ ] Error handling works (try declined card)

---

## 🎯 Success Criteria

**You've successfully integrated Paystack when:**

1. ✅ Deposit initiation returns real Paystack checkout URL
2. ✅ Payment completion on Paystack page works
3. ✅ Wallet is credited automatically
4. ✅ Transaction shows "Deposit via Paystack"
5. ✅ Logs show "payment provider: Paystack"
6. ✅ Webhook signature verification passes

---

## 📚 Additional Resources

### Paystack Documentation:
- Test Cards: https://paystack.com/docs/payments/test-payments/
- API Reference: https://paystack.com/docs/api/
- Webhooks: https://paystack.com/docs/payments/webhooks/
- Transaction Flow: https://paystack.com/docs/payments/accept-payments/

### Project Documentation:
- `internal/payments/payments.go` - Paystack implementation
- `internal/payments/provider.go` - Provider interface
- `docs/05-Modules/5.1-PAYMENT_ARCHITECTURE.md` - Payment architecture

---

## 🔄 Switching Back to Mock (For Testing)

If you want to switch back to mock provider:

```env
# Change this line in .env:
PAYMENT_PROVIDER=mock

# Then restart backend
```

---

## 🚀 Ready for Production?

Before going live:

1. **Switch to Live Keys:**
   ```env
   PAYSTACK_SECRET_KEY=sk_live_... (not sk_test_...)
   PAYSTACK_PUBLIC_KEY=pk_live_... (not pk_test_...)
   ```

2. **Set Production Webhook URL:**
   - Paystack Dashboard → Settings → Webhooks
   - URL: `https://your-domain.com/api/v1/webhooks/payment`

3. **Enable Logging:**
   - Monitor webhook deliveries in Paystack dashboard
   - Set up alerts for failed webhooks

4. **Test Thoroughly:**
   - Test small amount first (₦100)
   - Verify webhook delivery works
   - Test withdrawal flow

---

**Status:** ✅ Paystack integration ready to test!  
**Next Step:** Get webhook secret and test payment flow

**Questions?** Check the logs or Paystack dashboard for debugging!

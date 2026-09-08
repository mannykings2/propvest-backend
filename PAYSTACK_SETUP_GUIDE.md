# 🔐 Paystack Test Keys Setup Guide

## Overview

This guide shows you how to switch from the mock payment provider to real Paystack test mode. This lets you test with Paystack's actual infrastructure without processing real money.

---

## 📋 Prerequisites

1. A Paystack account ([Sign up here](https://dashboard.paystack.com/signup))
2. Test API keys (automatically available after signup)

---

## 🔑 Step 1: Get Your Paystack Test Keys

### 1.1 Login to Paystack Dashboard
Go to: https://dashboard.paystack.com/

### 1.2 Navigate to Settings
Click **Settings** → **API Keys & Webhooks**

### 1.3 Copy Your Test Keys

You'll see:

**Public Key (Test):**
```
pk_test_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

**Secret Key (Test):**
```
sk_test_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

**⚠️ Important:** 
- Test keys start with `pk_test_` and `sk_test_`
- Live keys start with `pk_live_` and `sk_live_`
- **NEVER** commit live keys to git!

---

## ⚙️ Step 2: Update Your .env File

Open `.env` and update the Payment Configuration section:

```bash
# Payment Configuration (Paystack)
PAYMENT_PROVIDER=paystack
PAYSTACK_SECRET_KEY=sk_test_YOUR_ACTUAL_KEY_HERE
PAYSTACK_PUBLIC_KEY=pk_test_YOUR_ACTUAL_KEY_HERE
PAYSTACK_WEBHOOK_SECRET=YOUR_WEBHOOK_SECRET_HERE
```

### Example (with fake keys):
```bash
PAYMENT_PROVIDER=paystack
PAYSTACK_SECRET_KEY=sk_test_abc123def456ghi789jkl012mno345
PAYSTACK_PUBLIC_KEY=pk_test_xyz987wvu654tsr321qpo098nml765
PAYSTACK_WEBHOOK_SECRET=whsec_abc123def456ghi789
```

### Where to get PAYSTACK_WEBHOOK_SECRET:

The webhook secret is optional for testing. If you want webhook verification:

1. Go to **Settings** → **API Keys & Webhooks**
2. Scroll down to **Webhook Secret**
3. Copy the secret key

**For development:** You can leave this empty or use your secret key as webhook secret (Paystack uses the secret key for webhook HMAC if webhook secret is not set).

---

## 🔄 Step 3: Restart Your Server

Stop the current server (Ctrl+C) and restart:

```bash
make run
```

You should see:
```
level=INFO msg="payment provider: Paystack"
```

(Instead of `"payment provider: mock"`)

---

## 🧪 Step 4: Test with Paystack

### 4.1 Initiate a Deposit

```bash
POST http://localhost:8081/api/v1/wallet/deposit
Authorization: Bearer <your_token>
Content-Type: application/json

{
  "amount": 1500000
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "authorization_url": "https://checkout.paystack.com/abc123xyz",
    "reference": "DEP-..."
  }
}
```

### 4.2 Complete Payment on Paystack

1. Copy the `authorization_url` from response
2. Open it in your browser
3. You'll see Paystack's hosted checkout page
4. Use Paystack test cards (see below)
5. Complete the payment

### 4.3 Paystack Test Cards

**Successful Payment:**
```
Card Number: 4084084084084081
Expiry: Any future date (e.g., 12/25)
CVV: 408
OTP: 123456
PIN: 1234
```

**Failed Payment:**
```
Card Number: 5060666666666666666
Expiry: Any future date
CVV: 123
```

**More test cards:** https://paystack.com/docs/payments/test-payments/#test-cards

---

## 🔔 Step 5: Setup Webhook (Optional for Local Testing)

For local development, Paystack webhooks won't reach your localhost. You have two options:

### Option A: Use Ngrok (Recommended for Testing)

1. **Install Ngrok:** https://ngrok.com/download

2. **Start Ngrok:**
   ```bash
   ngrok http 8081
   ```

3. **Copy the HTTPS URL:**
   ```
   Forwarding: https://abc123.ngrok.io -> http://localhost:8081
   ```

4. **Configure Paystack Webhook:**
   - Go to Paystack Dashboard → Settings → Webhooks
   - Add webhook URL: `https://abc123.ngrok.io/api/v1/webhooks/payment`
   - Save

5. **Now webhooks will work!** Paystack → Ngrok → Your localhost

### Option B: Manually Trigger Webhooks (Quick Testing)

For quick testing without ngrok, manually call the webhook endpoint in Postman after payment completes.

---

## 📊 Step 6: Verify in Paystack Dashboard

### Check Transaction
1. Go to **Transactions** in Paystack dashboard
2. You'll see your test payments
3. Click on a transaction to see details

### Check Logs
1. Go to **Logs** → **Event Logs**
2. You'll see webhook delivery attempts
3. Useful for debugging webhook issues

---

## 🔄 Switching Between Mock and Paystack

### Use Mock Provider (No real API calls):
```bash
# In .env
PAYMENT_PROVIDER=mock
# (or remove PAYMENT_PROVIDER entirely)
```

### Use Paystack Test Mode:
```bash
# In .env
PAYMENT_PROVIDER=paystack
PAYSTACK_SECRET_KEY=sk_test_...
```

### Use Paystack Live Mode (Production):
```bash
# In .env
PAYMENT_PROVIDER=paystack
PAYSTACK_SECRET_KEY=sk_live_...  # ⚠️ Real money!
```

---

## 🎯 Complete Test Flow with Paystack

### 1. Register/Login
```bash
POST /api/v1/auth/login
{
  "email": "test@example.com",
  "password": "SecurePass123!"
}
```

### 2. Check Initial Balance
```bash
GET /api/v1/wallet
Authorization: Bearer <token>

Response: { "main_balance": 0 }
```

### 3. Initiate Deposit
```bash
POST /api/v1/wallet/deposit
Authorization: Bearer <token>
{
  "amount": 1500000
}

Response: { "authorization_url": "https://checkout.paystack.com/..." }
```

### 4. Complete Payment
- Open authorization_url in browser
- Use test card: 4084084084084081
- Complete payment

### 5. Wait for Webhook
Paystack sends webhook to your server automatically (if ngrok is set up)

### 6. Check Updated Balance
```bash
GET /api/v1/wallet
Authorization: Bearer <token>

Response: { "main_balance": 1500000 }
```

---

## 🚨 Troubleshooting

### Error: "Invalid API key"
**Cause:** Wrong secret key or using live key in test mode

**Solution:** 
- Verify key starts with `sk_test_`
- Copy directly from Paystack dashboard
- No extra spaces or quotes

### Error: "Webhooks not arriving"
**Cause:** Webhooks can't reach localhost

**Solution:**
- Use ngrok to expose localhost
- OR manually test webhook endpoint in Postman
- Check Paystack dashboard logs

### Payment Success but Wallet Not Updated
**Possible causes:**
1. Webhook not configured
2. Webhook signature verification failed
3. Reference mismatch

**Solution:**
- Check server logs for webhook errors
- Verify PAYSTACK_WEBHOOK_SECRET is correct
- Manually call webhook endpoint to test

### "Authorization URL invalid"
**Cause:** Paystack API error

**Solution:**
- Check server logs for Paystack API errors
- Verify internet connection
- Check Paystack status page

---

## 🔒 Security Best Practices

### Development (.env - NOT committed to git):
```bash
PAYMENT_PROVIDER=paystack
PAYSTACK_SECRET_KEY=sk_test_...  # Test key is safe to use locally
```

### Production (Environment variables - Set on server):
```bash
PAYMENT_PROVIDER=paystack
PAYSTACK_SECRET_KEY=sk_live_...  # NEVER commit this!
PAYSTACK_WEBHOOK_SECRET=whsec_...  # NEVER commit this!
```

### .gitignore should include:
```
.env
.env.local
.env.*.local
```

---

## 📚 Additional Resources

- **Paystack Documentation:** https://paystack.com/docs/api/
- **Test Cards:** https://paystack.com/docs/payments/test-payments/
- **Webhook Guide:** https://paystack.com/docs/payments/webhooks/
- **API Reference:** https://paystack.com/docs/api/transaction/

---

## ✅ Checklist

- [ ] Created Paystack account
- [ ] Copied test API keys
- [ ] Updated `.env` with keys
- [ ] Set `PAYMENT_PROVIDER=paystack`
- [ ] Restarted server
- [ ] Tested deposit initiation
- [ ] Tested with Paystack test card
- [ ] (Optional) Setup ngrok for webhooks
- [ ] Verified balance updated after payment

---

## 🎉 Success Indicators

When everything is working:

✅ Server logs show: `"payment provider: Paystack"`  
✅ Deposit returns real Paystack checkout URL  
✅ Payment page loads on checkout.paystack.com  
✅ Test card payment succeeds  
✅ Webhook arrives (if ngrok configured)  
✅ Wallet balance updates correctly  
✅ Transaction appears in Paystack dashboard  

---

**Last Updated:** 2026-08-10  
**Paystack API Version:** v1  
**Support:** https://support.paystack.com/

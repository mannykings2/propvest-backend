# ⚡ Paystack Integration - Quick Start

**Good News:** Paystack is already implemented! Just configure and test.

---

## 🎯 Quick Setup (3 Steps)

### 1. Get Webhook Secret

Go to: https://dashboard.paystack.com/#/settings/developer

Copy the "Secret Hash" from Webhook section

### 2. Update `.env`

```env
PAYSTACK_WEBHOOK_SECRET=paste_your_secret_here
```

### 3. Restart Backend

```bash
# Stop if running (Ctrl+C)
make run
```

✅ **Check logs show:** `payment provider: Paystack`

---

## 🧪 Test Payment (5 Minutes)

### Step 1: Initiate Deposit
```http
POST http://localhost:8080/api/v1/wallet/deposit
Authorization: Bearer YOUR_TOKEN
Content-Type: application/json

{
  "amount_kobo": 150000
}
```

✅ **Copy `authorization_url` from response**

### Step 2: Pay with Test Card

Open `authorization_url` in browser

**Test Card:**
- Card: `4084084084084081`
- Expiry: `12/25`
- CVV: `123`
- PIN: `0000`
- OTP: `123456`

Click "Pay ₦1,500"

### Step 3: Verify Wallet

```http
GET http://localhost:8080/api/v1/wallet
Authorization: Bearer YOUR_TOKEN
```

✅ **Balance should be ₦1,500**

---

## ✅ Success Checklist

- [ ] `.env` has webhook secret
- [ ] Logs show "payment provider: Paystack"
- [ ] Deposit returns real Paystack URL
- [ ] Payment works with test card
- [ ] Wallet credited after payment

---

## 🆘 Troubleshooting

**Problem:** Still using mock  
**Fix:** Check `.env` has `PAYMENT_PROVIDER=paystack` and restart

**Problem:** Invalid signature  
**Fix:** Get webhook secret from dashboard and update `.env`

**Problem:** 401 Unauthorized from Paystack  
**Fix:** Verify `PAYSTACK_SECRET_KEY` starts with `sk_test_`

---

## 📖 Full Guide

See `PAYSTACK_INTEGRATION_GUIDE.md` for detailed explanations

---

**Time:** 5 minutes setup + 5 minutes testing = **10 minutes total**  
**Status:** Ready to test! 🚀

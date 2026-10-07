# Paystack Test Cards & Testing Guide

Complete guide for testing the PropVest payment and withdrawal workflow using Paystack test environment.

## 🎯 Quick Start: Testing Withdrawals

### Method 1: Via Deposit Flow (Recommended)

This tests your complete integration:

1. **Initiate Deposit** → 2. **Pay with Test Card** → 3. **Webhook Credits Wallet** → 4. **Test Withdrawal**

### Method 2: Direct Database Credit (Quick Testing)

Skip the deposit flow and directly credit your wallet using the SQL script:

```bash
# 1. Get your user ID
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT id, email FROM users;"

# 2. Edit scripts/credit_wallet.sql and replace 'YOUR-USER-ID-HERE'

# 3. Run the script
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -f scripts/credit_wallet.sql
```

---

## 💳 Paystack Test Cards

Paystack provides test cards to simulate different payment scenarios in test mode.

### ✅ Successful Transactions

#### Card 1: Standard Success
```
Card Number: 4084 0840 8408 4081
CVV: 408
Expiry: Any future date (e.g., 12/30)
PIN: 0000
OTP: 123456
```

#### Card 2: Mastercard Success
```
Card Number: 5060 6666 6666 6666 6666
CVV: 123
Expiry: Any future date
PIN: 1234
OTP: 123456
```

#### Card 3: Verve Success
```
Card Number: 5061 0205 5060 1054 7499
CVV: 123
Expiry: Any future date
PIN: 1111
OTP: 123456
```

### ❌ Failed Transactions (for testing error handling)

#### Insufficient Funds
```
Card Number: 5060 6666 6666 6666 6605
CVV: 123
Expiry: Any future date
PIN: 1234
```

#### Card Declined
```
Card Number: 4084 0840 8408 4094
CVV: 408
Expiry: Any future date
```

#### Expired Card
```
Card Number: 4084 0840 8408 4103
CVV: 408
Expiry: Any past date (e.g., 01/20)
```

### 🔄 Timeout Simulation
```
Card Number: 4084 0840 8408 4107
CVV: 408
Expiry: Any future date
PIN: 0000
OTP: 123456
(Will timeout after 30 seconds)
```

---

## 🧪 Complete Testing Workflow

### Step 1: Register/Login User

**POST** `http://localhost:8080/api/v1/auth/register`
```json
{
  "email": "test@propvest.com",
  "password": "SecurePass123!",
  "first_name": "John",
  "last_name": "Doe",
  "phone": "+2348012345678"
}
```

**POST** `http://localhost:8080/api/v1/auth/login`
```json
{
  "email": "test@propvest.com",
  "password": "SecurePass123!"
}
```

Save the JWT token from the response.

---

### Step 2: Check Wallet Balance

**GET** `http://localhost:8080/api/v1/wallet`

**Headers:**
```
Authorization: Bearer YOUR_JWT_TOKEN
```

Response:
```json
{
  "success": true,
  "data": {
    "id": "...",
    "user_id": "...",
    "main_balance": 0,
    "locked_balance": 0,
    "earnings_balance": 0,
    "currency": "NGN"
  }
}
```

---

### Step 3: Initiate Deposit

**POST** `http://localhost:8080/api/v1/wallet/deposit`

**Headers:**
```
Authorization: Bearer YOUR_JWT_TOKEN
Content-Type: application/json
```

**Body:**
```json
{
  "amount": 1000000
}
```
*Note: Amount is in kobo (1,000,000 kobo = ₦10,000)*

Response:
```json
{
  "success": true,
  "data": {
    "authorization_url": "https://checkout.paystack.com/xxxxxxxxx",
    "reference": "DEP-20240912-ABC123"
  }
}
```

---

### Step 4: Complete Payment

1. Copy the `authorization_url` from Step 3
2. Open it in your browser
3. Enter test card details:
   - **Card**: 4084 0840 8408 4081
   - **CVV**: 408
   - **Expiry**: 12/30
   - **PIN**: 0000
   - **OTP**: 123456

4. Paystack will redirect back to your frontend (or show success page)
5. Paystack sends webhook to your backend
6. Your webhook handler credits the wallet

---

### Step 5: Verify Wallet Updated

**GET** `http://localhost:8080/api/v1/wallet`

**Headers:**
```
Authorization: Bearer YOUR_JWT_TOKEN
```

Response should now show updated balance:
```json
{
  "success": true,
  "data": {
    "main_balance": 1000000,
    "locked_balance": 0,
    "earnings_balance": 0,
    "currency": "NGN"
  }
}
```

---

### Step 6: Test Withdrawal

**POST** `http://localhost:8080/api/v1/wallet/withdraw`

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

Response:
```json
{
  "success": true,
  "message": "Withdrawal initiated successfully",
  "data": {
    "id": "...",
    "type": "withdrawal",
    "amount": 50000,
    "balance_before": 1000000,
    "balance_after": 950000,
    "reference": "WDW-20240912-XYZ789",
    "description": "Withdrawal to 0123456789",
    "status": "pending",
    "created_at": "2024-09-12T10:30:00Z"
  }
}
```

---

### Step 7: Check Transaction History

**GET** `http://localhost:8080/api/v1/wallet/transactions?type=all&page=1&limit=10`

**Headers:**
```
Authorization: Bearer YOUR_JWT_TOKEN
```

Response:
```json
{
  "success": true,
  "data": {
    "transactions": [
      {
        "id": "...",
        "type": "withdrawal",
        "amount": 50000,
        "status": "pending",
        "reference": "WDW-20240912-XYZ789",
        "created_at": "..."
      },
      {
        "id": "...",
        "type": "deposit",
        "amount": 1000000,
        "status": "completed",
        "reference": "DEP-20240912-ABC123",
        "created_at": "..."
      }
    ],
    "pagination": {
      "page": 1,
      "limit": 10,
      "total": 2,
      "total_pages": 1
    }
  }
}
```

---

## 🏦 Test Bank Account Numbers

For testing withdrawals, use these test account numbers:

```
Account Number: 0123456789 (any 10-digit number works in test mode)
Bank Code: 058 (GTBank)
Account Name: Any name
```

**Common Bank Codes:**
- GTBank: `058`
- Access Bank: `044`
- Zenith Bank: `057`
- First Bank: `011`
- UBA: `033`

---

## 🎭 Testing Different Scenarios

### Scenario 1: Insufficient Balance
Try to withdraw more than your wallet balance:
```json
{
  "amount": 9999999999,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "John Doe"
}
```

Expected: `422 INSUFFICIENT_BALANCE`

### Scenario 2: Amount Too Small
Try to withdraw less than minimum (₦500):
```json
{
  "amount": 10000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "John Doe"
}
```

Expected: `422 AMOUNT_TOO_SMALL`

### Scenario 3: Invalid Account Number
```json
{
  "amount": 50000,
  "bank_code": "058",
  "account_number": "123",
  "account_name": "John Doe"
}
```

Expected: `400 VALIDATION_ERROR`

### Scenario 4: Rate Limiting
Make 4+ withdrawal requests within 1 hour:

Expected: `429 TOO_MANY_REQUESTS` after 3rd request

---

## 🔧 Troubleshooting

### Webhook Not Received

1. **Check ngrok is running:**
   ```bash
   ngrok http 8080
   ```

2. **Verify webhook URL in Paystack Dashboard:**
   - Settings → API Keys & Webhooks
   - Should be: `https://your-ngrok-url.ngrok.io/api/v1/webhook/paystack`

3. **Check logs:**
   ```bash
   # In your API terminal, you should see:
   [INFO] Webhook received from Paystack
   ```

4. **Manually trigger webhook:**
   Use the test tool at: https://dashboard.paystack.com/settings/developer

### Wallet Not Credited

Check webhook logs and database:
```sql
SELECT * FROM wallet_transactions 
WHERE reference = 'YOUR-REFERENCE' 
ORDER BY created_at DESC;
```

### Withdrawal Stuck in Pending

In test mode, Paystack doesn't actually process withdrawals. You need to:
1. Implement the worker to process the queue
2. Or manually update the status in database for testing

---

## 📝 Important Notes

1. **Test Mode vs Live Mode:**
   - Use `sk_test_xxx` keys for testing
   - Use `sk_live_xxx` keys for production
   - Never mix test and live keys

2. **Amounts:**
   - All amounts are in kobo (1 Naira = 100 kobo)
   - ₦10,000 = 1,000,000 kobo

3. **Test Cards:**
   - Only work with test API keys
   - Will be declined if used with live keys

4. **Webhook Secret:**
   - Different for test and live modes
   - Get from: Dashboard → Settings → API Keys & Webhooks

5. **No Real Money:**
   - Test transactions don't involve real money
   - Paystack test account has unlimited balance

---

## 🚀 Ready for Production

Before going live:

1. ✅ Switch to live API keys in `.env`
2. ✅ Update webhook URL to production domain
3. ✅ Test with real (small) amounts first
4. ✅ Enable webhook IP whitelisting
5. ✅ Set up monitoring and alerts
6. ✅ Implement proper error handling
7. ✅ Add retry logic for failed webhooks
8. ✅ Set up background worker for processing withdrawals

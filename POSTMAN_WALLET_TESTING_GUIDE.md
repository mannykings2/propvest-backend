# Complete Wallet Testing Guide: Deposit & Withdrawal with Paystack

This guide walks you through testing the complete deposit and withdrawal workflow using Postman and Paystack Test Mode.

---

## 📋 Prerequisites

### 1. Paystack Test Account Setup
1. Go to https://paystack.com and create an account
2. Navigate to **Settings → API Keys & Webhooks**
3. Copy your **Test Secret Key** (starts with `sk_test_`)
4. Copy your **Test Public Key** (starts with `pk_test_`)

### 2. Environment Setup

**Update your `.env` file:**
```env
# Paystack Configuration (TEST MODE)
PAYSTACK_SECRET_KEY=sk_test_your_test_secret_key_here
PAYSTACK_PUBLIC_KEY=pk_test_your_test_public_key_here
PAYSTACK_ENVIRONMENT=test

# Webhook Configuration
PAYSTACK_WEBHOOK_SECRET=your_webhook_secret_from_paystack_settings

# Withdrawal Configuration
MIN_WITHDRAWAL_AMOUNT=50000    # ₦500
MAX_WITHDRAWAL_AMOUNT=10000000  # ₦100,000
```

### 3. Start All Services
```bash
# Terminal 1: Start PostgreSQL and RabbitMQ
docker-compose up -d postgres rabbitmq

# Terminal 2: Start API
go run cmd/api/main.go

# Terminal 3: Start Worker
go run cmd/worker/main.go
```

Verify services:
- API: http://localhost:8080/api/v1/health
- RabbitMQ: http://localhost:15672 (guest/guest)
- PostgreSQL: Port 5432

---

## 🔧 Postman Setup

### Step 1: Create New Collection
1. Open Postman
2. Click **New → Collection**
3. Name it: `PropVest Wallet Testing`
4. Add description: `Complete deposit and withdrawal testing`

### Step 2: Create Environment Variables
1. Click **Environments** (left sidebar)
2. Click **+** to create new environment
3. Name it: `PropVest Local Test`
4. Add these variables:

| Variable | Initial Value | Current Value |
|----------|--------------|---------------|
| `base_url` | `http://localhost:8080/api/v1` | (same) |
| `access_token` | (leave empty) | (leave empty) |
| `user_email` | `test@example.com` | (same) |
| `user_password` | `SecureP@ss123` | (same) |
| `deposit_reference` | (leave empty) | (leave empty) |
| `withdrawal_reference` | (leave empty) | (leave empty) |

5. Click **Save**
6. Select this environment in the top-right dropdown

---

## 📝 Test Workflow: Complete End-to-End

### PART 1: USER REGISTRATION & AUTHENTICATION

#### Request 1: Register New User

**Method:** `POST`  
**URL:** `{{base_url}}/auth/register`  
**Headers:**
```
Content-Type: application/json
```

**Body (raw JSON):**
```json
{
  "first_name": "Test",
  "last_name": "User",
  "email": "{{user_email}}",
  "phone": "+2348012345678",
  "password": "{{user_password}}"
}
```

**Expected Response (201):**
```json
{
  "success": true,
  "message": "Registration successful",
  "data": {
    "user": {
      "id": "uuid-here",
      "first_name": "Test",
      "last_name": "User",
      "email": "test@example.com",
      "phone": "+2348012345678"
    },
    "access_token": "eyJhbGc...",
    "refresh_token": "eyJhbGc...",
    "expires_at": "2026-09-12T14:00:00Z"
  }
}
```

**Postman Test Script (Tests tab):**
```javascript
// Save access token for subsequent requests
if (pm.response.code === 201) {
    const jsonData = pm.response.json();
    pm.environment.set("access_token", jsonData.data.access_token);
    pm.test("Registration successful", () => {
        pm.expect(jsonData.success).to.be.true;
    });
}
```

---

#### Request 2: Login (Alternative if already registered)

**Method:** `POST`  
**URL:** `{{base_url}}/auth/login`  
**Headers:**
```
Content-Type: application/json
```

**Body (raw JSON):**
```json
{
  "email": "{{user_email}}",
  "password": "{{user_password}}"
}
```

**Expected Response (200):**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGc...",
    "refresh_token": "eyJhbGc...",
    "expires_at": "2026-09-12T14:00:00Z"
  }
}
```

**Postman Test Script:**
```javascript
if (pm.response.code === 200) {
    const jsonData = pm.response.json();
    pm.environment.set("access_token", jsonData.data.access_token);
}
```

---

### PART 2: DEPOSIT WORKFLOW (Fund Wallet)

#### Request 3: Check Initial Wallet Balance

**Method:** `GET`  
**URL:** `{{base_url}}/wallet`  
**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Expected Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "wallet-uuid",
    "user_id": "user-uuid",
    "main_balance": 0,
    "main_balance_formatted": "₦0.00",
    "earnings_balance": 0,
    "earnings_balance_formatted": "₦0.00",
    "locked_balance": 0,
    "available_balance": 0,
    "currency": "NGN"
  }
}
```

**Postman Test Script:**
```javascript
pm.test("Wallet exists", () => {
    const jsonData = pm.response.json();
    pm.expect(jsonData.success).to.be.true;
    pm.expect(jsonData.data.main_balance).to.equal(0);
});
```

---

#### Request 4: Initiate Deposit

**Method:** `POST`  
**URL:** `{{base_url}}/wallet/deposit`  
**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body (raw JSON):**
```json
{
  "amount_kobo": 1000000
}
```

**Note:** `1000000 kobo = ₦10,000`

**Expected Response (200):**
```json
{
  "success": true,
  "data": {
    "authorization_url": "https://checkout.paystack.com/abc123xyz",
    "access_code": "abc123xyz",
    "reference": "DEP-uuid-here",
    "amount_kobo": 1000000,
    "amount_formatted": "₦10,000.00"
  }
}
```

**Postman Test Script:**
```javascript
if (pm.response.code === 200) {
    const jsonData = pm.response.json();
    pm.environment.set("deposit_reference", jsonData.data.reference);
    pm.test("Deposit initiated", () => {
        pm.expect(jsonData.data.authorization_url).to.include("paystack.com");
    });
}
```

---

#### Request 5: Complete Payment (Browser Step)

**⚠️ IMPORTANT: This is done in your browser, not Postman**

1. Copy the `authorization_url` from the previous response
2. Paste it into your browser
3. You'll see the Paystack payment page
4. Use **Paystack Test Cards:**

**For Successful Payment:**
```
Card Number: 4084 0840 8408 4081
CVV: 408
Expiry: 12/30
PIN: 0000
OTP: 123456
```

**For Failed Payment (to test failure handling):**
```
Card Number: 5060 6666 6666 6666
CVV: 123
Expiry: 12/30
```

5. Click **Pay ₦10,000**
6. Enter the test card details
7. Complete the payment flow
8. You'll be redirected back to your callback URL

---

#### Request 6: Verify Payment (Simulate Webhook)

**⚠️ CRITICAL:** Paystack sends a webhook to your server when payment completes. In local testing, you need to simulate this.

**Option A: Using ngrok (Recommended for real webhook testing)**

1. Install ngrok: https://ngrok.com/download
2. Start ngrok:
```bash
ngrok http 8080
```
3. Copy the HTTPS URL (e.g., `https://abc123.ngrok.io`)
4. Go to Paystack Dashboard → Settings → API Keys & Webhooks
5. Add webhook URL: `https://abc123.ngrok.io/api/v1/webhooks/payment`
6. Now complete the payment in browser and the webhook will arrive automatically

**Option B: Manual Webhook Simulation (Quick Testing)**

**Method:** `POST`  
**URL:** `{{base_url}}/webhooks/payment`  
**Headers:**
```
Content-Type: application/json
X-Paystack-Signature: <computed-signature>
```

**Body (raw JSON):**
```json
{
  "event": "charge.success",
  "data": {
    "reference": "{{deposit_reference}}",
    "amount": 1000000,
    "currency": "NGN",
    "status": "success",
    "customer": {
      "email": "{{user_email}}"
    },
    "paid_at": "2026-09-12T12:00:00Z"
  }
}
```

**How to compute X-Paystack-Signature:**

**Quick PowerShell Script:**
```powershell
# Save as compute_signature.ps1
$secret = "your_paystack_secret_key"
$body = '{"event":"charge.success","data":{"reference":"DEP-xxx","amount":1000000,"currency":"NGN","status":"success"}}'
$hmac = New-Object System.Security.Cryptography.HMACSHA512
$hmac.Key = [Text.Encoding]::UTF8.GetBytes($secret)
$hash = $hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($body))
[BitConverter]::ToString($hash).Replace("-", "").ToLower()
```

**Or use the tool provided:**
```bash
# From project root
go run tools/generate_webhook_signature.go \
  --secret "your_paystack_secret_key" \
  --event "charge.success" \
  --reference "{{deposit_reference}}" \
  --amount 1000000
```

**Expected Response (200):**
```json
{
  "success": true,
  "message": "Webhook processed successfully"
}
```

---

#### Request 7: Verify Wallet Funded

**Method:** `GET`  
**URL:** `{{base_url}}/wallet`  
**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Expected Response (200):**
```json
{
  "success": true,
  "data": {
    "main_balance": 1000000,
    "main_balance_formatted": "₦10,000.00",
    "locked_balance": 0,
    "available_balance": 1000000,
    "currency": "NGN"
  }
}
```

**Postman Test Script:**
```javascript
pm.test("Wallet funded successfully", () => {
    const jsonData = pm.response.json();
    pm.expect(jsonData.data.main_balance).to.equal(1000000);
    pm.expect(jsonData.data.available_balance).to.equal(1000000);
});
```

---

### PART 3: WITHDRAWAL WORKFLOW

#### Request 8: Initiate Withdrawal

**Method:** `POST`  
**URL:** `{{base_url}}/wallet/withdraw`  
**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body (raw JSON):**
```json
{
  "amount_kobo": 50000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Note:** 
- `50000 kobo = ₦500` (minimum withdrawal)
- For **test mode**, Paystack will simulate the bank verification

**Expected Response (200):**
```json
{
  "success": true,
  "message": "Withdrawal initiated successfully",
  "data": {
    "id": "transaction-uuid",
    "reference": "WD-uuid-here",
    "type": "withdrawal",
    "amount": 50000,
    "amount_formatted": "₦500.00",
    "status": "pending",
    "balance_before": 1000000,
    "balance_after": 950000,
    "description": "Withdrawal to GTBank (0123456789)",
    "metadata": {
      "account_number": "0123456789",
      "account_name": "Test User",
      "bank_code": "058",
      "bank_name": "GTBank"
    },
    "created_at": "2026-09-12T12:00:00Z"
  }
}
```

**Postman Test Script:**
```javascript
if (pm.response.code === 200) {
    const jsonData = pm.response.json();
    pm.environment.set("withdrawal_reference", jsonData.data.reference);
    pm.test("Withdrawal initiated", () => {
        pm.expect(jsonData.data.status).to.equal("pending");
        pm.expect(jsonData.data.amount).to.equal(50000);
    });
}
```

---

#### Request 9: Verify Wallet Locked

**Method:** `GET`  
**URL:** `{{base_url}}/wallet`  
**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Expected Response (200):**
```json
{
  "success": true,
  "data": {
    "main_balance": 950000,
    "locked_balance": 50000,
    "available_balance": 900000,
    "main_balance_formatted": "₦9,500.00",
    "currency": "NGN"
  }
}
```

**Explanation:**
- `main_balance = 950000` (debited immediately)
- `locked_balance = 50000` (held until transfer completes)
- `available_balance = 900000` (main - locked)

**Postman Test Script:**
```javascript
pm.test("Funds locked correctly", () => {
    const jsonData = pm.response.json();
    pm.expect(jsonData.data.locked_balance).to.equal(50000);
    pm.expect(jsonData.data.available_balance).to.equal(900000);
});
```

---

#### Request 10: Check Transaction Status

**Method:** `GET`  
**URL:** `{{base_url}}/wallet/transactions?type=withdrawal&status=pending`  
**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Expected Response (200):**
```json
{
  "success": true,
  "data": {
    "transactions": [
      {
        "id": "transaction-uuid",
        "reference": "WD-uuid",
        "type": "withdrawal",
        "amount": 50000,
        "status": "pending",
        "created_at": "2026-09-12T12:00:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "limit": 20,
    "pages": 1
  }
}
```

---

#### Request 11: Wait for Worker Processing

**⏱️ WAIT 5-10 seconds**

The worker picks up the withdrawal from the queue and:
1. Calls Paystack `InitiateTransfer` API
2. Saves the transfer code
3. Waits for webhook or reconciliation

Check worker logs:
```bash
# In Terminal 3 where worker is running
# You should see:
# "processing withdrawal" reference=WD-xxx
# "transfer initiated" transfer_code=TRF-xxx
```

---

#### Request 12: Simulate Transfer Success Webhook

**⚠️ CRITICAL:** Paystack sends a webhook when transfer completes. Simulate it:

**Method:** `POST`  
**URL:** `{{base_url}}/webhooks/paystack/transfer`  
**Headers:**
```
Content-Type: application/json
X-Paystack-Signature: <computed-signature>
```

**Body (raw JSON):**
```json
{
  "event": "transfer.success",
  "data": {
    "transfer_code": "TRF-test-code",
    "reference": "{{withdrawal_reference}}",
    "amount": 50000,
    "currency": "NGN",
    "status": "success",
    "recipient": {
      "account_number": "0123456789",
      "bank_code": "058",
      "bank_name": "GTBank"
    },
    "completed_at": "2026-09-12T12:01:00Z"
  }
}
```

**How to get transfer_code:**

**Option A:** Check database:
```sql
SELECT external_reference FROM wallet_transactions 
WHERE reference = 'WD-xxx';
```

**Option B:** Use the tool:
```bash
go run tools/trigger_transfer_webhook.go \
  --reference "{{withdrawal_reference}}" \
  --status success
```

**Expected Response (200):**
```json
{
  "success": true,
  "message": "Webhook processed successfully"
}
```

---

#### Request 13: Verify Withdrawal Completed

**Method:** `GET`  
**URL:** `{{base_url}}/wallet/transactions?type=withdrawal&reference={{withdrawal_reference}}`  
**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Expected Response (200):**
```json
{
  "success": true,
  "data": {
    "transactions": [
      {
        "id": "transaction-uuid",
        "reference": "WD-uuid",
        "type": "withdrawal",
        "amount": 50000,
        "status": "completed",
        "external_reference": "TRF-test-code",
        "completed_at": "2026-09-12T12:01:00Z"
      }
    ]
  }
}
```

**Postman Test Script:**
```javascript
pm.test("Withdrawal completed", () => {
    const jsonData = pm.response.json();
    const tx = jsonData.data.transactions[0];
    pm.expect(tx.status).to.equal("completed");
    pm.expect(tx.external_reference).to.not.be.null;
});
```

---

#### Request 14: Verify Final Wallet Balance

**Method:** `GET`  
**URL:** `{{base_url}}/wallet`  
**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Expected Response (200):**
```json
{
  "success": true,
  "data": {
    "main_balance": 950000,
    "locked_balance": 0,
    "available_balance": 950000,
    "main_balance_formatted": "₦9,500.00",
    "currency": "NGN"
  }
}
```

**Explanation:**
- `main_balance = 950000` (₦10,000 - ₦500 = ₦9,500)
- `locked_balance = 0` (released after completion)
- `available_balance = 950000` (fully available)

**Postman Test Script:**
```javascript
pm.test("Withdrawal finalized correctly", () => {
    const jsonData = pm.response.json();
    pm.expect(jsonData.data.main_balance).to.equal(950000);
    pm.expect(jsonData.data.locked_balance).to.equal(0);
    pm.expect(jsonData.data.available_balance).to.equal(950000);
});
```

---

## 🧪 Additional Test Scenarios

### Test 1: Minimum Withdrawal Amount

**Request:** Withdraw ₦400 (below minimum)

**Body:**
```json
{
  "amount_kobo": 40000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058"
}
```

**Expected Response (422):**
```json
{
  "success": false,
  "message": "Minimum withdrawal amount is ₦500.00",
  "code": "minimum_withdrawal"
}
```

---

### Test 2: Maximum Withdrawal Amount

**Request:** Withdraw ₦150,000 (above maximum)

**Body:**
```json
{
  "amount_kobo": 15000000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058"
}
```

**Expected Response (422):**
```json
{
  "success": false,
  "message": "Maximum withdrawal amount is ₦100,000.00",
  "code": "maximum_withdrawal"
}
```

---

### Test 3: Insufficient Balance

**Request:** Withdraw more than available

**Body:**
```json
{
  "amount_kobo": 10000000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058"
}
```

**Expected Response (422):**
```json
{
  "success": false,
  "message": "Insufficient wallet balance",
  "code": "insufficient_funds"
}
```

---

### Test 4: Rate Limiting

**Request:** Try 4 withdrawals in quick succession

1st, 2nd, 3rd: Should succeed (200)
4th: Should be blocked (429)

**Expected Response (429):**
```json
{
  "success": false,
  "message": "Too many withdrawal attempts. Maximum 3 per hour. Try again at 14:30:00.",
  "code": "withdrawal_rate_limit"
}
```

**Headers:**
```
X-RateLimit-Limit: 3
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1694567890
Retry-After: 3540
```

---

## 🛠️ Troubleshooting

### Issue: "Unauthorized" (401)

**Cause:** Token expired or not set

**Fix:**
1. Re-run Login request (Request 2)
2. Check `{{access_token}}` environment variable is set
3. Verify `Authorization: Bearer {{access_token}}` header

---

### Issue: Deposit webhook not arriving

**Cause:** Paystack can't reach your localhost

**Fix:**
1. Use ngrok to expose your API:
```bash
ngrok http 8080
```
2. Update Paystack webhook URL to ngrok URL
3. Or manually simulate webhook (Request 6 Option B)

---

### Issue: Withdrawal stuck in "pending"

**Cause:** Worker not running or webhook not received

**Fix:**
1. Check worker is running: `ps aux | grep worker`
2. Check worker logs for errors
3. Wait 5 minutes for reconciliation to pick it up
4. Or manually simulate transfer webhook (Request 12)

---

### Issue: "Invalid signature" on webhook

**Cause:** Signature doesn't match body

**Fix:**
1. Use the signature generation tool:
```bash
go run tools/generate_webhook_signature.go --help
```
2. Ensure body is EXACTLY the same as used to compute signature
3. Check `PAYSTACK_WEBHOOK_SECRET` in `.env` matches Paystack dashboard

---

## 📊 Complete Test Checklist

### Deposit Flow ✅
- [ ] Register/Login user
- [ ] Check initial wallet (balance = 0)
- [ ] Initiate deposit
- [ ] Complete payment in browser (Paystack test card)
- [ ] Webhook received (auto or manual)
- [ ] Wallet balance updated

### Withdrawal Flow ✅
- [ ] Initiate withdrawal (₦500)
- [ ] Verify funds locked
- [ ] Check transaction status (pending)
- [ ] Wait for worker processing
- [ ] Transfer webhook received
- [ ] Transaction status = completed
- [ ] Locked funds released
- [ ] Final balance correct

### Validation Tests ✅
- [ ] Below minimum amount (422)
- [ ] Above maximum amount (422)
- [ ] Insufficient balance (422)
- [ ] Rate limiting (429 on 4th attempt)

---

## 🎯 Success Criteria

After completing all requests, you should have:

1. ✅ **Wallet funded** via Paystack deposit
2. ✅ **Withdrawal initiated** and funds locked
3. ✅ **Worker processed** transfer request
4. ✅ **Transfer completed** via webhook
5. ✅ **Wallet balance** reflects final amount
6. ✅ **Transaction history** shows all operations
7. ✅ **Validation rules** enforced correctly
8. ✅ **Rate limiting** working as expected

**Total Balance After Tests:**
- Started: ₦0
- Deposited: +₦10,000
- Withdrew: -₦500
- **Final: ₦9,500** ✅

---

## 📁 Postman Collection Export

Save all requests as a collection:

1. Click **...** on collection → **Export**
2. Choose **Collection v2.1**
3. Save as `PropVest-Wallet-Testing.postman_collection.json`
4. Share with team

**Collection structure:**
```
PropVest Wallet Testing/
├── 1. Authentication/
│   ├── Register User
│   └── Login
├── 2. Deposit Flow/
│   ├── Check Initial Balance
│   ├── Initiate Deposit
│   ├── Simulate Webhook
│   └── Verify Balance
├── 3. Withdrawal Flow/
│   ├── Initiate Withdrawal
│   ├── Verify Locked
│   ├── Check Status
│   ├── Simulate Transfer Webhook
│   └── Verify Completed
└── 4. Validation Tests/
    ├── Test Minimum Amount
    ├── Test Maximum Amount
    ├── Test Insufficient Balance
    └── Test Rate Limiting
```

---

**You're now ready to test the complete wallet workflow! 🎉**

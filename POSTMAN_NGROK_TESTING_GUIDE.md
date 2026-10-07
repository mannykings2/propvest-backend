# Complete Testing Guide: Postman + ngrok + Paystack

This guide shows you exactly how to test the complete deposit and withdrawal workflow using Postman with ngrok for real webhook testing.

---

## 🎯 What You'll Achieve

By the end, you'll have:
- ✅ Real Paystack webhooks delivered to your local machine
- ✅ Complete deposit flow tested end-to-end
- ✅ Complete withdrawal flow tested end-to-end
- ✅ All validations and rate limiting verified

---

## 📋 Prerequisites

### 1. Install ngrok

**Windows:**
```powershell
# Option 1: Download from website
# Go to https://ngrok.com/download
# Download ngrok-v3-stable-windows-amd64.zip
# Extract to C:\ngrok\

# Option 2: Using Chocolatey
choco install ngrok

# Option 3: Using Scoop
scoop install ngrok
```

**Verify installation:**
```powershell
ngrok version
# Should show: ngrok version 3.x.x
```

### 2. Create ngrok Account

1. Go to https://dashboard.ngrok.com/signup
2. Sign up (free plan is fine)
3. Copy your authtoken from https://dashboard.ngrok.com/get-started/your-authtoken

**Add authtoken:**
```powershell
ngrok config add-authtoken YOUR_AUTH_TOKEN_HERE
```

### 3. Get Paystack Test Credentials

1. Sign up at https://paystack.com
2. Go to **Settings → API Keys & Webhooks**
3. Copy:
   - **Test Secret Key** (starts with `sk_test_`)
   - **Test Public Key** (starts with `pk_test_`)

---

## 🚀 Setup Steps

### Step 1: Configure Environment

**Edit your `.env` file:**
```env
# Paystack Configuration (TEST MODE)
PAYSTACK_SECRET_KEY=sk_test_your_actual_test_key_here
PAYSTACK_PUBLIC_KEY=pk_test_your_actual_test_key_here
PAYSTACK_ENVIRONMENT=test

# Withdrawal Limits
MIN_WITHDRAWAL_AMOUNT=50000
MAX_WITHDRAWAL_AMOUNT=10000000

# Database
DATABASE_URL=postgresql://propvest:propvest@localhost:5432/propvest?sslmode=disable

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@localhost:5672/

# Server
PORT=8080
```

---

### Step 2: Start All Services

**Terminal 1 - Database & Queue:**
```powershell
# Start PostgreSQL and RabbitMQ
docker-compose up -d postgres rabbitmq

# Verify they're running
docker-compose ps
```

**Terminal 2 - API Server:**
```powershell
# Start the API
go run cmd/api/main.go

# You should see:
# Server starting on :8080
# Database connected
```

**Terminal 3 - Worker:**
```powershell
# Start the worker
go run cmd/worker/main.go

# You should see:
# Worker started
# Connected to RabbitMQ
# Listening for withdrawal jobs
```

**Terminal 4 - ngrok (Keep this open):**
```powershell
# Expose your API to the internet
ngrok http 8080

# You'll see output like:
# Forwarding  https://abc123def456.ngrok-free.app -> http://localhost:8080
```

**🔑 COPY the https URL** - You'll need it next!

Example: `https://abc123def456.ngrok-free.app`

---

### Step 3: Configure Paystack Webhooks

1. Go to https://dashboard.paystack.com/settings/developer
2. Scroll to **Webhooks** section
3. Click **Add Webhook URL**
4. Enter: `https://YOUR_NGROK_URL.ngrok-free.app/api/v1/webhooks/payment`
   
   Example: `https://abc123def456.ngrok-free.app/api/v1/webhooks/payment`

5. Click **Add**
6. Copy the **Webhook Secret** shown
7. Add it to your `.env`:
   ```env
   PAYSTACK_WEBHOOK_SECRET=wh_secret_from_paystack
   ```
8. **Restart API** (Terminal 2) for new secret to load

---

## 📬 Postman Setup

### Step 1: Create Collection

1. Open Postman
2. Click **New → Collection**
3. Name: `PropVest Wallet Testing`
4. Save

### Step 2: Create Environment

1. Click **Environments** (left sidebar)
2. Click **+ Create Environment**
3. Name: `PropVest Local + ngrok`
4. Add these variables:

| Variable | Initial Value | Type |
|----------|---------------|------|
| `api_url` | `http://localhost:8080/api/v1` | default |
| `access_token` | _(leave empty)_ | default |
| `user_email` | `test@example.com` | default |
| `user_password` | `SecureP@ss123` | default |
| `deposit_reference` | _(leave empty)_ | default |
| `withdrawal_reference` | _(leave empty)_ | default |

5. **Save**
6. Select this environment (top-right dropdown)

---

## 🧪 Complete Test Workflow

### TEST 1: Register & Login

#### Request: Register User

**Method:** `POST`  
**URL:** `{{api_url}}/auth/register`

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

**Tests (Tests tab):**
```javascript
if (pm.response.code === 201) {
    const data = pm.response.json();
    pm.environment.set("access_token", data.data.access_token);
    pm.test("✅ User registered", () => {
        pm.expect(data.success).to.be.true;
    });
    console.log("✅ Token saved:", data.data.access_token);
} else {
    console.log("❌ Registration failed:", pm.response.json());
}
```

**Click Send ▶**

**Expected:** `201 Created`

---

### TEST 2: Check Initial Wallet

#### Request: Get Wallet Balance

**Method:** `GET`  
**URL:** `{{api_url}}/wallet`

**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Tests:**
```javascript
const data = pm.response.json();
pm.test("✅ Wallet exists", () => {
    pm.expect(data.success).to.be.true;
    pm.expect(data.data.main_balance).to.equal(0);
});
console.log("💰 Balance:", data.data.main_balance_formatted);
```

**Expected:** Balance = ₦0.00

---

### TEST 3: Initiate Deposit (₦10,000)

#### Request: Start Deposit

**Method:** `POST`  
**URL:** `{{api_url}}/wallet/deposit`

**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body:**
```json
{
  "amount_kobo": 1000000
}
```

**Tests:**
```javascript
if (pm.response.code === 200) {
    const data = pm.response.json();
    pm.environment.set("deposit_reference", data.data.reference);
    const authUrl = data.data.authorization_url;
    
    pm.test("✅ Deposit initiated", () => {
        pm.expect(authUrl).to.include("paystack.com");
    });
    
    console.log("💳 Payment URL:", authUrl);
    console.log("📝 Reference:", data.data.reference);
    console.log("\n🚀 NEXT STEP: Copy the URL above and open in browser");
}
```

**Expected:** `200 OK` with Paystack URL

---

### TEST 4: Complete Payment (Browser)

**⚠️ SWITCH TO BROWSER NOW**

1. **Copy** the `authorization_url` from Postman response
2. **Paste** into your browser
3. You'll see Paystack payment page

**Use this TEST CARD:**
```
Card Number:  4084 0840 8408 4081
CVV:          408
Expiry:       12/30
PIN:          0000
OTP:          123456
```

4. Click **Pay ₦10,000.00**
5. Fill in the card details
6. Click **Pay**
7. Enter OTP: `123456`
8. Payment will complete!

**🎉 Paystack will AUTOMATICALLY send webhook to your API via ngrok!**

---

### TEST 5: Verify Webhook Received

**Check your API logs (Terminal 2):**
```
You should see:
✅ Webhook received: charge.success
✅ Payment verified: DEP-xxxxx
✅ Wallet credited: ₦10,000.00
```

**Check ngrok dashboard:**
1. Open http://localhost:4040 (ngrok inspector)
2. Click on the webhook request
3. You'll see the full request/response from Paystack

---

### TEST 6: Verify Wallet Funded

#### Request: Check Balance Again

**Method:** `GET`  
**URL:** `{{api_url}}/wallet`

**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Tests:**
```javascript
const data = pm.response.json();
pm.test("✅ Wallet funded!", () => {
    pm.expect(data.data.main_balance).to.equal(1000000);
    pm.expect(data.data.available_balance).to.equal(1000000);
});
console.log("💰 New Balance:", data.data.main_balance_formatted);
console.log("✅ DEPOSIT COMPLETE!");
```

**Expected:** Balance = ₦10,000.00

---

## 💸 WITHDRAWAL TESTING

### TEST 7: Initiate Withdrawal (₦500)

#### Request: Withdraw Funds

**Method:** `POST`  
**URL:** `{{api_url}}/wallet/withdraw`

**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body:**
```json
{
  "amount_kobo": 50000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Tests:**
```javascript
if (pm.response.code === 200) {
    const data = pm.response.json();
    pm.environment.set("withdrawal_reference", data.data.reference);
    
    pm.test("✅ Withdrawal initiated", () => {
        pm.expect(data.data.status).to.equal("pending");
        pm.expect(data.data.amount).to.equal(50000);
    });
    
    console.log("📤 Withdrawal Reference:", data.data.reference);
    console.log("⏳ Status:", data.data.status);
    console.log("💰 Amount:", data.data.amount_formatted);
}
```

**Expected:** `200 OK` with status = "pending"

---

### TEST 8: Check Wallet Locked

#### Request: Verify Funds Locked

**Method:** `GET`  
**URL:** `{{api_url}}/wallet`

**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Tests:**
```javascript
const data = pm.response.json();
pm.test("✅ Funds locked correctly", () => {
    pm.expect(data.data.main_balance).to.equal(950000);
    pm.expect(data.data.locked_balance).to.equal(50000);
    pm.expect(data.data.available_balance).to.equal(900000);
});
console.log("💰 Main Balance:", data.data.main_balance_formatted);
console.log("🔒 Locked:", "₦" + (data.data.locked_balance / 100).toFixed(2));
console.log("✅ Available:", "₦" + (data.data.available_balance / 100).toFixed(2));
```

**Expected:**
- Main: ₦9,500
- Locked: ₦500
- Available: ₦9,000

---

### TEST 9: Wait for Worker Processing

**⏱️ WAIT 5-10 SECONDS**

**Check Worker logs (Terminal 3):**
```
You should see:
🔄 Processing withdrawal: WD-xxxxx
🏦 Initiating transfer to 0123456789
✅ Transfer initiated: TRF-xxxxx
⏳ Waiting for webhook or reconciliation
```

**In test mode, Paystack will automatically send transfer webhook!**

---

### TEST 10: Verify Transfer Webhook

**Wait 5-10 more seconds...**

**Check API logs (Terminal 2):**
```
✅ Transfer webhook received: transfer.success
✅ Withdrawal finalized: WD-xxxxx
✅ Locked funds released
```

**Check ngrok inspector (http://localhost:4040):**
- You'll see the transfer webhook from Paystack
- Status: `transfer.success` or `transfer.failed`

---

### TEST 11: Verify Withdrawal Complete

#### Request: Check Transaction Status

**Method:** `GET`  
**URL:** `{{api_url}}/wallet/transactions?type=withdrawal&status=completed`

**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Tests:**
```javascript
const data = pm.response.json();
pm.test("✅ Withdrawal completed", () => {
    pm.expect(data.data.transactions.length).to.be.greaterThan(0);
    const tx = data.data.transactions[0];
    pm.expect(tx.status).to.equal("completed");
    pm.expect(tx.type).to.equal("withdrawal");
});
console.log("✅ WITHDRAWAL COMPLETE!");
console.log("📊 Transactions:", data.data.transactions.length);
```

**Expected:** Transaction status = "completed"

---

### TEST 12: Check Final Balance

#### Request: Final Wallet Check

**Method:** `GET`  
**URL:** `{{api_url}}/wallet`

**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Tests:**
```javascript
const data = pm.response.json();
pm.test("✅ Final balance correct", () => {
    pm.expect(data.data.main_balance).to.equal(950000);
    pm.expect(data.data.locked_balance).to.equal(0);
    pm.expect(data.data.available_balance).to.equal(950000);
});

console.log("\n🎉 TEST COMPLETE!");
console.log("══════════════════════════");
console.log("Starting:    ₦0.00");
console.log("+ Deposit:   ₦10,000.00");
console.log("- Withdraw:  ₦500.00");
console.log("══════════════════════════");
console.log("Final:       " + data.data.main_balance_formatted);
console.log("✅ ALL TESTS PASSED!");
```

**Expected:** Balance = ₦9,500.00, Locked = ₦0

---

## 🧪 Validation Tests

### TEST 13: Below Minimum

**Request:** Withdraw ₦400 (below ₦500 minimum)

**Body:**
```json
{
  "amount_kobo": 40000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058"
}
```

**Expected:** `422 Unprocessable Entity`
```json
{
  "success": false,
  "message": "Minimum withdrawal amount is ₦500.00",
  "code": "minimum_withdrawal"
}
```

---

### TEST 14: Above Maximum

**Request:** Withdraw ₦150,000 (above ₦100k maximum)

**Body:**
```json
{
  "amount_kobo": 15000000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058"
}
```

**Expected:** `422 Unprocessable Entity`
```json
{
  "success": false,
  "message": "Maximum withdrawal amount is ₦100,000.00",
  "code": "maximum_withdrawal"
}
```

---

### TEST 15: Rate Limiting

**Request:** Try 4 withdrawals quickly

Run the withdrawal request (TEST 7) **4 times** in quick succession.

**Expected:**
- 1st: ✅ `200 OK`
- 2nd: ✅ `200 OK`
- 3rd: ✅ `200 OK`
- 4th: ❌ `429 Too Many Requests`

**4th Response:**
```json
{
  "success": false,
  "message": "Too many withdrawal attempts. Maximum 3 per hour.",
  "code": "withdrawal_rate_limit"
}
```

**Headers:**
```
X-RateLimit-Limit: 3
X-RateLimit-Remaining: 0
Retry-After: 3540
```

---

## 📊 Complete Test Checklist

### ✅ Deposit Flow
- [x] Register/Login → Get token
- [x] Check wallet (₦0)
- [x] Initiate deposit (₦10k)
- [x] Pay with test card in browser
- [x] Webhook received via ngrok
- [x] Wallet balance = ₦10,000

### ✅ Withdrawal Flow
- [x] Initiate withdrawal (₦500)
- [x] Funds locked (₦500)
- [x] Worker processes transfer
- [x] Transfer webhook received via ngrok
- [x] Status = completed
- [x] Locked funds released
- [x] Final balance = ₦9,500

### ✅ Validation Tests
- [x] Below minimum (422)
- [x] Above maximum (422)
- [x] Rate limiting (429)

---

## 🐛 Troubleshooting

### Issue: ngrok tunnel not working

**Check:**
```powershell
# Is ngrok running?
# Terminal 4 should show "Forwarding https://..."

# Test ngrok URL
curl https://YOUR_NGROK_URL.ngrok-free.app/api/v1/health
```

**Fix:** Restart ngrok

---

### Issue: Webhook not arriving

**Check:**
1. **ngrok inspector:** http://localhost:4040
   - Do you see the webhook request?
   - If YES → Check API logs for errors
   - If NO → Check Paystack webhook URL is correct

2. **Paystack webhook URL:**
   - Settings → Webhooks
   - Should be: `https://YOUR_NGROK_URL.ngrok-free.app/api/v1/webhooks/payment`

3. **API is running:**
   - Terminal 2 should show server logs

**Fix:** Update webhook URL in Paystack, restart ngrok

---

### Issue: "Invalid signature" error

**Cause:** Webhook secret mismatch

**Fix:**
1. Get webhook secret from Paystack dashboard
2. Update `.env`:
   ```env
   PAYSTACK_WEBHOOK_SECRET=wh_secret_from_paystack
   ```
3. **Restart API** (Terminal 2)

---

### Issue: Withdrawal stuck "pending"

**Check:**
1. Worker running? (Terminal 3)
2. Worker logs show transfer initiated?
3. ngrok showing transfer webhook?

**Fix:**
- Wait 5 minutes for reconciliation
- Check RabbitMQ: http://localhost:15672
- Restart worker if needed

---

## 🎯 Success Criteria

After completing all tests:

```
✅ User registered and logged in
✅ Deposit initiated and paid
✅ Webhook received automatically via ngrok
✅ Wallet credited ₦10,000
✅ Withdrawal initiated ₦500
✅ Funds locked correctly
✅ Worker processed transfer
✅ Transfer webhook received via ngrok
✅ Withdrawal completed
✅ Final balance ₦9,500
✅ Validation tests passed
✅ Rate limiting working
```

**You now have a fully working wallet system! 🎉**

---

## 💡 Pro Tips

### Keep ngrok URL Stable

**Free plan:** URL changes every restart  
**Paid plan:** Get a permanent URL

```powershell
# With permanent domain (paid)
ngrok http --domain=your-domain.ngrok-free.app 8080
```

### Debug Webhooks

**ngrok inspector is your friend!**
- Open: http://localhost:4040
- See ALL requests to your API
- Replay webhooks
- Inspect headers/body

### Save Postman Collection

1. Click **...** on collection
2. **Export**
3. Save as `PropVest-Wallet-Tests.json`
4. Share with team

---

## 📁 Quick Reference

| Service | URL | Credentials |
|---------|-----|-------------|
| API | http://localhost:8080 | - |
| RabbitMQ | http://localhost:15672 | guest/guest |
| ngrok Inspector | http://localhost:4040 | - |
| Paystack Dashboard | https://dashboard.paystack.com | Your account |

**Test Card:**
```
4084 0840 8408 4081 | CVV: 408 | PIN: 0000 | OTP: 123456
```

**Environment Variables:**
```
api_url = http://localhost:8080/api/v1
access_token = (auto-saved)
deposit_reference = (auto-saved)
withdrawal_reference = (auto-saved)
```

---

**You're all set! Start testing! 🚀**

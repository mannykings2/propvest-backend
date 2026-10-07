# Withdrawal Testing & Debugging Guide

Complete step-by-step guide for testing withdrawals and debugging issues.

---

## 🔍 Step 1: Find the Logs

When your API server is running, logs appear in the terminal/console. Here's how to check them:

### Option A: Check Running Terminal
Look at the terminal where you ran:
```powershell
go run cmd/api/main.go
# or
.\api.exe
```

You should see log output like:
```
[INFO] Starting PropVest API server on port 8081
[INFO] Withdrawal request received user_id=xxx
[ERROR] account resolution failed error=...
```

### Option B: Enable Detailed Logging

If you're not seeing enough detail, ensure your logger is configured properly. Check `cmd/api/main.go`:

### Option C: Check Database Logs

```powershell
# Check the last withdrawal transaction
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT id, reference, amount, status, description, created_at FROM wallet_transactions WHERE type = 'withdrawal' ORDER BY created_at DESC LIMIT 5;"
```

---

## 🐛 Common Error: "Account Resolution Failed"

Your withdrawal is stuck because Paystack's **Account Resolution API** is being called to verify the bank account, and it's likely failing.

### Why This Happens:

1. **Invalid Paystack Secret Key**
2. **Network/API timeout**
3. **Invalid bank code**
4. **Account doesn't exist**
5. **Paystack test mode limitations**

### Quick Check:

```powershell
# Test if Paystack API is accessible
curl -H "Authorization: Bearer sk_test_YOUR_TEST_SECRET_KEY" https://api.paystack.co/bank
```

---

## 🎯 Step-by-Step: Complete Withdrawal Testing Workflow

### Prerequisites Setup

#### 1️⃣ Start Required Services

```powershell
# Terminal 1: Start Docker services
docker-compose up -d postgres redis rabbitmq

# Verify they're running
docker ps
```

#### 2️⃣ Start Your API Server

```powershell
# Terminal 2: Start API (watch for logs here!)
go run cmd/api/main.go
```

You should see:
```
[INFO] Starting PropVest API server on port 8081
[INFO] Connected to database
[INFO] Connected to Redis
[INFO] Payment provider: paystack
```

#### 3️⃣ Start ngrok (Optional - Only for Testing Full Deposit Flow)

```powershell
# Terminal 3: Start ngrok
ngrok http 8081
```

Copy the HTTPS URL (e.g., `https://abc123.ngrok-free.app`) and update:
- `.env` → `BASE_URL`
- Paystack Dashboard → Webhook URL

**Note:** You don't need ngrok for testing withdrawals, only for deposits that require webhook callbacks.

---

### Testing Workflow

#### ✅ Step 1: Register/Login User

**POST** `http://localhost:8081/api/v1/auth/register`

```json
{
  "email": "testuser@propvest.com",
  "password": "SecurePass123!",
  "first_name": "John",
  "last_name": "Doe",
  "phone": "+2348012345678"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "user": { ... },
    "access_token": "eyJhbGc...",
    "refresh_token": "eyJhbGc..."
  }
}
```

**💾 Save the `access_token`!**

---

#### ✅ Step 2: Credit Your Wallet

Use the SQL script to add funds:

```powershell
# Get your user ID
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT id, email FROM users WHERE email = 'testuser@propvest.com';"

# Output example:
#                   id                  |         email          
# --------------------------------------+------------------------
#  a1b2c3d4-e5f6-7890-abcd-ef1234567890 | testuser@propvest.com
```

Edit `scripts/credit_wallet.sql`:
```sql
v_user_id UUID := 'a1b2c3d4-e5f6-7890-abcd-ef1234567890'; -- ← Your UUID here
```

Run the script:
```powershell
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -f scripts/credit_wallet.sql
```

You should see:
```
NOTICE:  ✓ Wallet credited successfully!
NOTICE:    User ID: a1b2c3d4-e5f6-7890-abcd-ef1234567890
NOTICE:    Previous Balance: ₦0.00
NOTICE:    Amount Credited: ₦100000.00
NOTICE:    New Balance: ₦100000.00
```

---

#### ✅ Step 3: Verify Wallet Balance

**GET** `http://localhost:8081/api/v1/wallet`

**Headers:**
```
Authorization: Bearer YOUR_ACCESS_TOKEN_HERE
```

**Expected Response:**
```json
{
  "success": true,
  "data": {
    "id": "...",
    "user_id": "...",
    "main_balance": 10000000,
    "locked_balance": 0,
    "earnings_balance": 0,
    "currency": "NGN"
  }
}
```

✅ `main_balance: 10000000` = ₦100,000.00

---

#### ✅ Step 4: Test Withdrawal (The Critical Step!)

**POST** `http://localhost:8081/api/v1/wallet/withdraw`

**Headers:**
```
Authorization: Bearer YOUR_ACCESS_TOKEN_HERE
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

**Watch Your Terminal (where API is running) for logs!**

---

### 📊 Expected Logs & Responses

#### ✅ Success Case:

**Terminal Logs:**
```
[INFO] Withdrawal request received user_id=a1b2c3d4-e5f6-7890-abcd-ef1234567890 amount=50000
[INFO] Resolving bank account account_number=0123456789 bank_code=058
[INFO] Account verified account_name="John Doe" bank_name="Guaranty Trust Bank"
[INFO] Withdrawal initiated reference=WD-ABC123DEF456 amount=50000 transaction_id=xyz...
[INFO] Withdrawal queued to RabbitMQ
```

**Postman Response (200 OK):**
```json
{
  "success": true,
  "message": "Withdrawal initiated successfully",
  "data": {
    "id": "xyz-transaction-id",
    "type": "withdrawal",
    "amount": 50000,
    "balance_before": 10000000,
    "balance_after": 10000000,
    "reference": "WD-ABC123DEF456",
    "description": "Withdrawal to Guaranty Trust Bank (0123456789)",
    "status": "pending",
    "created_at": "2024-09-12T10:30:00Z"
  }
}
```

---

#### ❌ Error Case 1: Account Resolution Failed

**Terminal Logs:**
```
[INFO] Resolving bank account account_number=0123456789 bank_code=058
[ERROR] account resolution failed error="paystack api error: Could not resolve account name"
```

**Postman Response (422 or 400):**
```json
{
  "success": false,
  "error": {
    "code": "INVALID_BANK_ACCOUNT",
    "message": "Invalid bank account details"
  }
}
```

**Solution:**
1. **Check Paystack API Key** is correct in `.env`
2. **Use valid test account** (in test mode, some accounts may not resolve)
3. **Try different bank code** (058=GTBank, 044=Access, 057=Zenith)

---

#### ❌ Error Case 2: Insufficient Balance

**Terminal Logs:**
```
[ERROR] withdrawal transaction failed error="insufficient available balance"
```

**Postman Response (422):**
```json
{
  "success": false,
  "error": {
    "code": "INSUFFICIENT_FUNDS",
    "message": "Insufficient wallet balance"
  }
}
```

**Solution:** Credit your wallet again using the SQL script.

---

#### ❌ Error Case 3: Amount Too Small

**Postman Response (422):**
```json
{
  "success": false,
  "error": {
    "code": "AMOUNT_TOO_SMALL",
    "message": "Minimum withdrawal is ₦500.00"
  }
}
```

**Solution:** Use amount >= 50000 (₦500)

---

#### ❌ Error Case 4: Pending Withdrawal Exists

**Postman Response (422):**
```json
{
  "success": false,
  "error": {
    "code": "WITHDRAWAL_PENDING",
    "message": "You have a pending withdrawal"
  }
}
```

**Solution:** Complete or cancel the pending withdrawal first:

```powershell
# Check pending withdrawals
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT id, reference, amount, status FROM wallet_transactions WHERE type = 'withdrawal' AND status = 'pending';"

# Mark as completed (for testing)
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "UPDATE wallet_transactions SET status = 'completed' WHERE status = 'pending' AND type = 'withdrawal';"
```

---

## 🔧 Debugging Checklist

When withdrawal fails, check in this order:

### ✅ 1. API Server Running?
```powershell
# Check if running on port 8081
curl http://localhost:8081/api/v1/health
```

### ✅ 2. Database Connected?
```powershell
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT 1;"
```

### ✅ 3. Wallet Has Balance?
```powershell
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT user_id, main_balance/100.0 as balance_naira, locked_balance/100.0 as locked_naira FROM wallets;"
```

### ✅ 4. Valid JWT Token?
- Token expires after 15 minutes (see `ACCESS_TOKEN_TTL` in .env)
- If expired, login again to get a new token

### ✅ 5. Paystack API Accessible?
```powershell
curl -H "Authorization: Bearer sk_test_YOUR_TEST_SECRET_KEY" https://api.paystack.co/bank
```

Expected response:
```json
{
  "status": true,
  "message": "Banks retrieved",
  "data": [ ... ]
}
```

### ✅ 6. Check Recent Transactions
```powershell
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT id, type, amount/100.0 as amount_naira, status, reference, description, created_at FROM wallet_transactions ORDER BY created_at DESC LIMIT 5;"
```

---

## 🎭 Testing Different Scenarios

### Scenario 1: Small Withdrawal (₦500)
```json
{
  "amount": 50000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "John Doe"
}
```

### Scenario 2: Large Withdrawal (₦50,000)
```json
{
  "amount": 5000000,
  "bank_code": "044",
  "account_number": "0987654321",
  "account_name": "Jane Smith"
}
```

### Scenario 3: Different Banks
```json
{
  "amount": 100000,
  "bank_code": "057",
  "account_number": "1234567890",
  "account_name": "Mike Johnson"
}
```

**Nigerian Bank Codes:**
- GTBank: `058`
- Access Bank: `044`
- Zenith Bank: `057`
- First Bank: `011`
- UBA: `033`

---

## 🚨 Quick Fixes

### Fix 1: Reset Locked Balance

If funds are stuck in `locked_balance`:

```sql
-- View locked funds
SELECT user_id, main_balance/100.0, locked_balance/100.0 FROM wallets;

-- Unlock funds (for testing only!)
UPDATE wallets SET locked_balance = 0, main_balance = main_balance + locked_balance WHERE locked_balance > 0;
```

### Fix 2: Cancel Pending Withdrawal

```sql
-- View pending withdrawals
SELECT id, reference, amount/100.0, status FROM wallet_transactions WHERE status = 'pending' AND type = 'withdrawal';

-- Mark as failed (releases locked funds)
UPDATE wallet_transactions SET status = 'failed' WHERE status = 'pending' AND type = 'withdrawal';

-- Also unlock the funds
UPDATE wallets SET locked_balance = 0, main_balance = main_balance + locked_balance WHERE locked_balance > 0;
```

### Fix 3: Clear Rate Limit

If you hit rate limit (429 error):

```powershell
# Clear Redis rate limit cache
docker exec -it propvest_redis redis-cli FLUSHDB
```

---

## 📝 Full Testing Checklist

- [ ] Docker services running (postgres, redis, rabbitmq)
- [ ] API server running and showing logs
- [ ] User registered/logged in
- [ ] JWT token saved and not expired
- [ ] Wallet credited with test funds
- [ ] Wallet balance verified via API
- [ ] Withdrawal request with valid bank details
- [ ] Terminal shows detailed logs
- [ ] Transaction appears in database
- [ ] No errors in API logs

---

## 🎓 Understanding the Flow

```
User → POST /withdraw
  ↓
1. Validate amount (min/max)
  ↓
2. Check for pending withdrawal
  ↓
3. Resolve bank account (calls Paystack API) ← **LIKELY FAILING HERE**
  ↓
4. Lock funds in wallet
  ↓
5. Create pending transaction
  ↓
6. Queue to RabbitMQ
  ↓
7. Return response to user
  ↓
Worker picks up job → Calls Paystack Transfer API → Updates status
```

**Your issue is most likely at Step 3** where it tries to verify the bank account with Paystack.

---

## 💡 Pro Tips

1. **Always watch the terminal logs** - they show exactly what's happening
2. **Check database after each step** - confirms data was saved
3. **Test with small amounts first** - easier to debug
4. **Use valid bank codes** - invalid codes cause account resolution to fail
5. **Keep tokens fresh** - login again if token expired (15 min)

---

## 🆘 Still Stuck?

Share these details:

1. **Full error from terminal logs**
2. **Response from Postman**
3. **Result of this query:**
```powershell
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT * FROM wallet_transactions WHERE type = 'withdrawal' ORDER BY created_at DESC LIMIT 1;"
```

This will help identify the exact failure point!


# 🚀 Wallet System - Quick Testing Guide

**Start here to test the wallet system in 10 minutes!**

---

## ⚡ Quick Start (3 steps)

### 1. Start the System
```bash
# Terminal 1: Start database
docker compose up -d

# Terminal 2: Start backend
make run
```

**Expected output:**
```
2026/08/05 12:00:00 Loading configuration...
2026/08/05 12:00:00 Connecting to database...
2026/08/05 12:00:00 HTTP server listening :8080
```

### 2. Create a Test User
```bash
# Using curl
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "full_name": "Test User",
    "email": "test@example.com",
    "password": "SecurePass123!",
    "phone": "+2348012345678"
  }'
```

**Save the access_token from response!**

### 3. Test Wallet Endpoints

#### Check Wallet (should be ₦0)
```bash
curl http://localhost:8080/api/v1/wallet \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

---

## 📋 Complete Test Flow (Copy-Paste Ready)

### Step 1: Register
```http
POST http://localhost:8080/api/v1/auth/register
Content-Type: application/json

{
  "full_name": "John Doe",
  "email": "john@example.com",
  "password": "SecurePass123!",
  "phone": "+2348012345678"
}
```
✅ **Save `access_token` from response**

---

### Step 2: Get Wallet (Initial)
```http
GET http://localhost:8080/api/v1/wallet
Authorization: Bearer YOUR_ACCESS_TOKEN
```
✅ **Expected:** Balance = ₦0.00

---

### Step 3: Initiate Deposit
```http
POST http://localhost:8080/api/v1/wallet/deposit
Authorization: Bearer YOUR_ACCESS_TOKEN
Content-Type: application/json

{
  "amount_kobo": 150000
}
```
✅ **Save `reference` from response** (e.g., "DEP-abc123")

---

### Step 4: Simulate Webhook (Pretend to be Payment Provider)
```http
POST http://localhost:8080/api/v1/webhooks/payment
X-Mock-Signature: any_value
Content-Type: application/json

{
  "event": "charge.success",
  "data": {
    "reference": "YOUR_SAVED_REFERENCE",
    "amount": 150000,
    "status": "success"
  }
}
```
✅ **Expected:** 200 OK - "Webhook processed successfully"

---

### Step 5: Get Wallet (After Deposit)
```http
GET http://localhost:8080/api/v1/wallet
Authorization: Bearer YOUR_ACCESS_TOKEN
```
✅ **Expected:** Balance = ₦1,500.00

---

### Step 6: Get Transaction History
```http
GET http://localhost:8080/api/v1/wallet/transactions
Authorization: Bearer YOUR_ACCESS_TOKEN
```
✅ **Expected:** 1 completed deposit transaction

---

### Step 7: Request Withdrawal
```http
POST http://localhost:8080/api/v1/wallet/withdraw
Authorization: Bearer YOUR_ACCESS_TOKEN
Content-Type: application/json

{
  "amount_kobo": 50000,
  "account_number": "0123456789",
  "account_name": "John Doe",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```
✅ **Expected:** Transaction created, balance = ₦1,000.00

---

### Step 8: Get Wallet (After Withdrawal)
```http
GET http://localhost:8080/api/v1/wallet
Authorization: Bearer YOUR_ACCESS_TOKEN
```
✅ **Expected:** Balance = ₦1,000.00 (reduced by ₦500)

---

### Step 9: Get Transactions (All Types)
```http
GET http://localhost:8080/api/v1/wallet/transactions
Authorization: Bearer YOUR_ACCESS_TOKEN
```
✅ **Expected:** 2 transactions (1 deposit, 1 withdrawal)

---

### Step 10: Get Transactions (Deposits Only)
```http
GET http://localhost:8080/api/v1/wallet/transactions?type=deposit
Authorization: Bearer YOUR_ACCESS_TOKEN
```
✅ **Expected:** 1 deposit transaction

---

## 🎯 Expected Results Summary

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Register | User created, access_token received |
| 2 | Get Wallet | main_balance: 0, earnings_balance: 0 |
| 3 | Initiate Deposit | authorization_url + reference returned |
| 4 | Simulate Webhook | 200 OK, wallet credited |
| 5 | Get Wallet | main_balance: 150000 (₦1,500) |
| 6 | Get Transactions | 1 completed deposit |
| 7 | Request Withdrawal | Transaction created, status: pending |
| 8 | Get Wallet | main_balance: 100000 (₦1,000) |
| 9 | Get Transactions | 2 transactions total |
| 10 | Filter Deposits | 1 deposit transaction |

---

## ❌ Error Testing

### Test 1: Insufficient Balance
```http
POST http://localhost:8080/api/v1/wallet/withdraw
Authorization: Bearer YOUR_ACCESS_TOKEN
Content-Type: application/json

{
  "amount_kobo": 999999999,
  "account_number": "0123456789",
  "account_name": "John Doe",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```
✅ **Expected:** 422 - "Insufficient balance"

---

### Test 2: Invalid Amount
```http
POST http://localhost:8080/api/v1/wallet/deposit
Authorization: Bearer YOUR_ACCESS_TOKEN
Content-Type: application/json

{
  "amount_kobo": -100
}
```
✅ **Expected:** 400 - Validation error

---

### Test 3: Unauthorized Access
```http
GET http://localhost:8080/api/v1/wallet
```
✅ **Expected:** 401 - "Unauthorized"

---

### Test 4: Invalid Webhook Signature
```http
POST http://localhost:8080/api/v1/webhooks/payment
X-Mock-Signature: wrong_signature
Content-Type: application/json

{
  "data": {"reference": "DEP-123"}
}
```
✅ **Expected:** 400 - "Invalid signature"

---

## 🔧 Troubleshooting

### Problem: "Connection refused" error
**Solution:**
```bash
# Check if database is running
docker ps

# If not running, start it
docker compose up -d

# Check database logs
docker compose logs postgres
```

---

### Problem: "Unauthorized" error
**Solution:**
1. Check if you included the Authorization header
2. Verify token format: `Bearer YOUR_TOKEN` (with space after "Bearer")
3. Token expires in 15 minutes - login again if expired

---

### Problem: "Wallet not found"
**Solution:**
1. Wallet is created during registration
2. Make sure you registered successfully first
3. Check user_id matches the token

---

### Problem: Webhook doesn't credit wallet
**Solution:**
1. Check backend logs for errors
2. Verify reference matches the one from deposit
3. Check webhook signature (mock accepts any value)
4. Verify amount matches deposit amount

---

## 📊 Quick Health Check

Run these commands to verify system is working:

```bash
# 1. Check API is running
curl http://localhost:8080/health

# 2. Check database connection
docker exec -it propvest-postgres psql -U propvest -d propvest -c "SELECT COUNT(*) FROM users;"

# 3. Check migrations ran
docker exec -it propvest-postgres psql -U propvest -d propvest -c "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;"
```

**Expected:**
1. `{"status":"healthy"}`
2. Count of users
3. Latest version: 6

---

## 🎨 Postman Collection

Import these as separate requests in Postman:

### Request 1: Register
```
POST {{base_url}}/auth/register
Body: {see Step 1 above}
Tests: pm.environment.set("access_token", pm.response.json().data.access_token)
```

### Request 2: Get Wallet
```
GET {{base_url}}/wallet
Authorization: Bearer {{access_token}}
```

### Request 3: Initiate Deposit
```
POST {{base_url}}/wallet/deposit
Authorization: Bearer {{access_token}}
Body: {"amount_kobo": 150000}
Tests: pm.environment.set("deposit_reference", pm.response.json().data.reference)
```

### Request 4: Webhook
```
POST {{base_url}}/webhooks/payment
Headers: X-Mock-Signature: mock
Body: {
  "data": {
    "reference": "{{deposit_reference}}",
    "amount": 150000,
    "status": "success"
  }
}
```

### Request 5: Request Withdrawal
```
POST {{base_url}}/wallet/withdraw
Authorization: Bearer {{access_token}}
Body: {see Step 7 above}
```

### Request 6: Get Transactions
```
GET {{base_url}}/wallet/transactions
Authorization: Bearer {{access_token}}
```

---

## ⏱️ Time Estimates

- Setup (database + backend): **2 minutes**
- Register user: **30 seconds**
- Test deposit flow: **2 minutes**
- Test withdrawal flow: **1 minute**
- Test transactions: **1 minute**
- Test error scenarios: **2 minutes**

**Total: ~10 minutes**

---

## ✅ Success Checklist

- [ ] Backend starts without errors
- [ ] Can register new user
- [ ] Can get wallet (₦0 balance)
- [ ] Can initiate deposit
- [ ] Webhook credits wallet
- [ ] Wallet balance increases
- [ ] Can see deposit in transactions
- [ ] Can withdraw funds
- [ ] Wallet balance decreases
- [ ] Can see withdrawal in transactions
- [ ] Error handling works (insufficient balance, invalid amount)

**If all checked:** ✅ **Wallet system is working perfectly!**

---

## 🎉 Next Steps

Once testing is complete:
1. ✅ Mark Milestone 3 as tested
2. 🔜 Sign up for Paystack account
3. 🔜 Get test API keys
4. 🔜 Implement PaystackProvider
5. 🔜 Test real payment flow

---

**Need Help?**
- Check `MILESTONE_3_COMPLETE.md` for detailed documentation
- Check `postman/API_ENDPOINTS_REFERENCE.md` for full API reference
- Check backend logs for error details

**Happy Testing! 🚀**

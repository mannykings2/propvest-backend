# 📮 Postman Withdrawal Testing Guide

## Overview

This guide shows you how to test the complete withdrawal workflow using Postman, including:
1. Authentication (login)
2. Checking wallet balance
3. Initiating withdrawals
4. Monitoring transaction status
5. Testing webhooks
6. Simulating different scenarios

---

## Quick Setup

### Step 1: Start Your Services

**Terminal 1: API Server**
```powershell
.\api.exe
```

**Terminal 2: Ngrok**
```powershell
ngrok http 8081
```

**Terminal 3: Worker**
```powershell
.\worker.exe
```

### Step 2: Import Postman Collection

See `postman/PropVest_Withdrawal_Collection.json` (created below)

Or manually create requests following this guide.

---

## 🔐 Part 1: Authentication

### Request 1: Login

**Method:** `POST`  
**URL:** `http://localhost:8081/api/v1/auth/login`  
**Headers:**
```
Content-Type: application/json
```

**Body (raw JSON):**
```json
{
  "email": "testuser@example.com",
  "password": "Test1234!"
}
```

**Expected Response (200 OK):**
```json
{
  "success": true,
  "message": "Login successful",
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "token_type": "Bearer",
    "expires_in": 900,
    "user": {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "email": "testuser@example.com",
      "first_name": "Test",
      "last_name": "User"
    }
  }
}
```

**📋 IMPORTANT: Copy the `access_token` value!**

---

### Setup Authorization for All Requests

**In Postman:**

1. **Click on your collection** (left sidebar)
2. **Go to "Authorization" tab**
3. **Select Type:** `Bearer Token`
4. **Paste your access_token** in the Token field
5. **Click Save**

Now all requests in this collection will use this token automatically!

**OR for individual requests:**
1. Go to request's **Authorization** tab
2. Type: `Bearer Token`
3. Token: `paste_your_access_token_here`

---

## 💰 Part 2: Check Wallet Balance

### Request 2: Get Wallet

**Method:** `GET`  
**URL:** `http://localhost:8081/api/v1/wallet`  
**Headers:**
```
Authorization: Bearer YOUR_ACCESS_TOKEN
```

**Expected Response (200 OK):**
```json
{
  "success": true,
  "message": "Request successful",
  "data": {
    "id": "wallet-uuid",
    "user_id": "user-uuid",
    "main_balance": 10000000,
    "locked_balance": 0,
    "earnings_balance": 0,
    "currency": "NGN",
    "virtual_acct_no": null,
    "virtual_bank": null,
    "created_at": "2026-09-06T10:00:00Z",
    "updated_at": "2026-09-06T10:00:00Z"
  }
}
```

**Balance Breakdown:**
- `main_balance: 10000000` = ₦100,000 (in kobo)
- `locked_balance: 0` = ₦0 (nothing locked)
- **Available to withdraw:** ₦100,000

---

## 🏦 Part 3: Initiate Withdrawal

### Request 3: Withdraw Funds

**Method:** `POST`  
**URL:** `http://localhost:8081/api/v1/wallet/withdraw`  
**Headers:**
```
Content-Type: application/json
Authorization: Bearer YOUR_ACCESS_TOKEN
```

**Body (raw JSON):**
```json
{
  "amount_kobo": 5000000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Field Explanations:**
- `amount_kobo`: Amount in kobo (5000000 = ₦50,000)
- `account_number`: 10-digit Nigerian bank account
- `account_name`: Must match bank records (case-insensitive)
- `bank_code`: 3-digit bank code (see list below)
- `bank_name`: Bank name (for display only)

**Expected Response (200 OK):**
```json
{
  "success": true,
  "message": "Withdrawal initiated successfully",
  "data": {
    "id": "transaction-uuid",
    "type": "withdrawal",
    "amount": 5000000,
    "balance_before": 10000000,
    "balance_after": 10000000,
    "reference": "WD-ABC123XYZ",
    "description": "Withdrawal to GTBank (0123456789)",
    "status": "pending",
    "created_at": "2026-09-06T12:00:00Z"
  }
}
```

**📋 IMPORTANT: Copy the `reference` value (WD-ABC123XYZ)!**

---

### Nigerian Bank Codes Reference

Common banks you can use for testing:

```
044 - Access Bank
057 - Zenith Bank
058 - GTBank
011 - First Bank
033 - United Bank for Africa (UBA)
032 - Union Bank
214 - First City Monument Bank (FCMB)
221 - Stanbic IBTC Bank
082 - Keystone Bank
076 - Polaris Bank
```

---

## 📊 Part 4: Check Updated Balance

### Request 4: Get Wallet (After Withdrawal)

**Method:** `GET`  
**URL:** `http://localhost:8081/api/v1/wallet`  
**Headers:**
```
Authorization: Bearer YOUR_ACCESS_TOKEN
```

**Expected Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "main_balance": 10000000,
    "locked_balance": 5000000,
    "earnings_balance": 0,
    "currency": "NGN"
  }
}
```

**What Changed:**
- `main_balance: 10000000` = ₦100,000 (UNCHANGED) ✓
- `locked_balance: 5000000` = ₦50,000 (LOCKED) ⭐
- **Available to withdraw:** ₦50,000 (100k - 50k locked)

**This proves the locked balance pattern is working!**

---

## 📜 Part 5: Check Transaction History

### Request 5: Get Transactions

**Method:** `GET`  
**URL:** `http://localhost:8081/api/v1/wallet/transactions`  
**Headers:**
```
Authorization: Bearer YOUR_ACCESS_TOKEN
```

**Query Parameters (Optional):**
```
type: withdrawal
status: pending
page: 1
limit: 20
```

**In Postman:**
1. Click "Params" tab
2. Add key-value pairs:
   - `type` = `withdrawal`
   - `status` = `pending`
   - `page` = `1`
   - `limit` = `20`

**Expected Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "transactions": [
      {
        "id": "transaction-uuid",
        "type": "withdrawal",
        "amount": 5000000,
        "balance_before": 10000000,
        "balance_after": 10000000,
        "reference": "WD-ABC123XYZ",
        "description": "Withdrawal to GTBank (0123456789)",
        "status": "pending",
        "created_at": "2026-09-06T12:00:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "limit": 20,
    "pages": 1
  }
}
```

**Transaction Status Values:**
- `pending` - Waiting for processing
- `completed` - Successfully sent to bank
- `failed` - Transfer failed, funds returned

---

## 🔄 Part 6: Wait for Processing

### What Happens Next:

1. **Worker picks up the job** (if RabbitMQ is running)
2. **Calls Paystack API** to initiate transfer
3. **Paystack processes** (1-5 minutes)
4. **Webhook arrives** at your ngrok URL
5. **Wallet is finalized** (debited or reversed)

### Monitor Progress:

**Check logs in your terminals:**
- Terminal 1 (API): Watch for webhook
- Terminal 3 (Worker): Watch for processing

**Or keep checking transaction status:**

**Request 6: Check Transaction Status**  
Repeat Request 5 every 30 seconds until status changes.

---

## ✅ Part 7: Verify Final State

### Request 7: Get Wallet (After Completion)

**Method:** `GET`  
**URL:** `http://localhost:8081/api/v1/wallet`  
**Headers:**
```
Authorization: Bearer YOUR_ACCESS_TOKEN
```

**Expected Response (200 OK) - After Success:**
```json
{
  "success": true,
  "data": {
    "main_balance": 5000000,
    "locked_balance": 0,
    "earnings_balance": 0
  }
}
```

**What Changed:**
- `main_balance: 5000000` = ₦50,000 (DEBITED by 50k) ✓
- `locked_balance: 0` = ₦0 (CLEARED) ✓
- **Available:** ₦50,000

**Perfect! The withdrawal completed successfully! 🎉**

---

### Request 8: Check Final Transaction

**Method:** `GET`  
**URL:** `http://localhost:8081/api/v1/wallet/transactions`

**Expected Response:**
```json
{
  "transactions": [
    {
      "id": "transaction-uuid",
      "type": "withdrawal",
      "amount": 5000000,
      "reference": "WD-ABC123XYZ",
      "status": "completed",
      "created_at": "2026-09-06T12:00:00Z"
    }
  ]
}
```

**Status changed:** `pending` → `completed` ✓

---

## 🧪 Part 8: Test Failure Scenarios

### Test 1: Insufficient Balance

**Request:** POST Withdraw  
**Body:**
```json
{
  "amount_kobo": 99999999,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Expected Response (422):**
```json
{
  "success": false,
  "error": "insufficient balance: requested ₦999,999.99 but only ₦50,000.00 available",
  "code": "insufficient_funds"
}
```

---

### Test 2: Below Minimum Amount

**Body:**
```json
{
  "amount_kobo": 10000,
  "account_number": "0123456789",
  "account_name": "Test User",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Expected Response (422):**
```json
{
  "success": false,
  "error": "amount is below minimum withdrawal of ₦500.00",
  "code": "invalid_amount"
}
```

---

### Test 3: Invalid Bank Account

**Body:**
```json
{
  "amount_kobo": 5000000,
  "account_number": "0000000000",
  "account_name": "Invalid",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Expected Response (400):**
```json
{
  "success": false,
  "error": "Invalid bank account details",
  "code": "invalid_bank_account"
}
```

---

### Test 4: Account Name Mismatch

**Body:**
```json
{
  "amount_kobo": 5000000,
  "account_number": "0123456789",
  "account_name": "Wrong Name",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Expected Response (400):**
```json
{
  "success": false,
  "error": "account name mismatch: you entered 'Wrong Name' but bank records show 'Test User'",
  "code": "account_name_mismatch"
}
```

---

## 🪝 Part 9: Test Webhook Manually

If webhook doesn't arrive automatically, you can trigger it manually for testing.

### Request 9: Simulate Transfer Webhook

**Method:** `POST`  
**URL:** `http://localhost:8081/api/v1/webhooks/paystack/transfer`  
**Headers:**
```
Content-Type: application/json
X-Paystack-Signature: COMPUTED_SIGNATURE
```

**Body (raw JSON):**
```json
{
  "event": "transfer.success",
  "data": {
    "reference": "WD-ABC123XYZ",
    "transfer_code": "TRF_test123",
    "status": "success",
    "amount": 5000000,
    "currency": "NGN",
    "recipient": {
      "type": "nuban",
      "account_number": "0123456789",
      "bank_code": "058",
      "bank_name": "GTBank Plc"
    },
    "createdAt": "2026-09-06T12:00:00Z"
  }
}
```

**Computing the Signature:**

You need to compute HMAC-SHA512 signature. Use the tool:

```powershell
cd tools
go run generate_webhook_signature.go
```

Or use the trigger tool:
```powershell
go run trigger_transfer_webhook.go --reference WD-ABC123XYZ --event success
```

This will make the request for you with correct signature!

---

## 📦 Postman Collection Setup

### Create a New Collection

1. **Open Postman**
2. **Click "New" → "Collection"**
3. **Name it:** "PropVest Withdrawal Testing"
4. **Add Description:**
   ```
   Complete withdrawal workflow testing:
   - Authentication
   - Wallet operations
   - Withdrawal initiation
   - Transaction monitoring
   - Webhook testing
   ```

### Add Variables

1. **Click on your collection**
2. **Go to "Variables" tab**
3. **Add these variables:**

| Variable | Initial Value | Current Value |
|----------|---------------|---------------|
| `base_url` | `http://localhost:8081` | `http://localhost:8081` |
| `access_token` | `` | `paste_after_login` |
| `withdrawal_ref` | `` | `paste_after_withdrawal` |

### Use Variables in Requests

Replace hardcoded values:
- URL: `{{base_url}}/api/v1/wallet`
- Authorization: `Bearer {{access_token}}`

### Save Access Token Automatically

In the **Login request**, add this to the **Tests** tab:

```javascript
// Save access token to collection variable
pm.test("Login successful", function() {
    var jsonData = pm.response.json();
    pm.expect(jsonData.success).to.eql(true);
    pm.collectionVariables.set("access_token", jsonData.data.access_token);
});
```

Now the token is saved automatically after login!

---

## 🎯 Complete Test Workflow in Postman

### Organized Folders

Create these folders in your collection:

```
📁 PropVest Withdrawal Testing
  📁 1. Authentication
     - Login
     - Refresh Token
  📁 2. Wallet Operations
     - Get Wallet
     - Get Transactions
  📁 3. Withdrawal Flow
     - Initiate Withdrawal
     - Check Status (Pending)
     - Check Status (Completed)
  📁 4. Error Scenarios
     - Insufficient Balance
     - Below Minimum
     - Invalid Account
     - Name Mismatch
  📁 5. Webhooks
     - Simulate Success
     - Simulate Failure
```

### Test Scripts

Add these to your requests' **Tests** tab:

**Get Wallet:**
```javascript
pm.test("Status is 200", function() {
    pm.response.to.have.status(200);
});

pm.test("Wallet has main_balance", function() {
    var jsonData = pm.response.json();
    pm.expect(jsonData.data).to.have.property('main_balance');
    pm.expect(jsonData.data).to.have.property('locked_balance');
});

// Display balance in console
var data = pm.response.json().data;
console.log(`Main Balance: ₦${data.main_balance/100}`);
console.log(`Locked: ₦${data.locked_balance/100}`);
console.log(`Available: ₦${(data.main_balance - data.locked_balance)/100}`);
```

**Initiate Withdrawal:**
```javascript
pm.test("Withdrawal initiated", function() {
    pm.response.to.have.status(200);
    var jsonData = pm.response.json();
    pm.expect(jsonData.data.status).to.eql("pending");
    
    // Save reference for later
    pm.collectionVariables.set("withdrawal_ref", jsonData.data.reference);
    console.log("Withdrawal Reference:", jsonData.data.reference);
});
```

---

## 💡 Pro Tips

### 1. Use Environment for Different Setups

Create environments:
- **Local Development** (localhost:8081)
- **Ngrok Testing** (your ngrok URL)
- **Staging** (staging server URL)

### 2. Chain Requests with Tests

Use test scripts to automatically run next request:

```javascript
// After login succeeds, automatically get wallet
pm.test("Login successful", function() {
    pm.response.to.have.status(200);
    postman.setNextRequest("Get Wallet");
});
```

### 3. Pre-request Scripts

Add delays or computations:

```javascript
// Wait 5 seconds before checking status
setTimeout(function(){}, 5000);
```

### 4. Console Logging

Add to Tests tab:
```javascript
console.log("Response:", pm.response.json());
console.log("Balance:", pm.response.json().data.main_balance);
```

View logs: **View → Show Postman Console** (Alt+Ctrl+C)

### 5. Response Visualization

Add to Tests tab:
```javascript
var template = `
<h2>Wallet Balance</h2>
<table>
  <tr><td>Main:</td><td>₦{{main_balance}}</td></tr>
  <tr><td>Locked:</td><td>₦{{locked_balance}}</td></tr>
  <tr><td>Available:</td><td>₦{{available}}</td></tr>
</table>
`;

var data = pm.response.json().data;
data.available = data.main_balance - data.locked_balance;

pm.visualizer.set(template, data);
```

Click **Visualize** tab to see pretty output!

---

## 🔍 Debugging in Postman

### View Request Details

1. Click on a request in history (left sidebar)
2. See full request/response
3. Check headers, body, status code

### Common Issues

**401 Unauthorized:**
- Token expired → Login again
- Token not set → Check Authorization tab
- Wrong token → Copy from login response

**400 Bad Request:**
- Check request body JSON is valid
- Verify all required fields present
- Check data types (numbers vs strings)

**422 Unprocessable Entity:**
- Business logic error (insufficient funds, etc.)
- Read error message in response

**500 Internal Server Error:**
- Check API logs (Terminal 1)
- Database connection issue
- Bug in code

---

## ✅ Complete Test Checklist

Use this to verify everything works:

- [ ] Login successfully
- [ ] Get wallet balance
- [ ] See ₦100,000 available
- [ ] Initiate withdrawal (₦50,000)
- [ ] See `locked_balance` = ₦50,000
- [ ] See `available` = ₦50,000
- [ ] Wait 1-5 minutes
- [ ] Check transaction status changed to "completed"
- [ ] See `main_balance` = ₦50,000
- [ ] See `locked_balance` = ₦0
- [ ] Test insufficient balance error
- [ ] Test below minimum error
- [ ] Test invalid account error

---

## 📥 Import Ready-Made Collection

I'll create a Postman collection file next that you can import directly!

See: `postman/PropVest_Withdrawal_Collection.json`

**To Import:**
1. Open Postman
2. Click **Import** (top left)
3. Drag and drop the JSON file
4. Click **Import**
5. Done! All requests ready to use

---

**Ready to test? Open Postman and start with the Login request! 🚀**

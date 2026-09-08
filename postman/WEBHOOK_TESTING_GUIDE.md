# 🔐 Webhook Testing Guide - Complete Setup

## The Problem with Manual Signatures

When you manually copy/paste the signature, even tiny differences in JSON formatting (spaces, line breaks) will cause signature validation to fail. The signature MUST match the exact bytes being sent.

## ✅ Solution: Use Postman Pre-request Script

This automatically generates the correct signature for whatever JSON you send.

---

## 🚀 Quick Setup (Recommended Method)

### Step 1: Create the Request

**Method:** POST  
**URL:** `http://localhost:8081/api/v1/webhooks/payment`

### Step 2: Add the Body

Go to **Body** tab:
- Select **raw**
- Select **JSON**

Paste this:
```json
{
  "event": "charge.success",
  "data": {
    "reference": "DEP-123456",
    "amount": 1500000,
    "status": "success",
    "paid_at": "2026-08-10T05:08:06.000Z",
    "channel": "card",
    "currency": "NGN",
    "customer": {
      "email": "user@example.com"
    }
  }
}
```

### Step 3: Add Pre-request Script

Go to **Pre-request Script** tab and paste this entire script:

```javascript
// Webhook Signature Generator for Mock Provider
// This generates the X-Paystack-Signature header automatically

const secretKey = 'mock-secret-key'; // Must match server's mock provider key

// Get the request body
const requestBody = pm.request.body.raw;

if (!requestBody) {
    console.error('No request body found');
    return;
}

// Generate HMAC SHA-512 signature
const signature = CryptoJS.HmacSHA512(requestBody, secretKey).toString(CryptoJS.enc.Hex);

// Set the signature as a header
pm.request.headers.add({
    key: 'X-Paystack-Signature',
    value: signature
});

console.log('Generated signature:', signature);
console.log('For body:', requestBody);
```

### Step 4: Add Content-Type Header

Go to **Headers** tab and add:

| Key | Value |
|-----|-------|
| `Content-Type` | `application/json` |

**Note:** Don't manually add `X-Paystack-Signature` - the script does it automatically!

### Step 5: Send the Request

Click **Send**

Expected response:
```json
{
  "success": true,
  "message": "Webhook processed successfully"
}
```

---

## 🧪 Testing Different Scenarios

### Test 1: Successful Payment
```json
{
  "event": "charge.success",
  "data": {
    "reference": "DEP-TEST001",
    "amount": 1500000,
    "status": "success",
    "paid_at": "2026-08-10T05:08:06.000Z",
    "channel": "card",
    "currency": "NGN",
    "customer": {
      "email": "testuser@example.com"
    }
  }
}
```

### Test 2: Failed Payment
```json
{
  "event": "charge.failed",
  "data": {
    "reference": "DEP-TEST002",
    "amount": 1500000,
    "status": "failed",
    "paid_at": "2026-08-10T05:08:06.000Z",
    "channel": "card",
    "currency": "NGN",
    "customer": {
      "email": "testuser@example.com"
    }
  }
}
```

### Test 3: Different Amount
```json
{
  "event": "charge.success",
  "data": {
    "reference": "DEP-TEST003",
    "amount": 5000000,
    "status": "success",
    "paid_at": "2026-08-10T05:08:06.000Z",
    "channel": "card",
    "currency": "NGN",
    "customer": {
      "email": "bigspender@example.com"
    }
  }
}
```

---

## 🔧 Alternative: Manual Testing (If Script Doesn't Work)

If the Pre-request Script doesn't work in your Postman version, use the command-line tool:

### Step 1: Modify the Generator Tool

Edit `tools/generate_webhook_signature.go` and change the payload to match what you want to test.

### Step 2: Run the Tool
```bash
go run tools/generate_webhook_signature.go
```

### Step 3: Copy Output to Postman

**CRITICAL:** The body in Postman must be EXACTLY the same as what the tool generated, including:
- Same spacing
- Same line breaks
- Same field order
- No extra characters

---

## 🐛 Troubleshooting

### Error: "Invalid signature"

**Cause:** The signature doesn't match the body being sent.

**Solution:**
1. Make sure the Pre-request Script is in place
2. Check Console tab (View → Show Postman Console) to see generated signature
3. Verify body is valid JSON
4. Try without any extra spaces or formatting

### Error: "No request body"

**Cause:** Body is not set or wrong type

**Solution:**
1. Go to Body tab
2. Select "raw"
3. Select "JSON" from dropdown
4. Paste valid JSON

### Error: 404 Not Found

**Cause:** Wrong URL

**Solution:** Use exact URL: `http://localhost:8081/api/v1/webhooks/payment`

---

## 📋 Complete Testing Workflow

### 1. First, Create a Deposit

```
POST http://localhost:8081/api/v1/wallet/deposit
Authorization: Bearer <your_token>

Body:
{
  "amount": 1500000
}

Response: Copy the "reference" value (e.g., "DEP-abc123")
```

### 2. Test the Webhook with That Reference

```
POST http://localhost:8081/api/v1/webhooks/payment

Body:
{
  "event": "charge.success",
  "data": {
    "reference": "DEP-abc123",  ← Use actual reference from Step 1
    "amount": 1500000,
    "status": "success",
    "paid_at": "2026-08-10T05:08:06.000Z",
    "channel": "card",
    "currency": "NGN",
    "customer": {
      "email": "user@example.com"
    }
  }
}
```

### 3. Verify Wallet Was Credited

```
GET http://localhost:8081/api/v1/wallet
Authorization: Bearer <your_token>

Check that main_balance increased by the amount
```

---

## 🎯 Expected Behavior

### When Webhook Succeeds:
1. ✅ Returns 200 OK
2. ✅ Wallet balance increases
3. ✅ Payment status changes to "completed"
4. ✅ Transaction record created

### When Webhook Fails:
1. ❌ Returns error (400/401/500)
2. ❌ Wallet balance unchanged
3. ❌ Payment status remains "pending"

---

## 📖 Understanding Webhook Signatures

### Why Signatures Are Important

Without signature verification, anyone could send fake webhooks:

```
Attacker sends fake webhook:
POST /webhooks/payment
{
  "reference": "VICTIM-123",
  "status": "success",
  "amount": 10000000000
}

→ Without signature check: Victim gets ₦100M credit!
→ With signature check: Rejected (attacker doesn't have secret key)
```

### How Signatures Work

1. **Paystack has secret key** (only they and you know it)
2. **Paystack computes:** `HMAC-SHA512(secret_key, request_body)`
3. **Paystack sends:** Request body + signature header
4. **Your server computes:** `HMAC-SHA512(secret_key, request_body)`
5. **Your server compares:** If signatures match → authentic, else reject

### Why Exact Bytes Matter

```
Body 1: {"amount":1000}
Body 2: {"amount": 1000}  ← Extra space!

Same data, but different bytes → different signatures!
```

This is why auto-generation with Pre-request Script is crucial.

---

## 🔒 Security Notes

### Development (Mock Provider)
- Secret key: `mock-secret-key` (hardcoded)
- Signature verification: ✅ Enabled
- Real charges: ❌ No

### Production (Real Paystack)
- Secret key: From Paystack dashboard (env variable)
- Signature verification: ✅ MUST be enabled
- Real charges: ✅ Yes
- NEVER disable signature verification in production!

---

## 💡 Pro Tips

### Tip 1: Save Requests in Collection
Create a Postman Collection with:
- Deposit request
- Webhook success
- Webhook failure
- Check wallet balance

### Tip 2: Use Environment Variables
```
{{base_url}} = http://localhost:8081
{{secret_key}} = mock-secret-key
{{access_token}} = <your token>
```

### Tip 3: Check Server Logs
Watch the server console for detailed webhook processing logs:
```
time=... level=INFO msg="webhook received" reference=DEP-123
time=... level=INFO msg="wallet credited" user_id=... amount=1500000
```

### Tip 4: Test Edge Cases
- Duplicate webhooks (same reference twice)
- Wrong reference (non-existent)
- Mismatched amount
- Invalid status values
- Missing required fields

---

## 📞 Still Having Issues?

1. **Check Postman Console** (View → Show Postman Console)
   - Shows generated signature
   - Shows request body
   - Shows errors

2. **Check Server Logs**
   - Shows signature verification details
   - Shows webhook processing errors

3. **Verify Pre-request Script**
   - Script tab has the code
   - CryptoJS is available (built into Postman)
   - No JavaScript errors in console

4. **Try Minimal Example**
   ```json
   {"event":"test","data":{"reference":"TEST"}}
   ```
   If this works, gradually add fields back

---

**Last Updated:** 2026-08-10  
**Mock Secret Key:** `mock-secret-key`  
**Endpoint:** POST /api/v1/webhooks/payment

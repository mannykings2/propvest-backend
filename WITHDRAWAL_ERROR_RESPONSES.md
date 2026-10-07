# Withdrawal Error Responses

This document describes all possible error responses for the withdrawal endpoint, with HTTP status codes, error codes, and example responses.

---

## Endpoint

```
POST /api/v1/wallet/withdraw
```

### Headers
```
Authorization: Bearer <access_token>
Content-Type: application/json
```

### Request Body
```json
{
  "amount_kobo": 50000,
  "account_number": "0123456789",
  "account_name": "John Doe",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

---

## Success Response

### 200 OK
**When:** Withdrawal initiated successfully

```json
{
  "success": true,
  "message": "Withdrawal initiated successfully",
  "data": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "reference": "WD-123e4567",
    "type": "withdrawal",
    "amount": 50000,
    "amount_formatted": "₦500.00",
    "status": "pending",
    "balance_before": 150000,
    "balance_after": 100000,
    "description": "Withdrawal to GTBank (0123456789)",
    "created_at": "2026-09-09T12:00:00Z"
  },
  "request_id": "req-abc123"
}
```

**What happens next:**
1. Funds are locked in your wallet
2. Worker processes the bank transfer
3. You'll receive a notification when complete (1-5 minutes typically)
4. Transaction status updates to "completed" or "failed"

---

## Error Responses

### 1. Missing or Invalid Authentication

#### 401 Unauthorized
**When:** Access token is missing, expired, or invalid

```json
{
  "success": false,
  "message": "Unauthorized",
  "code": "unauthorized",
  "request_id": "req-abc123"
}
```

**How to fix:**
- Ensure `Authorization: Bearer <token>` header is present
- Check if access token has expired (call `/auth/refresh` to get new token)
- Verify token is valid (re-login if necessary)

---

### 2. Validation Errors

#### 400 Bad Request - Invalid JSON
**When:** Request body is not valid JSON

```json
{
  "success": false,
  "message": "Invalid request format",
  "code": "bad_request",
  "request_id": "req-abc123"
}
```

#### 422 Unprocessable Entity - Missing Required Fields
**When:** Required fields are missing or have invalid format

```json
{
  "success": false,
  "message": "Validation failed",
  "code": "validation_error",
  "errors": {
    "amount_kobo": ["This field is required"],
    "account_number": ["This field is required"],
    "bank_code": ["This field is required"]
  },
  "request_id": "req-abc123"
}
```

**How to fix:**
- Ensure all required fields are present
- Check field formats match the API specification

---

### 3. Insufficient Balance

#### 422 Unprocessable Entity
**When:** Wallet balance is less than withdrawal amount

```json
{
  "success": false,
  "message": "Insufficient wallet balance",
  "code": "insufficient_funds",
  "request_id": "req-abc123"
}
```

**Example scenario:**
```
Wallet balance: ₦400.00 (40,000 kobo)
Withdrawal request: ₦500.00 (50,000 kobo)
Available: ₦400.00 < ₦500.00 ❌
```

**How to fix:**
- Check wallet balance: `GET /api/v1/wallet`
- Reduce withdrawal amount
- Deposit more funds first

---

### 4. Amount Below Minimum

#### 422 Unprocessable Entity
**When:** Withdrawal amount is below minimum threshold (₦500 / 50,000 kobo)

```json
{
  "success": false,
  "message": "Minimum withdrawal amount is ₦500.00",
  "code": "minimum_withdrawal",
  "request_id": "req-abc123"
}
```

**Example scenario:**
```
Withdrawal request: ₦200.00 (20,000 kobo)
Minimum allowed: ₦500.00 (50,000 kobo)
₦200.00 < ₦500.00 ❌
```

**How to fix:**
- Increase withdrawal amount to at least ₦500.00 (50,000 kobo)

---

### 5. Amount Above Maximum

#### 422 Unprocessable Entity
**When:** Withdrawal amount exceeds maximum threshold (₦100,000 / 10,000,000 kobo)

```json
{
  "success": false,
  "message": "Maximum withdrawal amount is ₦100,000.00",
  "code": "maximum_withdrawal",
  "request_id": "req-abc123"
}
```

**Example scenario:**
```
Withdrawal request: ₦150,000.00 (15,000,000 kobo)
Maximum allowed: ₦100,000.00 (10,000,000 kobo)
₦150,000.00 > ₦100,000.00 ❌
```

**How to fix:**
- Split into multiple withdrawals (e.g., ₦100k today, ₦50k tomorrow)
- Reduce withdrawal amount to ₦100,000 or less

---

### 6. Invalid Bank Account

#### 422 Unprocessable Entity
**When:** Bank account number is invalid or doesn't exist

```json
{
  "success": false,
  "message": "Invalid bank account number",
  "code": "invalid_bank_account",
  "request_id": "req-abc123"
}
```

**Possible causes:**
- Account number doesn't exist with specified bank
- Account number has wrong format (not 10 digits for Nigerian banks)
- Bank code is incorrect
- Account is closed or suspended

**How to fix:**
- Verify account number with your bank
- Ensure bank code matches the account's bank
- Try a different bank account

---

### 7. Account Name Mismatch

#### 422 Unprocessable Entity
**When:** Provided account name doesn't match bank's records (fuzzy match threshold)

```json
{
  "success": false,
  "message": "Account name does not match bank records. Expected: 'JOHN DOE', Got: 'Jane Smith'",
  "code": "account_name_mismatch",
  "request_id": "req-abc123"
}
```

**Example scenarios:**

**Acceptable matches (pass):**
```
Bank: "JOHN DOE"          → Input: "John Doe"           ✓ (case insensitive)
Bank: "JOHN WILLIAMS DOE" → Input: "John W. Doe"        ✓ (abbreviated middle name)
Bank: "JOHN  DOE"         → Input: "John Doe"           ✓ (extra spaces)
```

**Unacceptable matches (fail):**
```
Bank: "JOHN DOE"          → Input: "Jane Smith"         ❌ (completely different)
Bank: "JOHN DOE"          → Input: "John Doey"          ❌ (typo)
Bank: "JOHN WILLIAMS DOE" → Input: "John Smith Doe"     ❌ (wrong middle name)
```

**How to fix:**
- Copy account name exactly as it appears on your bank statement
- Don't include special characters (Mr., Dr., etc.)
- Check for typos in first/last name
- Use the name format registered with your bank

---

### 8. Pending Withdrawal Already Exists

#### 422 Unprocessable Entity
**When:** User already has a pending withdrawal in progress

```json
{
  "success": false,
  "message": "You have a pending withdrawal. Please wait for it to complete.",
  "code": "withdrawal_pending",
  "request_id": "req-abc123"
}
```

**Why this restriction?**
- Prevents double withdrawal of same funds
- Ensures safe fund locking
- Avoids overdraft scenarios

**How to fix:**
- Wait for current withdrawal to complete (usually 1-5 minutes)
- Check transaction status: `GET /api/v1/wallet/transactions`
- If stuck > 10 minutes, contact support

---

### 9. Bank Account Resolution Failed

#### 500 Internal Server Error
**When:** Unable to verify account details with bank (temporary provider issue)

```json
{
  "success": false,
  "message": "Could not verify bank account details. Please try again.",
  "code": "transfer_failed",
  "request_id": "req-abc123"
}
```

**Possible causes:**
- Bank's API is temporarily down
- Network timeout
- Rate limit reached with bank's API
- Bank is undergoing maintenance

**How to fix:**
- Wait 1-2 minutes and retry
- Try again during banking hours (if outside hours)
- If persists > 5 minutes, contact support

---

### 10. Payment Provider Unavailable

#### 503 Service Unavailable
**When:** Payment provider (Paystack) is down or unreachable

```json
{
  "success": false,
  "message": "Payment provider is currently unavailable",
  "code": "provider_unavailable",
  "request_id": "req-abc123"
}
```

**Possible causes:**
- Paystack is experiencing downtime
- Network issues between our servers and Paystack
- Paystack rate limits exceeded
- Scheduled maintenance

**How to fix:**
- Check Paystack status: https://status.paystack.com
- Wait 5-10 minutes and retry
- If urgent, contact support for alternative withdrawal method

---

### 11. Transfer Initiation Failed

#### 500 Internal Server Error
**When:** Transfer request was sent but provider returned an error

```json
{
  "success": false,
  "message": "Withdrawal failed. Please try again.",
  "code": "withdrawal_failed",
  "request_id": "req-abc123"
}
```

**Possible causes:**
- Provider rejected the transfer (internal rules)
- Account flagged for security review
- Insufficient provider balance (our side - rare)
- Daily transfer limit reached (provider side)

**How to fix:**
- Retry after 5 minutes
- Try smaller amount
- If persists, contact support with request_id

---

### 12. Database Error

#### 500 Internal Server Error
**When:** Database query failed (connection error, timeout, etc.)

```json
{
  "success": false,
  "message": "An internal error occurred. Please try again later.",
  "code": "internal_error",
  "request_id": "req-abc123"
}
```

**Note:** Error details are not exposed for security. Request ID is logged for investigation.

**How to fix:**
- Retry after 30 seconds
- Check if wallet balance was debited: `GET /api/v1/wallet`
- If debited but status unknown, contact support immediately with request_id

---

## Error Handling Best Practices

### 1. Client-Side Error Handling

```javascript
try {
  const response = await fetch('/api/v1/wallet/withdraw', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${accessToken}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      amount_kobo: 50000,
      account_number: '0123456789',
      account_name: 'John Doe',
      bank_code: '058'
    })
  });

  const data = await response.json();

  if (!data.success) {
    // Handle error based on code
    switch (data.code) {
      case 'insufficient_funds':
        showMessage('Insufficient balance. Please deposit more funds.');
        redirectToDeposit();
        break;
      
      case 'minimum_withdrawal':
        showMessage('Minimum withdrawal is ₦500.00');
        break;
      
      case 'maximum_withdrawal':
        showMessage('Maximum withdrawal is ₦100,000.00. Please split into multiple withdrawals.');
        break;
      
      case 'account_name_mismatch':
        showMessage(`Account name mismatch: ${data.message}`);
        highlightField('account_name');
        break;
      
      case 'withdrawal_pending':
        showMessage('You have a pending withdrawal. Please wait for it to complete.');
        redirectToTransactions();
        break;
      
      case 'provider_unavailable':
        showMessage('Service temporarily unavailable. Please try again in a few minutes.');
        break;
      
      default:
        showMessage(data.message || 'An error occurred. Please try again.');
    }
    return;
  }

  // Success
  showSuccess(`Withdrawal of ₦${formatAmount(data.data.amount)} initiated!`);
  redirectToTransactions();

} catch (error) {
  // Network error
  showMessage('Network error. Please check your connection.');
}
```

### 2. Retry Strategy

**Retry these errors (transient):**
- `500` - Internal server error (after 30 seconds)
- `503` - Service unavailable (after 1 minute)
- `transfer_failed` - Transfer initiation failed (after 1 minute)

**Don't retry these errors (permanent):**
- `422` - Validation/business rule violations
- `401` - Unauthorized (need to re-authenticate)
- `insufficient_funds` - Need to deposit first
- `account_name_mismatch` - Need to fix input

### 3. User Feedback

**Good error messages:**
```
❌ "Minimum withdrawal is ₦500.00. You entered ₦200.00"
❌ "Insufficient balance (₦400.00). You need ₦500.00"
❌ "Account name doesn't match. Bank shows: JOHN DOE"
```

**Bad error messages:**
```
❌ "Error 422"
❌ "Validation failed"
❌ "Invalid request"
```

---

## Testing Error Scenarios

### Using Mock Provider

The mock provider supports special test account numbers:

```bash
# 1. Test minimum withdrawal error
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 10000,
    "account_number": "0123456789",
    "account_name": "John Doe",
    "bank_code": "058"
  }'
# Expected: 422 - "Minimum withdrawal amount is ₦500.00"

# 2. Test insufficient funds
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 999999999,
    "account_number": "0123456789",
    "account_name": "John Doe",
    "bank_code": "058"
  }'
# Expected: 422 - "Insufficient wallet balance"

# 3. Test invalid bank account
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "ERROR-1234",
    "account_name": "Test User",
    "bank_code": "058"
  }'
# Expected: 422 - "Invalid bank account number"

# 4. Test account name mismatch
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Wrong Name",
    "bank_code": "058"
  }'
# Expected: 422 - "Account name does not match bank records"

# 5. Test pending withdrawal (call twice rapidly)
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "PENDING-123",
    "account_name": "John Doe",
    "bank_code": "058"
  }'
# First call: 200 - Success
# Second call (within 10 min): 422 - "You have a pending withdrawal"
```

---

## Error Code Reference

Quick lookup table:

| HTTP Status | Error Code | Meaning | Action |
|------------|------------|---------|--------|
| 401 | `unauthorized` | Token invalid/expired | Re-authenticate |
| 400 | `bad_request` | Invalid JSON | Fix request format |
| 422 | `validation_error` | Missing/invalid fields | Check required fields |
| 422 | `insufficient_funds` | Not enough balance | Deposit more funds |
| 422 | `minimum_withdrawal` | Amount too small | Increase to ≥₦500 |
| 422 | `maximum_withdrawal` | Amount too large | Reduce to ≤₦100k |
| 422 | `invalid_bank_account` | Account doesn't exist | Verify account details |
| 422 | `account_name_mismatch` | Name doesn't match | Use exact bank name |
| 422 | `withdrawal_pending` | Already have pending | Wait for completion |
| 500 | `withdrawal_failed` | Transfer failed | Retry after 1 minute |
| 500 | `transfer_failed` | Provider error | Retry after 1 minute |
| 503 | `provider_unavailable` | Provider is down | Wait 5-10 minutes |
| 500 | `internal_error` | Server error | Retry after 30 seconds |

---

## Support

If you encounter an error not covered here:
1. Note the `request_id` from the error response
2. Check transaction status: `GET /api/v1/wallet/transactions`
3. Contact support with request_id and timestamp

**Support channels:**
- Email: support@propvest.com
- In-app: Settings → Help & Support
- Emergency (stuck funds): +234-XXX-XXX-XXXX

# Step 8: Handler Updates Summary

## What Was Done

### 1. Updated Error HTTP Status Code Mappings
**File:** `internal/errors/handler.go`

Added new withdrawal errors to HTTP status code mapping:

#### HTTP 422 (Unprocessable Entity) - Business Rule Violations
- `ErrInvalidBankAccount` - Invalid bank account number
- `ErrAccountNameMismatch` - Account name doesn't match bank records
- `ErrWithdrawalPending` - Already have a pending withdrawal

#### HTTP 500 (Internal Server Error) - Server Errors
- `ErrWithdrawalFailed` - Transfer initiation failed
- `ErrTransferFailed` - Transfer processing failed

#### HTTP 503 (Service Unavailable) - Provider Down
- `ErrPaymentProviderUnavailable` - Paystack/provider is unreachable

---

### 2. Added Machine-Readable Error Codes
**File:** `internal/errors/handler.go` - `ErrorCode()` function

New error codes for client-side error handling:

| Error | Code | HTTP Status |
|-------|------|-------------|
| ErrMinimumWithdrawal | `minimum_withdrawal` | 422 |
| ErrMaximumWithdrawal | `maximum_withdrawal` | 422 |
| ErrInvalidBankAccount | `invalid_bank_account` | 422 |
| ErrAccountNameMismatch | `account_name_mismatch` | 422 |
| ErrWithdrawalPending | `withdrawal_pending` | 422 |
| ErrWithdrawalFailed | `withdrawal_failed` | 500 |
| ErrPaymentProviderUnavailable | `provider_unavailable` | 503 |
| ErrTransferFailed | `transfer_failed` | 500 |

**Why error codes matter:**
```javascript
// Client can branch on specific errors
if (error.code === 'insufficient_funds') {
  redirectToDeposit();
} else if (error.code === 'withdrawal_pending') {
  showTransactionStatus();
}
```

---

### 3. Added Client-Safe Error Messages
**File:** `internal/errors/handler.go` - `ClientMessage()` function

All new withdrawal errors now return their predefined messages to clients:
- ✅ Safe to expose (no internal details)
- ✅ Human-readable
- ✅ Actionable (user knows what to fix)

Example:
```json
{
  "success": false,
  "message": "Minimum withdrawal amount is ₦500.00",
  "code": "minimum_withdrawal"
}
```

---

### 4. Created Comprehensive Error Documentation
**File:** `WITHDRAWAL_ERROR_RESPONSES.md`

Complete guide covering:
- ✅ All possible error responses
- ✅ HTTP status codes
- ✅ Error codes
- ✅ Example JSON responses
- ✅ How to fix each error
- ✅ Client-side error handling examples
- ✅ Retry strategies
- ✅ Testing with mock provider
- ✅ Quick reference table

---

## Error Response Format

### Standard Error Response
```json
{
  "success": false,
  "message": "Human-readable error message",
  "code": "machine_readable_code",
  "request_id": "req-abc123"
}
```

### Validation Error Response
```json
{
  "success": false,
  "message": "Validation failed",
  "code": "validation_error",
  "errors": {
    "amount_kobo": ["This field is required"],
    "account_number": ["This field is required"]
  },
  "request_id": "req-abc123"
}
```

---

## HTTP Status Code Strategy

### Client Errors (4xx) - User Can Fix
- **400 Bad Request** - Malformed JSON, invalid request structure
- **401 Unauthorized** - Token invalid/expired
- **422 Unprocessable Entity** - Business rule violations:
  - Insufficient balance
  - Amount too small/large
  - Invalid bank account
  - Name mismatch
  - Already have pending withdrawal

### Server Errors (5xx) - Should Retry
- **500 Internal Server Error** - Database error, transfer failed
- **503 Service Unavailable** - Payment provider down

---

## Before vs After

### Before Step 8
```json
// Generic error, not helpful
{
  "success": false,
  "error": "An error occurred"
}
```

### After Step 8
```json
// Specific, actionable error
{
  "success": false,
  "message": "Minimum withdrawal amount is ₦500.00",
  "code": "minimum_withdrawal",
  "request_id": "req-abc123"
}
```

---

## Handler Flow

The withdrawal handler now provides detailed feedback:

```
Request → Parse & Validate → Call Service → Service Error
                                                ↓
                                    Map to HTTP Status
                                    (handler.HTTPStatusFromError)
                                                ↓
                                    Extract Client Message
                                    (handler.ClientMessage)
                                                ↓
                                    Extract Error Code
                                    (handler.ErrorCode)
                                                ↓
                                    Return JSON Response
```

**Handler code:**
```go
func (h *WalletHandler) RequestWithdrawal(c *gin.Context) {
    // ... validation ...
    
    transaction, err := h.walletService.InitiateWithdrawal(ctx, userID, req)
    if err != nil {
        // Centralized error handling - maps to appropriate status
        h.handleError(c, err)  // ← Does all the heavy lifting
        return
    }
    
    response.SuccessWithMessage(c, http.StatusOK, "Withdrawal initiated successfully", transaction)
}
```

---

## Testing Error Responses

### Test Minimum Withdrawal Error
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 10000,
    "account_number": "0123456789",
    "account_name": "John Doe",
    "bank_code": "058"
  }'
```

**Expected Response:**
```json
{
  "success": false,
  "message": "Minimum withdrawal amount is ₦500.00",
  "code": "minimum_withdrawal",
  "request_id": "req-abc123"
}
```
**HTTP Status:** 422

---

### Test Insufficient Balance
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 999999999,
    "account_number": "0123456789",
    "account_name": "John Doe",
    "bank_code": "058"
  }'
```

**Expected Response:**
```json
{
  "success": false,
  "message": "Insufficient wallet balance",
  "code": "insufficient_funds",
  "request_id": "req-abc123"
}
```
**HTTP Status:** 422

---

### Test Account Name Mismatch
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Wrong Name",
    "bank_code": "058"
  }'
```

**Expected Response:**
```json
{
  "success": false,
  "message": "Account name does not match bank records. Expected: 'JOHN DOE', Got: 'Wrong Name'",
  "code": "account_name_mismatch",
  "request_id": "req-abc123"
}
```
**HTTP Status:** 422

---

## Build Verification

All code compiles successfully:
```bash
$ go build ./cmd/api
✓ SUCCESS

$ go build ./cmd/worker
✓ SUCCESS

$ go build ./internal/...
✓ SUCCESS
```

---

## Summary

✅ **Error Mapping:** All withdrawal errors map to appropriate HTTP status codes  
✅ **Error Codes:** Machine-readable codes for client-side handling  
✅ **Error Messages:** Human-readable, actionable messages  
✅ **Documentation:** Comprehensive error response guide  
✅ **Centralized:** All error handling in one place (`errors/handler.go`)  
✅ **Tested:** Build successful, ready for integration testing  

---

## Files Modified

1. `internal/errors/handler.go` - Added error mappings, codes, messages
2. `WITHDRAWAL_ERROR_RESPONSES.md` - New comprehensive documentation

---

## Next Steps

- ✅ Step 1-7: Core implementation complete
- ✅ Step 8: Handler updates complete
- ⏭️ Step 9: Testing guide (create comprehensive test scenarios)
- ⏭️ Step 10: Optional rate limiting (3 withdrawals/hour)
- ⏭️ Step 11: Final milestone verification

---

## Notes for Frontend Team

**Use error codes, not status codes, for branching:**
```javascript
// ❌ Don't do this (status codes can change)
if (response.status === 422) { ... }

// ✓ Do this (error codes are stable)
if (error.code === 'insufficient_funds') { ... }
```

**Always show user-friendly messages:**
```javascript
// ❌ Don't do this
alert(JSON.stringify(error));

// ✓ Do this
showMessage(error.message || 'An error occurred');
```

**Refer to WITHDRAWAL_ERROR_RESPONSES.md for:**
- Complete list of error codes
- Example responses
- Retry strategies
- User feedback best practices

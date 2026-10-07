# Withdrawal Validation Flow

## Overview
Complete validation flow for withdrawal requests with pre-flight checks and error handling.

---

## Step-by-Step Validation

### Step 1: Amount Validation
```go
// Check amount is positive
if req.Amount <= 0 {
    return ErrInvalidAmount
}

// Check minimum withdrawal (₦500 = 50,000 kobo)
if req.Amount < cfg.MinWithdrawalAmount {
    return NewMinimumWithdrawalError(cfg.MinWithdrawalAmount)
    // Error: "amount is below minimum withdrawal of ₦500.00"
}

// Check maximum withdrawal (₦100,000 = 10,000,000 kobo)
if req.Amount > cfg.MaxWithdrawalAmount {
    return NewMaximumWithdrawalError(cfg.MaxWithdrawalAmount)
    // Error: "amount exceeds maximum withdrawal of ₦100,000.00"
}
```

**Configuration:**
- `MIN_WITHDRAWAL_AMOUNT=50000` (₦500)
- `MAX_WITHDRAWAL_AMOUNT=10000000` (₦100,000)

---

### Step 2: Duplicate Prevention
```go
// Check for existing pending withdrawal
pendingWithdrawal, err := repo.GetPendingWithdrawalByUser(ctx, userID)
if pendingWithdrawal != nil {
    return ErrWithdrawalPending
    // Error: "you have a pending withdrawal, please wait for it to complete"
}
```

**Business Rule:** Users can only have ONE pending withdrawal at a time.

**Why This Matters:**
- Prevents race conditions (multiple withdrawals processing simultaneously)
- Prevents locked balance overflow (locking more than available)
- Simplifies user experience (clear status for pending withdrawal)
- Reduces fraud risk (limits exposure to failed transfers)

**Example:**
```
User has ₦100k, initiates ₦50k withdrawal
→ Status: pending, locked_balance=₦50k

User tries to initiate another ₦50k withdrawal
→ REJECTED: "You have a pending withdrawal"

After first withdrawal completes/fails:
→ User can initiate new withdrawal
```

---

### Step 3: Bank Account Resolution
```go
// Verify account exists and get account name from bank
resolution, err := provider.ResolveAccountNumber(ctx, accountNumber, bankCode)
if err != nil {
    return ErrInvalidBankAccount
    // Error: "invalid bank account details"
}
```

**What Happens:**
1. Call Paystack `/bank/resolve` API
2. Paystack queries NIBSS (Nigeria Interbank Settlement System)
3. Returns actual account holder name from bank

**Prevents:**
- Sending money to non-existent accounts
- Typos in account numbers
- Wrong bank code selection

**Example:**
```
Input:
  account_number: "0123456789"
  bank_code: "058" (GTBank)

Resolution Result:
  account_name: "John Doe"
  bank_name: "Guaranty Trust Bank"
```

---

### Step 4: Account Name Verification
```go
// Fuzzy match account names
if !accountNamesMatch(req.AccountName, resolution.AccountName) {
    return NewAccountNameMismatchError(req.AccountName, resolution.AccountName)
    // Error: "account name mismatch: you entered 'John Doe' but bank records show 'John D. Doe'"
}
```

**Fuzzy Matching Rules:**
1. Convert both names to uppercase
2. Remove special characters (dots, commas, hyphens)
3. Normalize whitespace (multiple spaces → single space)
4. Remove common suffixes (JR, SR, III, etc.)
5. Compare normalized strings

**Accepts:**
- "John Doe" ≈ "JOHN DOE" ✓
- "John Doe" ≈ "John D. Doe" ✓
- "John Doe Jr" ≈ "John Doe" ✓
- "O'Brien" ≈ "OBRIEN" ✓

**Rejects:**
- "John Doe" ≠ "Jane Smith" ✗
- "John Doe" ≠ "John Smith" ✗

**Why Fuzzy Matching:**
Banks format names differently:
- Some use ALL CAPS
- Some include middle initials
- Some include Jr/Sr suffixes
- Some remove special characters

---

### Step 5: Available Balance Check
```go
// Lock funds in database transaction
err = db.Transaction(func(tx *gorm.DB) error {
    wallet, err := repo.FindByUserIDForUpdate(ctx, userID, tx)
    
    // Check available balance (main_balance - locked_balance)
    availableBalance := wallet.MainBalance - wallet.LockedBalance
    if availableBalance < req.Amount {
        return ErrInsufficientFunds
    }
    
    // Lock funds
    return repo.LockFunds(ctx, userID, req.Amount, tx)
})
```

**Balance Calculation:**
```
Available Balance = Main Balance - Locked Balance

Example:
  main_balance = ₦100,000
  locked_balance = ₦30,000 (from pending withdrawal)
  available_balance = ₦70,000

User requests ₦80,000 withdrawal:
  ₦80,000 > ₦70,000 (available)
  → REJECTED: "insufficient wallet balance"
```

---

## Complete Validation Flow Diagram

```
┌─────────────────────────────────────────┐
│ User Request: Withdraw ₦50,000          │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│ Step 1: Amount Validation               │
│  ✓ Amount > 0                           │
│  ✓ Amount >= ₦500 (min)                 │
│  ✓ Amount <= ₦100,000 (max)             │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│ Step 2: Check Pending Withdrawal        │
│  ✓ No pending withdrawal exists         │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│ Step 3: Resolve Bank Account            │
│  → Call Paystack API                    │
│  → NIBSS verifies account               │
│  ← Returns: "John Doe" (GTBank)         │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│ Step 4: Verify Account Name             │
│  User entered: "John Doe"               │
│  Bank records: "John D. Doe"            │
│  Fuzzy match: ✓ MATCH                   │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│ Step 5: Check Available Balance         │
│  ← BEGIN TRANSACTION                    │
│  ← LOCK wallet FOR UPDATE               │
│  Available: ₦100k - ₦0 = ₦100k          │
│  Requested: ₦50k                        │
│  ✓ Sufficient funds                     │
│  → Lock ₦50k                            │
│  → Create pending transaction           │
│  ← COMMIT                               │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│ Step 6: Queue for Worker               │
│  → Publish to withdrawal queue          │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│ Step 7: Return Pending Status           │
│  {                                      │
│    "reference": "WD-ABC123",            │
│    "status": "pending",                 │
│    "amount": 50000                      │
│  }                                      │
└─────────────────────────────────────────┘
```

---

## Error Response Examples

### Amount Too Low
```json
{
  "error": "amount is below minimum withdrawal of ₦500.00",
  "code": "validation_error"
}
```
**HTTP Status:** 422 Unprocessable Entity

---

### Amount Too High
```json
{
  "error": "amount exceeds maximum withdrawal of ₦100,000.00",
  "code": "validation_error"
}
```
**HTTP Status:** 422 Unprocessable Entity

---

### Pending Withdrawal Exists
```json
{
  "error": "you have a pending withdrawal, please wait for it to complete",
  "code": "withdrawal_pending"
}
```
**HTTP Status:** 409 Conflict

---

### Invalid Bank Account
```json
{
  "error": "invalid bank account details",
  "code": "invalid_bank_account"
}
```
**HTTP Status:** 422 Unprocessable Entity

**Common Causes:**
- Account doesn't exist
- Invalid account number format
- Invalid bank code
- Bank API temporarily unavailable

---

### Account Name Mismatch
```json
{
  "error": "account name mismatch: you entered 'John Doe' but bank records show 'Jane Smith'",
  "code": "account_name_mismatch"
}
```
**HTTP Status:** 422 Unprocessable Entity

**User Action Required:** Verify account details and try again

---

### Insufficient Funds
```json
{
  "error": "insufficient wallet balance",
  "code": "insufficient_funds"
}
```
**HTTP Status:** 422 Unprocessable Entity

**Calculation:**
- Available balance: main_balance - locked_balance
- If user has pending withdrawal, locked_balance reduces available funds

---

## Testing Validation Flow

### Test Case 1: Amount Below Minimum
```bash
POST /api/v1/wallet/withdraw
{
  "amount": 40000,  // ₦400 (below ₦500 minimum)
  "account_number": "0123456789",
  "account_name": "John Doe",
  "bank_code": "058"
}

Expected: 422 with "amount is below minimum withdrawal of ₦500.00"
```

---

### Test Case 2: Amount Above Maximum
```bash
POST /api/v1/wallet/withdraw
{
  "amount": 15000000,  // ₦150,000 (above ₦100,000 maximum)
  "account_number": "0123456789",
  "account_name": "John Doe",
  "bank_code": "058"
}

Expected: 422 with "amount exceeds maximum withdrawal of ₦100,000.00"
```

---

### Test Case 3: Duplicate Pending Withdrawal
```bash
# First request
POST /api/v1/wallet/withdraw
{ "amount": 50000, ... }
Response: 200 OK, status="pending"

# Second request (while first is pending)
POST /api/v1/wallet/withdraw
{ "amount": 30000, ... }

Expected: 409 with "you have a pending withdrawal..."
```

---

### Test Case 4: Invalid Account
```bash
POST /api/v1/wallet/withdraw
{
  "amount": 50000,
  "account_number": "0000000000",  // Non-existent account
  "account_name": "John Doe",
  "bank_code": "058"
}

Expected: 422 with "invalid bank account details"
```

---

### Test Case 5: Account Name Mismatch
```bash
POST /api/v1/wallet/withdraw
{
  "amount": 50000,
  "account_number": "0123456789",
  "account_name": "Wrong Name",  // Doesn't match bank records
  "bank_code": "058"
}

Expected: 422 with "account name mismatch: ..."
```

---

### Test Case 6: Insufficient Funds
```bash
# User has ₦30k, tries to withdraw ₦50k
POST /api/v1/wallet/withdraw
{
  "amount": 50000,
  "account_number": "0123456789",
  "account_name": "John Doe",
  "bank_code": "058"
}

Expected: 422 with "insufficient wallet balance"
```

---

## Frontend Integration

### Display Withdrawal Limits
```javascript
const config = {
  minWithdrawal: 50000,  // ₦500
  maxWithdrawal: 10000000  // ₦100,000
};

// Format for display
const minDisplay = formatCurrency(config.minWithdrawal);  // "₦500.00"
const maxDisplay = formatCurrency(config.maxWithdrawal);  // "₦100,000.00"

// Show in UI
<p>Minimum: {minDisplay}, Maximum: {maxDisplay}</p>
```

---

### Client-Side Validation (Optional)
```javascript
function validateWithdrawal(amount) {
  if (amount < 50000) {
    return "Minimum withdrawal is ₦500.00";
  }
  if (amount > 10000000) {
    return "Maximum withdrawal is ₦100,000.00";
  }
  if (amount > availableBalance) {
    return "Insufficient balance";
  }
  return null;
}
```

**Note:** Always validate server-side. Client validation is UX only.

---

## Security Considerations

1. **Rate Limiting:** Max 3 withdrawal requests per hour per user
2. **2FA/OTP:** Consider requiring OTP for withdrawals > ₦50,000
3. **Suspicious Activity:** Flag unusually large withdrawals for review
4. **Account Verification:** Always verify bank account before locking funds
5. **Locked Balance Protection:** Database constraints prevent locked > main

---

## Performance Optimizations

1. **Cache Bank Codes:** Store bank code → bank name mapping in memory
2. **Async Account Resolution:** Consider pre-verifying saved bank accounts
3. **Index Optimization:** Index on (user_id, status, type) for pending check
4. **Connection Pooling:** Reuse Paystack API connections

---

## Monitoring & Alerts

**Track these metrics:**
- Validation failure rate by type
- Account resolution API latency
- Account name mismatch rate
- Insufficient funds rejection rate
- Duplicate pending withdrawal attempts

**Alert on:**
- High validation failure rate (> 20%)
- Account resolution API errors (> 5%)
- Sudden spike in withdrawal requests

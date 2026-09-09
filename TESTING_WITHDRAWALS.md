# Testing Withdrawal Flow with Mock Provider

This guide shows how to test the complete withdrawal flow using the Mock Provider (no real money, no real API calls).

---

## Prerequisites

1. Backend running: `go run cmd/api/main.go`
2. Database migrations applied
3. User registered and authenticated
4. Wallet has sufficient balance

---

## Test Scenario 1: Successful Withdrawal

### Step 1: Check Current Balance

```bash
GET /api/v1/wallet
Authorization: Bearer <your_jwt_token>
```

**Expected Response:**
```json
{
  "data": {
    "main_balance": 100000,
    "locked_balance": 0,
    "earnings_balance": 0,
    "currency": "NGN"
  }
}
```

### Step 2: Initiate Withdrawal

```bash
POST /api/v1/wallet/withdraw
Authorization: Bearer <your_jwt_token>
Content-Type: application/json

{
  "amount": 50000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "John Doe"
}
```

**Expected Response:**
```json
{
  "data": {
    "id": "uuid",
    "reference": "WD-ABC123XYZ",
    "type": "withdrawal",
    "amount": 50000,
    "status": "pending",
    "balance_before": 100000,
    "balance_after": 100000,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**What Happened:**
- ✅ Wallet locked ₦50,000 (`locked_balance = 50000`)
- ✅ Transaction created with status `pending`
- ✅ Withdrawal job queued for worker
- ✅ `main_balance` still 100,000 (not debited yet)

### Step 3: Check Balance After Initiation

```bash
GET /api/v1/wallet
```

**Expected Response:**
```json
{
  "data": {
    "main_balance": 100000,
    "locked_balance": 50000,
    "currency": "NGN"
  }
}
```

**Available balance = 100,000 - 50,000 = ₦50,000** ✅

### Step 4: Worker Processes Withdrawal

The worker automatically:
1. Picks up message from queue
2. Calls `provider.InitiateTransfer()`
3. Mock provider returns `status: "success"`
4. Worker calls `FinalizeWithdrawal()`
5. Releases locked balance and debits wallet

### Step 5: Check Final Balance

```bash
GET /api/v1/wallet
```

**Expected Response:**
```json
{
  "data": {
    "main_balance": 50000,
    "locked_balance": 0,
    "currency": "NGN"
  }
}
```

**✅ Withdrawal Complete!**
- Main balance debited: ₦100k → ₦50k
- Locked balance cleared: ₦50k → ₦0
- Available balance: ₦50k

### Step 6: Check Transaction History

```bash
GET /api/v1/wallet/transactions
```

**Expected Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "reference": "WD-ABC123XYZ",
      "type": "withdrawal",
      "amount": 50000,
      "status": "completed",
      "balance_before": 100000,
      "balance_after": 50000,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

---

## Test Scenario 2: Failed Withdrawal (Reversal)

### Step 1: Initiate Withdrawal with Failure Trigger

Use reference prefix `FAIL-` to simulate failure:

```bash
POST /api/v1/wallet/withdraw
Authorization: Bearer <your_jwt_token>

{
  "amount": 30000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "John Doe",
  "reference": "FAIL-TEST123"
}
```

### Step 2: Worker Processes

1. Worker calls `InitiateTransfer()`
2. Mock provider sees "FAIL-" prefix
3. Returns `status: "failed"`
4. Worker calls `FinalizeWithdrawal()` with `failed`
5. **Reversal**: Locked funds returned to available balance

### Step 3: Check Balance After Failure

```bash
GET /api/v1/wallet
```

**Expected Response:**
```json
{
  "data": {
    "main_balance": 50000,
    "locked_balance": 0,
    "currency": "NGN"
  }
}
```

**✅ Automatic Reversal!**
- Main balance unchanged: ₦50k (not debited)
- Locked balance cleared: ₦30k → ₦0
- Funds returned automatically

### Step 4: Check Transaction

```bash
GET /api/v1/wallet/transactions?reference=FAIL-TEST123
```

**Expected Response:**
```json
{
  "data": {
    "reference": "FAIL-TEST123",
    "status": "failed",
    "amount": 30000,
    "balance_before": 50000,
    "balance_after": 50000
  }
}
```

**Note:** `balance_after` = `balance_before` (no debit) ✅

---

## Test Scenario 3: Invalid Account

### Trigger Invalid Account Error

```bash
POST /api/v1/wallet/withdraw

{
  "amount": 10000,
  "bank_code": "058",
  "account_number": "0000000000",
  "account_name": "Invalid"
}
```

**Expected Response:**
```json
{
  "error": {
    "code": "INVALID_ACCOUNT",
    "message": "account not found"
  }
}
```

**What Happened:**
- ❌ Pre-flight check failed
- ❌ No funds locked
- ❌ No transaction created
- ✅ Wallet balance unchanged

---

## Test Scenario 4: Insufficient Balance

### Attempt Withdrawal Greater Than Available

```bash
POST /api/v1/wallet/withdraw

{
  "amount": 1000000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "John Doe"
}
```

**Expected Response:**
```json
{
  "error": {
    "code": "INSUFFICIENT_FUNDS",
    "message": "insufficient available balance: requested ₦10,000.00 but only ₦500.00 available"
  }
}
```

---

## Test Scenario 5: Account Name Mismatch

### Request Withdrawal with Wrong Name

Mock provider always returns "John Doe Mock" for account resolution.

```bash
POST /api/v1/wallet/withdraw

{
  "amount": 10000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "Jane Smith"
}
```

**Expected Response:**
```json
{
  "error": {
    "code": "ACCOUNT_NAME_MISMATCH",
    "message": "account name mismatch: you entered 'Jane Smith' but bank records show 'John Doe Mock'"
  }
}
```

---

## Test Scenario 6: Multiple Concurrent Withdrawals

### Attempt Two Withdrawals Simultaneously

```bash
# Terminal 1
POST /api/v1/wallet/withdraw
{ "amount": 30000, ... }

# Terminal 2 (immediately after)
POST /api/v1/wallet/withdraw
{ "amount": 30000, ... }
```

**Expected Behavior:**
- ✅ First request: Success, locks ₦30k
- ❌ Second request: Fails with "insufficient available balance"
- ✅ Database row locking prevents race conditions

---

## Monitoring Withdrawal Status

### Check Pending Withdrawals

```bash
GET /api/v1/wallet/transactions?status=pending&type=withdrawal
```

### Check Completed Withdrawals

```bash
GET /api/v1/wallet/transactions?status=completed&type=withdrawal
```

### Check Failed Withdrawals

```bash
GET /api/v1/wallet/transactions?status=failed&type=withdrawal
```

---

## Mock Provider Testing Patterns

The Mock Provider supports special test patterns based on reference or account number:

| Pattern | Trigger | Result |
|---------|---------|--------|
| Success | Any normal reference | `status: "success"` |
| Failure | Reference starts with `FAIL-` | `status: "failed"` |
| Pending | Reference starts with `PENDING-` | `status: "pending"` |
| API Error | Reference starts with `ERROR-` | Returns error |
| Invalid Account | `account_number: "0000000000"` | Account not found error |
| API Unavailable | `account_number: "9999999999"` | Bank API error |

### Example: Test Pending Status

```bash
POST /api/v1/wallet/withdraw
{
  "amount": 20000,
  "reference": "PENDING-TEST123",
  ...
}
```

Worker will see `status: "pending"` and can be configured to poll/wait.

---

## Database Inspection

### Check Wallet State

```sql
SELECT user_id, main_balance, locked_balance, 
       (main_balance - locked_balance) as available_balance
FROM wallets 
WHERE user_id = '<user_uuid>';
```

### Check Transaction Ledger

```sql
SELECT reference, type, amount, status, 
       balance_before, balance_after, created_at
FROM wallet_transactions
WHERE user_id = '<user_uuid>'
ORDER BY created_at DESC;
```

### Find Stuck Withdrawals

```sql
SELECT id, reference, amount, status, created_at
FROM wallet_transactions
WHERE type = 'withdrawal' 
  AND status = 'pending'
  AND created_at < NOW() - INTERVAL '30 minutes';
```

---

## Troubleshooting

### Withdrawal Stuck in Pending

**Possible causes:**
1. Worker not running
2. RabbitMQ not connected
3. Queue disabled in development

**Solution:**
```bash
# Check worker logs
# Manually process withdrawal
go run cmd/worker/main.go
```

### Balance Mismatch

**Check:**
```sql
-- Should always be true
SELECT * FROM wallets 
WHERE locked_balance > main_balance;  -- Should return 0 rows

-- Check locked vs pending
SELECT 
  w.locked_balance,
  COALESCE(SUM(wt.amount), 0) as pending_total
FROM wallets w
LEFT JOIN wallet_transactions wt 
  ON w.user_id = wt.user_id 
  AND wt.status = 'pending' 
  AND wt.type = 'withdrawal'
WHERE w.user_id = '<user_uuid>'
GROUP BY w.locked_balance;
```

### Manual Reversal (Emergency)

```sql
-- ONLY use in emergency!
BEGIN;

-- Release locked balance
UPDATE wallets 
SET locked_balance = locked_balance - <amount>
WHERE user_id = '<user_uuid>';

-- Mark transaction as failed
UPDATE wallet_transactions
SET status = 'failed'
WHERE reference = '<withdrawal_reference>';

COMMIT;
```

---

## Next Steps

Once mock testing passes:
1. Switch to Paystack test mode
2. Test with real (test) bank accounts
3. Verify webhook delivery with ngrok
4. Test full end-to-end flow
5. Deploy to staging
6. Production testing with small amounts

---

## Support

For issues:
1. Check logs: `tail -f logs/app.log`
2. Check database state (queries above)
3. Verify queue connection
4. Review `WITHDRAWAL_ARCHITECTURE.md`

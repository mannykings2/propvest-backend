# Withdrawal Complete Test Guide

This guide provides comprehensive testing procedures for the complete withdrawal implementation.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Test Environment Setup](#test-environment-setup)
3. [Unit Tests](#unit-tests)
4. [Integration Tests](#integration-tests)
5. [End-to-End Tests](#end-to-end-tests)
6. [Edge Cases & Error Scenarios](#edge-cases--error-scenarios)
7. [Performance Tests](#performance-tests)
8. [Security Tests](#security-tests)
9. [Checklist](#checklist)

---

## Prerequisites

### Required Tools
```bash
# HTTP client
curl --version

# JSON processor
jq --version

# PostgreSQL client
psql --version

# RabbitMQ management (optional)
# Access via: http://localhost:15672
```

### Environment Setup
```bash
# 1. Start services
docker-compose up -d postgres rabbitmq

# 2. Run migrations
make migrate-up

# 3. Start API
go run cmd/api/main.go

# 4. Start worker (in another terminal)
go run cmd/worker/main.go
```

### Test User Setup
```bash
# Create test user and get token
TOKEN=$(curl -s http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "first_name": "Test",
    "last_name": "User",
    "email": "test@example.com",
    "phone": "+2348012345678",
    "password": "SecureP@ss123"
  }' | jq -r '.data.access_token')

echo "TOKEN=$TOKEN"
```

### Fund Test Wallet
```bash
# Deposit ₦10,000 for testing
curl -X POST http://localhost:8080/api/v1/wallet/deposit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"amount_kobo": 1000000}'

# Get authorization URL and complete payment manually
# OR use test helper script to simulate webhook
```

---

## Test Environment Setup

### Configuration Check
```bash
# Verify withdrawal limits in .env
cat .env | grep WITHDRAWAL
# Expected:
# MIN_WITHDRAWAL_AMOUNT=50000  # ₦500
# MAX_WITHDRAWAL_AMOUNT=10000000  # ₦100,000
```

### Database Health Check
```sql
-- Check wallet table has locked_balance column
\d wallets;

-- Should show:
-- main_balance      | bigint
-- locked_balance    | bigint
-- earnings_balance  | bigint
```

### RabbitMQ Health Check
```bash
# Check queue exists
curl -u guest:guest http://localhost:15672/api/queues/%2F/withdrawal.process

# Should return 200 OK with queue stats
```

---

## Unit Tests

### 1. Configuration Tests

#### Test: Min/Max Withdrawal Limits Loaded
```bash
# Start API with test config
MIN_WITHDRAWAL_AMOUNT=50000 MAX_WITHDRAWAL_AMOUNT=10000000 \
  go run cmd/api/main.go

# Check logs for:
# "configuration loaded" min_withdrawal=50000 max_withdrawal=10000000
```

**Expected:** Config values loaded correctly

---

### 2. Error Helper Tests

#### Test: Error Messages Include Context
```go
// In internal/errors/withdrawal_errors_test.go
func TestWithdrawalErrorContext(t *testing.T) {
    err := NewInsufficientBalanceError(50000, 30000)
    expected := "Insufficient balance: need ₦500.00, have ₦300.00"
    assert.Contains(t, err.Error(), expected)
}
```

---

### 3. Repository Tests

#### Test: GetPendingWithdrawalByUser
```sql
-- Setup: Insert pending withdrawal
INSERT INTO wallet_transactions (
  id, user_id, wallet_id, type, amount, status, reference
) VALUES (
  gen_random_uuid(),
  '123e4567-e89b-12d3-a456-426614174000',
  '223e4567-e89b-12d3-a456-426614174000',
  'withdrawal',
  50000,
  'pending',
  'WD-TEST-001'
);

-- Test: Query should return the pending withdrawal
SELECT * FROM wallet_transactions
WHERE user_id = '123e4567-e89b-12d3-a456-426614174000'
  AND type = 'withdrawal'
  AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;
```

**Expected:** Returns 1 row with status='pending'

---

### 4. Service Validation Tests

#### Test: Amount Below Minimum
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 10000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```

**Expected Response:**
```json
{
  "success": false,
  "message": "Minimum withdrawal amount is ₦500.00",
  "code": "minimum_withdrawal"
}
```
**HTTP Status:** 422

---

#### Test: Amount Above Maximum
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 15000000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```

**Expected Response:**
```json
{
  "success": false,
  "message": "Maximum withdrawal amount is ₦100,000.00",
  "code": "maximum_withdrawal"
}
```
**HTTP Status:** 422

---

#### Test: Insufficient Balance
```bash
# Assuming wallet has ₦1,000 (100,000 kobo)
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 200000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```

**Expected Response:**
```json
{
  "success": false,
  "message": "Insufficient wallet balance",
  "code": "insufficient_funds"
}
```
**HTTP Status:** 422

---

#### Test: Duplicate Pending Withdrawal Prevention
```bash
# First withdrawal
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "PENDING-123",
    "account_name": "Test User",
    "bank_code": "058"
  }'

# Second withdrawal (within 10 minutes)
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```

**Expected Response (2nd call):**
```json
{
  "success": false,
  "message": "You have a pending withdrawal. Please wait for it to complete.",
  "code": "withdrawal_pending"
}
```
**HTTP Status:** 422

---

## Integration Tests

### 1. Full Withdrawal Flow (Success)

#### Step 1: Initiate Withdrawal
```bash
RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }')

echo $RESPONSE | jq '.'
```

**Expected Response:**
```json
{
  "success": true,
  "message": "Withdrawal initiated successfully",
  "data": {
    "id": "uuid",
    "reference": "WD-...",
    "type": "withdrawal",
    "amount": 50000,
    "status": "pending",
    "balance_before": 1000000,
    "balance_after": 950000
  }
}
```

**Extract Transaction ID:**
```bash
TRANSACTION_ID=$(echo $RESPONSE | jq -r '.data.id')
echo "Transaction ID: $TRANSACTION_ID"
```

---

#### Step 2: Verify Wallet Locked
```bash
curl -s http://localhost:8080/api/v1/wallet \
  -H "Authorization: Bearer $TOKEN" | jq '.'
```

**Expected:**
```json
{
  "success": true,
  "data": {
    "main_balance": 950000,
    "locked_balance": 50000,
    "available_balance": 900000
  }
}
```

**Verify in Database:**
```sql
SELECT 
  main_balance,
  locked_balance,
  main_balance - locked_balance AS available_balance
FROM wallets
WHERE user_id = '<user_id>';
```

---

#### Step 3: Verify Transaction Created
```sql
SELECT 
  id,
  reference,
  type,
  amount,
  status,
  external_reference,
  created_at
FROM wallet_transactions
WHERE id = '<transaction_id>';
```

**Expected:**
- type: 'withdrawal'
- amount: 50000
- status: 'pending'
- external_reference: NULL (not yet processed by worker)

---

#### Step 4: Verify Message Queued
```bash
# Check RabbitMQ queue
curl -u guest:guest \
  http://localhost:15672/api/queues/%2F/withdrawal.process | jq '.messages'
```

**Expected:** 1 message in queue

---

#### Step 5: Wait for Worker Processing
```bash
# Watch worker logs
tail -f logs/worker.log

# Or watch database
watch -n 1 'psql -U propvest -d propvest -c "SELECT status, external_reference FROM wallet_transactions WHERE id = '\''<transaction_id>'\'';"'
```

**Expected Changes:**
1. Worker consumes message
2. Calls Paystack InitiateTransfer
3. Saves transfer_code to external_reference
4. Status remains 'pending' (waits for webhook)

**Timeline:** 1-5 seconds

---

#### Step 6: Simulate Transfer Success Webhook
```bash
# Use webhook simulator tool
go run tools/trigger_transfer_webhook.go \
  --transfer-code "<transfer_code>" \
  --status success
```

**Or manually:**
```bash
TRANSFER_CODE="TRF_..." # From database

curl -X POST http://localhost:8080/api/v1/webhooks/payment/transfer \
  -H "X-Paystack-Signature: $(generate_signature)" \
  -H "Content-Type: application/json" \
  -d '{
    "event": "transfer.success",
    "data": {
      "transfer_code": "'$TRANSFER_CODE'",
      "status": "success",
      "amount": 50000,
      "recipient": {
        "account_number": "0123456789",
        "bank_code": "058"
      }
    }
  }'
```

---

#### Step 7: Verify Final State
```bash
# Check wallet (funds should be permanently debited)
curl -s http://localhost:8080/api/v1/wallet \
  -H "Authorization: Bearer $TOKEN" | jq '.'
```

**Expected:**
```json
{
  "success": true,
  "data": {
    "main_balance": 900000,
    "locked_balance": 0,
    "available_balance": 900000
  }
}
```

```sql
-- Verify in database
SELECT 
  main_balance,
  locked_balance,
  main_balance - locked_balance AS available_balance
FROM wallets
WHERE user_id = '<user_id>';

-- Expected:
-- main_balance: 900000 (debited ₦500)
-- locked_balance: 0 (released)
-- available: 900000
```

```sql
-- Verify transaction completed
SELECT status, external_reference
FROM wallet_transactions
WHERE id = '<transaction_id>';

-- Expected:
-- status: 'completed'
-- external_reference: 'TRF_...'
```

---

### 2. Full Withdrawal Flow (Failure)

#### Step 1-4: Same as Success Flow
(Initiate withdrawal, verify locked, verify queued)

#### Step 5: Simulate Transfer Failure Webhook
```bash
go run tools/trigger_transfer_webhook.go \
  --transfer-code "<transfer_code>" \
  --status failed \
  --reason "Insufficient balance in source account"
```

#### Step 6: Verify Reversal
```bash
# Check wallet (funds should be returned)
curl -s http://localhost:8080/api/v1/wallet \
  -H "Authorization: Bearer $TOKEN" | jq '.'
```

**Expected:**
```json
{
  "success": true,
  "data": {
    "main_balance": 1000000,
    "locked_balance": 0,
    "available_balance": 1000000
  }
}
```

```sql
-- Verify in database
SELECT 
  main_balance,
  locked_balance
FROM wallets
WHERE user_id = '<user_id>';

-- Expected:
-- main_balance: 1000000 (restored)
-- locked_balance: 0 (released)
```

```sql
-- Verify transaction failed
SELECT status, metadata->>'failure_reason' AS failure_reason
FROM wallet_transactions
WHERE id = '<transaction_id>';

-- Expected:
-- status: 'failed'
-- failure_reason: 'Insufficient balance in source account'
```

---

### 3. Reconciliation Test

#### Setup: Create Orphaned Pending Withdrawal
```sql
-- Insert transaction that's been pending for 15 minutes
INSERT INTO wallet_transactions (
  id, user_id, wallet_id, type, amount, status, reference,
  external_reference, created_at
) VALUES (
  gen_random_uuid(),
  '<user_id>',
  '<wallet_id>',
  'withdrawal',
  50000,
  'pending',
  'WD-ORPHAN-001',
  'TRF_test123',  -- Simulate worker processed but webhook missed
  NOW() - INTERVAL '15 minutes'
);
```

#### Trigger Reconciliation
```bash
# Wait for scheduled run (every 5 minutes)
# OR manually trigger (in worker code, call reconcilePendingWithdrawals)
```

#### Verify Reconciliation
```bash
# Watch worker logs
tail -f logs/worker.log | grep reconciliation

# Should see:
# "🔄 running reconciliation check..."
# "found pending withdrawals" count=1
# "verifying withdrawal status" transaction_id=...
# "transfer status verified" status=success
# "reconciling successful transfer"
```

```sql
-- Verify transaction updated
SELECT status, updated_at
FROM wallet_transactions
WHERE reference = 'WD-ORPHAN-001';

-- Expected:
-- status: 'completed' (or 'failed' depending on actual status)
-- updated_at: recent timestamp
```

---

## End-to-End Tests

### Test Case 1: Happy Path
**Scenario:** User successfully withdraws ₦5,000 to GTBank

```bash
# Setup: Fund wallet with ₦10,000
# Action: Withdraw ₦5,000
# Expected: 
#   - Balance reduces by ₦5,000
#   - Transaction status: pending → completed
#   - Funds reach bank account (simulated)
#   - User receives notification
```

---

### Test Case 2: Concurrent Withdrawal Prevention
**Scenario:** User tries to initiate two withdrawals simultaneously

```bash
# Action: Send two withdrawal requests at same time
(
  curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"amount_kobo": 50000, "account_number": "0123456789", "account_name": "Test User", "bank_code": "058"}' &
  
  curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"amount_kobo": 50000, "account_number": "0123456789", "account_name": "Test User", "bank_code": "058"}' &
  
  wait
)
```

**Expected:**
- First request: 200 OK - Withdrawal initiated
- Second request: 422 - "You have a pending withdrawal"

---

### Test Case 3: Overdraft Prevention
**Scenario:** User tries to withdraw more than available balance

```bash
# Setup: Wallet has ₦5,000
# Action: Try to withdraw ₦10,000
# Expected: 422 - "Insufficient wallet balance"
```

---

### Test Case 4: Invalid Bank Account
**Scenario:** User provides non-existent bank account

```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "ERROR-1234",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```

**Expected:** 422 - "Invalid bank account number"

---

### Test Case 5: Account Name Mismatch
**Scenario:** Provided name doesn't match bank records

```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Wrong Person",
    "bank_code": "058"
  }'
```

**Expected:** 422 - "Account name does not match bank records"

---

## Edge Cases & Error Scenarios

### Edge Case 1: Exactly Minimum Amount
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```
**Expected:** 200 OK - Success

---

### Edge Case 2: Exactly Maximum Amount
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 10000000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```
**Expected:** 200 OK - Success

---

### Edge Case 3: One Kobo Below Minimum
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 49999,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```
**Expected:** 422 - "Minimum withdrawal amount is ₦500.00"

---

### Edge Case 4: One Kobo Above Maximum
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 10000001,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```
**Expected:** 422 - "Maximum withdrawal amount is ₦100,000.00"

---

### Edge Case 5: Zero Amount
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 0,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```
**Expected:** 422 - Validation error

---

### Edge Case 6: Negative Amount
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": -50000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```
**Expected:** 422 - Validation error

---

### Edge Case 7: Withdrawn to Last Kobo
```bash
# Setup: Wallet has exactly ₦500 (50,000 kobo)
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```

**Expected:** 
- 200 OK - Success
- Final wallet balance: 0
- Transaction completes successfully

---

### Error Scenario 1: RabbitMQ Down
```bash
# Stop RabbitMQ
docker-compose stop rabbitmq

# Try withdrawal
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```

**Expected:** 
- Transaction created in database
- Funds locked
- Worker cannot process (no queue)
- Reconciliation will pick it up after 10 minutes

---

### Error Scenario 2: Database Connection Lost
```bash
# Stop PostgreSQL mid-request
docker-compose stop postgres

# Try withdrawal
# Expected: 500 - "An internal error occurred"
```

---

### Error Scenario 3: Paystack API Down
```bash
# Set mock provider to always fail
# Expected: 503 - "Payment provider is currently unavailable"
```

---

## Performance Tests

### Test 1: Response Time
```bash
# Measure withdrawal initiation time
time curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```

**Acceptable:** < 500ms

---

### Test 2: Worker Processing Time
```bash
# Measure time from queue to transfer initiation
# Check worker logs for timing
```

**Acceptable:** < 5 seconds

---

### Test 3: Concurrent Requests (Load Test)
```bash
# Apache Bench
ab -n 100 -c 10 -T application/json -H "Authorization: Bearer $TOKEN" \
  -p withdrawal_payload.json \
  http://localhost:8080/api/v1/wallet/withdraw

# Or use hey
hey -n 100 -c 10 -m POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"amount_kobo":50000,"account_number":"0123456789","account_name":"Test User","bank_code":"058"}' \
  http://localhost:8080/api/v1/wallet/withdraw
```

**Expected:**
- No race conditions
- No duplicate withdrawals
- All requests handled correctly
- Second+ requests return "withdrawal_pending" error

---

## Security Tests

### Test 1: Unauthorized Access
```bash
# No token
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```
**Expected:** 401 - "Unauthorized"

---

### Test 2: Expired Token
```bash
# Use expired token
EXPIRED_TOKEN="eyJhbGc..."

curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $EXPIRED_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```
**Expected:** 401 - "Token expired"

---

### Test 3: Withdraw from Another User's Wallet
**This should be impossible due to architecture**
- User ID comes from JWT token, not request body
- Service uses authenticated user_id
- Cannot specify target wallet

---

### Test 4: SQL Injection Attempts
```bash
curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount_kobo": 50000,
    "account_number": "0123456789; DROP TABLE wallets; --",
    "account_name": "Test User",
    "bank_code": "058"
  }'
```
**Expected:** 422 - Validation error (invalid account number format)

---

## Checklist

### ✅ Configuration
- [ ] MIN_WITHDRAWAL_AMOUNT set correctly (50,000 kobo)
- [ ] MAX_WITHDRAWAL_AMOUNT set correctly (10,000,000 kobo)
- [ ] Config values loaded on startup
- [ ] Validation uses config values

### ✅ Database
- [ ] `locked_balance` column exists on wallets table
- [ ] Migrations run successfully
- [ ] Indexes created for performance

### ✅ Validation
- [ ] Amount below minimum rejected (422)
- [ ] Amount above maximum rejected (422)
- [ ] Insufficient balance rejected (422)
- [ ] Invalid bank account rejected (422)
- [ ] Account name mismatch rejected (422)
- [ ] Duplicate pending withdrawal rejected (422)
- [ ] Zero/negative amount rejected (422)

### ✅ Wallet Locking
- [ ] Funds locked immediately on initiation
- [ ] Available balance calculated correctly (main - locked)
- [ ] Cannot withdraw more than available
- [ ] Locked balance visible in API response
- [ ] Locked funds released on success (cleared)
- [ ] Locked funds released on failure (returned)

### ✅ Transaction Creation
- [ ] Transaction record created with status='pending'
- [ ] Reference generated (WD-uuid format)
- [ ] Bank details saved in metadata
- [ ] Balance before/after recorded correctly
- [ ] User can view in transaction history

### ✅ Queue Integration
- [ ] Message published to withdrawal.process queue
- [ ] Message contains all required fields
- [ ] Queue survives RabbitMQ restart (durable)
- [ ] Worker consumes messages correctly

### ✅ Worker Processing
- [ ] Worker starts successfully
- [ ] Consumes withdrawal messages
- [ ] Calls provider.InitiateTransfer
- [ ] Saves transfer_code to external_reference
- [ ] Handles immediate success/failure
- [ ] Handles pending status (waits for webhook)
- [ ] Error handling for transient failures
- [ ] Retries on network errors

### ✅ Finalization
- [ ] FinalizeWithdrawal handles success correctly
- [ ] FinalizeWithdrawal handles failure correctly
- [ ] Idempotent (safe to call multiple times)
- [ ] Updates transaction status
- [ ] Releases locked funds appropriately
- [ ] Debits wallet on success
- [ ] Restores wallet on failure

### ✅ Reconciliation
- [ ] Runs every 5 minutes
- [ ] Finds pending withdrawals > 10 minutes old
- [ ] Calls provider.VerifyTransfer
- [ ] Finalizes based on actual status
- [ ] Handles all transfer statuses
- [ ] Logs reconciliation activity

### ✅ Error Handling
- [ ] All errors have appropriate HTTP status codes
- [ ] All errors have machine-readable codes
- [ ] All errors have helpful messages
- [ ] Errors don't leak internal details
- [ ] Request IDs logged for tracing

### ✅ Security
- [ ] Requires authentication
- [ ] Cannot withdraw from another user's wallet
- [ ] SQL injection prevented
- [ ] Input validation comprehensive
- [ ] Sensitive data not logged

### ✅ Performance
- [ ] Withdrawal initiation < 500ms
- [ ] Worker processing < 5 seconds
- [ ] No N+1 queries
- [ ] Database indexes utilized
- [ ] Concurrent requests handled correctly

### ✅ Documentation
- [ ] API endpoints documented
- [ ] Error responses documented
- [ ] Testing guide exists
- [ ] Architecture documented
- [ ] Deployment guide exists

---

## Common Issues & Solutions

### Issue: Withdrawal stuck in "pending"
**Diagnosis:**
```sql
SELECT * FROM wallet_transactions
WHERE status = 'pending'
AND created_at < NOW() - INTERVAL '10 minutes';
```

**Solutions:**
1. Check worker is running: `docker-compose ps worker`
2. Check worker logs: `docker-compose logs worker`
3. Check RabbitMQ queue: http://localhost:15672
4. Wait for reconciliation (runs every 5 min)
5. Manually trigger reconciliation if urgent

---

### Issue: Funds locked but transaction failed
**Diagnosis:**
```sql
SELECT w.locked_balance, COUNT(wt.*) AS pending_count
FROM wallets w
LEFT JOIN wallet_transactions wt
  ON wt.wallet_id = w.id
  AND wt.type = 'withdrawal'
  AND wt.status = 'pending'
WHERE w.user_id = '<user_id>'
GROUP BY w.id, w.locked_balance;
```

**Solutions:**
1. Find orphaned transactions
2. Manually finalize as failed
3. Release locked funds

```sql
-- Release locked funds
UPDATE wallets
SET locked_balance = 0
WHERE user_id = '<user_id>';

-- Mark transaction failed
UPDATE wallet_transactions
SET status = 'failed',
    metadata = metadata || '{"failure_reason":"Manual intervention"}'::jsonb
WHERE user_id = '<user_id>'
AND type = 'withdrawal'
AND status = 'pending';
```

---

### Issue: Worker not processing messages
**Diagnosis:**
```bash
# Check worker is running
docker-compose ps worker

# Check RabbitMQ connection
docker-compose logs worker | grep "RabbitMQ"

# Check queue has messages
curl -u guest:guest http://localhost:15672/api/queues/%2F/withdrawal.process
```

**Solutions:**
1. Restart worker: `docker-compose restart worker`
2. Check RabbitMQ is up: `docker-compose ps rabbitmq`
3. Check RABBITMQ_URL in .env
4. Check worker logs for errors

---

## Summary

This comprehensive test guide covers:
- ✅ Unit tests for individual components
- ✅ Integration tests for full flows
- ✅ End-to-end tests for user scenarios
- ✅ Edge cases and error scenarios
- ✅ Performance tests
- ✅ Security tests
- ✅ Complete checklist
- ✅ Troubleshooting guide

**All test scenarios should pass before production deployment.**

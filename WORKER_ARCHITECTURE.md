# Worker Architecture - Withdrawal Processing

## Overview
The worker is a separate background process that handles async operations that should NOT block HTTP requests, particularly withdrawal processing to banks.

---

## Why a Separate Worker Process?

### Problem: HTTP Timeout
Bank transfers can take 1-5 minutes to complete. If we processed withdrawals in the HTTP handler:
```
User requests withdrawal → Handler calls Paystack API → Wait 2-5 minutes → Timeout! ❌
```

### Solution: Queue + Worker Pattern
```
API Process:
  User requests → Lock funds → Queue message → Return "pending" ✓ (< 1 second)

Worker Process:
  Consume message → Call Paystack → Wait → Finalize → Notify user ✓
```

---

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                         API PROCESS                              │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ 1. POST /wallet/withdraw                              │    │
│  │    Amount: ₦50,000                                    │    │
│  │    Bank: GTBank (0123456789)                         │    │
│  └─────────────────┬──────────────────────────────────────┘    │
│                    │                                            │
│                    ▼                                            │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ 2. Validate (amount, account, balance)                │    │
│  └─────────────────┬──────────────────────────────────────┘    │
│                    │                                            │
│                    ▼                                            │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ 3. Database Transaction:                               │    │
│  │    - Lock wallet FOR UPDATE                            │    │
│  │    - LockFunds (locked += ₦50k)                       │    │
│  │    - CreateTransaction (status=pending)                │    │
│  │    - COMMIT                                            │    │
│  └─────────────────┬──────────────────────────────────────┘    │
│                    │                                            │
│                    ▼                                            │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ 4. Publish to RabbitMQ:                                │    │
│  │    Queue: withdrawal.process                           │    │
│  │    {                                                   │    │
│  │      transaction_id: "uuid",                          │    │
│  │      reference: "WD-ABC123",                          │    │
│  │      amount: 50000                                    │    │
│  │    }                                                  │    │
│  └─────────────────┬──────────────────────────────────────┘    │
│                    │                                            │
│                    ▼                                            │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ 5. Return Response (< 1 second):                       │    │
│  │    {                                                   │    │
│  │      status: "pending",                               │    │
│  │      reference: "WD-ABC123"                           │    │
│  │    }                                                  │    │
│  └────────────────────────────────────────────────────────┘    │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
                              │
                              │ RabbitMQ
                              │
┌──────────────────────────────┼────────────────────────────────────┐
│                         WORKER PROCESS                            │
├──────────────────────────────┴────────────────────────────────────┤
│                                                                   │
│  ┌────────────────────────────────────────────────────────┐     │
│  │ 1. Consume from Queue                                  │     │
│  │    Message: {transaction_id, reference, amount}       │     │
│  └─────────────────┬──────────────────────────────────────┘     │
│                    │                                             │
│                    ▼                                             │
│  ┌────────────────────────────────────────────────────────┐     │
│  │ 2. Fetch Transaction from Database                     │     │
│  │    - Get bank details from metadata                    │     │
│  │    - Verify status is still "pending"                  │     │
│  └─────────────────┬──────────────────────────────────────┘     │
│                    │                                             │
│                    ▼                                             │
│  ┌────────────────────────────────────────────────────────┐     │
│  │ 3. Call Paystack Transfer API                          │     │
│  │    provider.InitiateTransfer({                         │     │
│  │      amount: 50000,                                    │     │
│  │      account: "0123456789",                           │     │
│  │      bank: "058",                                      │     │
│  │      reference: "WD-ABC123"                           │     │
│  │    })                                                 │     │
│  └─────────────────┬──────────────────────────────────────┘     │
│                    │                                             │
│                    ▼                                             │
│  ┌────────────────────────────────────────────────────────┐     │
│  │ 4. Handle Response:                                    │     │
│  │                                                        │     │
│  │    If status == "success":                            │     │
│  │      → FinalizeWithdrawal(success=true)              │     │
│  │                                                        │     │
│  │    If status == "failed":                             │     │
│  │      → FinalizeWithdrawal(success=false)             │     │
│  │                                                        │     │
│  │    If status == "pending":                            │     │
│  │      → Wait for webhook or reconciliation            │     │
│  └─────────────────┬──────────────────────────────────────┘     │
│                    │                                             │
│                    ▼                                             │
│  ┌────────────────────────────────────────────────────────┐     │
│  │ 5. Save Transfer Code                                  │     │
│  │    Update transaction.external_reference              │     │
│  └────────────────────────────────────────────────────────┘     │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

---

## Worker Components

### 1. Withdrawal Processor
**Queue:** `withdrawal.process`

**Responsibilities:**
- Consume withdrawal messages from queue
- Fetch transaction details from database
- Call Paystack `InitiateTransfer` API
- Save transfer code
- Finalize if status is immediate (success/failed)

**Message Format:**
```json
{
  "transaction_id": "uuid-string",
  "user_id": "uuid-string",
  "amount_kobo": 50000,
  "reference": "WD-ABC123"
}
```

---

### 2. Reconciliation Worker
**Schedule:** Every 5 minutes

**Responsibilities:**
- Find pending withdrawals older than 10 minutes
- Call Paystack `VerifyTransfer` API
- Finalize based on current status

**Why Needed:**
Webhooks can be missed due to:
- Network failures
- Provider downtime
- Firewall blocks
- Server restarts during webhook delivery

**Query:**
```sql
SELECT * FROM wallet_transactions
WHERE type = 'withdrawal'
  AND status = 'pending'
  AND created_at < NOW() - INTERVAL '10 minutes'
```

---

### 3. Email Dispatcher
**Queue:** `email.dispatch`

**Responsibilities:**
- Send withdrawal receipts
- Send deposit confirmations
- Send status updates

**Status:** TODO (not yet implemented)

---

### 4. SMS Dispatcher
**Queue:** `sms.dispatch`

**Responsibilities:**
- Send OTP codes
- Send transaction alerts

**Status:** TODO (not yet implemented)

---

## Withdrawal Processing Flow

### Step-by-Step

#### Step 1: Consume Message
```go
mq.Consume(queue.QueueWithdrawalProcess, func(ctx, body []byte) error {
    var msg queue.WithdrawalMessage
    json.Unmarshal(body, &msg)
    
    // Process withdrawal...
})
```

#### Step 2: Fetch Transaction
```go
var txn models.WalletTransaction
db.Where("id = ?", transactionID).First(&txn)

// Idempotency check
if txn.Status != "pending" {
    return nil // Already processed
}
```

#### Step 3: Extract Bank Details
```go
var metadata map[string]interface{}
json.Unmarshal(txn.Metadata, &metadata)

bankCode := metadata["bank_code"].(string)
accountNumber := metadata["account_number"].(string)
```

#### Step 4: Initiate Transfer
```go
result, err := provider.InitiateTransfer(ctx, &payments.TransferRequest{
    Amount:        txn.Amount,
    AccountNumber: accountNumber,
    BankCode:      bankCode,
    Reference:     txn.Reference,
})
```

#### Step 5: Save Transfer Code
```go
db.Model(&models.WalletTransaction{}).
    Where("id = ?", transactionID).
    Update("external_reference", result.TransferCode)
```

#### Step 6: Finalize (If Immediate)
```go
switch result.Status {
case "success":
    walletService.FinalizeWithdrawal(ctx, txID, true, transferCode, "")
    
case "failed":
    walletService.FinalizeWithdrawal(ctx, txID, false, transferCode, reason)
    
case "pending":
    // Wait for webhook or reconciliation
}
```

---

## FinalizeWithdrawal Method

### Purpose
Completes or reverses a pending withdrawal based on transfer outcome.

### Parameters
```go
FinalizeWithdrawal(
    ctx context.Context,
    transactionID uuid.UUID,
    success bool,              // true = success, false = failed
    providerReference string,  // Paystack transfer_code
    failureReason string       // Only if failed
) error
```

### Success Flow
```go
// 1. Release locked funds (permanently debit wallet)
ReleaseFundsOnSuccess(userID, amount)
// Before: main=100k, locked=50k → After: main=50k, locked=0

// 2. Update transaction status
UPDATE wallet_transactions SET status='completed' WHERE id=...

// 3. Notify user
"Your withdrawal of ₦500.00 has been sent to your bank account"
```

### Failure Flow (Reversal)
```go
// 1. Release locked funds (return to available)
ReleaseFundsOnFailure(userID, amount)
// Before: main=100k, locked=50k → After: main=100k, locked=0

// 2. Update transaction status
UPDATE wallet_transactions SET status='failed' WHERE id=...

// 3. Notify user
"Your withdrawal could not be completed. Funds have been returned to your wallet"
```

### Idempotency
Safe to call multiple times:
```go
if txn.Status == "completed" || txn.Status == "failed" {
    return nil // Already finalized
}
```

---

## Error Handling

### Transient Errors (Retry)
These errors cause the message to be requeued:
```go
// Network timeout
return fmt.Errorf("transfer API timeout: %w", err)

// Provider temporarily unavailable
return fmt.Errorf("Paystack API error: %w", err)
```

### Permanent Errors (Don't Retry)
These errors acknowledge the message (don't requeue):
```go
// Invalid message format
log.Error("invalid JSON")
return nil // Don't requeue garbage

// Transaction not found
log.Error("transaction doesn't exist")
return nil // Don't requeue

// Already processed
log.Info("already finalized")
return nil // Idempotent
```

### Retry Strategy
RabbitMQ automatically retries failed messages with exponential backoff:
- Attempt 1: Immediate
- Attempt 2: After 1 minute
- Attempt 3: After 5 minutes
- Attempt 4: After 15 minutes
- After 5 attempts: Move to dead letter queue

---

## Reconciliation Safety Net

### Why Reconciliation?

**Webhook Failures:**
- Network outage during webhook delivery
- Server restart during webhook processing
- Firewall blocks webhook IP
- Provider webhook queue delay

**Solution:**
Reconciliation worker polls pending withdrawals every 5 minutes.

### How It Works

```go
// Every 5 minutes
ticker := time.NewTicker(5 * time.Minute)

// Find stale pending withdrawals (> 10 minutes old)
cutoff := time.Now().Add(-10 * time.Minute)
db.Where("status = ? AND created_at < ?", "pending", cutoff).Find(&txns)

// For each pending withdrawal
for _, txn := range txns {
    // Verify current status with Paystack
    result, _ := provider.VerifyTransfer(ctx, txn.ExternalReference)
    
    // Finalize based on actual status
    switch result.Status {
    case "success":
        walletService.FinalizeWithdrawal(ctx, txn.ID, true, ...)
    case "failed":
        walletService.FinalizeWithdrawal(ctx, txn.ID, false, ...)
    }
}
```

### Grace Period
Wait 10 minutes before reconciliation to:
- Give webhooks a chance to arrive first
- Avoid race conditions between webhook and reconciliation
- Reduce unnecessary API calls to Paystack

---

## Running the Worker

### Development
```bash
# Terminal 1: Start API
make run

# Terminal 2: Start Worker
go run cmd/worker/main.go
```

### Production (Docker)
```bash
docker-compose up -d
```

This starts:
- API process (port 8080)
- Worker process (background)
- PostgreSQL (port 5432)
- RabbitMQ (ports 5672, 15672)

---

## Monitoring & Debugging

### Check Worker Status
```bash
# View worker logs
docker-compose logs -f worker

# Check RabbitMQ queues
# Open: http://localhost:15672
# Login: guest / guest
# View queue: withdrawal.process
```

### Monitor Pending Withdrawals
```sql
SELECT
    id,
    reference,
    amount,
    status,
    external_reference,
    created_at,
    EXTRACT(EPOCH FROM (NOW() - created_at))/60 AS minutes_pending
FROM wallet_transactions
WHERE type = 'withdrawal' AND status = 'pending'
ORDER BY created_at DESC;
```

### Check Failed Withdrawals
```sql
SELECT
    id,
    reference,
    amount,
    status,
    metadata->'failure_reason' AS reason,
    created_at
FROM wallet_transactions
WHERE type = 'withdrawal' AND status = 'failed'
ORDER BY created_at DESC
LIMIT 10;
```

---

## Graceful Shutdown

Worker handles SIGINT/SIGTERM:
```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

// 1. Stop accepting new messages
// 2. Finish processing current message
// 3. Close queue connections
// 4. Close database connections
log.Info("Worker stopped gracefully")
```

**To stop worker:**
```bash
# Send SIGTERM
docker-compose stop worker

# Or send SIGINT
Ctrl+C
```

---

## Testing Worker Locally

### 1. Start Dependencies
```bash
docker-compose up -d postgres rabbitmq
```

### 2. Start API
```bash
go run cmd/api/main.go
```

### 3. Start Worker
```bash
go run cmd/worker/main.go
```

### 4. Trigger Withdrawal
```bash
# Login and get token
TOKEN=$(curl -s http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}' \
  | jq -r '.data.access_token')

# Initiate withdrawal
curl http://localhost:8080/api/v1/wallet/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount": 50000,
    "account_number": "0123456789",
    "account_name": "John Doe",
    "bank_code": "058"
  }'
```

### 5. Watch Logs
```bash
# Worker logs
tail -f logs/worker.log

# Check transaction status
psql -U propvest -d propvest -c \
  "SELECT reference, status, external_reference FROM wallet_transactions WHERE type='withdrawal' ORDER BY created_at DESC LIMIT 1;"
```

---

## Performance Optimization

### Queue Configuration
```go
// Prefetch count: How many messages to fetch at once
channel.Qos(
    1,     // prefetchCount (process 1 at a time for reliability)
    0,     // prefetchSize (0 = no limit)
    false, // global (false = per consumer)
)
```

### Concurrency
To process multiple withdrawals in parallel:
```bash
# Run multiple worker instances
docker-compose up --scale worker=3
```

Each worker processes messages independently.

---

## Troubleshooting

### Issue: Worker not consuming messages
**Check:**
1. RabbitMQ connection: `RABBITMQ_URL` in .env
2. Queue exists: Check RabbitMQ UI
3. Messages in queue: Check queue depth

### Issue: Withdrawals stuck in "pending"
**Check:**
1. Worker logs for errors
2. Run reconciliation manually
3. Check Paystack API status

### Issue: Duplicate processing
**Solution:** Already handled via idempotency checks
- Transaction status check before processing
- FinalizeWithdrawal checks status before updating

---

## Security Considerations

1. **Queue Access:** RabbitMQ requires authentication
2. **Message Validation:** Always validate message structure
3. **Transaction Verification:** Always verify with Paystack API
4. **Idempotency:** Safe to process same message multiple times
5. **Error Logging:** Don't log sensitive bank details

---

## Next Steps

1. ✅ Withdrawal processor implemented
2. ✅ Reconciliation worker implemented  
3. ⏳ Email dispatcher (TODO)
4. ⏳ SMS dispatcher (TODO)
5. ⏳ Metrics & monitoring (TODO)
6. ⏳ Dead letter queue handling (TODO)

---

## Summary

The worker architecture provides:
- ✅ Non-blocking withdrawal processing
- ✅ Automatic retries on transient failures
- ✅ Reconciliation safety net for missed webhooks
- ✅ Graceful shutdown
- ✅ Idempotent processing
- ✅ Clear error handling
- ✅ Production-ready reliability

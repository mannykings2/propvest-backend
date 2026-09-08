# Withdrawal System Architecture

## Overview

PropVest's withdrawal system implements a production-grade, webhook-driven architecture with locked balance management, idempotency guarantees, and automatic reconciliation.

---

## Key Design Decisions

### 1. **Locked Balance Pattern**

Instead of immediately debiting the wallet, we use a two-phase commit:

```
┌─────────────────────────────────────────────────────────────┐
│ TRADITIONAL APPROACH (❌ Problematic)                        │
├─────────────────────────────────────────────────────────────┤
│ User requests withdrawal                                    │
│ → Debit wallet immediately                                  │
│ → Call Paystack                                            │
│ → If Paystack fails: REVERSAL (complex, error-prone)      │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ LOCKED BALANCE APPROACH (✅ Production-Ready)                │
├─────────────────────────────────────────────────────────────┤
│ User requests withdrawal                                    │
│ → Lock funds (locked_balance += amount)                    │
│ → Call Paystack asynchronously                             │
│ → On success: debit main, clear lock                       │
│ → On failure: clear lock (funds return automatically)      │
└─────────────────────────────────────────────────────────────┘
```

**Why Locked Balance?**
- **Prevents double-spending** during pending withdrawals
- **Simplifies reversals** (just clear the lock)
- **Atomic operations** (one UPDATE statement)
- **Audit trail** (locked amount visible to users and admins)

---

## Database Schema

### Wallets Table

```sql
CREATE TABLE wallets (
    id UUID PRIMARY KEY,
    user_id UUID UNIQUE NOT NULL,
    main_balance BIGINT NOT NULL DEFAULT 0,      -- Total funds
    locked_balance BIGINT NOT NULL DEFAULT 0,    -- Reserved for withdrawals
    earnings_balance BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'NGN',
    
    CHECK (main_balance >= 0),
    CHECK (locked_balance >= 0),
    CHECK (locked_balance <= main_balance)       -- Can't lock more than you have
);

-- Available balance = main_balance - locked_balance (computed)
```

### Balance States

```
User has ₦100,000:
├─ main_balance: 100,000 kobo (total funds)
├─ locked_balance: 0 kobo (nothing pending)
└─ available: 100,000 kobo (can spend)

User requests ₦50,000 withdrawal:
├─ main_balance: 100,000 kobo (unchanged)
├─ locked_balance: 50,000 kobo (reserved)
└─ available: 50,000 kobo (100k - 50k)

Withdrawal succeeds:
├─ main_balance: 50,000 kobo (debited)
├─ locked_balance: 0 kobo (released)
└─ available: 50,000 kobo

Withdrawal fails:
├─ main_balance: 100,000 kobo (unchanged, automatic reversal)
├─ locked_balance: 0 kobo (released)
└─ available: 100,000 kobo (funds returned)
```

---

## Withdrawal Flow

### Complete Flow Diagram

```
POST /api/v1/wallet/withdraw
       │
       ▼
┌──────────────────────────────────────┐
│ 1. Validation                        │
│  ├─ Check amount >= MIN_WITHDRAWAL   │
│  ├─ Check amount <= MAX_WITHDRAWAL   │
│  ├─ Validate bank account format     │
│  └─ Check no pending withdrawals     │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│ 2. Pre-flight Check (Optional)       │
│  └─ ResolveAccountNumber()           │
│     └─ Verify account name matches   │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│ 3. Database Transaction (ACID)       │
│  ├─ Lock wallet row (FOR UPDATE)    │
│  ├─ Check available_balance          │
│  ├─ Lock funds (locked += amount)   │
│  ├─ Create WalletTransaction(PENDING)│
│  └─ COMMIT                           │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│ 4. Queue Job                         │
│  └─ Publish to QueueWithdrawalProcess│
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│ 5. Return Response                   │
│  └─ Status: PENDING                  │
└──────────────────────────────────────┘
       │
       ├─────────────────────┬─────────────────────┐
       │                     │                     │
       ▼                     ▼                     ▼
┌─────────────┐    ┌──────────────┐    ┌──────────────┐
│   Worker    │    │   Webhook    │    │ Reconciler   │
│  (async)    │    │ (real-time)  │    │  (fallback)  │
└─────────────┘    └──────────────┘    └──────────────┘
       │                     │                     │
       ▼                     ▼                     ▼
InitiateTransfer    VerifySignature     Poll Pending
       │                     │            (every 5 min)
       ▼                     ▼                     │
Save transfer_code   Find Withdrawal              │
       │                     │                     │
       └──────────┬──────────┴─────────────────────┘
                  ▼
         VerifyTransfer() → Paystack API
                  │
          ┌───────┴───────┐
          │               │
          ▼               ▼
      SUCCESS          FAILED
          │               │
          ▼               ▼
DB Transaction      DB Transaction
├─ Release locked   ├─ Release locked
├─ Debit main       ├─ Keep main (reversal)
├─ Status=completed ├─ Status=failed
├─ Create Payment   ├─ Create Payment
└─ Notify user      └─ Notify user
```

---

## Component Breakdown

### 1. Handler Layer (`internal/handlers/wallet.go`)

```go
func (h *WalletHandler) RequestWithdrawal(c *gin.Context) {
    // 1. Extract user ID from JWT
    userID := getUserIDFromContext(c)
    
    // 2. Validate request body
    var req dto.WithdrawRequest
    c.ShouldBindJSON(&req) // Amount, BankCode, AccountNumber, AccountName
    
    // 3. Call service
    transaction, err := h.walletService.InitiateWithdrawal(ctx, userID, req)
    
    // 4. Return PENDING status immediately
    response.Success(c, http.StatusOK, transaction)
}
```

### 2. Service Layer (`internal/services/wallet_service.go`)

```go
func (s *walletService) InitiateWithdrawal(ctx, userID, req) (*dto.TransactionResponse, error) {
    // 1. Validate amount
    if req.Amount < s.cfg.MinWithdrawalAmount {
        return nil, apperrors.NewMinimumWithdrawalError(s.cfg.MinWithdrawalAmount)
    }
    
    // 2. Optional: Pre-flight account verification
    resolution, err := s.provider.ResolveAccountNumber(ctx, req.AccountNumber, req.BankCode)
    if !accountNameMatches(resolution.AccountName, req.AccountName) {
        return nil, apperrors.NewAccountNameMismatchError(...)
    }
    
    // 3. Database transaction
    var ledger *models.WalletTransaction
    err := s.db.Transaction(func(tx *gorm.DB) error {
        // Lock wallet row
        wallet, _ := s.walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
        
        // Lock funds
        err := s.walletRepo.LockFunds(ctx, userID, req.Amount, tx)
        
        // Create pending transaction
        ledger = &models.WalletTransaction{
            WalletID:  wallet.ID,
            UserID:    userID,
            Type:      "withdrawal",
            Amount:    req.Amount,
            Status:    "pending",
            Reference: generateReference(), // WD-ABC123
        }
        return tx.Create(ledger).Error
    })
    
    // 4. Queue for async processing
    s.mq.Publish(ctx, queue.QueueWithdrawalProcess, queue.WithdrawalMessage{
        TransactionID: ledger.ID,
        Reference:     ledger.Reference,
    })
    
    // 5. Return pending status
    return txnToResponse(ledger), nil
}
```

### 3. Repository Layer (`internal/repositories/wallet_repository.go`)

```go
// LockFunds reserves funds for pending withdrawal
func (r *walletRepository) LockFunds(ctx, userID, amount, tx) error {
    // Lock wallet row
    var wallet models.Wallet
    tx.Clauses(clause.Locking{Strength: "UPDATE"}).
       Where("user_id = ?", userID).
       First(&wallet)
    
    // Check available balance
    available := wallet.MainBalance - wallet.LockedBalance
    if available < amount {
        return ErrInsufficientFunds
    }
    
    // Increase locked_balance
    tx.Model(&models.Wallet{}).
       Where("user_id = ?", userID).
       Update("locked_balance", gorm.Expr("locked_balance + ?", amount))
    
    return nil
}

// ReleaseFundsOnSuccess permanently debits wallet
func (r *walletRepository) ReleaseFundsOnSuccess(ctx, userID, amount, tx) error {
    tx.Model(&models.Wallet{}).
       Where("user_id = ?", userID).
       Updates(map[string]interface{}{
           "main_balance":   gorm.Expr("main_balance - ?", amount),
           "locked_balance": gorm.Expr("locked_balance - ?", amount),
       })
    return nil
}

// ReleaseFundsOnFailure reverses lock without debiting
func (r *walletRepository) ReleaseFundsOnFailure(ctx, userID, amount, tx) error {
    tx.Model(&models.Wallet{}).
       Where("user_id = ?", userID).
       Update("locked_balance", gorm.Expr("locked_balance - ?", amount))
    return nil
}
```

### 4. Worker (`cmd/worker/main.go`)

```go
func processWithdrawal(ctx, msg) error {
    // 1. Fetch transaction
    txn, _ := walletRepo.GetTransactionByReference(ctx, msg.Reference)
    
    // 2. Initiate Paystack transfer
    result, _ := provider.InitiateTransfer(ctx, &TransferRequest{
        Amount:        msg.AmountKobo,
        AccountNumber: extractFromMetadata(txn.Metadata, "account_number"),
        BankCode:      extractFromMetadata(txn.Metadata, "bank_code"),
        Reference:     msg.Reference,
    })
    
    // 3. Save transfer code
    // (stored in payment record for status tracking)
    
    // 4. Status will be updated by webhook or reconciler
    return nil
}
```

### 5. Webhook Handler (`internal/handlers/wallet.go`)

```go
func (h *WalletHandler) HandleTransferWebhook(c *gin.Context) {
    // 1. Verify signature
    signature := c.GetHeader("X-Paystack-Signature")
    body, _ := c.GetRawData()
    if !h.walletService.VerifyWebhookSignature(signature, body) {
        return c.JSON(401, "Invalid signature")
    }
    
    // 2. Parse event
    var event PaystackTransferEvent
    json.Unmarshal(body, &event)
    
    // 3. Find withdrawal
    txn, _ := walletRepo.GetTransactionByReference(ctx, event.Reference)
    
    // 4. Idempotency check
    if txn.Status != "pending" {
        return c.JSON(200, "Already processed")
    }
    
    // 5. Verify with Paystack API (don't trust webhook alone!)
    result, _ := provider.VerifyTransfer(ctx, event.TransferCode)
    
    // 6. Finalize withdrawal
    err := walletService.FinalizeWithdrawal(ctx, txn.ID, result.Status)
    
    return c.JSON(200, "OK")
}
```

### 6. Finalization Logic (`internal/services/wallet_service.go`)

```go
func (s *walletService) FinalizeWithdrawal(ctx, txnID, status) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        // Fetch transaction
        txn, _ := s.walletRepo.GetTransactionByID(ctx, txnID)
        
        if status == "success" {
            // Release funds and debit wallet
            err := s.walletRepo.ReleaseFundsOnSuccess(ctx, txn.UserID, txn.Amount, tx)
            
            // Update transaction status
            s.walletRepo.UpdateTransactionStatus(ctx, txnID, "completed")
            
            // Notify user
            s.notifier.Notify(ctx, txn.UserID, "Withdrawal successful", ...)
            
        } else {
            // Release lock, return funds
            err := s.walletRepo.ReleaseFundsOnFailure(ctx, txn.UserID, txn.Amount, tx)
            
            // Update transaction status
            s.walletRepo.UpdateTransactionStatus(ctx, txnID, "failed")
            
            // Notify user
            s.notifier.Notify(ctx, txn.UserID, "Withdrawal failed", ...)
        }
        
        return nil
    })
}
```

---

## Idempotency Guarantees

### 1. **Withdrawal Initiation**
- Check for pending withdrawals before creating new one
- Use transaction reference as unique constraint

### 2. **Webhook Processing**
- Check transaction status before processing
- If already `completed` or `failed`, return success immediately
- Verify with Paystack API (don't trust webhook payload alone)

### 3. **Fund Locking**
- Database constraint: `locked_balance <= main_balance`
- Row-level locking with `SELECT FOR UPDATE`
- Atomic updates using `locked_balance = locked_balance + ?`

---

## Reconciliation Strategy

### Worker Tasks

**1. Payout Processor** (immediate)
- Consumes `QueueWithdrawalProcess`
- Calls `InitiateTransfer()` to Paystack
- Saves transfer code for tracking

**2. Reconciliation Worker** (every 5 minutes)
- Finds withdrawals with status `pending` older than 10 minutes
- Calls `VerifyTransfer()` for each
- Finalizes based on Paystack response
- Catches missed webhooks or failed deliveries

```go
func reconciliationWorker() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        // Find stale pending withdrawals
        txns, _ := walletRepo.ListTransactions(ctx, 
            uuid.Nil, // all users
            "withdrawal",
            "pending",
            100, 0)
        
        for _, txn := range txns {
            if time.Since(txn.CreatedAt) > 10*time.Minute {
                // Verify status with Paystack
                result, _ := provider.VerifyTransfer(ctx, txn.Reference)
                
                // Finalize based on result
                walletService.FinalizeWithdrawal(ctx, txn.ID, result.Status)
            }
        }
    }
}
```

---

## Error Handling

### Common Failure Scenarios

| Scenario | Handling |
|----------|----------|
| Insufficient balance | Reject before DB transaction |
| Invalid bank account | Reject during pre-flight check |
| Paystack API down | Worker retries with exponential backoff |
| Transfer fails | Automatic reversal via `ReleaseFundsOnFailure()` |
| Webhook lost | Reconciler picks up after 10 minutes |
| Duplicate webhook | Idempotency check prevents double-processing |

---

## Security Considerations

### 1. **Webhook Verification**
```go
// ALWAYS verify signature
if !VerifyWebhookSignature(signature, body) {
    return 401 // Reject forged webhooks
}

// ALWAYS verify with API (don't trust webhook payload)
result := provider.VerifyTransfer(ctx, reference)
```

### 2. **Withdrawal Limits**
```env
MIN_WITHDRAWAL_AMOUNT=50000     # ₦500 (covers fees)
MAX_WITHDRAWAL_AMOUNT=10000000  # ₦100,000 (fraud prevention)
```

### 3. **Rate Limiting**
- Max 3 withdrawal requests per hour per user
- Max 1 pending withdrawal at a time

### 4. **Audit Trail**
- Every withdrawal creates immutable `WalletTransaction`
- Locked balance visible in wallet response
- Payment records store raw Paystack responses

---

## Testing Strategy

### Unit Tests
- Repository locked balance operations
- Service withdrawal validation
- Error handling and reversals

### Integration Tests
- Full withdrawal flow with mock provider
- Webhook processing
- Idempotency guarantees

### E2E Tests (with Paystack Test Mode)
- Real account resolution
- Real transfer initiation
- Webhook delivery

---

## Monitoring & Alerts

### Key Metrics
- **Pending withdrawal duration**: Alert if > 30 minutes
- **Failed withdrawal rate**: Alert if > 5%
- **Locked balance total**: Monitor for anomalies
- **Webhook delivery rate**: Alert if < 95%

### Dashboards
- Real-time withdrawal status distribution
- Average completion time
- Daily withdrawal volume

---

## Future Enhancements

1. **Multi-currency support** (USD, GBP, etc.)
2. **Withdrawal batching** (reduce provider fees)
3. **Scheduled withdrawals** (user sets date/time)
4. **Two-factor authentication** for large withdrawals
5. **Admin approval workflow** for amounts > ₦500k

---

## Troubleshooting

### Withdrawal stuck in pending
1. Check worker logs for errors
2. Manually call `VerifyTransfer()` with reference
3. Run reconciliation worker manually

### Balance mismatch
1. Query locked_balance from database
2. Sum pending withdrawal amounts
3. Check audit trail in wallet_transactions

### Failed reversals
1. Check database constraints (locked <= main)
2. Verify transaction status transitions
3. Manual reversal: `UPDATE wallets SET locked_balance = locked_balance - ? WHERE user_id = ?`

---

## References

- [Wallet Design](docs/05-Modules/5.4-WALLET_DESIGN.md)
- [Transaction Model](docs/02-Database/2.3-TRANSACTION_MODEL.md)
- [Payment Architecture](docs/05-Modules/5.1-PAYMENT_ARCHITECTURE.md)
- [Paystack Transfer API](https://paystack.com/docs/transfers/)

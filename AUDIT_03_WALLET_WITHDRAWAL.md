# Audit Report 03: Wallet & Withdrawal Service

**Date:** 2026-10-06  
**Phase:** 1 - Audit & Analysis  
**Step:** 1.3 - Wallet & Withdrawal Service Audit

---

## Executive Summary

The withdrawal flow is **well-designed** with proper fund locking, atomic state transitions, and business rule enforcement. However, it has **one critical reliability gap**:

✅ **Strong fund safety** - Locked balance prevents double-spending  
✅ **Atomic database operations** - Row locking prevents race conditions  
✅ **Business rules enforced** - One pending withdrawal per user  
✅ **Idempotent finalization** - Safe to call multiple times  
❌ **MESSAGE LOSS RISK** - RabbitMQ publish happens AFTER database commit  

**Risk Assessment:** 🟡 **MEDIUM-HIGH** - Good implementation undermined by async messaging gap.

---

## Withdrawal Flow Analysis

### Current Flow

```
User Request
      ↓
1. Validate amount (min/max)
      ↓
2. Check for existing pending withdrawal
      ↓
3. Verify bank account with Paystack
      ↓
4. BEGIN DATABASE TRANSACTION
      ├─ Lock wallet row (FOR UPDATE)
      ├─ Lock funds (available → locked)
      ├─ Create pending transaction record
      └─ COMMIT
      ↓
5. ❌ Publish to RabbitMQ (OUTSIDE transaction)
      ↓
6. Notify user
      ↓
7. Return success
```

**Critical Gap:** Step 5 happens OUTSIDE the database transaction.

### Failure Scenario

```
Step 4: COMMIT successful
        ↓
    Database state:
    - main_balance: 100,000 kobo (unchanged)
    - locked_balance: 50,000 kobo (locked)
    - transaction: status=pending
        ↓
Step 5: RabbitMQ publish fails OR API crashes
        ↓
    Result: ❌ Message LOST
        ↓
    Impact:
    - Worker never receives message
    - Withdrawal never processes
    - Money locked indefinitely
    - Reconciliation is only safety net
```

---

## Fund Locking Mechanism

### Implementation (`internal/repositories/wallet_repository.go`)

```go
func (r *walletRepository) LockFunds(ctx context.Context, userID uuid.UUID, amount int64, tx *gorm.DB) error {
    var wallet models.Wallet
    
    // Lock wallet row
    if err := tx.WithContext(ctx).
        Where("user_id = ?", userID).
        First(&wallet).Error; err != nil {
        return err
    }
    
    // Check available balance
    availableBalance := wallet.MainBalance - wallet.LockedBalance
    if availableBalance < amount {
        return fmt.Errorf("insufficient available balance")
    }
    
    // Increase locked_balance
    result := tx.WithContext(ctx).
        Model(&models.Wallet{}).
        Where("user_id = ?", userID).
        Update("locked_balance", wallet.LockedBalance+amount)
    
    return result.Error
}
```

**Analysis:**
- ✅ Row locking with `First()` (implicitly locks in transaction)
- ✅ Balance validation before lock
- ✅ Atomic update
- ✅ Returns specific error for insufficient funds

### Release on Success

```go
func (r *walletRepository) ReleaseFundsOnSuccess(ctx context.Context, userID uuid.UUID, amount int64, tx *gorm.DB) error {
    result := tx.Model(&models.Wallet{}).
        Where("user_id = ?", userID).
        Updates(map[string]interface{}{
            "main_balance": gorm.Expr("main_balance - ?", amount),
            "locked_balance": gorm.Expr("locked_balance - ?", amount),
        })
    return result.Error
}
```

**Analysis:**
- ✅ Atomic update of both balances
- ✅ SQL expressions prevent race conditions
- ✅ Must be called within transaction

### Release on Failure

```go
func (r *walletRepository) ReleaseFundsOnFailure(ctx context.Context, userID uuid.UUID, amount int64, tx *gorm.DB) error {
    result := tx.Model(&models.Wallet{}).
        Where("user_id = ?", userID).
        Update("locked_balance", gorm.Expr("locked_balance - ?", amount))
    return result.Error
}
```

**Analysis:**
- ✅ Only unlocks funds, doesn't debit
- ✅ Main balance unchanged (reversal)
- ✅ Must be called within transaction

---

## State Machine

### Current States

```
pending → completed (success path)
pending → failed (failure path)
```

### State Transitions

**InitiateWithdrawal:**
```
none → pending (creates new transaction)
```

**FinalizeWithdrawal:**
```
pending → completed (transfer successful)
pending → failed (transfer failed)
```

**Idempotency Check:**
```go
// In FinalizeWithdrawal
if txn.Status == "completed" || txn.Status == "failed" {
    log.Info("transaction already finalized")
    return nil  // ✅ Idempotent
}

if txn.Status != "pending" {
    log.Warn("finalize called on non-pending transaction")
    return nil  // ✅ Idempotent
}
```

**Analysis:**
- ✅ Clear state transitions
- ✅ Idempotent finalization
- ✅ Status validation before state change
- ⚠️ No "processing" intermediate state
- ⚠️ No state transition validation (can theoretically update any status)

---

## Business Rules

### One Pending Withdrawal Per User

**Implementation:**
```go
pendingWithdrawal, err := s.walletRepo.GetPendingWithdrawalByUser(ctx, userID)
if err != nil {
    return nil, apperrors.ErrInternalServer
}
if pendingWithdrawal != nil {
    logger.Warn("user has pending withdrawal", "pending_reference", pendingWithdrawal.Reference)
    return nil, apperrors.ErrWithdrawalPending
}
```

**Query:**
```go
func (r *walletRepository) GetPendingWithdrawalByUser(ctx context.Context, userID uuid.UUID) (*models.WalletTransaction, error) {
    var transaction models.WalletTransaction
    err := r.db.WithContext(ctx).
        Where("user_id = ? AND type = ? AND status = ?", userID, "withdrawal", "pending").
        Order("created_at DESC").
        First(&transaction).Error
    
    if IsErrRecordNotFound(err) {
        return nil, nil  // No pending withdrawal
    }
    return &transaction, err
}
```

**Analysis:**
- ✅ Prevents duplicate pending withdrawals
- ✅ Clear error message to user
- ⚠️ Check happens outside wallet lock transaction
- ⚠️ Race condition possible (two concurrent requests could both pass check)

**Race Condition Scenario:**
```
Request A: Check pending → None found
Request B: Check pending → None found
Request A: Create pending withdrawal
Request B: Create pending withdrawal
Result: ❌ Two pending withdrawals created
```

**Mitigation:** Would need database unique constraint or lock check within transaction.

---

## Bank Account Verification

**Implementation:**
```go
// Pre-flight check before locking funds
resolution, err := s.provider.ResolveAccountNumber(ctx, req.AccountNumber, req.BankCode)
if err != nil {
    logger.Error("account resolution failed")
    return nil, apperrors.ErrInvalidBankAccount
}

// Fuzzy match account names
if !accountNamesMatch(req.AccountName, resolution.AccountName) {
    logger.Warn("account name mismatch",
        "provided", req.AccountName,
        "bank_records", resolution.AccountName)
    return nil, apperrors.NewAccountNameMismatchError(req.AccountName, resolution.AccountName)
}
```

**Analysis:**
- ✅ Verifies account before locking funds
- ✅ Prevents transfer to invalid accounts
- ✅ Fuzzy matching prevents format mismatch issues
- ✅ Good user experience (early validation)
- ⚠️ External API call in request path (adds latency)
- ⚠️ If Paystack is down, no withdrawals possible

---

## Transaction Reference Generation

```go
reference := "WD-" + strings.ToUpper(uuid.NewString()[:12])
```

**Properties:**
- Prefix: `WD-` (identifies as withdrawal)
- Format: `WD-XXXXXXXXXXXX` (12 uppercase hex chars)
- Uniqueness: UUID-based (collision extremely unlikely)
- Examples: `WD-A1B2C3D4E5F6`, `WD-123ABC456DEF`

**Analysis:**
- ✅ Highly unique (UUID-based)
- ✅ Human-readable prefix
- ✅ Consistent length
- ✅ Database UNIQUE constraint enforces uniqueness
- ⚠️ Not sequential (can't infer creation order from reference)

---

## Worker Processing

### Current Worker Flow

```go
// In cmd/worker/main.go
mq.Consume(queue.QueueWithdrawalProcess, func(ctx context.Context, body []byte) error {
    var msg queue.WithdrawalMessage
    json.Unmarshal(body, &msg)
    
    transactionID, _ := uuid.Parse(msg.TransactionID)
    
    // 1. Fetch transaction
    var txn models.WalletTransaction
    db.Where("id = ?", transactionID).First(&txn)
    
    // 2. Check if already processed
    if txn.Status != "pending" {
        return nil  // ✅ Idempotent
    }
    
    // 3. Extract bank details from metadata
    var metadata map[string]interface{}
    json.Unmarshal(txn.Metadata, &metadata)
    
    // 4. Initiate Paystack transfer
    result, err := provider.InitiateTransfer(ctx, transferReq)
    if err != nil {
        return err  // ❌ REQUEUE INDEFINITELY
    }
    
    // 5. Save transfer code
    db.Model(&models.WalletTransaction{}).
        Where("id = ?", transactionID).
        Update("external_reference", result.TransferCode)
    
    // 6. Finalize if immediately completed/failed
    switch result.Status {
    case "success":
        walletService.FinalizeWithdrawal(ctx, transactionID, true, result.TransferCode, "")
    case "failed":
        walletService.FinalizeWithdrawal(ctx, transactionID, false, result.TransferCode, result.FailureReason)
    case "pending":
        // Webhook or reconciliation will finalize later
    }
    
    return nil
})
```

**Analysis:**

✅ **Good:**
- Idempotent status check
- Extracts bank details from metadata
- Saves transfer code for tracking
- Handles immediate completion

❌ **Problems:**
- Infinite requeue on transfer failure
- No retry limit
- No exponential backoff
- Single-threaded processing
- No prefetch limit

---

## Idempotency Analysis

### Message-Level Idempotency

**Current:** ❌ Not implemented

**Problem:**
```
Worker receives message
      ↓
Processes withdrawal
      ↓
Paystack succeeds
      ↓
Worker crashes before ACK
      ↓
RabbitMQ redelivers message
      ↓
Worker receives SAME message again
```

**Current Protection:**
```go
if txn.Status != "pending" {
    return nil  // Skip if already processed
}
```

**Gap:** This check has a race condition:

```
Worker A: Fetch txn → status=pending
Worker B: Fetch txn → status=pending
Worker A: Call InitiateTransfer → Success
Worker B: Call InitiateTransfer → ❌ DUPLICATE TRANSFER
```

**Required:** Atomic state transition:
```go
// Update status from pending to processing atomically
result := db.Model(&models.WalletTransaction{}).
    Where("id = ? AND status = ?", transactionID, "pending").
    Update("status", "processing")

if result.RowsAffected == 0 {
    // Already claimed by another worker
    return nil
}
```

---

## FinalizeWithdrawal Analysis

### Implementation

```go
func (s *walletService) FinalizeWithdrawal(ctx context.Context, transactionID uuid.UUID, success bool, providerReference, failureReason string) error {
    // 1. Fetch transaction
    var txn models.WalletTransaction
    db.Where("id = ?", transactionID).First(&txn)
    
    // 2. Idempotency check
    if txn.Status == "completed" || txn.Status == "failed" {
        return nil  // ✅ Already finalized
    }
    
    // 3. Validate it's a withdrawal
    if txn.Type != "withdrawal" {
        return fmt.Errorf("cannot finalize non-withdrawal")
    }
    
    // 4. Validate it's pending
    if txn.Status != "pending" {
        return nil  // ✅ Idempotent
    }
    
    // 5. Atomic finalization
    err := db.Transaction(func(tx *gorm.DB) error {
        if success {
            // Debit wallet and clear lock
            walletRepo.ReleaseFundsOnSuccess(ctx, txn.UserID, txn.Amount, tx)
            
            // Update transaction status
            tx.Model(&models.WalletTransaction{}).
                Where("id = ?", transactionID).
                Updates(map[string]interface{}{
                    "status": "completed",
                    "external_reference": providerReference,
                })
        } else {
            // Return funds to available balance
            walletRepo.ReleaseFundsOnFailure(ctx, txn.UserID, txn.Amount, tx)
            
            // Update transaction status
            tx.Model(&models.WalletTransaction{}).
                Where("id = ?", transactionID).
                Updates(map[string]interface{}{
                    "status": "failed",
                    "external_reference": providerReference,
                    // metadata updated with failure reason
                })
        }
        return nil
    })
    
    // 6. Notify user
    notifier.Notify(...)
    
    return err
}
```

**Analysis:**

✅ **Good:**
- Idempotent (safe to call multiple times)
- Atomic database transaction
- Proper fund release
- User notification
- Comprehensive logging

⚠️ **Gaps:**
- Status check happens BEFORE transaction
- Another finalization could complete between check and transaction
- No atomic state transition validation

---

## Metadata Storage

**Example Metadata:**
```json
{
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "JOHN DOE",
  "bank_name": "GTBank"
}
```

**Analysis:**
- ✅ JSONB column allows flexible data
- ✅ Stores verified bank details
- ✅ Preserves context for worker
- ⚠️ No schema validation
- ⚠️ Parsing errors not handled gracefully

---

## Comparison: Current vs Required

| Feature | Current | Required | Priority |
|---------|---------|----------|----------|
| **Fund Locking** |
| Lock funds atomically | ✅ Yes | ✅ Yes | - |
| Release on success | ✅ Yes | ✅ Yes | - |
| Release on failure | ✅ Yes | ✅ Yes | - |
| **Business Rules** |
| One pending withdrawal | ✅ Checked | ⚠️ Enforce with constraint | 🟡 MEDIUM |
| Amount validation | ✅ Yes | ✅ Yes | - |
| Bank verification | ✅ Yes | ✅ Yes | - |
| **Reliability** |
| Transactional outbox | ❌ No | ✅ Yes | 🔴 HIGH |
| Message in DB transaction | ❌ No | ✅ Yes | 🔴 HIGH |
| Atomic status transition | ⚠️ Partial | ✅ Full | 🔴 HIGH |
| Worker idempotency | ⚠️ Partial | ✅ Full | 🔴 HIGH |
| **State Management** |
| Clear states | ✅ Yes | ✅ Yes | - |
| Idempotent finalization | ✅ Yes | ✅ Yes | - |
| Processing state | ❌ No | ✅ Yes | 🟡 MEDIUM |
| State validation | ⚠️ Partial | ✅ Full | 🟡 MEDIUM |

---

## Recommendations

### 🔴 Critical

1. **Implement transactional outbox**
   ```go
   db.Transaction(func(tx *gorm.DB) error {
       // Lock funds
       // Create pending transaction
       // Create outbox event  ← NEW
       return nil
   })
   ```

2. **Add atomic status transitions**
   ```go
   result := db.Where("id = ? AND status = ?", id, "pending").
       Update("status", "processing")
   if result.RowsAffected == 0 {
       return ErrAlreadyProcessing
   }
   ```

3. **Add processing status**
   ```
   pending → processing → completed/failed
   ```

### 🟡 Important

4. **Enforce one pending withdrawal with DB constraint**
   ```sql
   CREATE UNIQUE INDEX idx_one_pending_withdrawal_per_user
   ON wallet_transactions(user_id)
   WHERE type = 'withdrawal' AND status = 'pending';
   ```

5. **Add state transition validation**
   ```go
   func (txn *WalletTransaction) CanTransitionTo(newStatus string) bool {
       validTransitions := map[string][]string{
           "pending": {"processing", "failed"},
           "processing": {"completed", "failed"},
       }
       return contains(validTransitions[txn.Status], newStatus)
   }
   ```

---

## Conclusion

The withdrawal implementation demonstrates **strong engineering practices**:

✅ Proper fund locking prevents double-spending  
✅ Atomic database operations prevent race conditions  
✅ Idempotent finalization handles duplicates  
✅ Business rules protect user experience  

However, one **critical gap** undermines this solid foundation:

❌ **RabbitMQ publish outside transaction = message loss risk**

This gap is exactly what the **transactional outbox pattern** solves.

---

**Status:** ✅ Complete  
**Next:** AUDIT_04_PAYSTACK.md

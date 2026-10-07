# Wallet Repository Methods Reference

## Overview
Complete list of wallet repository methods available for withdrawal implementation.

---

## Basic Operations

### `Create(ctx, wallet) error`
Creates a new wallet during user registration.

### `FindByID(ctx, walletID) (*Wallet, error)`
Retrieves wallet by UUID.

### `FindByUserID(ctx, userID) (*Wallet, error)`
Retrieves user's wallet (one-to-one relationship).

### `Update(ctx, wallet) error`
Updates wallet record. Prefer `UpdateBalance()` for balance changes.

---

## Transaction Operations

### `CreateTransaction(ctx, txn) error`
Creates immutable ledger entry. Called for every balance change.

### `GetTransactionByID(ctx, transactionID) (*WalletTransaction, error)`
**NEW** - Fetches transaction by UUID. Used by worker to get withdrawal details.

### `GetTransactionByReference(ctx, reference) (*WalletTransaction, error)`
Fetches transaction by reference (e.g., "WD-ABC123"). For idempotency checks.

### `GetTransactions(ctx, walletID, limit, offset) ([]WalletTransaction, error)`
Paginated transaction history.

### `ListTransactions(ctx, userID, txType, status, limit, offset) ([]WalletTransaction, int64, error)`
Filtered, paginated transactions with total count. Optional filters for type and status.

### `TransactionExists(ctx, reference) (bool, error)`
Checks if transaction exists. More efficient than fetching full record.

### `UpdateTransactionStatus(ctx, transactionID, status) error`
Updates transaction status. Used by worker to mark withdrawals completed/failed.

---

## Locked Balance Operations
**For withdrawal flow - MUST be used within database transactions**

### `LockFunds(ctx, userID, amount, tx) error`
Reserves funds for pending withdrawal.

**Flow:**
```
Before: main=100k, locked=0, available=100k
After:  main=100k, locked=50k, available=50k
```

**Used when:** User initiates withdrawal

### `ReleaseFundsOnSuccess(ctx, userID, amount, tx) error`
Permanently debits wallet after successful withdrawal.

**Flow:**
```
Before: main=100k, locked=50k, available=50k
After:  main=50k, locked=0, available=50k
```

**Used when:** Transfer to bank succeeds

### `ReleaseFundsOnFailure(ctx, userID, amount, tx) error`
Returns locked funds to available balance after failed withdrawal (reversal).

**Flow:**
```
Before: main=100k, locked=50k, available=50k
After:  main=100k, locked=0, available=100k (funds returned)
```

**Used when:** Transfer to bank fails

---

## Balance Updates

### `UpdateBalance(ctx, walletID, mainBalance, earningsBalance) error`
Updates wallet balances with row-level locking. Use within transaction.

### `FindByUserIDForUpdate(ctx, userID, tx) (*Wallet, error)`
Fetches wallet with `SELECT FOR UPDATE` lock. Prevents concurrent modifications.

---

## Withdrawal-Specific

### `GetPendingWithdrawalByUser(ctx, userID) (*WalletTransaction, error)`
**NEW** - Checks if user has a pending withdrawal.

**Business Rule:** Users can only have ONE pending withdrawal at a time.

**Returns:**
- `*WalletTransaction` - Pending withdrawal found
- `nil` - No pending withdrawal (user can withdraw)
- `error` - Database error

**Used when:** Validating new withdrawal request

---

## Complete Withdrawal Flow Example

```go
// 1. Check for existing pending withdrawal
pending, err := repo.GetPendingWithdrawalByUser(ctx, userID)
if pending != nil {
    return ErrWithdrawalPending
}

// 2. Lock funds in transaction
err = repo.Transaction(ctx, func(tx *gorm.DB) error {
    // Lock funds
    if err := repo.LockFunds(ctx, userID, amount, tx); err != nil {
        return err
    }
    
    // Create pending transaction
    txn := &models.WalletTransaction{
        UserID: userID,
        Type: "withdrawal",
        Amount: amount,
        Status: "pending",
        Reference: "WD-" + uuid.NewString(),
    }
    return repo.CreateTransaction(ctx, txn)
})

// 3. Queue for worker processing
queue.Publish(ctx, "withdrawal.process", WithdrawalMessage{...})

// 4. Worker processes
txn, err := repo.GetTransactionByID(ctx, transactionID)

// 5a. On success
err = repo.Transaction(ctx, func(tx *gorm.DB) error {
    repo.ReleaseFundsOnSuccess(ctx, userID, amount, tx)
    repo.UpdateTransactionStatus(ctx, transactionID, "completed")
    return nil
})

// 5b. On failure (reversal)
err = repo.Transaction(ctx, func(tx *gorm.DB) error {
    repo.ReleaseFundsOnFailure(ctx, userID, amount, tx)
    repo.UpdateTransactionStatus(ctx, transactionID, "failed")
    return nil
})
```

---

## Database Schema

### Wallets Table
```sql
main_balance BIGINT       -- Total balance
locked_balance BIGINT     -- Reserved for pending withdrawals
earnings_balance BIGINT   -- Rental income
available_balance         -- Computed: main_balance - locked_balance
```

### Wallet Transactions Table
```sql
id UUID PRIMARY KEY
user_id UUID
wallet_id UUID
type VARCHAR              -- "withdrawal", "deposit", etc.
status VARCHAR            -- "pending", "completed", "failed"
amount BIGINT
balance_before BIGINT
balance_after BIGINT
reference VARCHAR UNIQUE  -- "WD-ABC123"
metadata JSONB            -- Bank details, etc.
```

---

## Safety Rules

1. **Always use transactions** for balance operations
2. **Lock wallet row** before modifying balance (`FindByUserIDForUpdate`)
3. **Never directly update main_balance** - use locked balance flow
4. **Transaction records are immutable** - never UPDATE or DELETE
5. **Check for pending withdrawals** before creating new ones
6. **Verify locked_balance constraints** in database (locked <= main)

---

## Testing Patterns

### Test Success Flow
```go
// User has ₦100k, withdraws ₦50k
repo.LockFunds(ctx, userID, 50000, tx)
// main=100k, locked=50k, available=50k

repo.ReleaseFundsOnSuccess(ctx, userID, 50000, tx)
// main=50k, locked=0, available=50k
```

### Test Failure Flow (Reversal)
```go
repo.LockFunds(ctx, userID, 50000, tx)
// main=100k, locked=50k, available=50k

repo.ReleaseFundsOnFailure(ctx, userID, 50000, tx)
// main=100k, locked=0, available=100k (reverted)
```

### Test Duplicate Prevention
```go
pending, err := repo.GetPendingWithdrawalByUser(ctx, userID)
if pending != nil {
    t.Error("should prevent duplicate pending withdrawal")
}
```

---

## Error Handling

- `ErrInsufficientFunds` - Not enough available balance
- `ErrWithdrawalPending` - User already has pending withdrawal
- `gorm.ErrRecordNotFound` - Transaction/wallet not found
- Database errors - Wrapped with context

---

## Next Steps

Use these methods to implement:
1. Service layer validation (Step 6)
2. Worker withdrawal processor (Step 7)
3. Reversal logic (Step 8)

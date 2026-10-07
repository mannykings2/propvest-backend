# Audit Report 01: Database Layer

**Date:** 2026-10-06  
**Phase:** 1 - Audit & Analysis  
**Step:** 1.1 - Database Layer Audit

---

## Executive Summary

PropVest uses a **production-grade database architecture** with:
- ✅ Versioned SQL migrations (golang-migrate)
- ✅ GORM as ORM
- ✅ PostgreSQL as database
- ✅ Proper transaction handling
- ✅ Foreign key constraints
- ✅ Unique constraints for idempotency
- ✅ Indexes for performance
- ✅ Updated_at triggers
- ✅ Immutable ledger pattern (wallet_transactions)

The system is **well-architected** and **suitable as a foundation** for outbox pattern implementation.

---

## Database Connection & Migrations

### Current Implementation

**File:** `internal/database/database.go`

**Migration System:**
- Uses `golang-migrate/migrate` library
- Migrations located in `internal/database/migrations/`
- File naming: `NNNNNN_description.up.sql` and `NNNNNN_description.down.sql`
- Version tracking via `migrations_schema_version` table
- Rollback support for every migration

**Connection Pool Configuration:**
```go
DB.SetMaxOpenConns(cfg.DBMaxOpenConns)
DB.SetMaxIdleConns(cfg.DBMaxIdleConns)
DB.SetConnMaxLifetime(parsed from config, default 1 hour)
DB.SetConnMaxIdleTime(parsed from config, default 15 minutes)
```

**Production Features:**
- ✅ Connection pooling
- ✅ Configurable pool settings
- ✅ Dirty state detection
- ✅ Idempotent migrations
- ✅ Explicit SQL (no AutoMigrate in production)

---

## Existing Migrations

**Total Migrations:** 13

| Migration | Purpose | Key Tables/Features |
|-----------|---------|---------------------|
| 000001 | Initial schema | users, wallets, wallet_transactions, properties |
| 000002 | Refresh tokens | refresh_tokens table |
| 000003 | OTP verification | otp_verifications table |
| 000004 | User avatar & email | avatar, email_verified columns |
| 000005 | Wallet currency | currency column on wallets |
| 000006 | Refresh token timestamps | created_at, updated_at on refresh_tokens |
| 000007 | **Extended wallet_transactions** | user_id, external_reference, idempotency_key, metadata |
| 000008 | Verification tokens | verification_tokens table |
| 000009 | Payments | payments table |
| 000010 | Investments | investments table |
| 000011 | Notifications/Documents/Audit | notifications, property_documents, audit_logs |
| 000012 | **Locked balance** | locked_balance on wallets (fund reservation) |
| 000013 | Payment events | payment_events table |

**Critical Migrations for Outbox:**
- **Migration 000007** already implements partial idempotency via `idempotency_key` column
- **Migration 000012** implements proper fund locking for withdrawals

---

## Core Tables

### 1. Users Table

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_code VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    phone VARCHAR(20) UNIQUE,
    password_hash TEXT NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    kyc_status VARCHAR(50) NOT NULL DEFAULT 'pending',
    role VARCHAR(50) NOT NULL DEFAULT 'investor',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);
```

**Indexes:**
- `idx_users_email` on `email`
- `idx_users_role` on `role`
- `idx_users_deleted_at` on `deleted_at`

**Constraints:**
- UNIQUE on `user_code`, `email`, `phone`

---

### 2. Wallets Table

```sql
CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    main_balance BIGINT NOT NULL DEFAULT 0 CHECK (main_balance >= 0),
    locked_balance BIGINT NOT NULL DEFAULT 0,  -- Added in migration 000012
    earnings_balance BIGINT NOT NULL DEFAULT 0 CHECK (earnings_balance >= 0),
    virtual_acct_no VARCHAR(50),
    virtual_bank VARCHAR(100),
    currency VARCHAR(10) NOT NULL DEFAULT 'NGN',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

**Indexes:**
- `idx_wallets_user_id` UNIQUE on `user_id` (1:1 relationship)
- `idx_wallets_locked_balance` on `locked_balance` WHERE `locked_balance > 0`

**Constraints:**
- UNIQUE on `user_id` (enforces one wallet per user)
- CHECK `main_balance >= 0`
- CHECK `earnings_balance >= 0`
- CHECK `locked_balance >= 0`
- CHECK `locked_balance <= main_balance` (cannot lock more than available)

**Financial Semantics:**
- `main_balance`: Total balance including locked funds
- `locked_balance`: Funds reserved for pending withdrawals
- `available_balance`: Computed as `main_balance - locked_balance`
- `earnings_balance`: Separate tracking for investment returns

---

### 3. Wallet Transactions Table (LEDGER)

```sql
CREATE TABLE wallet_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id UUID NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
    user_id UUID,  -- Added in migration 000007
    type VARCHAR(50) NOT NULL,
    amount BIGINT NOT NULL,
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    reference VARCHAR(255) UNIQUE NOT NULL,
    external_reference VARCHAR(255),  -- Added in migration 000007
    idempotency_key VARCHAR(255),     -- Added in migration 000007
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'completed',
    metadata JSONB,  -- Added in migration 000007
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

**Indexes:**
- `idx_wallet_transactions_wallet_id` on `wallet_id`
- `idx_wallet_transactions_user_id` on `user_id`
- `idx_wallet_transactions_reference` UNIQUE on `reference`
- `idx_wallet_transactions_external_reference` on `external_reference`
- `idx_wallet_transactions_idempotency_key` UNIQUE on `idempotency_key` WHERE NOT NULL
- `idx_wallet_transactions_type` on `type`
- `idx_wallet_transactions_created_at` on `created_at DESC`

**Transaction Types:**
- `deposit`
- `withdrawal`
- `investment`
- `refund`
- `reversal`
- `fee`
- `rental_income`
- `transfer`

**Status Values:**
- `pending`
- `processing`
- `completed`
- `failed`
- `reversed`

**Ledger Properties:**
- ✅ **Immutable**: Rows are NEVER updated or deleted once completed
- ✅ **Append-only**: Corrections are new compensating rows
- ✅ **Audit trail**: Every balance change creates one row
- ✅ **Double-entry semantics**: balance_before + amount = balance_after

**Existing Idempotency:**
- `reference`: Internal unique reference (e.g., "WD-ABC123")
- `idempotency_key`: Client-supplied key for deduplication
- `external_reference`: Paystack/provider reference

---

## Transaction Patterns

### Current Withdrawal Flow

```sql
BEGIN TRANSACTION;

-- 1. Lock wallet row
SELECT * FROM wallets WHERE user_id = ? FOR UPDATE;

-- 2. Lock funds
UPDATE wallets 
SET locked_balance = locked_balance + ?
WHERE user_id = ? 
  AND (main_balance - locked_balance) >= ?;

-- 3. Create pending ledger entry
INSERT INTO wallet_transactions (
    wallet_id, user_id, type, amount,
    balance_before, balance_after,
    reference, status, metadata
) VALUES (..., 'pending', ...);

COMMIT;

-- 4. Queue message to RabbitMQ (OUTSIDE transaction)
```

**⚠️ FAILURE POINT IDENTIFIED:**

The RabbitMQ publish happens **AFTER** the database commit. If RabbitMQ is down or the API crashes after commit, the withdrawal message is **lost**.

---

### Current Deposit Flow

Based on webhook handling, deposits follow:

```sql
BEGIN TRANSACTION;

-- 1. Check idempotency_key
SELECT * FROM wallet_transactions WHERE idempotency_key = ?;

-- 2. If not exists, create ledger entry and credit wallet
INSERT INTO wallet_transactions (...);
UPDATE wallets SET main_balance = main_balance + ? WHERE id = ?;

COMMIT;
```

✅ This flow is **safer** because it's triggered by webhook, not API request.

---

## Model Definitions

### Wallet Model

**File:** `internal/models/wallet.go`

```go
type Wallet struct {
    ID              uuid.UUID
    UserID          uuid.UUID
    MainBalance     int64   // kobo (smallest unit)
    LockedBalance   int64   // reserved for pending withdrawals
    EarningsBalance int64   // investment returns
    VirtualAcctNo   *string
    VirtualBank     *string
    Currency        string  // default 'NGN'
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

// Computed properties
func (w *Wallet) AvailableBalance() int64 {
    return w.MainBalance - w.LockedBalance
}

func (w *Wallet) CanWithdraw(amount int64) bool {
    return w.AvailableBalance() >= amount && amount > 0
}
```

---

### WalletTransaction Model

```go
type WalletTransaction struct {
    ID                uuid.UUID
    WalletID          uuid.UUID
    UserID            uuid.UUID
    Type              string  // deposit|withdrawal|investment|etc
    Amount            int64
    BalanceBefore     int64
    BalanceAfter      int64
    Reference         string  // internal unique reference
    ExternalReference *string // Paystack reference
    IdempotencyKey    *string // client idempotency key
    Description       string
    Status            string  // pending|processing|completed|failed
    Metadata          datatypes.JSON
    CreatedAt         time.Time
}
```

---

## Existing Idempotency Mechanisms

### 1. Transaction Reference

**Field:** `reference`  
**Uniqueness:** Database UNIQUE constraint  
**Purpose:** Internal deduplication  
**Example:** `"WD-ABC123DEF456"`

**Current Behavior:**
- Generated by API: `"WD-" + strings.ToUpper(uuid.NewString()[:12])`
- Guaranteed unique by database constraint
- Cannot create duplicate withdrawal with same reference

---

### 2. Idempotency Key

**Field:** `idempotency_key`  
**Uniqueness:** UNIQUE WHERE NOT NULL  
**Purpose:** Client-side retry deduplication  
**Current Usage:** Limited (not enforced at API level yet)

**Database Schema:**
```sql
CREATE UNIQUE INDEX idx_wallet_transactions_idempotency_key
    ON wallet_transactions(idempotency_key)
    WHERE idempotency_key IS NOT NULL;
```

---

### 3. External Reference

**Field:** `external_reference`  
**Purpose:** Paystack transfer code tracking  
**Current Usage:** Set by worker after `InitiateTransfer()`

---

## Constraints & Invariants

### Database-Level Invariants

1. **One wallet per user:**
   ```sql
   UNIQUE (user_id) on wallets table
   ```

2. **Non-negative balances:**
   ```sql
   CHECK (main_balance >= 0)
   CHECK (earnings_balance >= 0)
   CHECK (locked_balance >= 0)
   ```

3. **Locked cannot exceed main:**
   ```sql
   CHECK (locked_balance <= main_balance)
   ```

4. **Unique transaction references:**
   ```sql
   UNIQUE (reference) on wallet_transactions
   ```

5. **Unique idempotency keys (when present):**
   ```sql
   UNIQUE (idempotency_key) WHERE idempotency_key IS NOT NULL
   ```

---

## Transaction Isolation

**GORM Default:** `READ COMMITTED` (PostgreSQL default)

**Current Transaction Usage:**
```go
db.Transaction(func(tx *gorm.DB) error {
    // All operations use tx, not db
    // Automatic ROLLBACK on error
    // Automatic COMMIT on nil return
})
```

**Row Locking:**
```go
// FindByUserIDForUpdate locks the wallet row
wallet, err := walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
// Uses: SELECT ... FOR UPDATE
```

✅ **Proper pessimistic locking** prevents concurrent withdrawal race conditions.

---

## Analysis & Findings

### ✅ Strengths

1. **Proper migration system** with rollback support
2. **Immutable ledger** (wallet_transactions)
3. **Existing idempotency infrastructure** (idempotency_key column)
4. **Fund locking mechanism** (locked_balance)
5. **Atomic database transactions** with proper row locking
6. **Database constraints** enforce critical invariants
7. **Good indexing** for query performance
8. **Denormalized user_id** in transactions for efficient queries

### ⚠️ Gaps & Risks

1. **❌ No outbox table** - Events can be lost if RabbitMQ is unavailable
2. **❌ RabbitMQ publish outside DB transaction** - Creates reliability gap
3. **⚠️ Idempotency key not enforced at API level** - Column exists but not used
4. **⚠️ No pending withdrawal state machine enforcement** - Status transitions not validated
5. **⚠️ No dedicated idempotency tracking table** - Relies on ledger uniqueness
6. **⚠️ Infinite Nack requeue** - Worker requeues failed messages indefinitely

### 🔧 Required Changes

1. **Create outbox_events table** for transactional event storage
2. **Move RabbitMQ publish inside DB transaction** via outbox pattern
3. **Add API-level idempotency middleware** for withdrawal requests
4. **Add state transition validation** for withdrawal status changes
5. **Add retry limits and DLQ** for failed message processing
6. **Add stale event recovery** for crashed dispatcher scenarios

---

## Database Schema Readiness for Outbox

### Existing Foundation

✅ **UUID support** - `gen_random_uuid()` available  
✅ **JSONB support** - Used for metadata storage  
✅ **Partial indexes** - Already used (e.g., locked_balance WHERE > 0)  
✅ **Row-level locking** - `FOR UPDATE` in use  
✅ **Timestamp tracking** - All tables have created_at/updated_at  
✅ **Transaction support** - ACID guarantees working  

### Required Additions

**New Table:** `outbox_events`

Proposed columns:
- `id` - UUID primary key
- `event_type` - VARCHAR (e.g., 'withdrawal.process')
- `aggregate_type` - VARCHAR (e.g., 'wallet_transaction')
- `aggregate_id` - UUID (reference to business entity)
- `payload` - JSONB (message body)
- `status` - VARCHAR (pending|claimed|published|failed)
- `attempts` - INTEGER (retry count)
- `available_at` - TIMESTAMP (for delayed retry)
- `claimed_at` - TIMESTAMP (when dispatcher claimed)
- `published_at` - TIMESTAMP (when successfully published)
- `last_error` - TEXT (failure reason)
- `created_at` - TIMESTAMP
- `updated_at` - TIMESTAMP

**Required Indexes:**
- `idx_outbox_events_status_available` on `(status, available_at)` for claiming
- `idx_outbox_events_aggregate` on `(aggregate_type, aggregate_id)` for queries
- `idx_outbox_events_created_at` on `created_at` for chronological processing

**Required Constraints:**
- CHECK `attempts >= 0`
- CHECK `status IN ('pending', 'claimed', 'published', 'failed')`

---

## Migration Strategy

### Approach

**Option A: Single Migration** (Recommended)
- Create outbox_events table in one migration
- Add all indexes and constraints
- Clean and focused

**Option B: Incremental Migrations**
- Migration 1: Basic table
- Migration 2: Indexes
- Migration 3: Constraints
- More complex, not necessary

**Recommendation:** Use **Option A** - Single comprehensive migration

### Migration Number

**Next Available:** `000014_create_outbox_events.up.sql`

### Rollback Considerations

✅ **Safe to rollback** - New table, no data dependencies  
✅ **No FK constraints to existing tables** - Independent  
⚠️ **Code must handle missing table gracefully** during rollback  

---

## Compatibility Assessment

### GORM Compatibility

✅ **Outbox model can use standard GORM patterns:**
```go
type OutboxEvent struct {
    ID            uuid.UUID
    EventType     string
    AggregateType string
    AggregateID   uuid.UUID
    Payload       datatypes.JSON
    Status        string
    Attempts      int
    AvailableAt   time.Time
    ClaimedAt     *time.Time
    PublishedAt   *time.Time
    LastError     *string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

✅ **Row locking works:**
```go
// Claim events using FOR UPDATE SKIP LOCKED
db.Raw(`
    SELECT * FROM outbox_events
    WHERE status = 'pending' 
      AND available_at <= NOW()
    ORDER BY created_at
    LIMIT ?
    FOR UPDATE SKIP LOCKED
`, batchSize).Scan(&events)
```

---

## Conclusion

The PropVest database layer is **well-designed and production-ready** as a foundation for implementing the outbox pattern. The existing infrastructure (migrations, constraints, row locking, immutable ledger) demonstrates mature engineering practices.

### Key Takeaways

1. ✅ **Strong foundation** - Migration system, constraints, and indexes in place
2. ✅ **Partial idempotency** - Infrastructure exists but not fully utilized
3. ✅ **Fund locking works** - Proper atomic operations for withdrawal safety
4. ❌ **Missing outbox** - Critical gap for reliable async processing
5. ⚠️ **Reliability window** - RabbitMQ publish outside transaction creates loss risk

### Next Steps

**Phase 1 Remaining:**
- ✅ Step 1.1: Database audit (THIS DOCUMENT)
- ⏭️ Step 1.2: RabbitMQ implementation audit
- ⏭️ Step 1.3: Wallet & withdrawal service audit
- ⏭️ Step 1.4: Paystack integration audit
- ⏭️ Step 1.5: Reconciliation audit
- ⏭️ Step 1.6: Notification services audit
- ⏭️ Step 1.7: Audit summary

---

**Document Status:** ✅ Complete  
**Next Audit:** AUDIT_02_RABBITMQ.md

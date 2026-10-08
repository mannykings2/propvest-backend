# Investment Module - Phase 1 Audit Results

**Date:** 2026-10-06  
**Phase:** 1 - Repository Audit  
**Status:** ✅ Complete  
**Duration:** ~1.5 hours

---

## Executive Summary

The existing codebase has a solid foundation for the Investment Module. All critical infrastructure is in place:
- ✅ Wallet locking and transaction support
- ✅ Property locking support
- ✅ Outbox pattern implementation
- ✅ Basic investment model and repository

**Key Findings:**
- Latest migration: `000020` → Next will be `000021`
- Property status: Uses `"active"` (not `"approved"`)
- Wallet has `MainBalance`, `EarningsBalance`, `LockedBalance` fields
- Transaction types include: deposit, withdrawal, investment, refund, etc.
- All locking mechanisms (`FindByIDForUpdate`) already exist

---

## 1. Investment Model Audit

### Current State (`internal/models/investment.go`)

**Existing Fields:**
```go
ID         uuid.UUID  // ✅
UserID     uuid.UUID  // ✅
PropertyID uuid.UUID  // ✅
Slots      int        // ✅
AmountKobo int64      // ✅
Status     string     // ✅
Reference  string     // ✅
CreatedAt  time.Time  // ✅
UpdatedAt  time.Time  // ✅
DeletedAt  gorm.DeletedAt  // ✅
Property   Property   // ✅ (for eager loading)
```

**Status Constants:**
```go
InvestmentStatusActive    = "active"
InvestmentStatusCompleted = "completed"
InvestmentStatusCancelled = "cancelled"
InvestmentStatusRefunded  = "refunded"
```

### Missing Fields (Need to Add):
- ❌ `UnitPriceKobo int64` - **CRITICAL** (price snapshot)
- ❌ `Currency string` - Currency tracking
- ❌ `IdempotencyKey *string` - Idempotency support
- ❌ `CancelledAt *time.Time` - Lifecycle tracking
- ❌ `CompletedAt *time.Time` - Lifecycle tracking
- ❌ `RefundedAt *time.Time` - Lifecycle tracking

---

## 2. Investment Repository Audit

### Current State (`internal/repositories/investment_repository.go`)

**Existing Methods:**
```go
✅ Create(ctx, inv, tx) error
✅ FindByID(ctx, id) (*Investment, error)
✅ ListByUser(ctx, userID, limit, offset) ([]Investment, int64, error)
✅ PortfolioSummary(ctx, userID) (totalInvested, count int64, error)
✅ CountAll(ctx) (int64, error)
✅ SumAll(ctx) (int64, error)
```

**Good Practices Found:**
- ✅ Accepts transaction (`tx *gorm.DB`) parameter in Create
- ✅ Context-aware
- ✅ Uses Preload for Property in FindByID
- ✅ Pagination support in ListByUser

### Missing Methods (Need to Add):
- ❌ `FindByIDForUpdate(ctx, id, tx)` - **CRITICAL** (row locking)
- ❌ `FindByUserAndIdempotencyKey(ctx, userID, key, tx)` - Idempotency
- ❌ `HasActiveInvestmentByUserAndProperty(ctx, tx, userID, propertyID)` - Investor count
- ❌ `ListByProperty(ctx, propertyID, query)` - Admin feature
- ❌ `ListAll(ctx, query)` - Admin feature
- ❌ `Metrics(ctx)` - Admin metrics

---

## 3. Wallet Repository Audit

### Current State (`internal/repositories/wallet_repository.go`)

**Existing Methods - ALL PERFECT FOR OUR NEEDS:**
```go
✅ FindByUserID(ctx, userID) (*Wallet, error)
✅ FindByUserIDForUpdate(ctx, userID, tx) (*Wallet, error) - **CRITICAL** ✅
✅ Update(ctx, wallet) error
✅ CreateTransaction(ctx, tx *WalletTransaction) error - **PERFECT** ✅
✅ TransactionExists(ctx, reference) (bool, error)
✅ GetTransactionByReference(ctx, reference) (*WalletTransaction, error)
✅ LockFunds(ctx, userID, amount, tx) error
✅ ReleaseFundsOnSuccess(ctx, userID, amount, tx) error
✅ ReleaseFundsOnFailure(ctx, userID, amount, tx) error
```

**Wallet Model Fields:**
```go
✅ MainBalance int64      - Spendable balance (CORRECT)
✅ EarningsBalance int64   - Separate earnings (CORRECT)
✅ LockedBalance int64     - Withdrawal locks (CORRECT)
✅ Currency string         - Currency tracking (CORRECT)
```

**Transaction Types (from code analysis):**
```go
deposit | withdrawal | investment | refund | reversal | fee | rental_income | transfer
```

**Sign Convention:**
- Amount is **always positive** in WalletTransaction
- Debit/credit determined by Type
- For investment: Type="investment", Amount=positive value

### No Changes Needed! ✅

---

## 4. Property Repository Audit

### Current State (`internal/repositories/property_repository.go`)

**Existing Methods:**
```go
✅ FindByID(ctx, id) (*Property, error)
✅ FindByIDForUpdate(ctx, id, tx) (*Property, error) - **CRITICAL** ✅
✅ Update(ctx, property) error
✅ UpdatePartial(ctx, id, updates map[string]any) error
✅ IncrementFunding(ctx, id, amount, units, investorDelta, tx) - **PERFECT!** ✅
```

**IncrementFunding Method (EXCELLENT):**
```go
// Updates: RaisedAmount, UnitsSold, InvestorCount
// Uses transaction
// Atomic operation
```

**Property Model Fields:**
```go
✅ Status string           - Investment state
✅ TargetAmount int64      - Funding goal
✅ RaisedAmount int64      - Current funding
✅ UnitPrice int64         - Price per slot
✅ TotalUnits int          - Total slots
✅ UnitsSold int           - Slots sold
✅ MinimumInvestment int64 - Minimum purchase
✅ InvestorCount int       - Unique investors
```

**Property Status Values:**
```go
"draft"     - Not published
"active"    - Accepting investments ✅ (NOT "approved")
"funded"    - 100% funded
"completed" - Investment period ended
```

**Property Methods:**
```go
✅ CanAcceptInvestments() bool - Returns: Status == "active" && !IsFullyFunded()
✅ IsFullyFunded() bool        - Checks if RaisedAmount >= TargetAmount
✅ IsPublic() bool             - Returns: Status in ["active", "funded", "completed"]
✅ IsDraft() bool              - Returns: Status == "draft"
```

### No Changes Needed! ✅

---

## 5. Outbox Repository Audit

### Current State (`internal/repositories/outbox_repository.go`)

**Existing Methods:**
```go
✅ CreateEvent(ctx, event, tx) error - **PERFECT** ✅
✅ ClaimEvents(ctx, workerID, batchSize) ([]OutboxEvent, error)
✅ MarkProcessed(ctx, eventID) error
✅ MarkFailed(ctx, eventID, error) error
✅ IncrementAttempts(ctx, eventID, nextAvailableAt, error) error
✅ ListPendingEvents(ctx, limit, offset) ([]OutboxEvent, int64, error)
```

**OutboxEvent Model:**
```go
✅ ID uuid.UUID
✅ AggregateID uuid.UUID   - Investment ID
✅ AggregateType string    - "investment"
✅ EventType string        - "investment.created", etc.
✅ Payload datatypes.JSON  - Event details
✅ Status string           - pending, processed, failed
✅ CreatedAt time.Time
```

**Event Type Constants (from outbox_event.go):**
```go
✅ EventTypePropertyPublished  = "property.published"
✅ EventTypePropertyFunded     = "property.funded"
✅ EventTypePropertyCompleted  = "property.completed"
```

### Need to Add Event Constants:
- ❌ `EventTypeInvestmentCreated = "investment.created"`
- ❌ `EventTypeInvestmentCancelled = "investment.cancelled"`
- ❌ `EventTypeInvestmentCompleted = "investment.completed"` (future)

---

## 6. Transaction Pattern Audit

### Existing Pattern (from wallet_service.go):
```go
err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    // All operations here
    // If error returned, automatic ROLLBACK
    // If no error, automatic COMMIT
    return nil
})
```

**This is PERFECT for our needs!** ✅

### Locking Order Verification:
- Wallet repository uses: `tx.Clauses(clause.Locking{Strength: "UPDATE"})`
- Property repository uses: `tx.Clauses(clause.Locking{Strength: "UPDATE"})`
- Both use the same mechanism ✅

**Decision:** Lock order will be **Property → Wallet** (as designed)

---

## 7. Migration Numbering

**Current Latest Migration:** `000020_add_soft_delete_to_property_media`

**Next Migration Will Be:** `000021_enhance_investments`

**Migration Files to Create:**
- `000021_enhance_investments.up.sql`
- `000021_enhance_investments.down.sql`

---

## 8. Idempotency Mechanism

### Current Status:
- ❌ No existing idempotency infrastructure found
- ❌ Need to implement via unique constraint on `(user_id, idempotency_key)`

### Decision:
**Use database constraint instead of separate table**
- Add `idempotency_key VARCHAR(255)` to investments table
- Add `UNIQUE(user_id, idempotency_key) WHERE idempotency_key IS NOT NULL`
- Simpler than separate table
- Leverages database atomicity

### Request Hash Storage:
- Store hash in investment metadata (JSON field) or
- Compare on application side (property_id + slots)

---

## 9. DTO Audit

### Current State (`internal/dto/investment_dto.go`)

**Existing DTOs:**
```go
✅ CreateInvestmentRequest {
    PropertyID uuid.UUID `json:"property_id" binding:"required"`
    Slots      int       `json:"slots" binding:"required,gt=0"`
}

✅ InvestmentResponse {
    ID, PropertyID, Slots, AmountKobo, Status, Reference, CreatedAt
    Property *PropertyResponse `json:"property,omitempty"`
}

✅ PortfolioSummaryResponse {
    TotalInvested, ActiveCount, WalletBalance, EarningsBalance
}
```

### Need to Add/Enhance:
- Add `UnitPriceKobo` to InvestmentResponse
- Add `Currency` to InvestmentResponse
- Create `InvestmentListQuery` (pagination, filters, sorting)
- Create `InvestmentListResponse` (investments + pagination)
- Create `AdminInvestmentListQuery` (includes user_id, property_id filters)
- Create `CancelInvestmentRequest` (reason field)
- Create `InvestmentMetricsResponse` (admin metrics)

---

## 10. Error Handling Audit

### Existing Error System (`internal/errors/errors.go`)

**Existing Error Codes:**
```go
✅ "validation_error"       - Input validation failed
✅ "business_logic_error"   - Business rule violation
✅ "not_found"              - Resource doesn't exist
✅ "forbidden"              - Authorization failed
✅ "insufficient_funds"     - Not enough balance ✅ (PERFECT!)
✅ "resource_unavailable"   - Property sold out, etc. ✅ (PERFECT!)
```

### Need to Add:
- ❌ `"idempotency_conflict"` - Same key, different request (maybe - check if exists)

**Method:**
```go
errors.NewAppError(message string, code string) *AppError
```

---

## Summary of Changes Needed

### Phase 2: Database & Model
1. Create migration 000021
2. Add 6 fields to Investment model
3. Add unique constraints and indexes

### Phase 3: Repository Layer
1. Add 6 new methods to InvestmentRepository
2. Implement all 6 methods

### Phase 4: DTO Enhancement
1. Enhance InvestmentResponse
2. Create 5 new DTO structs

### Phase 5-11: Service, Handler, Routes, Testing, Docs
- Build on top of existing solid foundation
- All infrastructure already in place
- Just need to implement business logic

---

## Risk Assessment

### Low Risk ✅
- Wallet infrastructure is perfect
- Property infrastructure is perfect
- Outbox pattern works
- Transaction pattern established
- Locking mechanisms exist

### Medium Risk ⚠️
- Idempotency implementation (new pattern)
- Concurrency testing required
- Complex transaction logic in CreateInvestment

### High Risk ❌
- None! Foundation is excellent

---

## Conclusion

**The existing codebase is EXCELLENT for building the Investment Module.**

All critical infrastructure exists:
- ✅ Row-level locking
- ✅ Transaction support
- ✅ Wallet balance management
- ✅ Property funding tracking
- ✅ Outbox pattern
- ✅ Error handling
- ✅ Logging infrastructure

**We can proceed confidently to Phase 2: Implementation**

---

## Phase 1 Task Completion

✅ 1.1 Review existing Investment model - COMPLETE  
✅ 1.2 Review Investment repository - COMPLETE  
✅ 1.3 Review Wallet repository - COMPLETE  
✅ 1.4 Review Property repository - COMPLETE  
✅ 1.5 Review Wallet model - COMPLETE  
✅ 1.6 Review Property model - COMPLETE  
✅ 1.7 Review Outbox repository - COMPLETE  
✅ 1.8 Check current migration number - COMPLETE (000020)  
✅ 1.9 Review idempotency mechanism - COMPLETE (need to create)  
✅ 1.10 Review transaction helper pattern - COMPLETE  

**Phase 1: ✅ COMPLETE**

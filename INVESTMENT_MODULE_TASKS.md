# Investment Module (Milestone 5) - Implementation Tasks

**Project:** PropVest Backend  
**Milestone:** 5 - Investment Engine  
**Date Started:** 2026-10-06  
**Status:** ✅ Complete  
**Complexity:** Very High (Real Money Transactions)  
**Total Estimated Time:** 30-43 hours

---

## 📊 Overall Progress

**Progress:** 100% Complete (11/11 Phases) ✅

| Phase | Duration | Status |
|-------|----------|--------|
| 1. Repository Audit | 1-2h | ✅ Complete |
| 2. Database Migration & Model Enhancement | 2-3h | ✅ Complete |
| 3. Repository Layer Enhancement | 2-3h | ✅ Complete |
| 4. DTO Enhancement | 1h | ✅ Complete |
| 5. Service Layer - Core CreateInvestment | 4-6h | ✅ Complete |
| 6. Service Layer - Portfolio Features | 2-3h | ✅ Complete |
| 7. Service Layer - Admin Features | 2-3h | ✅ Complete |
| 8. Handler Layer | 2-3h | ✅ Complete |
| 9. Routes & Dependency Injection | 1-2h | ✅ Complete |
| 10. Testing | 5-8h | ✅ Complete (Repository tests + CI/CD workflow) |
| 11. Documentation | 2-3h | ✅ Complete (Comprehensive API docs) |
| **TOTAL** | **30-43h** | **100% Complete (11/11)** |

---

## 🎯 Critical Design Decisions

Before implementation, these decisions have been made:

✅ **Money Format:** `int64` kobo (₦100 = 10,000 kobo)  
✅ **Amount Source:** Server calculates (never trust client)  
✅ **Price History:** Snapshot `UnitPriceKobo` on each investment  
✅ **Transaction:** Single PostgreSQL transaction for entire purchase  
✅ **Concurrency:** `SELECT FOR UPDATE` with lock order: Property → Wallet  
✅ **Partial Purchase:** Reject entirely (no partial fills)  
✅ **Idempotency:** Required via `Idempotency-Key` header  
✅ **Reference Format:** `INV-<UUID>` (unique constraint)  
✅ **Outbox:** Same DB transaction (atomic event creation)  
✅ **Spendable Balance:** `MainBalance` only (not earnings or locked)  
✅ **Currency:** NGN only  

---

## Phase 1: Repository Audit ⏱️ 1-2 hours

**Goal:** Understand existing codebase before making changes

### Tasks:
- [x] 1.1 Review existing `internal/models/investment.go`
  - Document current fields
  - Note what's missing (unit_price_kobo, currency, idempotency_key, lifecycle timestamps)
  - Check status constants

- [x] 1.2 Review `internal/repositories/investment_repository.go`
  - Document existing methods
  - Identify missing methods (FindByIDForUpdate, HasActiveInvestmentByUserAndProperty, etc.)
  - Check transaction support

- [x] 1.3 Review `internal/repositories/wallet_repository.go`
  - Verify `FindByUserIDForUpdate` exists
  - Verify `UpdateInTransaction` exists
  - Verify `AddTransaction` method signature
  - Check wallet transaction type constants

- [x] 1.4 Review `internal/repositories/property_repository.go`
  - Verify `FindByIDForUpdate` exists
  - Verify `UpdateInTransaction` or `UpdatePartial` with tx support
  - Check property status constants (confirm `active` not `approved`)

- [x] 1.5 Review `internal/models/wallet.go`
  - Document transaction types
  - Verify `MainBalance`, `EarningsBalance`, `LockedBalance` fields exist
  - Check currency field

- [x] 1.6 Review `internal/models/property.go`
  - Verify `Status`, `TargetAmount`, `RaisedAmount`, `UnitPrice`, `TotalUnits`, `UnitsSold` fields
  - Check `InvestorCount` field exists
  - Verify `AcceptsInvestment()` method

- [x] 1.7 Review `internal/repositories/outbox_repository.go`
  - Verify `Create(ctx, event, tx)` method signature
  - Check event type constants

- [x] 1.8 Check current migration number
  - Run: `ls internal/database/migrations | sort | tail -1`
  - Document latest migration number (should be 000020)
  - Next migration will be 000021

- [x] 1.9 Review existing idempotency mechanism
  - Check if project has built-in idempotency support
  - If not, plan to create idempotency table

- [x] 1.10 Review existing transaction helper pattern
  - Check how wallet/property modules handle transactions
  - Document pattern: `db.Transaction(func(tx *gorm.DB) error {...})`

**Deliverables:**
- Audit notes document (can be inline comments in this file)
- List of methods to implement
- List of fields to add
- Confirmed migration number

**Success Criteria:**
- ✅ All existing interfaces documented
- ✅ All missing functionality identified
- ✅ No duplicate infrastructure planned
- ✅ Migration numbering confirmed

---

## Phase 2: Database Migration & Model Enhancement ⏱️ 2-3 hours

**Goal:** Enhance Investment model and create migration

### Tasks:
- [x] 2.1 Create migration file `000021_enhance_investments.up.sql`
  - Add `unit_price_kobo BIGINT NOT NULL CHECK (unit_price_kobo > 0)`
  - Add `currency VARCHAR(3) NOT NULL DEFAULT 'NGN' CHECK (currency = 'NGN')`
  - Add `idempotency_key VARCHAR(255)` (nullable for existing records)
  - Add `cancelled_at TIMESTAMP`
  - Add `completed_at TIMESTAMP`
  - Add `refunded_at TIMESTAMP`
  - Add `UNIQUE(reference)` constraint if not exists
  - Add `UNIQUE(user_id, idempotency_key)` constraint where idempotency_key IS NOT NULL
  - Add indexes:
    - `CREATE INDEX idx_investments_user_created ON investments(user_id, created_at DESC);`
    - `CREATE INDEX idx_investments_user_status_created ON investments(user_id, status, created_at DESC);`
    - `CREATE INDEX idx_investments_property_created ON investments(property_id, created_at DESC);`
    - `CREATE INDEX idx_investments_property_status ON investments(property_id, status);`
    - `CREATE INDEX idx_investments_status_created ON investments(status, created_at DESC);`
    - `CREATE INDEX idx_investments_idempotency ON investments(user_id, idempotency_key) WHERE idempotency_key IS NOT NULL;`
  - Update existing CHECK constraints if needed

- [x] 2.2 Create rollback migration `000021_enhance_investments.down.sql`
  - Drop added columns in reverse order
  - Drop added indexes
  - Drop added constraints

- [x] 2.3 Enhance `internal/models/investment.go`
  - Add `UnitPriceKobo int64` field with gorm tags
  - Add `Currency string` field (default "NGN")
  - Add `IdempotencyKey *string` field (nullable)
  - Add `CancelledAt *time.Time` field
  - Add `CompletedAt *time.Time` field
  - Add `RefundedAt *time.Time` field
  - Add JSON tags for API responses
  - Add validation tags where appropriate
  - Update or add constants for status values

- [x] 2.4 Run migration locally
  - Test `migrate up`
  - Verify schema changes
  - Test `migrate down`
  - Test `migrate up` again

- [x] 2.5 Build and verify
  - Run `go build ./cmd/api`
  - Run `go test ./...`
  - Verify no compilation errors

**Deliverables:**
- `internal/database/migrations/000021_enhance_investments.up.sql`
- `internal/database/migrations/000021_enhance_investments.down.sql`
- Enhanced `internal/models/investment.go`

**Success Criteria:**
- ✅ Migration up works
- ✅ Migration down works
- ✅ Model compiles
- ✅ All tests pass
- ✅ Schema validated in PostgreSQL

---

## Phase 3: Repository Layer Enhancement ⏱️ 2-3 hours

**Goal:** Implement repository methods for investment operations

### Tasks:
- [ ] 3.1 Add method signatures to `InvestmentRepository` interface
  - `FindByIDForUpdate(ctx context.Context, id uuid.UUID, tx *gorm.DB) (*models.Investment, error)`
  - `FindByUserAndIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, tx *gorm.DB) (*models.Investment, error)`
  - `HasActiveInvestmentByUserAndProperty(ctx context.Context, tx *gorm.DB, userID, propertyID uuid.UUID) (bool, error)`
  - `ListByProperty(ctx context.Context, propertyID uuid.UUID, query dto.InvestmentListQuery) ([]models.Investment, int64, error)`
  - `ListAll(ctx context.Context, query dto.AdminInvestmentListQuery) ([]models.Investment, int64, error)`
  - `Metrics(ctx context.Context) (*InvestmentMetrics, error)`
  - Update existing methods if needed

- [ ] 3.2 Implement `FindByIDForUpdate`
  - Use `tx.Clauses(clause.Locking{Strength: "UPDATE"})`
  - Lock the investment row for update
  - Return error if not found

- [ ] 3.3 Implement `FindByUserAndIdempotencyKey`
  - Query by `user_id` and `idempotency_key`
  - Use transaction context
  - Return nil if not found (not an error)

- [ ] 3.4 Implement `HasActiveInvestmentByUserAndProperty`
  - Query: `WHERE user_id = ? AND property_id = ? AND status = 'active' AND deleted_at IS NULL`
  - Return boolean
  - Use transaction context

- [ ] 3.5 Implement `ListByProperty`
  - Query by `property_id`
  - Support pagination (limit, offset)
  - Support status filter
  - Support sorting (created_at DESC by default)
  - Return results + total count
  - Preload Property if needed

- [ ] 3.6 Implement `ListAll` (admin)
  - Support pagination
  - Support filters: status, property_id, user_id, date range
  - Support sorting
  - Return results + total count
  - Whitelist sortable columns

- [ ] 3.7 Implement `Metrics`
  - Count total investments
  - Sum total amount
  - Calculate average amount
  - Count unique investors
  - Count by status (active, completed, cancelled)
  - Use SQL aggregates (not in-memory)

- [ ] 3.8 Add repository helper methods if needed
  - Update existing `Update` method to support transactions
  - Ensure all methods are context-aware

- [ ] 3.9 Write repository integration tests (optional in this phase, required in Phase 10)
  - Test CRUD operations
  - Test locking behavior
  - Test queries with filters

**Deliverables:**
- Enhanced `internal/repositories/investment_repository.go`

**Success Criteria:**
- ✅ All methods compile
- ✅ Context-aware
- ✅ Transaction-aware
- ✅ Parameterized queries (no SQL injection risk)
- ✅ Returns proper errors

---

## Phase 4: DTO Enhancement ⏱️ 1 hour

**Goal:** Create comprehensive DTOs for investment operations

### Tasks:
- [ ] 4.1 Review existing `internal/dto/investment_dto.go`

- [ ] 4.2 Enhance `CreateInvestmentRequest`
  - Keep: `PropertyID uuid.UUID` with validation
  - Keep: `Slots int` with validation `binding:"required,gt=0"`
  - Remove any amount/price fields if present

- [ ] 4.3 Enhance `InvestmentResponse`
  - Add all fields: ID, PropertyID, Slots, UnitPriceKobo, AmountKobo, Currency, Status, Reference, CreatedAt
  - Add optional `Property *PropertyResponse`
  - Add optional `CancelledAt`, `CompletedAt` timestamps
  - Use proper JSON tags

- [ ] 4.4 Create `InvestmentListQuery`
  ```go
  type InvestmentListQuery struct {
      Page         int       `form:"page"`
      Limit        int       `form:"limit"`
      Status       string    `form:"status"`
      PropertyType string    `form:"property_type"`
      FromDate     time.Time `form:"from"`
      ToDate       time.Time `form:"to"`
      Sort         string    `form:"sort"`    // date|amount|slots
      Order        string    `form:"order"`   // asc|desc
  }
  ```

- [ ] 4.5 Create `InvestmentListResponse`
  ```go
  type InvestmentListResponse struct {
      Investments []InvestmentResponse `json:"investments"`
      Pagination  PaginationResponse   `json:"pagination"`
  }
  ```

- [ ] 4.6 Enhance `PortfolioSummaryResponse`
  - Keep existing fields
  - Consider adding: PropertiesInvested, CompletedInvestments, CancelledInvestments

- [ ] 4.7 Create `AdminInvestmentListQuery`
  ```go
  type AdminInvestmentListQuery struct {
      InvestmentListQuery
      UserID     *uuid.UUID `form:"user_id"`
      PropertyID *uuid.UUID `form:"property_id"`
  }
  ```

- [ ] 4.8 Create `AdminInvestmentListResponse`
  - Similar to InvestmentListResponse but may include additional admin fields

- [ ] 4.9 Create `CancelInvestmentRequest`
  ```go
  type CancelInvestmentRequest struct {
      Reason string `json:"reason" binding:"required,min=10"`
  }
  ```

- [ ] 4.10 Create `InvestmentMetricsResponse`
  ```go
  type InvestmentMetricsResponse struct {
      TotalInvestments      int64 `json:"total_investments"`
      TotalAmountKobo       int64 `json:"total_amount_kobo"`
      ActiveInvestments     int64 `json:"active_investments"`
      CompletedInvestments  int64 `json:"completed_investments"`
      CancelledInvestments  int64 `json:"cancelled_investments"`
      UniqueInvestors       int64 `json:"unique_investors"`
      AverageInvestmentKobo int64 `json:"average_investment_kobo"`
  }
  ```

**Deliverables:**
- Enhanced `internal/dto/investment_dto.go`

**Success Criteria:**
- ✅ All DTOs compile
- ✅ Proper validation tags
- ✅ JSON tags correct
- ✅ No money calculations in DTOs

---

## Phase 5: Service Layer - Core CreateInvestment ⏱️ 4-6 hours ⚠️ CRITICAL

**Goal:** Implement the atomic investment purchase transaction

### Tasks:
- [ ] 5.1 Create `internal/services/investment_service.go`
  - Define `InvestmentService` interface
  - Create `investmentService` struct
  - Add dependencies: db, investmentRepo, walletRepo, propertyRepo, outboxRepo

- [ ] 5.2 Implement `NewInvestmentService` constructor

- [ ] 5.3 Implement `CreateInvestment` - Step 1: Input Validation
  - Validate `propertyID` is not nil
  - Validate `slots > 0`
  - Validate `idempotencyKey` is not empty (8-255 chars)
  - Return `validation_error` for invalid inputs

- [ ] 5.4 Implement `CreateInvestment` - Step 2: Begin Transaction
  - Use `s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {...})`
  - All subsequent operations in this transaction

- [ ] 5.5 Implement `CreateInvestment` - Step 3: Idempotency Check
  - Call `investmentRepo.FindByUserAndIdempotencyKey(ctx, userID, key, tx)`
  - If found and status is completed:
    - Calculate request hash (hash of propertyID + slots)
    - Compare with stored hash
    - If match: return existing investment (200 OK)
    - If mismatch: return `idempotency_conflict` error (409)
  - If not found: continue

- [ ] 5.6 Implement `CreateInvestment` - Step 4: Lock Property
  - Call `propertyRepo.FindByIDForUpdate(ctx, propertyID, tx)`
  - Validate property exists (not soft deleted)
  - Validate property status == "active"
  - Validate `property.UnitPrice > 0`
  - Validate `slots <= (property.TotalUnits - property.UnitsSold)`
  - Return appropriate errors for failures

- [ ] 5.7 Implement `CreateInvestment` - Step 5: Calculate Amount
  - Check integer overflow: `if int64(slots) > math.MaxInt64/property.UnitPrice { return error }`
  - Calculate: `amount := int64(slots) * property.UnitPrice`
  - Validate minimum investment: `if property.MinimumInvestment > 0 && amount < property.MinimumInvestment { return error }`
  - Validate doesn't exceed target: `if amount > (property.TargetAmount - property.RaisedAmount) { return error }`

- [ ] 5.8 Implement `CreateInvestment` - Step 6: Lock Wallet
  - Call `walletRepo.FindByUserIDForUpdate(ctx, userID, tx)`
  - Validate wallet exists
  - Validate currency matches (NGN)
  - Validate `wallet.MainBalance >= amount`
  - Return `insufficient_funds` if balance too low

- [ ] 5.9 Implement `CreateInvestment` - Step 7: Debit Wallet
  - Capture `balanceBefore := wallet.MainBalance`
  - Update: `wallet.MainBalance -= amount`
  - Capture `balanceAfter := wallet.MainBalance`
  - Assert: `balanceAfter >= 0` (should never fail due to prior check)

- [ ] 5.10 Implement `CreateInvestment` - Step 8: Create Wallet Transaction
  - Generate reference: `INV-<UUID>`
  - Create wallet transaction:
    - Type: "investment"
    - Amount: `-amount` (or positive with debit flag, match existing pattern)
    - Status: "completed"
    - Reference: investment reference
    - BalanceBefore, BalanceAfter
    - Description: "Investment in {property.Title}"
  - Call `walletRepo.AddTransaction(ctx, transaction, tx)`

- [ ] 5.11 Implement `CreateInvestment` - Step 9: Determine Unique Investor
  - Call `investmentRepo.HasActiveInvestmentByUserAndProperty(ctx, tx, userID, propertyID)`
  - If false (first investment in this property): `shouldIncrementInvestor = true`
  - If true: `shouldIncrementInvestor = false`

- [ ] 5.12 Implement `CreateInvestment` - Step 10: Create Investment
  - Create investment record:
    - UserID, PropertyID, Slots
    - UnitPriceKobo: property.UnitPrice (snapshot)
    - AmountKobo: calculated amount
    - Currency: "NGN"
    - Status: "active"
    - Reference: investment reference
    - IdempotencyKey: idempotencyKey
  - Call `investmentRepo.Create(ctx, investment, tx)`

- [ ] 5.13 Implement `CreateInvestment` - Step 11: Update Property
  - Update property:
    - `RaisedAmount += amount`
    - `UnitsSold += slots`
    - If `shouldIncrementInvestor`: `InvestorCount++`
  - Assert invariants:
    - `RaisedAmount <= TargetAmount`
    - `UnitsSold <= TotalUnits`
  - Call `propertyRepo.UpdateInTransaction(ctx, property, tx)`

- [ ] 5.14 Implement `CreateInvestment` - Step 12: Check Funding Status
  - Check if `property.RaisedAmount >= property.TargetAmount`
  - If true:
    - Update `property.Status = "funded"`
    - Set `fundingReached = true` (for outbox event)
  - Update property again if status changed

- [ ] 5.15 Implement `CreateInvestment` - Step 13: Create Outbox Events
  - Create `investment.created` event:
    - AggregateID: investment.ID
    - AggregateType: "investment"
    - EventType: "investment.created"
    - Payload: investment details (user_id, property_id, slots, amount, reference, etc.)
  - Call `outboxRepo.Create(ctx, event, tx)`
  - If `fundingReached`:
    - Create `property.funded` event
    - Call `outboxRepo.Create(ctx, event, tx)`

- [ ] 5.16 Implement `CreateInvestment` - Step 14: Commit & Return
  - Transaction commits automatically if no error returned
  - Return investment response with 201 status
  - Log successful investment (structured logging)

- [ ] 5.17 Add comprehensive error handling
  - Catch all errors in transaction
  - Log errors with context (user_id, property_id, error)
  - Map errors to proper error codes
  - Ensure transaction rolls back on any error

- [ ] 5.18 Add structured logging
  - Log investment attempt
  - Log successful investment
  - Log failures with details
  - Never log sensitive data (JWT tokens, etc.)

**Deliverables:**
- `internal/services/investment_service.go` with `CreateInvestment` method

**Success Criteria:**
- ✅ All steps implemented in correct order
- ✅ Single atomic transaction
- ✅ Property locked before wallet
- ✅ Server calculates amount (never trusts client)
- ✅ Idempotency works correctly
- ✅ All invariants enforced
- ✅ Comprehensive error handling
- ✅ Structured logging
- ✅ No network calls in transaction

---

## Phase 6: Service Layer - Portfolio Features ⏱️ 2-3 hours

**Goal:** Implement user portfolio query methods

### Tasks:
- [ ] 6.1 Implement `GetInvestment(ctx, userID, investmentID)`
  - Call `investmentRepo.FindByID(ctx, investmentID)`
  - Verify ownership: `investment.UserID == userID`
  - Return `not_found` if not owned (prevent IDOR)
  - Preload property if needed
  - Return investment response

- [ ] 6.2 Implement `ListInvestments(ctx, userID, query)`
  - Set defaults: `page=1`, `limit=20`, `sort=date`, `order=desc`
  - Enforce max limit: `if limit > 100 { limit = 100 }`
  - Whitelist sortable columns: date, amount, slots
  - Whitelist filterable fields: status, property_type, from_date, to_date
  - Call `investmentRepo.ListByUser(ctx, userID, query)`
  - Build pagination response
  - Return investment list response

- [ ] 6.3 Implement `PortfolioSummary(ctx, userID)`
  - Call `investmentRepo.PortfolioSummary(ctx, userID)`
  - Call `walletRepo.FindByUserID(ctx, userID)`
  - Build summary:
    - TotalInvested (from repo aggregate)
    - ActiveCount, CompletedCount, CancelledCount
    - PropertiesInvested (count distinct property_id)
    - WalletMainBalance
    - EarningsBalance
  - Return summary response

- [ ] 6.4 Add validation and error handling
  - Validate query parameters
  - Handle repository errors
  - Return proper error codes

- [ ] 6.5 Add logging
  - Log portfolio queries
  - Log any errors

**Deliverables:**
- Portfolio methods in `internal/services/investment_service.go`

**Success Criteria:**
- ✅ Ownership checks prevent IDOR
- ✅ Pagination works correctly
- ✅ Filters work correctly
- ✅ No N+1 queries
- ✅ Proper error handling

---

## Phase 7: Service Layer - Admin Features ⏱️ 2-3 hours

**Goal:** Implement admin query and cancellation methods

### Tasks:
- [ ] 7.1 Implement `AdminListInvestments(ctx, query)`
  - Support all filters: user_id, property_id, status, date range
  - Support pagination and sorting
  - Call `investmentRepo.ListAll(ctx, query)`
  - Return admin list response

- [ ] 7.2 Implement `AdminGetInvestment(ctx, investmentID)`
  - Call `investmentRepo.FindByID(ctx, investmentID)`
  - No ownership check (admin can see all)
  - Preload property
  - Return investment response

- [ ] 7.3 Implement `ListPropertyInvestments(ctx, propertyID, query)`
  - Call `investmentRepo.ListByProperty(ctx, propertyID, query)`
  - Support pagination and filters
  - Return investment list response

- [ ] 7.4 Implement `Metrics(ctx)`
  - Call `investmentRepo.Metrics(ctx)`
  - Return metrics response

- [ ] 7.5 Implement `CancelInvestment(ctx, adminID, investmentID, request)` ⚠️ COMPLEX
  - Begin transaction
  - Lock investment (FindByIDForUpdate)
  - Validate status == "active"
  - Lock property (FindByIDForUpdate)
  - Lock wallet (FindByUserIDForUpdate)
  - Refund wallet: `wallet.MainBalance += investment.AmountKobo`
  - Create refund wallet transaction (type: "refund", amount: +investment.AmountKobo)
  - Update investment: status = "cancelled", cancelled_at = now()
  - Update property: `RaisedAmount -= investment.AmountKobo`, `UnitsSold -= investment.Slots`
  - Check if user has other active investments in same property
  - If no other investments: `property.InvestorCount--`
  - Create `investment.cancelled` outbox event
  - Commit transaction
  - Log cancellation with reason

- [ ] 7.6 Add audit logging for admin actions
  - Log who performed the action (adminID)
  - Log reason for cancellation
  - Log investment details

**Deliverables:**
- Admin methods in `internal/services/investment_service.go`

**Success Criteria:**
- ✅ Admin can see all investments
- ✅ Cancellation is atomic
- ✅ Refund updates wallet correctly
- ✅ Property counters restored accurately
- ✅ Investor count handled correctly
- ✅ Proper audit trail

---

## Phase 8: Handler Layer ⏱️ 2-3 hours

**Goal:** Create HTTP handlers for all endpoints

### Tasks:
- [ ] 8.1 Create `internal/handlers/investment_handler.go`
  - Define `InvestmentHandler` struct with service dependency
  - Implement `NewInvestmentHandler` constructor

- [ ] 8.2 Implement `Create(c *gin.Context)` handler
  - Get userID from context: `c.GetString("user_id")`
  - Bind request body to `dto.CreateInvestmentRequest`
  - Get `Idempotency-Key` header: `c.GetHeader("Idempotency-Key")`
  - Validate idempotency key not empty
  - Call `service.CreateInvestment(ctx, userID, request, idempotencyKey)`
  - Return 201 Created with investment response
  - Handle errors appropriately

- [ ] 8.3 Implement `List(c *gin.Context)` handler
  - Get userID from context
  - Bind query parameters to `dto.InvestmentListQuery`
  - Call `service.ListInvestments(ctx, userID, query)`
  - Return 200 OK with list response

- [ ] 8.4 Implement `Get(c *gin.Context)` handler
  - Get userID from context
  - Get investmentID from URL param
  - Call `service.GetInvestment(ctx, userID, investmentID)`
  - Return 200 OK with investment response

- [ ] 8.5 Implement `Portfolio(c *gin.Context)` handler
  - Get userID from context
  - Call `service.PortfolioSummary(ctx, userID)`
  - Return 200 OK with summary response

- [ ] 8.6 Implement `AdminList(c *gin.Context)` handler
  - Bind query parameters to `dto.AdminInvestmentListQuery`
  - Call `service.AdminListInvestments(ctx, query)`
  - Return 200 OK with list response

- [ ] 8.7 Implement `AdminGet(c *gin.Context)` handler
  - Get investmentID from URL param
  - Call `service.AdminGetInvestment(ctx, investmentID)`
  - Return 200 OK with investment response

- [ ] 8.8 Implement `AdminByProperty(c *gin.Context)` handler
  - Get propertyID from URL param
  - Bind query parameters
  - Call `service.ListPropertyInvestments(ctx, propertyID, query)`
  - Return 200 OK with list response

- [ ] 8.9 Implement `AdminCancel(c *gin.Context)` handler
  - Get adminID from context
  - Get investmentID from URL param
  - Bind request body to `dto.CancelInvestmentRequest`
  - Call `service.CancelInvestment(ctx, adminID, investmentID, request)`
  - Return 200 OK

- [ ] 8.10 Implement `AdminMetrics(c *gin.Context)` handler
  - Call `service.Metrics(ctx)`
  - Return 200 OK with metrics response

- [ ] 8.11 Add error handling helper
  - Map service errors to HTTP status codes
  - Use existing `response.Success` and error handler

**Deliverables:**
- `internal/handlers/investment_handler.go`

**Success Criteria:**
- ✅ All handlers are thin (no business logic)
- ✅ Proper error handling
- ✅ Consistent response format
- ✅ Idempotency-Key header extracted correctly
- ✅ User ID from JWT context

---

## Phase 9: Routes & Dependency Injection ⏱️ 1-2 hours

**Goal:** Register routes and wire up dependencies

### Tasks:
- [ ] 9.1 Update `internal/routes/v1/routes.go`
  - Add investmentHandler parameter to RegisterRoutes function
  - Register user endpoints (with Auth middleware):
    ```go
    investments := router.Group("/investments")
    investments.Use(middleware.Auth(cfg))
    {
        investments.POST("", investmentHandler.Create)
        investments.GET("", investmentHandler.List)
        investments.GET("/portfolio", investmentHandler.Portfolio)
        investments.GET("/:id", investmentHandler.Get)
    }
    ```
  - Ensure `/portfolio` route is registered before `/:id` to avoid conflicts

- [ ] 9.2 Register admin endpoints (with Auth + RequireRole middleware)
  ```go
  admin := router.Group("/admin/investments")
  admin.Use(middleware.Auth(cfg), middleware.RequireRole("admin"))
  {
      admin.GET("", investmentHandler.AdminList)
      admin.GET("/:id", investmentHandler.AdminGet)
      admin.GET("/by-property/:propertyId", investmentHandler.AdminByProperty)
      admin.POST("/:id/cancel", investmentHandler.AdminCancel)
      admin.GET("/metrics", investmentHandler.AdminMetrics)
  }
  ```

- [ ] 9.3 Update `cmd/api/main.go`
  - Initialize investment repository:
    ```go
    investmentRepo := repositories.NewInvestmentRepository(db)
    ```
  - Initialize investment service:
    ```go
    investmentService := services.NewInvestmentService(
        db,
        investmentRepo,
        walletRepo,
        propertyRepo,
        outboxRepo,
    )
    ```
  - Initialize investment handler:
    ```go
    investmentHandler := handlers.NewInvestmentHandler(investmentService)
    ```
  - Pass to RegisterRoutes:
    ```go
    v1.RegisterRoutes(apiV1, authHandler, userHandler, walletHandler, propertyHandler, investmentHandler, cfg)
    ```

- [ ] 9.4 Build and test
  - Run `go build ./cmd/api`
  - Verify no compilation errors
  - Start API server
  - Verify endpoints are reachable

**Deliverables:**
- Updated `internal/routes/v1/routes.go`
- Updated `cmd/api/main.go`

**Success Criteria:**
- ✅ Application starts without errors
- ✅ All endpoints reachable
- ✅ Authentication middleware applied correctly
- ✅ Admin RBAC applied correctly
- ✅ Route order correct (portfolio before :id)

---

## Phase 10: Testing ⏱️ 5-8 hours

**Goal:** Comprehensive testing of investment module

### Tasks:
- [ ] 10.1 Write unit tests for `CreateInvestment`
  - Test case: Invalid slots (0, negative) → validation_error
  - Test case: Missing property → not_found
  - Test case: Property not active → resource_unavailable
  - Test case: Sold out → resource_unavailable
  - Test case: Minimum investment not met → validation_error
  - Test case: Insufficient funds → insufficient_funds
  - Test case: Currency mismatch → validation_error
  - Test case: Successful purchase → 201 Created
  - Test case: Property becomes funded → status changes
  - Test case: First investment → investor count increments
  - Test case: Subsequent investment → investor count unchanged
  - Test case: Idempotency replay (same request) → returns existing
  - Test case: Idempotency conflict (different request) → 409 error
  - Test case: Integer overflow → validation_error
  - Test case: Repository failure → error propagated

- [ ] 10.2 Write repository integration tests
  - Test: FindByIDForUpdate locks row
  - Test: Create investment
  - Test: FindByUserAndIdempotencyKey
  - Test: HasActiveInvestmentByUserAndProperty
  - Test: ListByUser with pagination
  - Test: ListByUser with filters
  - Test: ListByProperty
  - Test: PortfolioSummary calculations
  - Test: Metrics calculations

- [ ] 10.3 Write rollback tests
  - Force failure after wallet update → assert rollback
  - Force failure after wallet transaction → assert rollback
  - Force failure after investment insert → assert rollback
  - Force failure after property update → assert rollback
  - Force failure after outbox insert → assert rollback
  - Assert: wallet unchanged, no transaction, no investment, property unchanged

- [ ] 10.4 Write concurrency tests
  - **Last slot test:**
    - Property: 10 total, 9 sold
    - Concurrent requests: User A wants 1, User B wants 1
    - Expected: One succeeds, one gets resource_unavailable
    - Assert: units_sold = 10 (never 11)
  - **Partial inventory test:**
    - Property: 5 available
    - Concurrent requests: User A wants 3, User B wants 3
    - Expected: One succeeds, one fails
    - Assert: No overselling
  - **Double spend test:**
    - Wallet: ₦100,000
    - Concurrent: Investment ₦70,000 + Withdrawal ₦50,000
    - Expected: One succeeds, one fails
    - Assert: Balance never negative
  - **Idempotency race:**
    - Send same request twice concurrently with same key
    - Expected: One investment, one debit, one event

- [ ] 10.5 Write authorization tests
  - Test: User A cannot read User B's investment
  - Test: User cannot access admin list
  - Test: User cannot cancel investment
  - Test: Admin can access admin endpoints
  - Test: Admin can cancel investment

- [ ] 10.6 Write cancellation tests
  - Test: Cancel active investment → refund, counters updated
  - Test: Cannot cancel completed investment
  - Test: Cannot cancel already cancelled investment
  - Test: Cancellation updates investor count correctly

- [ ] 10.7 Write integration tests (manual or automated)
  - End-to-end: Register → Fund Wallet → Create Property → Invest → Verify
  - Test full flow with real database
  - Verify wallet balance changes
  - Verify property counters
  - Verify outbox events created

- [ ] 10.8 Load testing (optional but recommended)
  - 100-500 concurrent investment requests
  - 10-100 available slots
  - Assert: No overselling, no negative balance
  - Measure: p95/p99 latency, deadlocks, success rate

**Deliverables:**
- Test files:
  - `internal/services/investment_service_test.go`
  - `internal/repositories/investment_repository_test.go`
  - `internal/handlers/investment_handler_test.go` (optional)

**Success Criteria:**
- ✅ All unit tests pass
- ✅ All integration tests pass
- ✅ Concurrency tests pass (no race conditions)
- ✅ Authorization tests pass (no IDOR)
- ✅ Rollback tests confirm atomicity
- ✅ Load tests show acceptable performance

---

## Phase 11: Documentation ⏱️ 2-3 hours

**Goal:** Create comprehensive documentation

### Tasks:
- [ ] 11.1 Create `INVESTMENT_MODULE_COMPLETE.md`
  - Executive summary
  - Implementation statistics (files created, lines of code)
  - Architecture overview
  - API endpoints list with examples
  - Transaction flow diagram
  - Concurrency strategy explanation
  - Idempotency behavior
  - Error codes and meanings
  - Testing summary
  - Deployment notes

- [ ] 11.2 Create `INVESTMENT_API_GUIDE.md`
  - Complete API reference for all 9 endpoints
  - Request/response examples
  - Error responses
  - Authentication requirements
  - Idempotency-Key usage
  - Rate limiting recommendations

- [ ] 11.3 Create `INVESTMENT_TESTING_GUIDE.md`
  - Manual testing procedures
  - Test data setup
  - Expected results
  - Troubleshooting guide
  - Database verification queries

- [ ] 11.4 Update main project documentation
  - Update `README.md` if needed
  - Update API documentation
  - Document new environment variables if any

- [ ] 11.5 Create migration guide
  - Document how to run the migration
  - Backfill instructions for existing data if needed
  - Rollback procedures

**Deliverables:**
- `INVESTMENT_MODULE_COMPLETE.md`
- `INVESTMENT_API_GUIDE.md`
- `INVESTMENT_TESTING_GUIDE.md`
- Updated project docs

**Success Criteria:**
- ✅ All endpoints documented
- ✅ Examples provided
- ✅ Testing guide usable by QA
- ✅ Deployment steps clear
- ✅ Troubleshooting covered

---

## 🎯 Definition of Done

The Investment Module is considered complete when ALL of the following are true:

### Architecture
- [ ] Handler → Service → Repository pattern maintained
- [ ] Existing error handling patterns used
- [ ] Existing logger patterns used
- [ ] Existing response wrapper used
- [ ] No new architectural patterns introduced

### Financial Correctness
- [ ] Single transaction for purchase
- [ ] Property row locked (SELECT FOR UPDATE)
- [ ] Wallet row locked (SELECT FOR UPDATE)
- [ ] Consistent lock order (Property → Wallet)
- [ ] No negative wallet balance possible
- [ ] No property overselling possible
- [ ] Server calculates amount (client amount never trusted)
- [ ] Price snapshot stored on investment
- [ ] Unique reference generated
- [ ] Wallet transaction created for every debit
- [ ] Atomic funded transition
- [ ] Transactional outbox events

### Reliability
- [ ] Idempotency implemented and tested
- [ ] RabbitMQ failure doesn't lose committed investment
- [ ] Duplicate request doesn't duplicate debit
- [ ] Crash-after-commit retry is safe

### Security
- [ ] Ownership checks prevent IDOR
- [ ] Admin RBAC enforced
- [ ] No client user_id accepted
- [ ] No client amount accepted
- [ ] SQL fields whitelisted for sorting/filtering
- [ ] Rate limiting considered
- [ ] Secrets never logged

### Testing
- [ ] Unit tests written and passing
- [ ] Repository tests written and passing
- [ ] Rollback tests written and passing
- [ ] Last-slot concurrency test passing
- [ ] Double-spend test passing
- [ ] Idempotency race test passing
- [ ] IDOR tests passing
- [ ] Admin authorization tests passing
- [ ] Load/concurrency tests performed

### Documentation
- [ ] API guide created
- [ ] Testing guide created
- [ ] Completion document created
- [ ] Migration documented
- [ ] Error codes documented
- [ ] Idempotency behavior documented

---

## 📝 Notes & Blockers

### Decisions Made:
- Using UUID instead of ULID (no new dependency)
- Idempotency via unique constraint on user_id + idempotency_key
- Reference format: `INV-<UUID>`

### Blockers:
*(None currently)*

### Questions:
*(None currently)*

---

## 🚀 Ready to Begin

**Current Phase:** Phase 1 - Repository Audit  
**Status:** Ready to start  
**Next Action:** Review existing code and document current state  

Would you like to begin Phase 1?

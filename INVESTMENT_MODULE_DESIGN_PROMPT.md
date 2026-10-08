# Investment Module (Milestone 5) - Design Request for External AI

**Date:** 2026-10-06  
**Project:** PropVest Backend  
**Milestone:** 5 - Investment Engine  
**Complexity:** Very High  
**Purpose:** Generate a comprehensive, production-grade implementation plan for the Investment Module

---

## 🎯 YOUR TASK

You are a Principal Software Architect tasked with designing the **Investment Module (Milestone 5)** for PropVest - a real estate fractional investment platform backend written in Go.

**Deliverable:** Create a comprehensive, production-grade implementation plan similar to the Property Module implementation (see reference below). The plan should be ready for a junior-to-mid-level developer (or AI assistant) to execute step-by-step.

---

## 📋 PROJECT CONTEXT

### Platform Overview
PropVest is a fintech platform that enables fractional real estate investment. Users can:
1. Browse investment properties
2. Fund their wallet via payment gateway (Paystack)
3. **Purchase property slots/shares (THIS MODULE)**
4. Track their portfolio
5. Earn returns on investments
6. Withdraw earnings

### Technology Stack
- **Language:** Go 1.21+
- **Framework:** Gin (HTTP router)
- **Database:** PostgreSQL 15
- **ORM:** GORM v2
- **File Storage:** Cloudinary
- **Message Queue:** RabbitMQ (via Outbox pattern)
- **Payment:** Paystack
- **Authentication:** JWT with refresh tokens
- **Architecture:** Clean Architecture (Handler → Service → Repository)

### Current Milestone Status
- ✅ **Milestone 1:** Authentication (COMPLETE)
- ✅ **Milestone 2:** User Management (COMPLETE)
- ✅ **Milestone 3:** Wallet System (COMPLETE - deposits, withdrawals, transactions)
- ✅ **Milestone 4:** Property Module (COMPLETE - CRUD, images, documents, publishing)
- ⏳ **Milestone 5:** Investment Engine (TO BE DESIGNED - THIS IS YOUR TASK)

---

## 🏗️ EXISTING IMPLEMENTATION

### Current Investment Foundation (Basic Schema Only)

**Model (internal/models/investment.go):**
```go
type Investment struct {
    ID         uuid.UUID
    UserID     uuid.UUID
    PropertyID uuid.UUID
    Slots      int    // Number of slots purchased
    AmountKobo int64  // Total amount paid (kobo)
    Status     string // active, completed, cancelled, refunded
    Reference  string // Unique reference (ties to wallet transaction)
    CreatedAt  time.Time
    UpdatedAt  time.Time
    DeletedAt  gorm.DeletedAt
    Property   Property // For eager loading
}
```

**Repository (basic CRUD exists):**
- `Create(ctx, inv, tx)` - Create investment (transactional)
- `FindByID(ctx, id)` - Get single investment
- `ListByUser(ctx, userID, limit, offset)` - User's investments (paginated)
- `PortfolioSummary(ctx, userID)` - Aggregate stats (total invested, count)
- `CountAll(ctx)` - Total investments platform-wide
- `SumAll(ctx)` - Total amount invested platform-wide

**DTOs (basic structures exist):**
```go
type CreateInvestmentRequest struct {
    PropertyID uuid.UUID
    Slots      int
}

type InvestmentResponse struct {
    ID, PropertyID, Slots, AmountKobo, Status, Reference, CreatedAt
    Property *PropertyResponse // optional
}

type PortfolioSummaryResponse struct {
    TotalInvested, ActiveCount, WalletBalance, EarningsBalance
}
```

**What's Missing (Your Job):**
- ❌ Service layer (business logic)
- ❌ Handler layer (HTTP endpoints)
- ❌ Routes registration
- ❌ Dependency injection
- ❌ Comprehensive validation
- ❌ Outbox event integration
- ❌ Testing strategy
- ❌ Documentation

---

## 🔑 BUSINESS REQUIREMENTS

### Core Investment Flow
The investment purchase is the **most critical transaction** in the platform. It must be:
1. **Atomic** - All changes succeed or fail together
2. **Consistent** - Business rules always enforced
3. **Isolated** - Concurrent purchases don't oversell
4. **Durable** - Once confirmed, never lost

### Investment Purchase Steps (Single Transaction)
```
1. Validate user is authenticated
2. Validate property exists and accepts investments
3. Validate slots available (not oversold)
4. Calculate total cost (slots × property.SlotPrice)
5. Validate wallet has sufficient balance
6. LOCK property row (SELECT FOR UPDATE)
7. LOCK wallet row (SELECT FOR UPDATE)
8. Debit wallet (main_balance -= amount)
9. Create wallet transaction (type: investment, reference: INV-xxx)
10. Create investment record
11. Update property funding (raised_amount += amount, units_sold += slots)
12. Update property funding percentage
13. Check if property reached 100% funding
14. If funded, update property status to 'funded'
15. Create outbox event (investment.created)
16. COMMIT transaction
17. Return investment confirmation
```

**ALL STEPS MUST BE IN ONE DATABASE TRANSACTION**

### Business Rules (Must Enforce)
1. **Wallet Rules:**
   - Balance cannot go negative
   - Must have sufficient `main_balance` (not earnings)
   - Transaction record required for every debit

2. **Property Rules:**
   - Property must exist and not be soft-deleted
   - Property status must be `active` or `approved` (depends on your status model)
   - Property must have available slots (units_sold + slots <= total_units)
   - Cannot invest in draft/rejected/completed properties
   - Raised amount cannot exceed target amount

3. **Investment Rules:**
   - Slots must be > 0
   - Amount must equal slots × current property.SlotPrice
   - User cannot buy more slots than available
   - Minimum investment may apply (check property.MinimumInvestment if exists)
   - Reference must be unique (INV-{timestamp}-{random} format)

4. **Concurrency Rules:**
   - Use `SELECT FOR UPDATE` to lock property and wallet rows
   - Prevent race conditions (two users buying last slot simultaneously)
   - Prevent double-spending (user investing while withdrawing)

5. **Status Transitions:**
   - Investment starts as `active`
   - Property automatically becomes `funded` at 100% funding
   - Future: Investment becomes `completed` when property matures

### Portfolio Management
Users need to:
- View all their investments (paginated)
- See individual investment details
- View portfolio summary (total invested, active count, properties)
- Filter investments (by status, property type, date range)
- Sort investments (by date, amount, ROI)

### Admin Requirements
Admins need to:
- View all platform investments (with filters)
- View investments by property
- See aggregated metrics (total investments, total amount, property funding rates)
- Cancel/refund investments (manual intervention, rare)

---

## 📊 EXISTING MODULES TO INTEGRATE WITH

### 1. Wallet Module (Milestone 3 - COMPLETE)

**Repository Methods Available:**
```go
walletRepo.FindByUserID(ctx, userID) (*Wallet, error)
walletRepo.FindByUserIDForUpdate(ctx, userID, tx) (*Wallet, error)  // LOCKS ROW
walletRepo.Update(ctx, wallet) error
walletRepo.UpdateInTransaction(ctx, wallet, tx) error
walletRepo.AddTransaction(ctx, txn, tx) error  // Creates transaction record
```

**Models:**
```go
type Wallet struct {
    ID              uuid.UUID
    UserID          uuid.UUID
    MainBalance     int64  // Investable balance (kobo)
    EarningsBalance int64  // Returns/dividends (kobo)
    LockedBalance   int64  // Pending withdrawals (kobo)
    Currency        string // "NGN"
}

type WalletTransaction struct {
    ID          uuid.UUID
    WalletID    uuid.UUID
    Type        string  // deposit, withdrawal, investment, refund, adjustment
    Amount      int64   // kobo (can be negative for debits)
    BalanceBefore int64
    BalanceAfter  int64
    Reference   string  // Unique reference
    Status      string  // pending, completed, failed
    Description string
}
```

**Transaction Types:**
- `deposit` - User funded wallet
- `withdrawal` - User withdrew funds
- `investment` - User purchased property slots (THIS MODULE CREATES THESE)
- `refund` - Investment cancelled/refunded
- `adjustment` - Manual admin correction

### 2. Property Module (Milestone 4 - COMPLETE)

**Repository Methods Available:**
```go
propertyRepo.FindByID(ctx, id) (*Property, error)
propertyRepo.FindByIDForUpdate(ctx, id, tx) (*Property, error)  // LOCKS ROW
propertyRepo.Update(ctx, property) error
propertyRepo.UpdatePartial(ctx, id, updates map[string]any) error
```

**Relevant Property Model Fields:**
```go
type Property struct {
    ID             uuid.UUID
    Title          string
    Status         string  // draft, active, funded, completed
    TargetAmount   int64   // Total funding goal (kobo)
    RaisedAmount   int64   // Current funding (kobo)
    UnitPrice      int64   // Price per slot (kobo)
    TotalUnits     int     // Total slots available
    UnitsSold      int     // Slots sold so far
    MinimumInvestment int64 // Minimum purchase amount (kobo)
    ROIPercent     float64 // Expected annual ROI
    DurationMonths int     // Investment duration
    InvestorCount  int     // Number of unique investors
    // ... other fields
}
```

**Property Status Values:**
- `draft` - Not published
- `active` - Accepting investments
- `funded` - 100% funded (no more investments)
- `completed` - Investment period ended

**Property Methods (exist in model):**
```go
property.AcceptsInvestment() bool  // Returns true if status allows investing
```

### 3. Outbox Pattern (Milestone 4 - COMPLETE)

**Outbox Repository Available:**
```go
outboxRepo.Create(ctx, event, tx) error  // Add event in same transaction
```

**Event Structure:**
```go
type OutboxEvent struct {
    ID            uuid.UUID
    AggregateID   uuid.UUID  // Investment ID
    AggregateType string     // "investment"
    EventType     string     // "investment.created", "investment.cancelled"
    Payload       datatypes.JSON
    Status        string     // pending, processed, failed
    CreatedAt     time.Time
}
```

**Event Types to Create:**
- `investment.created` - New investment made
- `investment.completed` - Investment matured (future)
- `investment.cancelled` - Admin cancelled investment
- `property.funded` - Property reached 100% funding (emit from investment service)

**Event Consumers (Background Workers - Already Exist):**
- Email notifications
- Analytics tracking
- Search index updates

### 4. Authentication & Authorization (Milestones 1-2 - COMPLETE)

**Middleware Available:**
```go
middleware.Auth(cfg)  // Extracts user_id from JWT, adds to context
middleware.RequireRole("admin")  // Enforces role-based access
```

**Context Helper:**
```go
userID := c.GetString("user_id")  // Get authenticated user from context
```

---

## 📐 ARCHITECTURE PATTERNS TO FOLLOW

### Project Structure (Existing Pattern)
```
internal/
├── models/               # GORM models (domain entities)
│   ├── investment.go     # ✅ EXISTS (basic)
│   ├── wallet.go         # ✅ EXISTS
│   └── property.go       # ✅ EXISTS
├── repositories/         # Data access layer
│   ├── investment_repository.go  # ✅ EXISTS (basic CRUD)
│   ├── wallet_repository.go      # ✅ EXISTS
│   └── property_repository.go    # ✅ EXISTS
├── services/             # Business logic layer
│   ├── investment_service.go     # ❌ NEEDS TO BE CREATED
│   ├── wallet_service.go         # ✅ EXISTS
│   └── property_service.go       # ✅ EXISTS
├── handlers/             # HTTP handlers
│   ├── investment_handler.go     # ❌ NEEDS TO BE CREATED
│   ├── wallet.go                 # ✅ EXISTS
│   └── property_handler.go       # ✅ EXISTS
├── dto/                  # Request/response types
│   ├── investment_dto.go  # ✅ EXISTS (basic, may need enhancement)
│   ├── wallet_dto.go      # ✅ EXISTS
│   └── property_dto.go    # ✅ EXISTS
├── routes/v1/
│   └── routes.go         # Route registration
└── middleware/
    ├── auth.go           # ✅ EXISTS
    └── rbac.go           # ✅ EXISTS
```

### Error Handling Pattern (Existing)
```go
// Use centralized error types
errors.NewAppError(message, code)

// Error codes used across project:
// - "validation_error" - Input validation failed
// - "business_logic_error" - Business rule violation
// - "not_found" - Resource doesn't exist
// - "forbidden" - Authorization failed
// - "insufficient_funds" - Not enough balance
// - "resource_unavailable" - Property sold out, etc.
```

### Logging Pattern (Existing)
```go
// Use package-level logger (NOT struct field)
import "github.com/mannykings2/propvest-backend/internal/logger"

logger.Info("Investment created", map[string]interface{}{
    "investment_id": inv.ID,
    "user_id": userID,
    "property_id": propertyID,
    "amount_kobo": amount,
})

logger.Error("Investment failed", map[string]interface{}{
    "error": err.Error(),
    "user_id": userID,
})
```

### Response Pattern (Existing)
```go
// Success responses
response.Success(c, http.StatusOK, data)
response.Success(c, http.StatusCreated, investment)

// Error responses (automatic via middleware)
return errors.NewAppError("Insufficient funds", "insufficient_funds")
```

### Transaction Pattern (Critical for Investment)
```go
// Always use transactions for multi-table operations
tx := r.db.Begin()
defer func() {
    if r := recover(); r != nil {
        tx.Rollback()
    }
}()

// Lock rows before updates
wallet, err := walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
property, err := propertyRepo.FindByIDForUpdate(ctx, propertyID, tx)

// Perform operations
// ...

if err := tx.Commit().Error; err != nil {
    return err
}
```

---

## 📚 REFERENCE: Property Module Implementation Plan

The Property Module was implemented in 13 phases over ~20 hours with excellent results. Your Investment Module design should follow a similar structure.

### Property Module Phases (Reference):
1. **Repository Audit** - Review existing code
2. **Database Migrations** - Create tables with proper indexes/constraints
3. **GORM Models** - Domain entities with helper methods
4. **Repository Layer** - Data access interfaces + implementations
5. **DTOs** - Request/response structures with validation
6. **Service Layer** - Business logic (the core)
7. **Handlers** - HTTP endpoint controllers
8. **Routes** - Endpoint registration
9. **Dependency Injection** - Wire everything in main.go
10. **Outbox Integration** - Event publishing
11. **Testing** - Unit tests for critical paths
12. **Integration Testing** - Manual testing guide
13. **Documentation** - API reference and guides

### Key Success Factors from Property Module:
- ✅ Comprehensive validation at every layer
- ✅ Clear separation of concerns
- ✅ Extensive error handling
- ✅ Transaction safety
- ✅ Soft delete support
- ✅ Detailed logging
- ✅ Complete documentation
- ✅ Testing coverage

---

## 🎨 DESIGN REQUIREMENTS FOR YOUR PLAN

### 1. Be Comprehensive
- Cover ALL aspects: models, repos, services, handlers, routes, tests, docs
- Consider edge cases (race conditions, insufficient funds, overselling)
- Include validation at every layer
- Plan for both happy path and error scenarios

### 2. Be Production-Grade
- **Security:** Prevent double-spending, overselling, unauthorized access
- **Performance:** Use indexes, avoid N+1 queries, lock only what's needed
- **Reliability:** Transactional integrity, idempotency, retry safety
- **Observability:** Structured logging, error tracking, metrics

### 3. Be Specific
- Provide exact function signatures
- Specify database indexes and constraints
- List exact validation rules
- Define error messages and codes
- Show example requests/responses

### 4. Break into Phases
- Each phase should be 1-4 hours of implementation time
- Phases should be independent where possible
- Phases should have clear acceptance criteria
- Estimate total implementation time

### 5. Consider Testing
- Unit tests for business logic
- Integration tests for the full flow
- Load testing considerations (concurrent investments)
- Edge case testing (last slot, insufficient funds, etc.)

---

## 🔍 SPECIFIC AREAS REQUIRING DESIGN

### 1. Service Layer (Most Critical)
**CreateInvestment(ctx, userID, request) method:**
- Input validation
- Property validation
- Wallet validation
- Transaction orchestration
- Event publishing
- Error handling

**Key Questions to Address:**
- How to handle property reaching 100% funding during the transaction?
- How to prevent race conditions when multiple users invest simultaneously?
- What happens if outbox event creation fails?
- How to calculate and verify the exact amount?
- Should we recalculate slot price or trust the client?
- What if property price changes between validation and purchase?

### 2. Concurrency Control
- Which rows to lock? (wallet, property, both?)
- Lock order to prevent deadlocks?
- Lock duration minimization?
- Retry strategy for lock contention?

### 3. Validation Strategy
- Client-side validation (what to send to frontend)
- Handler-level validation (request structure)
- Service-level validation (business rules)
- Database-level validation (constraints)

### 4. Error Scenarios to Handle
- Property doesn't exist
- Property doesn't accept investments (wrong status)
- Not enough slots available
- Insufficient wallet balance
- Property price changed during purchase
- Wallet locked (pending withdrawal)
- Database transaction failure
- Outbox event creation failure
- User already invested (if there's a unique constraint)

### 5. Portfolio Features
- List user's investments (pagination, filters, sorting)
- Portfolio summary (total, count, properties, ROI projections)
- Investment details (single investment view)
- Export functionality (CSV/PDF - future consideration)

### 6. Admin Features
- View all investments
- View investments by property
- Platform metrics (total invested, average investment size)
- Cancel/refund investment (manual intervention)
- Investment analytics

### 7. API Endpoints to Design
**User Endpoints (Authenticated):**
- `POST /api/v1/investments` - Create investment
- `GET /api/v1/investments` - List user's investments
- `GET /api/v1/investments/:id` - Get single investment
- `GET /api/v1/investments/portfolio` - Portfolio summary

**Admin Endpoints (Admin Role Required):**
- `GET /api/v1/admin/investments` - List all investments
- `GET /api/v1/admin/investments/:id` - Get any investment
- `GET /api/v1/admin/investments/by-property/:propertyId` - Investments for property
- `POST /api/v1/admin/investments/:id/cancel` - Cancel investment
- `GET /api/v1/admin/investments/metrics` - Platform metrics

### 8. Testing Strategy
- Unit tests for service layer (mock repos)
- Repository tests (if needed)
- Integration tests (full flow with test database)
- Load tests (concurrent investment attempts)
- Chaos testing (transaction failures, rollbacks)

---

## 📋 DELIVERABLE FORMAT

Please provide a document titled:
**"INVESTMENT_MODULE_IMPLEMENTATION_PLAN.md"**

### Required Sections:

1. **Executive Summary**
   - Module overview
   - Key challenges
   - Implementation approach
   - Estimated timeline

2. **Architecture Design**
   - Component diagram
   - Data flow diagram
   - Transaction flow diagram
   - Concurrency control strategy

3. **Database Design**
   - Schema changes (if any)
   - Indexes required
   - Constraints required
   - Migration strategy

4. **Service Layer Design**
   - CreateInvestment() detailed algorithm
   - GetInvestment() implementation
   - ListInvestments() with filtering/sorting
   - PortfolioSummary() calculation
   - CancelInvestment() (admin) workflow

5. **API Design**
   - Complete endpoint list
   - Request/response examples
   - Error responses
   - Status codes

6. **Validation Rules**
   - Input validation
   - Business rule validation
   - Database constraints
   - Error messages

7. **Error Handling Strategy**
   - Error types and codes
   - Rollback scenarios
   - Retry strategies
   - Client error responses

8. **Testing Strategy**
   - Unit test coverage
   - Integration test scenarios
   - Load test approach
   - Edge cases to test

9. **Phase-by-Phase Implementation Plan**
   - Phase 1: Repository Enhancement
   - Phase 2: Service Layer - Core Logic
   - Phase 3: Service Layer - Portfolio Features
   - Phase 4: Handler Layer
   - Phase 5: Routes & Dependency Injection
   - Phase 6: Outbox Integration
   - Phase 7: Validation & Error Handling
   - Phase 8: Testing
   - Phase 9: Documentation
   - (Adjust phases as you see fit)

10. **Implementation Details**
    - For each phase:
      - Tasks breakdown
      - Acceptance criteria
      - Dependencies
      - Time estimate
      - Testing requirements
      - Documentation requirements

11. **Security Considerations**
    - Authorization checks
    - Input sanitization
    - SQL injection prevention
    - Rate limiting recommendations
    - Audit logging

12. **Performance Considerations**
    - Database query optimization
    - Index strategy
    - Lock minimization
    - Caching opportunities

13. **Monitoring & Observability**
    - Metrics to track
    - Logs to capture
    - Alerts to configure
    - Dashboard requirements

14. **Future Enhancements**
    - Investment returns calculation
    - Dividend distribution
    - Secondary market trading
    - Investment recommendations

---

## ⚠️ CRITICAL CONSTRAINTS

### Must Follow:
1. **Use existing patterns** - Don't invent new architectural patterns
2. **Extend existing models** - The Investment model already exists, enhance it if needed
3. **Use existing error handling** - `errors.NewAppError(msg, code)`
4. **Use existing logger** - Package-level `logger.Info/Error`, not struct field
5. **Follow naming conventions** - Match existing code style
6. **Money in kobo** - All amounts are int64 kobo (₦100 = 10000 kobo)
7. **PostgreSQL only** - Don't design for multiple databases
8. **Clean Architecture** - Handler → Service → Repository, no skipping layers

### Must Avoid:
1. ❌ Breaking existing wallet/property implementations
2. ❌ Introducing new dependencies without justification
3. ❌ Skipping transaction safety for performance
4. ❌ Assuming database isolation levels (use explicit locking)
5. ❌ Hardcoding configuration values
6. ❌ Using float64 for money calculations
7. ❌ Inventing new response formats (use existing response.Success)

---

## 🎯 SUCCESS CRITERIA

Your design will be considered successful if:

1. ✅ It can be implemented phase-by-phase by a junior-mid developer
2. ✅ It handles all concurrency edge cases safely
3. ✅ It maintains transactional integrity
4. ✅ It follows existing project patterns
5. ✅ It includes comprehensive validation
6. ✅ It provides detailed error handling
7. ✅ It includes testing strategy
8. ✅ It is production-ready (not a prototype)
9. ✅ It considers performance and scalability
10. ✅ It includes complete API documentation

---

## 📞 QUESTIONS TO ADDRESS IN YOUR DESIGN

1. **Transaction Scope:** What happens if the outbox event creation fails after the investment is committed? Should outbox be in the same transaction or separate?

2. **Property Status Transition:** When exactly should property status change from `active` to `funded`? In the same transaction or async?

3. **Investor Count:** Should we track unique investor count? If yes, how to handle it transactionally?

4. **Minimum Investment:** Some properties may have minimum investment amounts - how to validate this?

5. **Maximum Investment:** Should there be a maximum slots per user limit? How to enforce it?

6. **Wallet Locking:** What if user has a pending withdrawal (locked balance)? Should we consider locked balance in availability check?

7. **Reference Format:** Investment reference format - suggest a pattern (e.g., INV-{timestamp}-{random})

8. **Idempotency:** If user retries the same investment request (network failure), how to prevent duplicate investments?

9. **Partial Fulfillment:** If user requests 10 slots but only 7 available, should we allow partial purchase or reject entirely?

10. **Price Consistency:** Should we snapshot the slot price at investment time? What if admin changes price during purchase flow?

---

## 🎓 EXAMPLE SERVICE METHOD STRUCTURE

Here's a skeleton of what the core `CreateInvestment` service method structure might look like (for your reference):

```go
func (s *InvestmentService) CreateInvestment(ctx context.Context, userID uuid.UUID, req dto.CreateInvestmentRequest) (*models.Investment, error) {
    // 1. Input validation
    // 2. Start transaction
    // 3. Lock and load property (SELECT FOR UPDATE)
    // 4. Validate property state
    // 5. Calculate investment amount
    // 6. Lock and load wallet (SELECT FOR UPDATE)
    // 7. Validate wallet balance
    // 8. Debit wallet
    // 9. Create wallet transaction
    // 10. Create investment record
    // 11. Update property funding
    // 12. Check if property fully funded
    // 13. Create outbox event
    // 14. Commit transaction
    // 15. Return investment
}
```

Your job is to flesh out each step with:
- Exact validation logic
- Error handling
- Locking strategy
- Rollback scenarios
- Logging statements
- Comments explaining "why" not just "what"

---

## 🚀 FINAL NOTES

- **This is the most critical module** - It handles real money and must be bulletproof
- **Err on the side of caution** - Reject investment rather than risk overselling or double-spending
- **Think about scale** - This will handle thousands of concurrent investments
- **Consider failure modes** - What happens when DB fails? When RabbitMQ is down? When Cloudinary is slow?
- **Be opinionated** - Make decisions and justify them. Don't leave things vague.
- **Use comments** - Explain your reasoning, especially for complex business logic

---

## 📎 ATTACHMENTS TO REVIEW (if needed)

If you need clarification on existing implementations, refer to these files from the completed Property Module:
- `PROPERTY_MODULE_IMPLEMENTATION_PLAN.md` - Shows the phase structure
- `PROPERTY_MODULE_COMPLETE.md` - Shows the final deliverables
- `internal/services/property_service.go` - Example service layer pattern

---

**Good luck! We're counting on you to design a production-grade Investment Module that will safely handle millions of naira in transactions.** 🚀

---

**END OF PROMPT**

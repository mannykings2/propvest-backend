# INVESTMENT_MODULE_IMPLEMENTATION_PLAN.md

**Project:** PropVest Backend\
**Milestone:** 5 --- Investment Engine\
**Date:** 2026-10-07\
**Status:** Implementation-ready design\
**Risk:** Critical --- real-money transaction path

------------------------------------------------------------------------

## 1. Executive Summary

The Investment Module is the transactional boundary between a user's
wallet and a property investment. It is **not ordinary CRUD**.

A successful purchase must atomically:

1.  authenticate the user;
2.  lock the property;
3.  verify the property is still investable;
4.  calculate `slots × server-side UnitPrice`;
5.  verify slots remain;
6.  lock the wallet;
7.  verify `main_balance`;
8.  debit the wallet;
9.  create the wallet transaction;
10. create the investment;
11. update property funding counters;
12. update unique investor count;
13. transition `active → funded` when the funding condition is reached;
14. create outbox event(s);
15. commit once.

### Core architectural decisions

  Concern                           Decision
  --------------------------------- -------------------------------------------------
  Money                             `int64` kobo
  Amount from client                **Never trusted**
  Unit price                        Read from locked Property row
  Price history                     Snapshot `UnitPriceKobo` on Investment
  Transaction                       One PostgreSQL transaction
  Property concurrency              `SELECT FOR UPDATE`
  Wallet concurrency                `SELECT FOR UPDATE`
  Lock order                        **Property → Wallet**
  Partial purchase                  Reject entirely
  Idempotency                       Required
  Reference                         `INV-<ULID>`; use UUID if avoiding a dependency
  Outbox                            Same DB transaction
  RabbitMQ                          Asynchronous; never inside purchase transaction
  Funding status                    Updated synchronously in purchase transaction
  Investor count                    Unique active investor count
  Spendable balance                 `MainBalance` only
  Existing investment price         Immutable snapshot
  Price mutation after investment   Block ordinary edits
  Currency                          NGN unless existing schema says otherwise

**Estimated implementation:** 30--43 focused hours including testing and
hardening.

### The most important rule

> Never trust a client-supplied financial amount, never make an
> inventory decision from an unlocked property read, and never commit a
> wallet debit without its corresponding investment and outbox event.

------------------------------------------------------------------------

# 2. Architecture Design

## 2.1 Component diagram

``` text
HTTP
 │
 ▼
┌──────────────────────┐
│ Investment Handler   │
│ bind + auth + DTO    │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│ Investment Service   │
│ business rules       │
│ transaction boundary │
│ idempotency           │
└───────┬────────┬─────┘
        │        │
        ▼        ▼
┌────────────┐ ┌────────────┐
│ Property   │ │ Wallet     │
│ Repository │ │ Repository │
│ FOR UPDATE │ │ FOR UPDATE │
└─────┬──────┘ └─────┬──────┘
      │              │
      └──────┬───────┘
             ▼
       PostgreSQL TX
        │   │   │
        ▼   ▼   ▼
   investment wallet property
        │
        ▼
      outbox
        │
     COMMIT
        │
        ▼
 Outbox Dispatcher
        │
        ▼
    RabbitMQ
```

## 2.2 Transaction/data flow

``` text
POST /investments
      │
      ▼
JWT user_id
      │
      ▼
Validate DTO + Idempotency-Key
      │
      ▼
BEGIN
      │
      ├── resolve idempotency
      ├── LOCK property
      ├── validate status/slots/price
      ├── calculate amount
      ├── LOCK wallet
      ├── validate main_balance
      ├── debit wallet
      ├── create wallet transaction
      ├── create investment
      ├── update property counters
      ├── update investor_count
      ├── active → funded if threshold reached
      ├── insert investment.created outbox
      ├── insert property.funded outbox if applicable
      │
      └── COMMIT
             │
             ▼
          201 Created
```

No Paystack, Cloudinary, HTTP, or RabbitMQ call is allowed while the
purchase transaction holds locks.

------------------------------------------------------------------------

# 3. Database Design

## 3.1 Investment model enhancement

The existing model should be extended, not replaced:

``` go
type Investment struct {
    ID             uuid.UUID
    UserID         uuid.UUID
    PropertyID     uuid.UUID
    Slots          int
    UnitPriceKobo  int64
    AmountKobo     int64
    Currency       string
    Status         string
    Reference      string
    IdempotencyKey *string

    CancelledAt    *time.Time
    CompletedAt    *time.Time
    RefundedAt     *time.Time

    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      gorm.DeletedAt

    Property       *Property
}
```

`UnitPriceKobo` is critical. Existing investments must not change value
when an administrator later changes the property's current price.

## 3.2 Migration

Create the next sequential migration after the current Property
migration. Do **not** hard-code a migration number until the repository
is checked.

Add, as required:

-   `unit_price_kobo`
-   `currency`
-   `idempotency_key` or the project's existing idempotency mechanism
-   lifecycle timestamps
-   constraints
-   indexes
-   unique reference constraint

Do not cascade-delete financial records.

## 3.3 Constraints

Recommended:

``` sql
CHECK (slots > 0)
CHECK (unit_price_kobo > 0)
CHECK (amount_kobo > 0)
CHECK (currency = 'NGN')
CHECK (status IN ('active', 'completed', 'cancelled', 'refunded'))
UNIQUE(reference)
```

Foreign keys:

``` text
user_id     → users.id
property_id → properties.id
```

Use the project's existing FK/delete conventions.

## 3.4 Indexes

At minimum:

``` sql
UNIQUE(reference)

INDEX(user_id, created_at DESC)

INDEX(user_id, status, created_at DESC)

INDEX(property_id, created_at DESC)

INDEX(property_id, status)

INDEX(status, created_at DESC)
```

Do not create every conceivable index. Verify query plans after
implementation.

## 3.5 Idempotency

If the project already has an idempotency mechanism, reuse it.

Otherwise create a dedicated table:

``` text
investment_idempotency_keys
---------------------------
id UUID PK
user_id UUID NOT NULL
idempotency_key VARCHAR(255) NOT NULL
request_hash VARCHAR(64) NOT NULL
investment_id UUID NULL
status VARCHAR(20) NOT NULL
created_at TIMESTAMP NOT NULL
expires_at TIMESTAMP NULL

UNIQUE(user_id, idempotency_key)
```

Semantics:

-   same user + same key + same request → return original investment;
-   same user + same key + different request →
    `409 idempotency_conflict`;
-   different user + same key → allowed;
-   network failure after commit + retry with same key → original
    investment returned.

------------------------------------------------------------------------

# 4. Service Layer

## 4.1 Interface

``` go
type InvestmentService interface {
    CreateInvestment(ctx context.Context, userID uuid.UUID,
        req dto.CreateInvestmentRequest, idempotencyKey string) (*dto.InvestmentResponse, error)

    GetInvestment(ctx context.Context, userID, investmentID uuid.UUID) (*dto.InvestmentResponse, error)

    ListInvestments(ctx context.Context, userID uuid.UUID,
        query dto.InvestmentListQuery) (*dto.InvestmentListResponse, error)

    PortfolioSummary(ctx context.Context, userID uuid.UUID) (*dto.PortfolioSummaryResponse, error)

    AdminListInvestments(ctx context.Context,
        query dto.AdminInvestmentListQuery) (*dto.AdminInvestmentListResponse, error)

    AdminGetInvestment(ctx context.Context,
        investmentID uuid.UUID) (*dto.InvestmentResponse, error)

    ListPropertyInvestments(ctx context.Context, propertyID uuid.UUID,
        query dto.InvestmentListQuery) (*dto.InvestmentListResponse, error)

    CancelInvestment(ctx context.Context, adminID, investmentID uuid.UUID,
        req dto.CancelInvestmentRequest) error

    Metrics(ctx context.Context) (*dto.InvestmentMetricsResponse, error)
}
```

## 4.2 CreateInvestment algorithm

### Step 1 --- Validate request

Required:

``` text
property_id != nil UUID
slots > 0
Idempotency-Key present
```

Do not accept:

``` text
user_id
amount_kobo
unit_price
status
reference
```

from the client.

### Step 2 --- Begin transaction

Prefer the existing project's GORM transaction convention:

``` go
err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    ...
    return nil
})
```

### Step 3 --- Resolve idempotency

Within the transaction, look up:

``` text
user_id + idempotency_key
```

Compare the stored request hash to the current request.

If completed, return the original investment. If the same key was used
with different inputs, return conflict.

### Step 4 --- Lock property FIRST

``` go
property, err := propertyRepo.FindByIDForUpdate(ctx, propertyID, tx)
```

This is the inventory lock.

Then validate **while locked**:

``` text
not soft deleted
status == active
UnitPrice > 0
slots <= TotalUnits - UnitsSold
```

Do not accept `approved` unless that status actually exists in the
Property module. The established model is
`draft → active → funded → completed`.

### Step 5 --- Calculate amount on the server

``` go
if int64(slots) > math.MaxInt64/property.UnitPrice {
    return validation_error
}

amount := int64(slots) * property.UnitPrice
```

Never trust a client amount.

Minimum investment:

``` go
if property.MinimumInvestment > 0 &&
   amount < property.MinimumInvestment {
    return validation_error
}
```

Also reject if:

``` text
amount > TargetAmount - RaisedAmount
```

### Step 6 --- Lock wallet SECOND

``` go
wallet, err := walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
```

Only `MainBalance` is spendable.

Do not include:

``` text
EarningsBalance
LockedBalance
```

in investment availability.

Validate:

``` go
if wallet.MainBalance < amount {
    return insufficient_funds
}
```

### Step 7 --- Debit wallet

Capture:

``` text
balance_before
balance_after
```

Then:

``` go
wallet.MainBalance -= amount
```

Invariant:

``` text
MainBalance >= 0
```

### Step 8 --- Create wallet transaction

Use existing wallet conventions:

``` text
type = investment
amount = -amount            // if existing wallet uses negative debits
status = completed
reference = investment reference
balance_before = old balance
balance_after = new balance
```

Do not invent a second wallet-ledger format.

### Step 9 --- Generate reference

Preferred:

``` text
INV-<ULID>
```

Fallback:

``` text
INV-<UUID>
```

with a database unique constraint.

Do not use a timestamp alone.

### Step 10 --- Determine unique investor

Use:

``` go
HasActiveInvestmentByUserAndProperty(
    ctx, tx, userID, property.ID,
)
```

If no existing active investment:

``` text
InvestorCount++
```

Otherwise leave it unchanged.

The property is already locked, so concurrent purchases for the same
property are serialized.

### Step 11 --- Create investment

Store:

``` text
UserID
PropertyID
Slots
UnitPriceKobo = property.UnitPrice
AmountKobo = calculated amount
Currency
Status = active
Reference
```

### Step 12 --- Update property

``` text
RaisedAmount += amount
UnitsSold += slots
```

Defensive invariants:

``` text
RaisedAmount <= TargetAmount
UnitsSold <= TotalUnits
```

### Step 13 --- Funded transition

The transition occurs **inside the same transaction**.

Recommended condition:

``` text
RaisedAmount >= TargetAmount
```

and, where the product requires both inventory and funding completion:

``` text
UnitsSold >= TotalUnits
```

When funded:

``` text
Status = funded
```

Do not perform this asynchronously. Otherwise another request could see
an incorrectly active property.

### Step 14 --- Outbox

Create:

``` text
investment.created
```

and, if status changed:

``` text
property.funded
```

using:

``` go
outboxRepo.Create(ctx, event, tx)
```

If outbox insertion fails, return the error and roll back the whole
transaction.

### Step 15 --- Commit

On any failure:

``` text
wallet debit → rolled back
wallet transaction → rolled back
investment → rolled back
property counters → rolled back
investor count → rolled back
funded status → rolled back
outbox → rolled back
idempotency state → rolled back
```

After commit, return `201`.

------------------------------------------------------------------------

# 5. Concurrency Control

## 5.1 Lock order

For investment purchase:

``` text
Property → Wallet
```

For any future workflow touching both resources, preserve this global
order.

This prevents one workflow from doing:

``` text
Property → Wallet
```

while another does:

``` text
Wallet → Property
```

which could deadlock.

## 5.2 Last-slot example

``` text
Property: 10 total, 9 sold

User A requests 1
User B requests 1

A locks property
B waits

A sees 1 available
A purchases
A commits

B gets property lock
B sees 0 available
B gets resource_unavailable
```

Never allow `UnitsSold = 11`.

## 5.3 Partial fulfillment

Reject entirely.

If:

``` text
7 slots available
request = 10
```

return:

``` text
409 resource_unavailable
```

Do not silently buy 7.

## 5.4 Double spending

Because both investment and withdrawal lock the wallet row:

``` text
investment → wallet FOR UPDATE
withdrawal  → wallet FOR UPDATE
```

only one balance mutation can proceed at a time.

------------------------------------------------------------------------

# 6. Portfolio Features

## User endpoints

``` text
GET /api/v1/investments
GET /api/v1/investments/:id
GET /api/v1/investments/portfolio
```

List query:

``` text
page=1
limit=20
status=active
property_type=residential
from=2026-01-01
to=2026-12-31
sort=date|amount|slots
order=asc|desc
```

Whitelist sortable columns. Never concatenate arbitrary client input
into SQL.

Default:

``` text
page=1
limit=20
max limit=100
sort=date
order=desc
```

### GetInvestment

Query ownership directly:

``` sql
WHERE id = ? AND user_id = ?
```

Do not fetch another user's investment and then decide in Go.

This prevents IDOR.

### Portfolio summary

Current safe fields:

``` text
total_invested_kobo
active_investments
completed_investments
cancelled_investments
properties_invested
wallet_main_balance_kobo
earnings_balance_kobo
```

Do not label a projected ROI as realized earnings.

If projected return is later implemented, define the exact ROI formula
first and avoid floating-point money arithmetic.

------------------------------------------------------------------------

# 7. Admin Features

``` text
GET  /api/v1/admin/investments
GET  /api/v1/admin/investments/:id
GET  /api/v1/admin/investments/by-property/:propertyId
POST /api/v1/admin/investments/:id/cancel
GET  /api/v1/admin/investments/metrics
```

All require:

``` go
middleware.Auth(cfg)
middleware.RequireRole("admin")
```

### Admin metrics

Calculate in SQL:

``` text
COUNT(*)
SUM(amount_kobo)
AVG(amount_kobo)
COUNT(DISTINCT user_id)
```

Suggested response:

``` json
{
  "total_investments": 1250,
  "total_amount_kobo": 85000000000,
  "active_investments": 1098,
  "completed_investments": 122,
  "cancelled_investments": 30,
  "unique_investors": 742,
  "average_investment_kobo": 68000000
}
```

------------------------------------------------------------------------

# 8. Cancellation / Refund

Cancellation is **not** a simple status update.

Endpoint:

``` text
POST /api/v1/admin/investments/:id/cancel
```

Request:

``` json
{
  "reason": "Property documentation issue"
}
```

Transaction:

``` text
lock property
lock investment
lock wallet
validate investment == active
mark investment cancelled
refund wallet main_balance
create refund wallet transaction
reduce property raised_amount
reduce property units_sold
recalculate unique investor count
create investment.cancelled outbox event
commit
```

Use a consistent lock order for this workflow too.

Do not permit cancellation after an irreversible settlement/maturity
stage.

If the user has another active investment in the same property, do not
decrement `InvestorCount`.

------------------------------------------------------------------------

# 9. API Design

## 9.1 Create

``` http
POST /api/v1/investments
Authorization: Bearer <JWT>
Idempotency-Key: 01K7Q3D7N9QWJ8Z6B6D3P9M4J2
Content-Type: application/json
```

``` json
{
  "property_id": "b9c9c1f4-3b9c-4a9a-8a4d-2a2d7a7e7f11",
  "slots": 5
}
```

No amount is supplied.

### Success

``` http
201 Created
```

``` json
{
  "success": true,
  "data": {
    "id": "4e5b6f5c-...",
    "property_id": "b9c9c1f4-...",
    "slots": 5,
    "unit_price_kobo": 1000000,
    "amount_kobo": 5000000,
    "currency": "NGN",
    "status": "active",
    "reference": "INV-01K7Q3D7N9QWJ8Z6B6D3P9M4J2",
    "created_at": "2026-10-07T18:00:00Z"
  }
}
```

Use the existing `response.Success` wrapper.

## 9.2 Error mapping

  Condition                Code                       Recommended HTTP
  ------------------------ ------------------------ ------------------
  malformed request        `validation_error`                      400
  property missing         `not_found`                             404
  not owner                `not_found`                             404
  inactive property        `resource_unavailable`                  409
  sold out                 `resource_unavailable`                  409
  insufficient funds       `insufficient_funds`                    422
  minimum not met          `validation_error`                      422
  idempotency mismatch     `idempotency_conflict`                  409
  forbidden admin action   `forbidden`                             403
  unexpected DB failure    internal error                          500

Use the project's existing HTTP error mapping if it differs.

------------------------------------------------------------------------

# 10. Validation

## Handler

``` text
property_id: valid UUID, required
slots: integer > 0
Idempotency-Key: required, 8–255 chars
```

## Service

Revalidate:

``` text
property exists
property not deleted
status == active
UnitPrice > 0
slots > 0
slots <= available
amount > 0
amount <= remaining target
amount >= minimum investment
wallet exists
currency matches
main_balance >= amount
```

## Database

Protect:

``` text
positive slots
positive amounts
valid status
valid currency
unique reference
foreign keys
```

Application rules remain authoritative; database constraints are the
final safety net.

------------------------------------------------------------------------

# 11. Error and Retry Strategy

## Outbox failure

**Same transaction.**

If:

``` text
wallet debit succeeds
investment succeeds
outbox insert fails
```

then:

``` text
ROLLBACK ALL
```

Never commit the financial state first.

If the transaction commits and RabbitMQ is down:

``` text
investment remains committed
outbox remains pending
worker retries later
```

## Database transient errors

Only retry known transient errors such as:

``` text
deadlock detected
serialization failure
```

At most 2--3 attempts with backoff/jitter.

Never retry business failures such as:

``` text
insufficient funds
sold out
inactive property
idempotency conflict
```

------------------------------------------------------------------------

# 12. Repository Design

Recommended methods:

``` go
type InvestmentRepository interface {
    Create(ctx context.Context, inv *models.Investment, tx *gorm.DB) error

    FindByID(ctx context.Context, id uuid.UUID) (*models.Investment, error)

    FindByIDForUpdate(
        ctx context.Context, id uuid.UUID, tx *gorm.DB,
    ) (*models.Investment, error)

    FindByUserAndIdempotencyKey(
        ctx context.Context, userID uuid.UUID,
        key string, tx *gorm.DB,
    ) (*models.Investment, error)

    HasActiveInvestmentByUserAndProperty(
        ctx context.Context, tx *gorm.DB,
        userID, propertyID uuid.UUID,
    ) (bool, error)

    ListByUser(
        ctx context.Context, userID uuid.UUID,
        query dto.InvestmentListQuery,
    ) ([]models.Investment, int64, error)

    ListByProperty(
        ctx context.Context, propertyID uuid.UUID,
        query dto.InvestmentListQuery,
    ) ([]models.Investment, int64, error)

    ListAll(
        ctx context.Context,
        query dto.AdminInvestmentListQuery,
    ) ([]models.Investment, int64, error)

    PortfolioSummary(
        ctx context.Context, userID uuid.UUID,
    ) (*PortfolioSummary, error)

    Metrics(ctx context.Context) (*InvestmentMetrics, error)
}
```

Property additions, if absent:

``` go
FindByIDForUpdate(ctx context.Context, id uuid.UUID, tx *gorm.DB)
UpdateInTransaction(ctx context.Context, property *models.Property, tx *gorm.DB)
```

Reuse the existing Wallet repository.

Do not put business rules into repositories.

------------------------------------------------------------------------

# 13. DTOs

``` go
type CreateInvestmentRequest struct {
    PropertyID uuid.UUID `json:"property_id" binding:"required"`
    Slots      int       `json:"slots" binding:"required,gt=0"`
}
```

The response should include:

``` go
type InvestmentResponse struct {
    ID             uuid.UUID
    PropertyID     uuid.UUID
    Slots          int
    UnitPriceKobo  int64
    AmountKobo     int64
    Currency       string
    Status         string
    Reference      string
    CreatedAt      time.Time
    Property       *PropertyResponse
}
```

Do not expose internal persistence fields such as `DeletedAt`.

------------------------------------------------------------------------

# 14. Handlers and Routes

Handlers remain thin:

``` go
func (h *InvestmentHandler) Create(c *gin.Context) {
    userID := authenticatedUserID(c)

    var req dto.CreateInvestmentRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        h.handleError(c, errors.NewAppError(
            "Invalid request body", "validation_error",
        ))
        return
    }

    key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))

    investment, err := h.service.CreateInvestment(
        c.Request.Context(), userID, req, key,
    )
    if err != nil {
        h.handleError(c, err)
        return
    }

    response.Success(c, http.StatusCreated, investment)
}
```

Routes:

``` go
auth := router.Group("/api/v1")
auth.Use(middleware.Auth(cfg))

auth.POST("/investments", investmentHandler.Create)
auth.GET("/investments", investmentHandler.List)
auth.GET("/investments/portfolio", investmentHandler.Portfolio)
auth.GET("/investments/:id", investmentHandler.Get)

admin := router.Group("/api/v1/admin")
admin.Use(middleware.Auth(cfg))
admin.Use(middleware.RequireRole("admin"))

admin.GET("/investments", investmentHandler.AdminList)
admin.GET("/investments/:id", investmentHandler.AdminGet)
admin.GET("/investments/by-property/:propertyId", investmentHandler.AdminByProperty)
admin.POST("/investments/:id/cancel", investmentHandler.AdminCancel)
admin.GET("/investments/metrics", investmentHandler.AdminMetrics)
```

Ensure `/investments/portfolio` is registered safely relative to `/:id`.

------------------------------------------------------------------------

# 15. Dependency Injection

Follow the existing composition root:

``` go
investmentRepo := repositories.NewInvestmentRepository(db)

investmentService := services.NewInvestmentService(
    db,
    investmentRepo,
    walletRepo,
    propertyRepo,
    outboxRepo,
)

investmentHandler := handlers.NewInvestmentHandler(
    investmentService,
)
```

Do not instantiate repositories inside handlers.

Do not introduce a new dependency-injection framework.

------------------------------------------------------------------------

# 16. Outbox Events

Required:

``` text
investment.created
property.funded
investment.cancelled
investment.completed   // future
```

Example payload:

``` json
{
  "schema_version": 1,
  "investment_id": "...",
  "user_id": "...",
  "property_id": "...",
  "reference": "INV-...",
  "slots": 5,
  "unit_price_kobo": 1000000,
  "amount_kobo": 5000000,
  "currency": "NGN",
  "occurred_at": "2026-10-07T18:00:00Z"
}
```

Do not put credentials or unnecessary sensitive wallet information in
events.

Consumers must be idempotent because delivery is at-least-once.

------------------------------------------------------------------------

# 17. Security

-   User ID always comes from JWT context.
-   Never accept user ID from request body.
-   Scope user investment queries by authenticated user.
-   Admin endpoints require `RequireRole("admin")`.
-   Return non-disclosing `404` for another user's investment.
-   Whitelist sortable/filterable SQL fields.
-   Use GORM parameters; no string-built SQL from user input.
-   Rate-limit investment creation.
-   Log financial operations without tokens/secrets.
-   Do not put credentials in documentation.
-   Do not allow ordinary admin PATCH to change `UnitPrice`,
    `TargetAmount`, `TotalUnits`, or `MinimumInvestment` after the first
    investment.

Recommended property financial-term policy:

``` text
draft:
    editable

active, zero investments:
    editable

active, has investments:
    financial terms locked

funded/completed:
    financial terms locked
```

A correction after investment requires a dedicated audited operation.

------------------------------------------------------------------------

# 18. Performance

## Lock minimization

Inside the transaction only perform:

``` text
SELECT FOR UPDATE
validation
local arithmetic
INSERT/UPDATE
outbox INSERT
COMMIT
```

Do not perform network calls.

## Query optimization

Avoid N+1 queries for portfolio pages. Use controlled joins/preloads.

## Aggregates

Use SQL aggregates:

``` text
COUNT
SUM
AVG
COUNT(DISTINCT user_id)
```

Do not load all investments into memory.

## Caching

Never use stale cache values to authorize a purchase.

Caching may be used for public/read-only property displays, but the
purchase transaction always reads PostgreSQL.

------------------------------------------------------------------------

# 19. Monitoring and Observability

## Metrics

Track:

``` text
investment_create_total
investment_create_success_total
investment_create_failure_total
investment_insufficient_funds_total
investment_sold_out_total
investment_idempotency_replay_total
investment_transaction_duration_seconds
investment_transaction_deadlock_total
investment_amount_kobo_total
investment_outbox_pending
investment_outbox_failed
property_funded_total
investment_cancel_total
```

## Structured logs

Success:

``` text
Investment created
```

Fields:

``` text
investment_id
user_id
property_id
slots
amount_kobo
reference
duration_ms
```

Failure:

``` text
Investment failed
```

Fields:

``` text
user_id
property_id
error_code
duration_ms
```

Never log JWTs, passwords, Paystack secrets, or other credentials.

## Alerts

Alert on:

``` text
outbox backlog growth
outbox failures
database deadlocks
p95/p99 investment latency
negative-balance constraint failures
property counter discrepancies
unexpected cancellation spikes
```

------------------------------------------------------------------------

# 20. Testing Strategy

## 20.1 Unit tests

Cover:

1.  invalid slots;
2.  missing property;
3.  inactive property;
4.  sold out;
5.  minimum investment failure;
6.  insufficient funds;
7.  currency mismatch;
8.  successful purchase;
9.  property becomes funded;
10. first investment increments investor count;
11. subsequent investment does not increment it;
12. idempotency replay;
13. idempotency request mismatch;
14. integer overflow;
15. repository failure propagation.

## 20.2 Repository integration tests

Use real PostgreSQL.

Test:

``` text
FindByIDForUpdate
Create
ListByUser
ListByProperty
PortfolioSummary
HasActiveInvestmentByUserAndProperty
Metrics
filters
sorting
```

## 20.3 Rollback tests

Force failure after:

``` text
wallet update
wallet transaction
investment insert
property update
outbox insert
```

Then assert:

``` text
wallet unchanged
wallet transaction absent
investment absent
property unchanged
outbox absent
```

## 20.4 Concurrency tests

### Last slot

``` text
10 total
9 sold
A requests 1
B requests 1
```

Expected:

``` text
one success
one resource_unavailable
units_sold = 10
```

### Partial inventory

``` text
5 available
A requests 3
B requests 3
```

Expected:

``` text
one success
one failure
no overselling
```

### Double spend

``` text
wallet = ₦100,000
investment = ₦70,000
withdrawal = ₦50,000
```

Expected:

``` text
never negative
```

### Idempotency race

Send the same request concurrently with the same key.

Expected:

``` text
one investment
one wallet debit
one investment.created event
```

## 20.5 Authorization

Test:

``` text
User A cannot read User B investment
User cannot access admin list
User cannot cancel
Admin can access admin endpoints
```

## 20.6 Load test

Start with:

``` text
100–500 concurrent requests
10–100 available slots
```

Assert:

``` text
successful slots <= total units
successful amount <= target
wallet never negative
no duplicate idempotency result
```

Measure:

``` text
p95/p99 latency
lock wait
deadlocks
DB CPU
success/failure ratio
```

------------------------------------------------------------------------

# 21. Phase-by-Phase Implementation Plan

## Phase 1 --- Repository Audit

**Time:** 1--2 hours

Tasks:

-   inspect Investment model/repository;
-   inspect Wallet transaction conventions;
-   inspect Property `AcceptsInvestment`;
-   inspect existing transaction helper;
-   inspect Outbox repository;
-   inspect migration numbering;
-   identify existing idempotency support.

Acceptance:

-   exact current interfaces known;
-   wallet debit sign confirmed;
-   property status confirmed;
-   no duplicate infrastructure planned.

------------------------------------------------------------------------

## Phase 2 --- Database + Model

**Time:** 2--3 hours

Tasks:

-   enhance Investment;
-   add price snapshot;
-   add currency;
-   add lifecycle fields where required;
-   add constraints/indexes;
-   add idempotency mechanism if absent;
-   write rollback migration.

Acceptance:

``` text
migration up works
migration down works
go test ./...
```

------------------------------------------------------------------------

## Phase 3 --- Repository Layer

**Time:** 2--3 hours

Implement:

``` text
Create
FindByID
FindByIDForUpdate
FindByUserAndIdempotencyKey
HasActiveInvestmentByUserAndProperty
ListByUser
ListByProperty
ListAll
PortfolioSummary
Metrics
```

Acceptance:

-   context-aware;
-   transaction-aware;
-   parameterized;
-   integration tests pass.

------------------------------------------------------------------------

## Phase 4 --- Core Service

**Time:** 4--6 hours

Implement `CreateInvestment`.

Exact sequence:

``` text
idempotency
→ BEGIN
→ lock property
→ validate property
→ calculate amount
→ lock wallet
→ validate balance
→ wallet debit
→ wallet transaction
→ investment
→ property counters
→ investor count
→ funded transition
→ outbox
→ COMMIT
```

Acceptance:

-   all invariants enforced;
-   no network calls in transaction;
-   all-or-nothing behavior.

------------------------------------------------------------------------

## Phase 5 --- Portfolio

**Time:** 2--3 hours

Implement:

``` text
GetInvestment
ListInvestments
PortfolioSummary
```

Add:

``` text
pagination
filters
sorting
ownership checks
```

Acceptance:

-   no IDOR;
-   no N+1;
-   limit \<= 100;
-   deterministic ordering.

------------------------------------------------------------------------

## Phase 6 --- Admin

**Time:** 2--3 hours

Implement:

``` text
AdminList
AdminGet
ByProperty
Metrics
Cancel/Refund
```

Acceptance:

-   RBAC;
-   cancellation transactional;
-   refund wallet transaction created;
-   property counters restored;
-   investor count correct.

------------------------------------------------------------------------

## Phase 7 --- Handlers + Routes + DI

**Time:** 2--3 hours

Tasks:

-   create handler;
-   register routes;
-   add auth/RBAC;
-   bind DTO;
-   read Idempotency-Key;
-   wire service/repository/handler.

Acceptance:

-   application starts;
-   endpoints reachable;
-   handler contains no business logic.

------------------------------------------------------------------------

## Phase 8 --- Outbox + Reliability

**Time:** 1--2 hours

Tasks:

-   event payloads;
-   transactional event creation;
-   verify dispatcher;
-   test RabbitMQ outage.

Acceptance:

-   DB transaction succeeds while RabbitMQ is down;
-   pending outbox remains durable;
-   worker can publish later.

------------------------------------------------------------------------

## Phase 9 --- Validation + Error Hardening

**Time:** 1--2 hours

Review:

``` text
overflow
currency
minimum investment
maximum rules
state transitions
idempotency
HTTP mapping
```

Acceptance:

-   stable error codes;
-   no client financial amount trusted.

------------------------------------------------------------------------

## Phase 10 --- Testing

**Time:** 5--8 hours

Run:

``` text
unit
repository
rollback
concurrency
idempotency
authorization
load
```

Acceptance:

-   no overselling;
-   no double spending;
-   no partial financial commits.

------------------------------------------------------------------------

## Phase 11 --- Documentation

**Time:** 2--3 hours

Create:

``` text
INVESTMENT_MODULE_IMPLEMENTATION_PLAN.md
INVESTMENT_MODULE_COMPLETE.md
INVESTMENT_API_GUIDE.md
INVESTMENT_TESTING_GUIDE.md
```

Document API, idempotency, errors, concurrency, wallet effects, and
property status behavior.

------------------------------------------------------------------------

# 22. File Plan

``` text
internal/
├── models/
│   └── investment.go
├── repositories/
│   └── investment_repository.go
├── services/
│   └── investment_service.go
├── handlers/
│   └── investment_handler.go
├── dto/
│   └── investment_dto.go
├── database/
│   └── migrations/
│       └── <next>_investment_hardening.sql
└── routes/
    └── v1/
        └── routes.go

tests:
├── internal/services/investment_service_test.go
├── internal/repositories/investment_repository_test.go
└── internal/handlers/investment_handler_test.go
```

Use the project's actual existing file naming conventions if they
differ.

------------------------------------------------------------------------

# 23. Financial Invariants

After every committed transaction:

### Wallet

``` text
main_balance >= 0
earnings_balance >= 0
locked_balance >= 0
```

### Property

``` text
0 <= units_sold <= total_units
0 <= raised_amount <= target_amount
investor_count >= 0
```

### Investment

``` text
slots > 0
unit_price_kobo > 0
amount_kobo > 0
amount_kobo = slots × unit_price_kobo
reference unique
status valid
```

### Aggregate consistency

For active investments:

``` text
property.units_sold
    = SUM(active investment slots)
```

and:

``` text
property.raised_amount
    = SUM(active investment amounts)
```

subject to the project's explicit cancellation/refund accounting policy.

A future reconciliation job should compare these values and alert on
discrepancies rather than silently correcting money.

------------------------------------------------------------------------

# 24. Explicit Answers to the Ten Design Questions

### 1. Outbox failure after investment?

Impossible as a committed partial state. The outbox insert is inside the
same DB transaction. Failure rolls everything back.

### 2. Property reaching 100%?

Change `active → funded` in the same transaction as the final purchase.

### 3. Investor count?

Yes. Count unique active investors. Increment only on the user's first
active investment in that property.

### 4. Minimum investment?

Compare the calculated server-side amount to `MinimumInvestment`.

### 5. Maximum investment?

Do not invent a platform-wide maximum. Support an optional
property-level maximum when the product defines it.

### 6. Pending withdrawal?

Only `MainBalance` is spendable. Locked withdrawal funds are not
available.

### 7. Reference?

`INV-<ULID>` preferred; `INV-<UUID>` acceptable if avoiding a
dependency.

### 8. Retry/idempotency?

Require `Idempotency-Key`, scoped by authenticated user, with
request-hash validation.

### 9. Partial fulfillment?

Reject. Never silently purchase fewer slots.

### 10. Price consistency?

Lock the property, calculate from its current `UnitPrice`, and snapshot
that price into the investment.

------------------------------------------------------------------------

# 25. Future Enhancements

## Returns

Create separate return/earning ledger records. Never alter original
investment amount to represent profit.

## Dividends

Future flow:

``` text
property distribution
→ calculate investor entitlement
→ earnings ledger
→ wallet earnings balance
```

Every distribution must be idempotent.

## Secondary market

Do not overload `Investment`.

Use separate concepts such as:

``` text
sell_orders
buy_orders
trades
settlements
```

## Recommendations

Keep recommendations outside the financial transaction path.

## Reconciliation

Future scheduled checks:

``` text
wallet ledger ↔ wallet balances
investments ↔ property counters
```

Discrepancies should generate alerts/manual investigation.

------------------------------------------------------------------------

# 26. Definition of Done

### Architecture

-   [ ] Handler → Service → Repository maintained.
-   [ ] Existing error handling used.
-   [ ] Existing logger used.
-   [ ] Existing response wrapper used.
-   [ ] No new architecture invented.

### Financial correctness

-   [ ] One transaction for purchase.
-   [ ] Property row locked.
-   [ ] Wallet row locked.
-   [ ] Consistent lock order.
-   [ ] No negative wallet balance.
-   [ ] No property overselling.
-   [ ] Server-calculated amount.
-   [ ] Price snapshot.
-   [ ] Unique reference.
-   [ ] Wallet transaction for every debit.
-   [ ] Atomic funded transition.
-   [ ] Transactional outbox.

### Reliability

-   [ ] Idempotency implemented.
-   [ ] RabbitMQ failure does not lose committed investment.
-   [ ] Duplicate request does not duplicate debit.
-   [ ] Crash-after-commit retry is safe.

### Security

-   [ ] Ownership checks.
-   [ ] Admin RBAC.
-   [ ] No client user ID.
-   [ ] No client amount.
-   [ ] SQL fields whitelisted.
-   [ ] Rate limiting considered.
-   [ ] Secrets never logged.

### Testing

-   [ ] Unit tests.
-   [ ] Repository tests.
-   [ ] Rollback tests.
-   [ ] Last-slot concurrency test.
-   [ ] Double-spend test.
-   [ ] Idempotency race test.
-   [ ] IDOR tests.
-   [ ] Admin tests.
-   [ ] Load/concurrency tests.

### Documentation

-   [ ] API guide.
-   [ ] Testing guide.
-   [ ] Completion document.
-   [ ] Migration documented.
-   [ ] Error codes documented.
-   [ ] Idempotency behavior documented.

------------------------------------------------------------------------

# 27. Recommended Execution Order

``` text
Audit existing code
      ↓
Confirm schema/migration number
      ↓
Harden Investment model/schema
      ↓
Add repository locking/query methods
      ↓
Repository tests
      ↓
Implement CreateInvestment
      ↓
Add outbox
      ↓
Rollback tests
      ↓
Concurrency tests
      ↓
Portfolio queries
      ↓
Admin queries
      ↓
Cancellation/refund
      ↓
Handlers
      ↓
Routes
      ↓
Dependency injection
      ↓
Integration tests
      ↓
Load/concurrency tests
      ↓
Observability review
      ↓
Documentation
      ↓
Final review
```

Do not start with handlers. The transaction and its invariants are the
core of this milestone.

------------------------------------------------------------------------

# 28. Final Architectural Position

The Investment Module should be treated as a **ledger-producing
transactional workflow**.

The desired invariant is:

``` text
ONE COMMITTED INVESTMENT
=
ONE WALLET DEBIT
+
ONE WALLET TRANSACTION
+
ONE INVESTMENT RECORD
+
PROPERTY CAPACITY/FUNDING UPDATE
+
REQUIRED OUTBOX EVENT(S)
```

All of these must commit together.

Concurrency safety comes from:

``` text
property row lock
+
wallet row lock
+
consistent lock ordering
```

Reliability comes from:

``` text
database transaction
+
outbox
+
idempotency
```

Historical correctness comes from:

``` text
server-calculated amount
+
price snapshot
+
immutable financial record
+
wallet audit transaction
```

Production safety comes from:

``` text
structured logging
+
metrics
+
reconciliation
+
concurrency testing
```

This design intentionally rejects ambiguous or risky behavior: no
partial fills, no client-controlled amount, no asynchronous
funding-status transition, no RabbitMQ dependency inside the purchase
transaction, and no ordinary price mutation after users have invested.

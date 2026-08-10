# 🔍 Milestone 3: Wallet System - Investigation & Implementation Plan

**Date:** 2026-08-05  
**Investigator:** Senior Backend Engineer  
**Status:** Investigation Complete, Ready for Implementation

---

## 📊 Executive Summary

Milestone 3 (Wallet System) is **~60% complete**. The service layer, repository layer, and data models are fully implemented with excellent architecture. **Only the HTTP layer (handlers + routes) is missing.**

**Estimated Time to Complete:** 2-3 hours  
**Risk Level:** Low (leveraging existing, well-designed code)  
**External Dependencies:** Payment provider configuration (Paystack/Flutterwave)

---

## 🔍 Investigation Findings

### ✅ What Already Exists (Excellent Quality)

#### 1. **Data Models** (`internal/models/`)
**Status:** ✅ **COMPLETE**

- ✅ `Wallet` model (8 fields)
  - MainBalance, EarningsBalance (in kobo - correct financial design)
  - Currency support (NGN default, multi-currency ready)
  - Virtual account fields (for future provider integration)
  - One-to-one with User (enforced by unique index)

- ✅ `WalletTransaction` model (14 fields)
  - Append-only ledger design (correct!)
  - BalanceBefore/BalanceAfter for audit trail
  - Reference + ExternalReference for idempotency
  - JSONB metadata for provider payloads
  - Status tracking (pending/completed/failed)

- ✅ `Payment` model (13 fields)
  - Provider-side record (separate from wallet ledger - good design!)
  - Raw payload storage for reconciliation
  - Status lifecycle (pending/success/failed)
  - Channel tracking (deposit/withdrawal)

**Quality Assessment:** Excellent separation of concerns. Models follow financial best practices.

---

#### 2. **Repository Layer** (`internal/repositories/`)
**Status:** ✅ **COMPLETE**

- ✅ `WalletRepository` - 13 methods
  - CRUD operations
  - `FindByUserIDForUpdate()` - Row locking for concurrency (correct!)
  - `TransactionExists()` - Idempotency check
  - `ListTransactions()` - Filtered pagination
  - Transaction safety built-in

- ✅ `PaymentRepository` - 6 methods
  - `FindByReference()` - Lookup by reference
  - `MarkSuccess()/MarkFailed()` - Status updates
  - `ListByUser()` - Payment history

**Quality Assessment:** Production-ready. Proper use of DB transactions, row locking, and idempotency patterns.

---

#### 3. **Service Layer** (`internal/services/wallet_service.go`)
**Status:** ✅ **COMPLETE** (300+ lines, fully implemented)

**Interface Methods:**
1. ✅ `GetWallet()` - Retrieve user's wallet
2. ✅ `InitiateDeposit()` - Start payment flow
3. ✅ `CreditFromPaymentReference()` - Webhook handler (idempotent!)
4. ✅ `InitiateWithdrawal()` - Debit wallet, queue payout
5. ✅ `GetTransactionHistory()` - Paginated ledger
6. ✅ `VerifyWebhookSignature()` - Provider signature validation

**Key Design Decisions** (all correct):
- ✅ Money stored as int64 kobo (no float arithmetic)
- ✅ Deposits are idempotent (duplicate webhooks safe)
- ✅ Withdrawals debit immediately, process async
- ✅ Every balance change writes ledger row
- ✅ Transactions use row-level locking (SELECT FOR UPDATE)
- ✅ Balance can never go negative (guarded)

**Quality Assessment:** Excellent. Follows financial transaction best practices. Production-grade error handling.

---

#### 4. **DTOs** (`internal/dto/wallet_dto.go`)
**Status:** ✅ **COMPLETE**

- ✅ `WalletResponse` - Public wallet representation
- ✅ `DepositRequest/Response` - Deposit flow
- ✅ `WithdrawRequest` - Withdrawal flow
- ✅ `TransactionResponse` - Ledger row response

**Quality Assessment:** Clean, well-documented, correct field types.

---

#### 5. **Database Migrations**
**Status:** ✅ **COMPLETE**

- ✅ `wallets` table (migration 000001 + 000005 for currency)
- ✅ `wallet_transactions` table (migration 000001)
- ✅ `payments` table (migration 000001)

**Quality Assessment:** Schema is production-ready.

---

### ❌ What's Missing (Quick Win!)

#### 1. **HTTP Handlers** (`internal/handlers/wallet.go`)
**Status:** ❌ **DOES NOT EXIST**

**Needed:**
- Handler struct with dependencies
- 5 HTTP handler methods
- Request validation
- Error mapping to HTTP status codes
- Response formatting

**Estimated Time:** 1-2 hours

---

#### 2. **Routes Registration** (`internal/routes/v1/routes.go`)
**Status:** ❌ **NOT REGISTERED**

**Needed:**
- Add wallet routes group
- Register 5 endpoints
- Apply auth middleware
- Wire up handler

**Estimated Time:** 15 minutes

---

#### 3. **Dependency Injection** (`cmd/api/main.go`)
**Status:** ❌ **NOT WIRED**

**Needed:**
- Create WalletHandler instance
- Wire up dependencies
- Pass to router

**Estimated Time:** 15 minutes

---

#### 4. **Payment Provider Integration** (`internal/payments/`)
**Status:** ❌ **PACKAGE DOES NOT EXIST**

**What's Needed:**
```go
// internal/payments/provider.go
type Provider interface {
    Name() string
    InitializeDeposit(ctx, email, amountKobo, reference) (*InitResult, error)
    VerifyTransaction(ctx, reference) (*VerifyResult, error)
    VerifyWebhookSignature(signature, body) bool
}
```

**Implementations needed:**
1. **MockProvider** (for development/testing)
2. **PaystackProvider** (for production - Nigeria)
3. **FlutterwaveProvider** (alternative)

**Estimated Time:** 2-3 hours for mock + one real provider

---

#### 5. **Queue Package** (`internal/queue/`)
**Status:** ❌ **PACKAGE DOES NOT EXIST**

**What's Needed:**
- Queue client interface
- Mock implementation (logs to console)
- Redis implementation (optional)

**Note:** Service already checks `if s.mq != nil` so it's safe to pass nil for now.

**Estimated Time:** 1 hour for mock, 3-4 hours for Redis

---

## 📋 Implementation Plan

### Phase 1: HTTP Layer (2-3 hours) 🔴 **DO THIS FIRST**

This gets wallet endpoints working with mock payment provider.

#### Task 1.1: Create Payment Provider Package (1 hour)
**Priority:** 🔴 CRITICAL

**What to create:**
```
internal/
└── payments/
    ├── provider.go      # Interface definition
    └── mock.go          # Mock provider for development
```

**Why first:** WalletService depends on `payments.Provider` interface.

**Teaching Notes:**
- Explain interface-based design
- Why mock providers are essential for testing
- How to design provider-agnostic payment flows

---

#### Task 1.2: Create Wallet Handler (1-1.5 hours)
**Priority:** 🔴 CRITICAL

**File:** `internal/handlers/wallet.go`

**Structure:**
```go
type WalletHandler struct {
    walletService services.WalletService
}

// 5 handler methods:
// 1. GetWallet()          - GET /wallet
// 2. InitiateDeposit()    - POST /wallet/deposit
// 3. RequestWithdrawal()  - POST /wallet/withdraw
// 4. GetTransactions()    - GET /wallet/transactions
// 5. HandleWebhook()      - POST /webhooks/payment (public, no auth!)
```

**Teaching Focus:**
- HTTP handler pattern (request → service → response)
- Request validation with Gin binding
- Error mapping (service error → HTTP status code)
- Webhook security (signature verification)
- Idempotency handling
- Why webhook endpoint is public (external system calls it)

---

#### Task 1.3: Register Routes (15 minutes)
**Priority:** 🔴 CRITICAL

**File:** `internal/routes/v1/routes.go`

**Changes:**
```go
// Add wallet routes group
walletGroup := authenticated.Group("/wallet")
{
    walletGroup.GET("", walletHandler.GetWallet)
    walletGroup.POST("/deposit", walletHandler.InitiateDeposit)
    walletGroup.POST("/withdraw", walletHandler.RequestWithdrawal)
    walletGroup.GET("/transactions", walletHandler.GetTransactions)
}

// Webhook endpoint (public, no auth!)
router.POST("/webhooks/payment", walletHandler.HandleWebhook)
```

**Teaching Focus:**
- Route grouping for organization
- Middleware application (auth)
- Public vs authenticated routes
- RESTful resource naming

---

#### Task 1.4: Wire Dependencies (15 minutes)
**Priority:** 🔴 CRITICAL

**File:** `cmd/api/main.go`

**Changes:**
```go
// Create payment provider (mock for now)
paymentProvider := payments.NewMockProvider()

// Create wallet service
walletService := services.NewWalletService(
    walletRepo,
    userRepo,
    paymentRepo,
    paymentProvider,
    notificationService,
    nil, // queue client (optional)
    cfg,
    db,
)

// Create wallet handler
walletHandler := handlers.NewWalletHandler(walletService)

// Pass to router setup
v1.SetupRoutes(router, authHandler, userHandler, walletHandler, cfg)
```

**Teaching Focus:**
- Dependency injection pattern
- Constructor pattern
- Interface composition
- Why we inject dependencies vs global state

---

### Phase 2: Testing & Validation (30 minutes)

#### Task 2.1: Update Postman Collection
Add 5 new requests:
1. GET /wallet
2. POST /wallet/deposit
3. POST /wallet/withdraw
4. GET /wallet/transactions
5. POST /webhooks/payment (for testing)

#### Task 2.2: Manual Testing Flow
1. Register/login user
2. Get wallet (should show ₦0.00)
3. Initiate deposit
4. Simulate webhook (mock provider)
5. Get wallet (should show deposited amount)
6. View transaction history
7. Request withdrawal
8. Get wallet (balance reduced)

---

### Phase 3: Payment Provider Integration (2-3 hours) 🟡 **OPTIONAL FOR MVP**

#### Task 3.1: Create Paystack Provider
**File:** `internal/payments/paystack.go`

**What it does:**
- Real API calls to Paystack
- Initialize transaction
- Verify transaction
- Webhook signature validation

**Configuration needed:**
```env
PAYSTACK_SECRET_KEY=sk_test_xxxxx
PAYSTACK_PUBLIC_KEY=pk_test_xxxxx
```

**Teaching Focus:**
- External API integration
- API authentication (Bearer token)
- Webhook signature verification (HMAC)
- Error handling for external services
- Retry strategies

---

#### Task 3.2: Create Flutterwave Provider (Alternative)
**File:** `internal/payments/flutterwave.go`

Similar to Paystack, but Flutterwave API.

---

### Phase 4: Queue Implementation (1-3 hours) 🟢 **NICE TO HAVE**

#### Task 4.1: Create Queue Package
**File:** `internal/queue/client.go`

**Implementations:**
1. **MockQueue** - Logs to console
2. **RedisQueue** - Real message broker

**Why it's optional:**
Wallet service already checks `if s.mq != nil`, so withdrawal processing works without queue (just slower, synchronous).

**Teaching Focus:**
- Async processing benefits
- Message queue patterns
- When to use queues vs sync
- Redis as message broker

---

## 🎯 Recommended Execution Order

### **Today: Get Wallet Endpoints Working** (2-3 hours total)

```
1. Create payments package with mock provider     (1 hour)
   ↓
2. Create wallet handler                          (1-1.5 hours)
   ↓
3. Register routes                                (15 min)
   ↓
4. Wire dependencies in main.go                   (15 min)
   ↓
5. Test in Postman                                (30 min)
```

**Result:** Fully functional wallet system with mock payments!

---

### **This Week: Add Real Payment Provider** (2-3 hours)

```
1. Sign up for Paystack account                   (15 min)
   ↓
2. Get API keys (test mode)                       (5 min)
   ↓
3. Implement PaystackProvider                     (2 hours)
   ↓
4. Update config to use Paystack                  (5 min)
   ↓
5. Test real payment flow                         (30 min)
```

**Result:** Production-ready payment processing!

---

### **Next Week: Add Queue (Optional)** (1-3 hours)

```
1. Install Redis                                  (15 min)
   ↓
2. Implement queue package                        (1-2 hours)
   ↓
3. Create worker process                          (1 hour)
   ↓
4. Test async withdrawal processing               (30 min)
```

**Result:** Async withdrawal processing for better performance!

---

## 📖 Teaching Plan

### For Each Task, I Will Explain:

#### 1. **The "Why"**
- Why this file exists
- Why this pattern is used
- Why these dependencies are needed
- Why this approach vs alternatives

#### 2. **The Architecture**
- How it fits in the request flow
- How it interacts with other layers
- How data flows through the system
- How errors propagate

#### 3. **The Details**
- Every struct field explained
- Every parameter explained
- Every return value explained
- Business rules enforced
- Edge cases handled

#### 4. **The Trade-offs**
- Performance implications
- Security considerations
- Maintainability impact
- Scalability concerns

#### 5. **Go-Specific Concepts**
- Interface composition
- Pointer vs value receivers
- Error handling patterns
- Context usage
- Goroutine safety (if relevant)

---

## 🎓 Key Learning Topics

### Topics Covered in Milestone 3:

1. **Financial Transaction Safety**
   - Why integer arithmetic for money
   - Row-level locking (SELECT FOR UPDATE)
   - Transaction isolation levels
   - Idempotency patterns
   - Audit trail design

2. **Payment Provider Integration**
   - Interface-based design
   - Provider abstraction
   - Webhook security
   - Signature verification
   - External API error handling

3. **HTTP Layer Design**
   - Handler patterns
   - Request validation
   - Error mapping
   - Response formatting
   - Public vs authenticated routes

4. **Async Processing** (optional)
   - Message queues
   - Background workers
   - When to use async vs sync
   - Error retry strategies

5. **Testing Strategies**
   - Mock providers for testing
   - Integration testing with database
   - Webhook testing
   - Idempotency testing

---

## ⚠️ Critical Implementation Notes

### 1. Financial Safety Rules (NEVER VIOLATE)
- ✅ Money is always int64 kobo (never float)
- ✅ Balance can NEVER go negative
- ✅ Every balance change writes ONE ledger row
- ✅ Ledger rows are IMMUTABLE (never UPDATE or DELETE)
- ✅ Use transactions for atomic updates
- ✅ Use row locking for concurrent operations

### 2. Idempotency Requirements
- ✅ Deposits must be idempotent (duplicate webhooks)
- ✅ Check `TransactionExists()` before crediting
- ✅ Use unique references for deduplication
- ✅ Store provider payloads for reconciliation

### 3. Security Requirements
- ✅ Validate webhook signatures (never trust webhooks blindly)
- ✅ Server-side verify all transactions
- ✅ Auth required for all wallet endpoints (except webhook)
- ✅ Rate limiting on withdrawal requests (prevent abuse)

### 4. Error Handling
- ✅ Map service errors to appropriate HTTP status codes
- ✅ Never expose internal errors to clients
- ✅ Log all payment failures for investigation
- ✅ Return user-friendly error messages

---

## 📊 Progress Tracking

### Before Implementation:
- [x] Models complete
- [x] Repository complete
- [x] Service complete
- [x] DTOs complete
- [x] Migrations complete
- [ ] Payment provider (mock needed)
- [ ] Handlers (not started)
- [ ] Routes (not registered)
- [ ] DI wiring (not done)
- [ ] Testing (not started)

### After Phase 1 (HTTP Layer):
- [x] Mock payment provider
- [x] Wallet handler (5 methods)
- [x] Routes registered
- [x] Dependencies wired
- [x] Postman collection updated
- [x] Manual testing complete

### After Phase 2 (Real Provider):
- [x] Paystack provider
- [x] Real payment flow tested
- [x] Webhook tested with real signature

### After Phase 3 (Queue - Optional):
- [x] Queue package
- [x] Redis integration
- [x] Worker process
- [x] Async withdrawals

---

## 🚀 Ready to Proceed

**Recommendation:** Start with Phase 1 (HTTP Layer) - 2-3 hours to get fully functional wallet with mock payments.

**Benefits:**
1. Quick win - working feature in hours
2. Can test entire flow immediately
3. Real provider can be added later without changing handlers
4. Builds confidence in the architecture

**Next Steps:**
1. Get your approval on this plan
2. Start with Task 1.1 (Payment Provider Package)
3. Proceed through tasks with full explanations
4. Test each component as we build

---

**Status:** ✅ **Investigation Complete, Ready for Implementation**

Would you like me to proceed with Phase 1 (HTTP Layer)?


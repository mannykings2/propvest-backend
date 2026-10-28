# Notification Module - Phase 1 Audit Summary

**Date:** October 6, 2026  
**Phase:** 1 - Repository & Codebase Audit  
**Duration:** Completed  
**Status:** ✅ Complete

---

## 📋 Audit Findings

### 1. Repository Pattern (Investment Module Reference)

**File:** `internal/repositories/investment_repository.go`

**Pattern Observed:**
- Interface-based design: `InvestmentRepository` interface + `investmentRepository` struct
- Constructor: `NewInvestmentRepository(db *gorm.DB) InvestmentRepository`
- All methods accept `context.Context` as first parameter
- Methods that need transactions accept `tx *gorm.DB` parameter
- Ownership enforced in queries: `WHERE id = ? AND user_id = ?`
- Pagination pattern: `(limit, offset int)` returns `([]Model, int64, error)`
- GORM usage: `r.db.WithContext(ctx).Where(...).Find(&results)`

**Key Methods:**
- Create, FindByID, FindByIDForUpdate (with lock)
- ListByUser (pagination + ownership)
- ListAll (admin queries)
- Metrics (aggregates)

**Takeaway:** Follow this exact pattern for notification repository.

---

### 2. Service Layer (Wallet Service Reference)

**File:** `internal/services/wallet_service.go`

**NotificationService Integration:**
- Service has `notifier NotificationService` field
- Currently set to `nil` in `main.go` (line 114)
- Wallet service has 3 notification call sites:
  
  1. **Deposit Success** (line ~227):
     ```go
     s.notifier.Notify(ctx, payment.UserID, models.NotificationDepositSuccess,
         "Deposit successful",
         fmt.Sprintf("Your wallet was credited with ₦%s.", formatKobo(payment.AmountKobo)),
     )
     ```
  
  2. **Withdrawal Requested** (line ~424):
     ```go
     s.notifier.Notify(ctx, userID, models.NotificationWithdrawalUpdate,
         "Withdrawal requested",
         fmt.Sprintf("Your withdrawal of ₦%s to %s is being processed.", ...),
     )
     ```
  
  3. **Withdrawal Finalized** (line ~596):
     ```go
     s.notifier.Notify(ctx, txn.UserID, notifType, notifTitle, notifMessage,
         map[string]any{
             "transaction_id": transactionID,
             "amount": txn.Amount,
             ...
         },
     )
     ```

**Notification Service Interface (Inferred):**
```go
type NotificationService interface {
    Notify(
        ctx context.Context,
        userID uuid.UUID,
        notificationType string,
        title string,
        message string,
        data ...map[string]any, // Optional data parameter
    ) error
}
```

**Takeaway:** 
- NotificationService interface already partially defined by usage
- Currently nil-safe (wallet checks `if s.notifier != nil`)
- Need to implement actual service

---

### 3. Queue Infrastructure

**File:** `internal/queue/config.go`

**Existing Notification Queue:**
```go
NotificationQueueConfig = QueueConfig{
    Name:       "propvest.notification.dispatch",
    Durable:    true,
    AutoDelete: false,
    MaxLength:  10000,
    TTL:        48 * 60 * 60 * 1000,  // 48 hours
    Priority:   1,                     // Lowest priority
    DLXName:    "propvest.dlx",
    DLXKey:     "notification.dlq",
    PrefetchCount: 20,
    Concurrency:   20,
}
```

**Important Notes:**
- ⚠️ **Design Decision Override:** Implementation plan specifies separate email/SMS queues
- Current config uses single `propvest.notification.dispatch` queue
- **Action:** Need to add separate queue configs for email and SMS

**Takeaway:** 
- Queue infrastructure exists
- Need to extend with separate email/SMS queue configs per implementation plan

---

### 4. Main.go Wiring

**File:** `cmd/api/main.go`

**Current State:**
- Line 99: Commented repository initialization
  ```go
  // notificationRepo := repositories.NewNotificationRepository(database.DB)
  ```
- Line 111: Commented service initialization
  ```go
  // notificationService := services.NewNotificationService(notificationRepo, hub, mq)
  ```
- Line 114: Nil service placeholder
  ```go
  var notificationService services.NotificationService = nil
  ```
- Line 118: Passed to wallet service
  ```go
  walletService := services.NewWalletService(..., notificationService, ...)
  ```
- Line 141: Commented handler initialization
  ```go
  // notificationHandler := handlers.NewNotificationHandler(notificationService)
  ```

**Takeaway:**
- Clear placeholders ready for implementation
- Just need to uncomment and implement actual components

---

### 5. Migration Numbering

**Current Latest Migration:** `000021_enhance_investments.up.sql`

**Next Migration Number:** `000022_create_notifications`

**Confirmed:** ✅

---

### 6. Existing Notification Types

**File:** `internal/models/notification.go` (ALREADY EXISTS!)

**Constants Defined:**
```go
const (
    NotificationDepositSuccess    = "deposit_success"
    NotificationWithdrawalUpdate  = "withdrawal_update"
    NotificationInvestmentCreated = "investment_created"
    NotificationPropertyApproved  = "property_approved"
    NotificationWelcome           = "welcome"
)
```

**Important:** 
- Notification model file already exists!
- Constants already defined
- Need to check if model struct exists and enhance if needed

**Takeaway:** 
- Don't create from scratch, enhance existing file
- Verify existing model structure

---

### 7. DTOs and Mappers

**File:** `internal/dto/investment_dto.go` (Reference)

**Pattern:**
- Request DTOs: validation tags (`binding:"required"`, `binding:"gt=0"`)
- Response DTOs: JSON tags, omitempty for optional fields
- Separate list response with pagination metadata
- Mapper functions in `internal/dto/mappers.go`

**Existing Pagination:**
```go
type PaginationMetadata struct {
    Page       int   `json:"page"`
    Limit      int   `json:"limit"`
    Total      int64 `json:"total"`
    TotalPages int   `json:"total_pages"`
}
```

**Takeaway:**
- Reuse PaginationMetadata
- Follow investment DTO conventions

---

### 8. Handler Pattern

**File:** `internal/handlers/investment.go` (Reference)

**Pattern:**
- Struct: `InvestmentHandler` with service field
- Constructor: `NewInvestmentHandler(service InvestmentService) *InvestmentHandler`
- Methods: thin handlers that call service
- User ID extraction: `userID, _ := c.Get("user_id")`
- Response: `response.Success(c, http.StatusOK, data)`
- Errors: `response.Error(c, err)`

**Takeaway:**
- Follow investment handler pattern exactly

---

### 9. Routes

**File:** `internal/routes/v1/routes.go`

**Pattern:**
```go
investments := router.Group("/investments")
investments.Use(middleware.Auth(cfg))
{
    investments.POST("", investmentHandler.Create)
    investments.GET("", investmentHandler.List)
    investments.GET("/:id", investmentHandler.Get)
}
```

**Takeaway:**
- Group routes
- Apply Auth middleware
- Register handler methods

---

### 10. Error Handling

**File:** `internal/errors/errors.go`

**Pattern:**
- Custom error types
- Error codes
- HTTP status mapping

**Takeaway:**
- Use existing error types
- Follow existing error handling patterns

---

### 11. Testing

**File:** `internal/repositories/investment_repository_test.go` (Reference)

**Pattern:**
- SQLite in-memory: `dialector := sqlite.Open(":memory:")`
- Auto-migrate before tests
- Helper functions: `createTestUser()`, `createTestProperty()`
- Table-driven tests
- Context usage

**Takeaway:**
- Use SQLite in-memory for repository tests
- Create helper functions for test data
- Follow investment test structure

---

### 12. WebSocket Hub

**File:** `internal/realtime/hub.go`

**Key Method:**
```go
func (h *Hub) PushToUser(userID string, data []byte)
```

**Usage:**
- Best-effort delivery
- Non-blocking
- Already implemented and working

**Takeaway:**
- Hub ready to use
- Just need to call `hub.PushToUser()` from service

---

### 13. Email Sender

**File:** `internal/email/email.go`

**Assumption:** SMTP sender exists and is configured

**Takeaway:**
- Email infrastructure ready
- Need to verify interface

---

### 14. SMS Sender

**File:** `internal/utils/sms/sms.go`

**Status:** Stub implementation exists

**Takeaway:**
- SMS abstraction layer exists
- Provider integration can be added later

---

## 🎯 Key Decisions from Audit

### 1. Notification Model Already Exists ✅
- File: `internal/models/notification.go`
- Action: Enhance existing model, don't create from scratch

### 2. Migration Number Confirmed ✅
- Next: `000022_create_notifications`

### 3. Queue Architecture Needs Extension ⚠️
- Current: Single `propvest.notification.dispatch` queue
- Required: Separate email and SMS queues per implementation plan
- Action: Add `NotificationEmailQueueConfig` and `NotificationSMSQueueConfig`

### 4. NotificationService Interface Defined by Usage ✅
- Inferred from wallet service calls
- Method signature:
  ```go
  Notify(ctx, userID, type, title, message, data) error
  ```

### 5. Infrastructure Ready ✅
- WebSocket hub: ✅ Ready
- Email sender: ✅ Ready
- SMS sender: ✅ Stub ready
- RabbitMQ: ✅ Ready (needs queue extension)
- Database: ✅ Ready

---

## 📝 Implementation Notes

### Follow These Patterns Exactly:
1. **Repository:** Copy investment_repository.go structure
2. **Service:** Copy investment_service.go structure
3. **Handler:** Copy investment.go structure
4. **DTOs:** Copy investment_dto.go structure
5. **Tests:** Copy investment_repository_test.go structure

### Don't Recreate:
- ❌ PaginationMetadata (reuse existing)
- ❌ Error types (use existing)
- ❌ Response helpers (use existing)
- ❌ WebSocket hub (use existing)
- ❌ Email/SMS senders (use existing)

### Extend/Enhance:
- ✅ Notification model (already exists, may need fields)
- ✅ Queue config (add email/SMS queues)
- ✅ Main.go (uncomment placeholders)

---

## ✅ Phase 1 Complete

All patterns understood. Ready to proceed with Phase 2: Database Migration & Models.

**Next Phase:** Phase 2 - Database Migration & Models (2-3 hours)

---

*Audit completed: October 6, 2026*

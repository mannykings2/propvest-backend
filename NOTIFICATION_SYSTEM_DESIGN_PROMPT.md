# Notification System Implementation - Design Prompt for AI

**Purpose:** This document contains ALL context, information, and requirements needed to design a comprehensive implementation plan for the PropVest Notification System (Milestone 6).

---

## 🎯 OBJECTIVE

Design a complete, production-ready implementation plan for the **Notification System (Milestone 6)** for the PropVest backend. The plan should follow the same structure and quality as the successfully completed Investment Module (Milestone 5).

---

## 📋 PROJECT CONTEXT

### Project Overview
- **Name:** PropVest Backend
- **Tech Stack:** Go (Golang), Gin framework, PostgreSQL, GORM, RabbitMQ
- **Architecture:** Clean architecture with layered design (Repository → Service → Handler)
- **Current Status:** 67% complete (Milestones 0-5 done)
- **Latest Migration:** 000021_enhance_investments
- **Next Milestone:** 6 - Notification System

### Completed Milestones
✅ **Milestone 0:** Foundation (Repository pattern, Service layer, DI)  
✅ **Milestone 1:** Authentication (JWT, Refresh tokens, RBAC)  
✅ **Milestone 2:** User Management (Profile, Avatar, Preferences)  
✅ **Milestone 3:** Wallet System (Deposits, Withdrawals, Transactions, Paystack integration)  
✅ **Milestone 4:** Property Module (CRUD, Approval workflow, Media uploads)  
✅ **Milestone 5:** Investment Engine (Atomic purchases, Idempotency, Portfolio) ← **Just Completed**  

### Pending Milestones
📋 **Milestone 6:** Notification System ← **YOU ARE DESIGNING THIS**  
📋 **Milestone 7:** Administration (Dashboards, User management, Audit logs)  
📋 **Milestone 8:** Production Hardening (Testing, CI/CD, Monitoring)  

---

## 🏗️ EXISTING INFRASTRUCTURE (Already Built - ~5%)

### 1. Queue Configuration (`internal/queue/config.go`)

```go
// NotificationQueueConfig configures the notification dispatch queue.
// Notifications are lowest priority (1) - can be delayed if system is busy.
// Higher concurrency (20) since email/SMS services can handle parallel requests.
NotificationQueueConfig = QueueConfig{
    Name:       "propvest.notification.dispatch",
    Durable:    true,
    AutoDelete: false,
    Priority:   1,                          // Lowest priority
    DLXName:    "propvest.dlx",
    DLXKey:     "notification.dlq",
    PrefetchCount: 20,                      // High prefetch for high-throughput
    Concurrency:   20,                      // High concurrency
}
```

**Note:** Queue infrastructure exists, consumers need to be implemented.

### 2. Service Interface Stub (`cmd/api/main.go`)

```go
// Notification service: writes in-app rows, pushes over WebSocket, and
// enqueues email/SMS work onto RabbitMQ.
// TODO: Uncomment when notification module is implemented (Milestone 6)
// notificationService := services.NewNotificationService(notificationRepo, hub, mq)

// For now, use nil for notification service in wallet service (it's optional)
var notificationService services.NotificationService = nil

// Wallet service already expects notification service
walletService := services.NewWalletService(
    walletRepo, 
    userRepo, 
    paymentRepo, 
    outboxRepo, 
    paymentProvider, 
    notificationService,  // Currently nil, needs real implementation
    mq, 
    cfg, 
    database.DB
)
```

**Note:** Interface exists, implementation needed.

### 3. Notification Calls in Wallet Service (`internal/services/wallet_service.go`)

```go
// After deposit success (line ~227)
if user, uerr := s.userRepo.FindByID(ctx, payment.UserID); uerr == nil {
    s.notifier.Notify(ctx, payment.UserID, models.NotificationDepositSuccess,
        "Deposit successful",
        fmt.Sprintf("Your wallet was credited with ₦%s.", formatKobo(payment.AmountKobo)),
    )
}

// After withdrawal requested (line ~424)
if s.notifier != nil {
    s.notifier.Notify(ctx, userID, models.NotificationWithdrawalUpdate,
        "Withdrawal requested",
        fmt.Sprintf("Your withdrawal of ₦%s to %s is being processed.",
            formatKobo(amount), req.AccountNumber),
    )
}
```

**Note:** Wallet service has 2+ notification trigger points, currently no-ops.

### 4. WebSocket Hub (`internal/realtime/hub.go`)

```go
// PushToUser queues data for delivery to all of a user's connections. Non-
// blocking; if the hub buffer is full the message is dropped (realtime pushes
// are best-effort — the durable copy lives in the notifications table).
func (h *Hub) PushToUser(userID string, data []byte) {
    select {
    case h.pushChan <- userPush{userID: userID, data: data}:
    default:
        // Best-effort: drop if buffer full
    }
}
```

**Note:** WebSocket hub fully implemented and working.

### 5. Email Sender (`internal/email/email.go`)

Email infrastructure exists with SMTP configuration. Can send transactional emails.

### 6. SMS Sender (`internal/utils/sms/sms.go`)

SMS stub exists. Provider integration (Termii, Twilio, etc.) can be added.

### 7. Outbox Events (`internal/models/outbox_event.go`)

```go
// EventTypeRealtimePush triggers realtime notification to connected clients
// Queue: propvest.realtime.push
// Payload: RealtimeMessage
EventTypeRealtimePush = "realtime.push"
```

---

## 🎯 NOTIFICATION SYSTEM REQUIREMENTS

### Core Features Required

#### 1. In-App Notifications
- Persistent notification history in database
- User can view, read, delete notifications
- Unread badge count
- Notification types: deposit, withdrawal, investment, property, system
- Each notification has: title, message, metadata (JSON), timestamps

#### 2. Multi-Channel Delivery
- **In-App:** Always created (durable record)
- **Email:** Transactional emails via RabbitMQ queue
- **SMS:** Critical alerts via RabbitMQ queue
- **Push:** Realtime WebSocket push to connected clients

#### 3. User Preferences
- Users can enable/disable each channel globally
- Users can configure per-notification-type preferences
- Example: Enable email for deposits, disable SMS for investments
- Preferences persist in database

#### 4. Async Dispatch
- Email/SMS dispatched via RabbitMQ (non-blocking)
- Queue consumers process messages in background (worker process)
- Retry logic for failed deliveries
- Dead Letter Queue (DLQ) for permanent failures

#### 5. Templates
- Email templates (HTML, responsive, branded)
- SMS templates (plain text, <160 chars)
- Template variables for personalization
- Templates for: deposit, withdrawal, investment, property updates

#### 6. API Endpoints
- List notifications (paginated)
- Get single notification
- Get unread count
- Mark notification as read
- Mark all as read
- Delete notification
- Get notification preferences
- Update notification preferences

---

## 🗄️ DATABASE REQUIREMENTS

### Tables Needed

#### 1. `notifications` table
- `id` UUID (PK)
- `user_id` UUID (FK → users.id, CASCADE)
- `type` VARCHAR(50) - notification type (deposit, withdrawal, investment, property, system)
- `title` VARCHAR(255) - notification title
- `message` TEXT - notification message
- `data` JSONB - additional metadata (investment_id, property_id, amount, etc.)
- `read_at` TIMESTAMP - when user read it (null = unread)
- `created_at` TIMESTAMP - when notification was created

**Indexes:**
- `idx_notifications_user_created` - (user_id, created_at DESC) for listing
- `idx_notifications_user_read` - (user_id, read_at) for read/unread queries
- `idx_notifications_type` - (type) for filtering
- `idx_notifications_unread` - (user_id) WHERE read_at IS NULL (partial index)

#### 2. `notification_preferences` table
- `user_id` UUID (PK, FK → users.id, CASCADE)
- `email_enabled` BOOLEAN (default TRUE) - enable/disable email globally
- `sms_enabled` BOOLEAN (default TRUE) - enable/disable SMS globally
- `push_enabled` BOOLEAN (default TRUE) - enable/disable push globally
- `email_deposit` BOOLEAN (default TRUE) - email for deposits
- `email_withdrawal` BOOLEAN (default TRUE) - email for withdrawals
- `email_investment` BOOLEAN (default TRUE) - email for investments
- `email_property` BOOLEAN (default TRUE) - email for property updates
- `sms_deposit` BOOLEAN (default TRUE) - SMS for deposits
- `sms_withdrawal` BOOLEAN (default TRUE) - SMS for withdrawals
- `sms_investment` BOOLEAN (default FALSE) - SMS for investments (disabled by default)
- `updated_at` TIMESTAMP - last preference update

### Migration
- **Number:** 000022_create_notifications
- **Up:** Create both tables with indexes
- **Down:** Drop both tables (CASCADE)

---

## 🏛️ ARCHITECTURE REQUIREMENTS

### Layered Design (Follow Investment Module Pattern)

#### 1. Models (`internal/models/`)
- `notification.go` - Notification struct with GORM/JSON tags
- `notification_preference.go` - NotificationPreference struct
- Add notification type constants:
  ```go
  const (
      NotificationTypeDeposit     = "deposit"
      NotificationTypeWithdrawal  = "withdrawal"
      NotificationTypeInvestment  = "investment"
      NotificationTypeProperty    = "property"
      NotificationTypeSystem      = "system"
  )
  ```

#### 2. Repository Layer (`internal/repositories/`)
- `notification_repository.go`
  - Interface: NotificationRepository
  - Implementation: notificationRepository
  - Methods: Create, FindByID, FindByUser, FindUnreadByUser, CountUnread, MarkAsRead, MarkAllAsRead, Delete, DeleteOldNotifications
- `notification_preference_repository.go`
  - Interface: NotificationPreferenceRepository
  - Implementation: notificationPreferenceRepository
  - Methods: FindByUser, Upsert, CreateDefault

#### 3. Service Layer (`internal/services/`)
- `notification_service.go`
  - Interface: NotificationService
  - Implementation: notificationService
  - Dependencies: notificationRepo, preferenceRepo, hub (WebSocket), mq (RabbitMQ), logger
  - Core Methods:
    - `Notify(ctx, userID, type, title, message, data)` - Main notification creation + dispatch
    - `SendEmail(ctx, userID, subject, body)` - Queue email
    - `SendSMS(ctx, userID, message)` - Queue SMS
    - `SendPush(ctx, userID, data)` - Send WebSocket push
    - `GetNotifications(ctx, userID, page, limit)` - List with pagination
    - `GetNotification(ctx, userID, notificationID)` - Get single (ownership check)
    - `GetUnreadCount(ctx, userID)` - Count unread
    - `MarkAsRead(ctx, userID, notificationID)` - Mark as read (ownership check)
    - `MarkAllAsRead(ctx, userID)` - Mark all as read
    - `DeleteNotification(ctx, userID, notificationID)` - Delete (ownership check)
    - `GetPreferences(ctx, userID)` - Get user preferences
    - `UpdatePreferences(ctx, userID, req)` - Update preferences
    - `CleanupOldNotifications(ctx, daysOld)` - Background cleanup

#### 4. Handler Layer (`internal/handlers/`)
- `notification.go`
  - Struct: NotificationHandler
  - Dependency: notificationService
  - Methods: List, Get, GetUnreadCount, MarkAsRead, MarkAllAsRead, Delete, GetPreferences, UpdatePreferences
  - All handlers extract userID from JWT context
  - All handlers use existing response helpers (response.Success, response.Error)

#### 5. DTO Layer (`internal/dto/`)
- `notification_dto.go`
  - NotificationResponse
  - NotificationListResponse (includes unread count + pagination)
  - NotificationPreferenceResponse
  - UpdateNotificationPreferenceRequest
- Add mappers to `mappers.go`:
  - NotificationToResponse
  - NotificationsToResponse
  - PreferenceToResponse

#### 6. Queue Consumers (`cmd/worker/main.go`)
- Email consumer: Listen to `propvest.notification.dispatch`, filter `type == "email"`
- SMS consumer: Listen to `propvest.notification.dispatch`, filter `type == "sms"`
- Push consumer: Listen to `propvest.realtime.push` queue
- Message format:
  ```go
  type NotificationMessage struct {
      UserID      string                 `json:"user_id"`
      Type        string                 `json:"type"`        // "email" | "sms" | "push"
      Subject     string                 `json:"subject"`     // email only
      Body        string                 `json:"body"`
      TemplateID  string                 `json:"template_id"` // optional
      Data        map[string]interface{} `json:"data"`        // template vars
  }
  ```

#### 7. Templates (`templates/`)
- `email/` directory:
  - `base.html` - Base layout with PropVest branding
  - `deposit.html` - Deposit success email
  - `withdrawal.html` - Withdrawal status email
  - `investment.html` - Investment confirmed email
  - `property.html` - Property update email
- `sms/` directory:
  - `deposit.txt` - Deposit SMS (<160 chars)
  - `withdrawal.txt` - Withdrawal SMS
  - `investment.txt` - Investment SMS
- Template renderer utility in `internal/utils/templates/`

---

## 📡 API ENDPOINTS SPECIFICATION

All endpoints require JWT authentication (`middleware.Auth`).

### Notification Endpoints

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/api/v1/notifications` | List | List user's notifications (paginated) |
| GET | `/api/v1/notifications/unread-count` | GetUnreadCount | Get unread count |
| GET | `/api/v1/notifications/:id` | Get | Get single notification (ownership check) |
| PATCH | `/api/v1/notifications/:id/read` | MarkAsRead | Mark as read (ownership check) |
| PATCH | `/api/v1/notifications/read-all` | MarkAllAsRead | Mark all as read |
| DELETE | `/api/v1/notifications/:id` | Delete | Delete notification (ownership check) |

### Preference Endpoints

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/api/v1/users/me/notification-preferences` | GetPreferences | Get preferences |
| PATCH | `/api/v1/users/me/notification-preferences` | UpdatePreferences | Update preferences |

**Route Registration:** Update `internal/routes/v1/routes.go` to register these endpoints.

---

## 🔧 INTEGRATION REQUIREMENTS

### 1. Update `cmd/api/main.go`
- Initialize notification repository: `notificationRepo := repositories.NewNotificationRepository(database.DB)`
- Initialize preference repository: `notificationPrefRepo := repositories.NewNotificationPreferenceRepository(database.DB)`
- Initialize notification service: `notificationService := services.NewNotificationService(notificationRepo, notificationPrefRepo, hub, mq, logger)`
- Pass real `notificationService` to wallet service (replace `nil`)
- Initialize notification handler: `notificationHandler := handlers.NewNotificationHandler(notificationService)`
- Pass to route registration

### 2. Enable Notification Calls in Services
- **Wallet Service:** Already has 2+ notification calls (currently no-ops with `nil` check)
- **Investment Service:** Add notification calls:
  - After investment created: "Investment confirmed"
  - After property fully funded: Notify all investors "Property funded"
- **Property Service:** Add notification call (if approval workflow exists):
  - After property approved: Notify developer "Property approved"

### 3. Worker Process (`cmd/worker/main.go`)
- Register notification queue consumers:
  - `mq.Consume("propvest.notification.dispatch", handleNotificationDispatch)`
  - `mq.Consume("propvest.realtime.push", handleRealtimePush)`
- Implement consumer handlers

---

## 🧪 TESTING REQUIREMENTS

Follow Investment Module testing pattern (23 repository tests, CI/CD).

### Repository Tests (`internal/repositories/notification_repository_test.go`)
- Test all CRUD operations
- Test pagination
- Test ownership checks
- Test unread queries
- Test DeleteOldNotifications
- Use SQLite in-memory (like property/investment tests)

### Service Tests (`internal/services/notification_service_test.go`)
- Test Notify creates in-app notification
- Test multi-channel dispatch (email, SMS, push)
- Test user preferences are respected
- Test ownership checks in Get/MarkAsRead/Delete
- Mock repository and queue client

### Handler Tests
- Test all endpoints
- Test JWT authentication
- Test ownership checks (IDOR prevention)
- Test pagination
- Test error handling

### Integration Tests
- End-to-end test: User deposits → Notification created + email queued
- End-to-end test: User invests → Notification created
- Test queue consumers
- Test WebSocket push

### CI/CD
- Create `.github/workflows/test-notification-module.yml`
- Run on push/PR
- Report coverage

---

## 📚 REFERENCE IMPLEMENTATIONS

### Investment Module (Milestone 5) - Completed Successfully
- **Repository:** 13 methods, transaction-aware, context-aware
- **Service:** 7 methods, atomic workflows, comprehensive error handling
- **Handler:** 7 endpoints, thin layer, proper auth
- **DTOs:** 6 request/response types, 3 mappers
- **Testing:** 23 repository tests, CI/CD pipeline
- **Documentation:** API docs, testing guide, completion summary
- **Pattern:** Look at `internal/repositories/investment_repository.go`, `internal/services/investment_service.go`, `internal/handlers/investment.go` for code style

### Wallet Module (Milestone 3) - Reference for Queue Usage
- Uses RabbitMQ for async withdrawal processing
- Queue message publishing pattern
- Worker consumer pattern in `cmd/worker/main.go`
- Pattern: Look at `internal/services/wallet_service.go` for queue publishing

---

## 📖 CODING STANDARDS (from docs/06-Engineering/6.1-CODING_STANDARDS.md)

### Key Principles
- Clean Architecture: Repository → Service → Handler
- Dependency Injection: All dependencies injected via constructor
- Context-Aware: Every function accepts `context.Context`
- Error Handling: Return errors, don't panic; map to proper error codes
- Logging: Structured logging with context
- Validation: Validate at DTO/service layer
- Ownership Checks: Prevent IDOR vulnerabilities
- Pagination: Max 100 per page, defaults
- Parameterized Queries: No SQL injection

### Code Style
- Follow existing patterns in investment/wallet/property modules
- GORM for database operations
- Gin for HTTP framework
- Standard Go naming conventions
- Comments for exported functions

---

## 🎯 SUCCESS CRITERIA

The Notification Module is complete when:

✅ **Database:**
- Migration 000022 created (up + down)
- Tables created with proper indexes
- Models defined with GORM/JSON tags

✅ **Repository:**
- All CRUD methods implemented
- Ownership checks in place
- Pagination working
- Context-aware

✅ **Service:**
- Notify method creates in-app + dispatches channels
- User preferences respected
- Multi-channel dispatch (email, SMS, push)
- All retrieval/update methods working
- Ownership checks prevent IDOR

✅ **Handler:**
- 8 endpoints implemented
- JWT authentication required
- Ownership checks
- Proper error handling

✅ **Queue Consumers:**
- Email consumer working
- SMS consumer working
- Push consumer working
- Retry logic implemented

✅ **Templates:**
- Email templates (HTML, responsive)
- SMS templates (<160 chars)
- Template renderer working

✅ **Integration:**
- Wired in main.go
- Routes registered
- Wallet service notifications working
- Investment service notifications added

✅ **Testing:**
- Repository tests (≥20 tests)
- Service tests
- Handler tests
- CI/CD pipeline configured
- Coverage ≥80%

✅ **Documentation:**
- API documentation
- Testing guide
- Implementation summary

✅ **Build:**
- `go build ./cmd/api` succeeds
- `go build ./cmd/worker` succeeds
- No compilation errors
- All tests pass

---

## 📋 DELIVERABLES EXPECTED

1. **Implementation Plan Document** (What YOU will design)
   - Phase-by-phase breakdown (like Investment Module had 11 phases)
   - Each phase with tasks, acceptance criteria, deliverables
   - Time estimates per phase
   - Clear dependencies and order

2. **Architecture Design**
   - Database schema (exact DDL)
   - Repository interfaces and methods
   - Service interfaces and methods
   - Handler methods
   - DTO structures
   - Queue message structures
   - Template structure

3. **Integration Points**
   - Exact changes needed in main.go
   - Exact changes needed in routes.go
   - Exact changes needed in wallet_service.go
   - Exact changes needed in investment_service.go (if applicable)
   - Queue consumer registration

4. **Testing Strategy**
   - Test structure
   - Test cases to implement
   - Coverage targets

---

## 🔍 CODEBASE FILES TO REFERENCE

### Current Infrastructure (READ THESE)
- `internal/queue/config.go` - Queue configuration (NotificationQueueConfig exists)
- `internal/realtime/hub.go` - WebSocket hub (PushToUser method)
- `cmd/api/main.go` - Dependency injection (notificationService stub)
- `internal/services/wallet_service.go` - Notification calls (s.notifier.Notify)
- `internal/email/email.go` - Email sender
- `internal/utils/sms/sms.go` - SMS sender stub
- `cmd/worker/main.go` - Worker process structure

### Reference Implementations (FOLLOW THESE PATTERNS)
- `internal/repositories/investment_repository.go` - Repository pattern
- `internal/services/investment_service.go` - Service pattern
- `internal/handlers/investment.go` - Handler pattern
- `internal/dto/investment_dto.go` - DTO pattern
- `internal/repositories/investment_repository_test.go` - Testing pattern
- `INVESTMENT_MODULE_TASKS.md` - Task breakdown pattern
- `INVESTMENT_MODULE_API_DOCUMENTATION.md` - Documentation pattern

### Architecture Documentation (UNDERSTAND THESE)
- `docs/05-Modules/5.5-NOTIFICATION_SYSTEM.md` - Official notification design
- `docs/06-Engineering/6.1-CODING_STANDARDS.md` - Coding standards
- `docs/06-Engineering/6.2-ERROR_HANDLING_AND_LOGGING.md` - Error handling
- `docs/06-Engineering/6.3-TESTING_STRATEGY.md` - Testing strategy
- `docs/02-Database/2.1-DATABASE_ENGINEERING.md` - Database standards
- `docs/07-Operations/7.3-BACKGROUND_JOBS.md` - Queue/background jobs

---

## ⚠️ CRITICAL NOTES

### What Already Exists (Don't Re-Design)
✅ Queue infrastructure (NotificationQueueConfig)  
✅ WebSocket hub (PushToUser)  
✅ Email sender  
✅ SMS sender stub  
✅ Service interface reference  
✅ Notification calls in wallet service  

### What Needs Implementation (Your Focus)
❌ Database tables (notifications, notification_preferences)  
❌ Repository layer (notification_repository, notification_preference_repository)  
❌ Service implementation (notification_service)  
❌ Handler layer (notification.go)  
❌ Queue consumers (email, SMS, push)  
❌ Templates (email HTML, SMS text)  
❌ Integration/wiring  
❌ Tests  

### Design Decisions to Make
1. **Notification Retention:** How long to keep old notifications? (e.g., 90 days, then cleanup)
2. **Batch Operations:** Support bulk delete? Bulk mark as read?
3. **Notification Priority:** High priority flag for urgent notifications?
4. **Template Engine:** Go html/template or external library?
5. **Email Provider:** SMTP (current) or future SendGrid/Mailgun/SES?
6. **SMS Provider:** Which provider to integrate? (Termii, Twilio, Vonage)

---

## 🎓 EXAMPLE: Investment Module Structure (FOLLOW THIS PATTERN)

The Investment Module (Milestone 5) was implemented in **11 phases:**

1. Repository Audit (1-2h)
2. Database Migration & Model Enhancement (2-3h)
3. Repository Layer Enhancement (2-3h)
4. DTO Enhancement (1h)
5. Service Layer - Core CreateInvestment (4-6h)
6. Service Layer - Portfolio Features (2-3h)
7. Service Layer - Admin Features (2-3h)
8. Handler Layer (2-3h)
9. Routes & Dependency Injection (1-2h)
10. Testing (5-8h)
11. Documentation (2-3h)

**Total:** 30-43 hours across 11 phases

**Your notification plan should have similar structure:**
- Clear phases with time estimates
- Each phase has tasks, acceptance criteria, deliverables
- Progressive implementation (database → repository → service → handler → integration → testing)

---

## 📤 OUTPUT FORMAT REQUESTED

Please provide the implementation plan in **markdown format** with:

1. **Overview Section**
   - Module purpose
   - Key features
   - Success criteria

2. **Phase Breakdown** (similar to Investment Module's 11 phases)
   - Phase number and name
   - Time estimate
   - Goal statement
   - Detailed tasks (checkboxes)
   - Acceptance criteria
   - Deliverables

3. **Architecture Details**
   - Database schema (DDL)
   - Model structures
   - Repository interfaces
   - Service interfaces
   - DTO structures
   - API endpoints table

4. **Integration Points**
   - Code changes needed (specific files and locations)

5. **Testing Strategy**
   - Test structure
   - Test cases

---

## 🚀 FINAL REQUEST

**Design a complete, production-ready implementation plan for the PropVest Notification System (Milestone 6) following the structure and quality of the Investment Module.**

The plan should be:
- ✅ Comprehensive (covers all aspects)
- ✅ Actionable (specific tasks, not vague)
- ✅ Ordered (correct implementation sequence)
- ✅ Testable (clear acceptance criteria)
- ✅ Documented (clear deliverables)

The plan will be reviewed, modified if necessary, then broken down into tracked implementation phases.

---

**End of Design Prompt**

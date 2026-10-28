# PropVest Milestone 6 — Notification System
## AI Coding Agent Implementation Specification

> **Purpose:** This document is the implementation handoff for an AI coding agent. It converts the Notification System design into an ordered, production-oriented execution plan.
>
> **Repository:** PropVest Backend
>
> **Milestone:** 6 — Notification System
>
> **Stack:** Go, Gin, PostgreSQL, GORM, RabbitMQ
>
> **Architecture:** Clean Architecture — Repository → Service → Handler
>
> **Important:** Work against the existing codebase. Reuse existing patterns and infrastructure. Do not redesign unrelated modules.

---

# 1. Mission

Implement Milestone 6 as a complete, production-ready Notification System.

The implementation must provide:

- Persistent in-app notifications.
- Notification history.
- Read/unread state.
- Unread badge count.
- Notification deletion.
- Email notifications.
- SMS notifications.
- Realtime WebSocket notifications.
- Global notification preferences.
- Per-notification-type email/SMS preferences.
- RabbitMQ asynchronous delivery.
- Retry and DLQ handling.
- Notification templates.
- Notification APIs.
- Wallet integration.
- Investment integration.
- Property integration.
- Automated tests.
- Documentation.
- Successful API/worker builds and test execution.

The existing design specifies the core features, database tables, architecture, endpoints, integrations, tests, and success criteria. Preserve those requirements while applying the production-hardening decisions in this document.

---

# 2. Existing Infrastructure — Reuse It

Do NOT recreate these components:

- `internal/queue/config.go`
  - Existing notification queue configuration.
- `internal/realtime/hub.go`
  - Existing WebSocket hub and `PushToUser`.
- `internal/email/email.go`
  - Existing SMTP email sender.
- `internal/utils/sms/sms.go`
  - Existing SMS abstraction/stub.
- `internal/models/outbox_event.go`
  - Existing outbox/realtime event infrastructure.
- Notification service interface reference in `cmd/api/main.go`.
- Existing notification call sites in `internal/services/wallet_service.go`.
- Existing Clean Architecture patterns.
- Existing repository/service/handler/DTO/test conventions.

Reference implementations:

- `internal/repositories/investment_repository.go`
- `internal/services/investment_service.go`
- `internal/handlers/investment.go`
- `internal/dto/investment_dto.go`
- `internal/repositories/investment_repository_test.go`
- `cmd/worker/main.go`

Also inspect:

- `docs/05-Modules/5.5-NOTIFICATION_SYSTEM.md`
- `docs/06-Engineering/6.1-CODING_STANDARDS.md`
- `docs/06-Engineering/6.2-ERROR_HANDLING_AND_LOGGING.md`
- `docs/06-Engineering/6.3-TESTING_STRATEGY.md`
- `docs/02-Database/2.1-DATABASE_ENGINEERING.md`
- `docs/07-Operations/7.3-BACKGROUND_JOBS.md`

---

# 3. Non-Negotiable Engineering Rules

Follow the existing project's conventions.

## Architecture

Use:

```text
Repository → Service → Handler
```

Use dependency injection.

Do not put business logic inside handlers.

## Context

Every repository/service operation must accept `context.Context`.

## Errors

- Return errors.
- Do not panic for expected application failures.
- Use existing application error types/helpers.
- Follow existing HTTP error mapping.

## Logging

Use the project's existing structured logger.

Include useful context such as:

- user ID
- notification ID
- notification type
- delivery channel
- queue/message ID where available

Do not log:

- passwords
- authentication tokens
- full sensitive financial data
- unnecessary PII

## Validation

Validate request data at the DTO/service boundary according to existing project conventions.

## Pagination

Follow the existing project's pagination conventions.

Maximum page size must be 100.

## Security

Every notification operation must be scoped to the authenticated user.

Never trust a notification ID alone.

Queries for user-owned notifications should effectively enforce:

```sql
WHERE id = ? AND user_id = ?
```

This is required to prevent IDOR vulnerabilities.

## Database

Use GORM where the project already uses GORM.

Use parameterized queries.

Do not introduce SQL injection risks.

---

# 4. Important Production-Hardening Decisions

These decisions override ambiguous parts of the initial design.

## 4.1 Separate Email and SMS RabbitMQ Queues

Do NOT create separate email and SMS consumers that compete on the same queue and then filter messages by type.

Avoid:

```text
propvest.notification.dispatch
    ├── Email Consumer
    └── SMS Consumer
```

Instead use a notification exchange and separate queues:

```text
propvest.notification
    │
    ├── notification.email
    │       ↓
    │   propvest.notification.email
    │       ↓
    │   Email Consumer
    │
    └── notification.sms
            ↓
        propvest.notification.sms
            ↓
        SMS Consumer
```

Keep realtime push on its existing/relevant queue:

```text
propvest.realtime.push
```

Reuse the existing queue infrastructure where possible. Extend it only where necessary.

---

# 5. Notification Architecture

Target architecture:

```text
Wallet Service
       │
Investment Service
       │
Property Service
       │
       ▼
Notification Service
       │
       ├───────────────┐
       │               │
       ▼               ▼
 PostgreSQL          RabbitMQ
 notifications          │
       │          ┌─────┴─────┐
       │          │           │
       │          ▼           ▼
       │        Email         SMS
       │        Queue        Queue
       │          │           │
       │          ▼           ▼
       │        Worker       Worker
       │          │           │
       │          ▼           ▼
       │        SMTP        SMS Provider
       │
       ▼
Frontend Notification API

Realtime path:

Notification Service
       ↓
Realtime Queue
       ↓
Realtime Consumer
       ↓
WebSocket Hub
       ↓
Connected Client
```

The database is the source of truth for in-app notifications.

WebSocket delivery is best-effort.

If WebSocket delivery fails, the durable notification must still exist in PostgreSQL.

---

# 6. Notification Lifecycle

Keep the in-app notification lifecycle separate from email/SMS delivery lifecycle.

## In-app

```text
CREATED
   ↓
UNREAD
   ↓
READ
   ↓
DELETED
```

`read_at = NULL` means unread.

## Delivery

```text
QUEUED
   ↓
PROCESSING
   ├── SUCCESS → ACK
   │
   └── FAILURE
         ↓
       RETRY
         ↓
       RETRY
         ↓
       RETRY
         ↓
       DLQ
```

Do not mix read state with email/SMS delivery state.

---

# 7. Idempotency

Queue messages must include the notification ID.

Use:

```go
type NotificationMessage struct {
    NotificationID string                 `json:"notification_id"`
    UserID         string                 `json:"user_id"`
    Type           string                 `json:"type"`
    Channel        string                 `json:"channel"`
    Subject        string                 `json:"subject,omitempty"`
    Body           string                 `json:"body,omitempty"`
    TemplateID     string                 `json:"template_id,omitempty"`
    Data           map[string]interface{} `json:"data,omitempty"`
}
```

The notification ID provides a stable identity for tracing and future delivery-idempotency improvements.

Do not allow retry behavior to create uncontrolled duplicate processing.

If the existing codebase already has an idempotency/outbox mechanism suitable for this, reuse it.

---

# 8. Transactional Rule

Never perform external email/SMS network calls inside a database transaction.

Bad:

```text
BEGIN
update wallet
send email
COMMIT
```

Good:

```text
BEGIN
update wallet
create notification/outbox event
COMMIT

worker
   ↓
send email
```

When notification creation participates in an existing database transaction, keep the database work transactional.

External delivery remains asynchronous.

---

# 9. Notification Types

Add/use:

```go
const (
    NotificationTypeDeposit    = "deposit"
    NotificationTypeWithdrawal = "withdrawal"
    NotificationTypeInvestment = "investment"
    NotificationTypeProperty   = "property"
    NotificationTypeSystem     = "system"
)
```

Do not introduce additional types unless required by the existing product design.

---

# 10. Phase 1 — Repository and Codebase Audit

## Estimate

1–2 hours.

## Goal

Understand the exact current implementation before making changes.

## Tasks

- [ ] Inspect repository interfaces.
- [ ] Inspect `investment_repository.go`.
- [ ] Inspect `investment_service.go`.
- [ ] Inspect `investment.go`.
- [ ] Inspect DTO conventions.
- [ ] Inspect `mappers.go`.
- [ ] Inspect `cmd/api/main.go`.
- [ ] Inspect `cmd/worker/main.go`.
- [ ] Inspect RabbitMQ publisher implementation.
- [ ] Inspect queue configuration.
- [ ] Inspect WebSocket hub.
- [ ] Inspect email sender.
- [ ] Inspect SMS abstraction.
- [ ] Inspect migration numbering.
- [ ] Inspect existing tests and test helpers.
- [ ] Inspect error definitions.
- [ ] Inspect logging conventions.
- [ ] Inspect existing outbox implementation.

## Acceptance Criteria

- Existing implementation patterns are understood.
- No duplicate infrastructure is created.
- Notification code fits the existing architecture.
- Migration numbering is confirmed.
- Existing queue behavior is understood.
- Existing worker startup/consumer conventions are understood.

## Deliverable

A short internal implementation audit or notes before coding.

---

# 11. Phase 2 — Database Migration and Models

## Estimate

2–3 hours.

## Migration

Create:

```text
000022_create_notifications.up.sql
000022_create_notifications.down.sql
```

## Notifications table

Use:

```sql
CREATE TABLE notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    data JSONB,
    read_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

Indexes:

```sql
CREATE INDEX idx_notifications_user_created
ON notifications(user_id, created_at DESC);

CREATE INDEX idx_notifications_user_read
ON notifications(user_id, read_at);

CREATE INDEX idx_notifications_type
ON notifications(type);

CREATE INDEX idx_notifications_unread
ON notifications(user_id)
WHERE read_at IS NULL;
```

## Preferences table

```sql
CREATE TABLE notification_preferences (
    user_id UUID PRIMARY KEY
        REFERENCES users(id) ON DELETE CASCADE,

    email_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sms_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    push_enabled BOOLEAN NOT NULL DEFAULT TRUE,

    email_deposit BOOLEAN NOT NULL DEFAULT TRUE,
    email_withdrawal BOOLEAN NOT NULL DEFAULT TRUE,
    email_investment BOOLEAN NOT NULL DEFAULT TRUE,
    email_property BOOLEAN NOT NULL DEFAULT TRUE,

    sms_deposit BOOLEAN NOT NULL DEFAULT TRUE,
    sms_withdrawal BOOLEAN NOT NULL DEFAULT TRUE,
    sms_investment BOOLEAN NOT NULL DEFAULT FALSE,

    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

## Important

Do not add per-type push preferences unless the existing product requirements explicitly require them.

Push remains globally controlled for this milestone.

## Down migration

Drop:

```text
notification_preferences
notifications
```

in the correct dependency order.

## Model files

Create:

```text
internal/models/notification.go
internal/models/notification_preference.go
```

Follow existing GORM conventions.

## Acceptance Criteria

- Migration applies successfully.
- Migration rollback works.
- Tables exist.
- Foreign keys exist.
- Required indexes exist.
- Models map correctly to database columns.

---

# 12. Phase 3 — Repository Layer

## Estimate

3–4 hours.

Create:

```text
internal/repositories/notification_repository.go
internal/repositories/notification_preference_repository.go
```

## Notification repository interface

Implement:

```go
type NotificationRepository interface {
    Create(ctx context.Context, notification *models.Notification) error

    FindByID(
        ctx context.Context,
        id uuid.UUID,
    ) (*models.Notification, error)

    FindByUser(
        ctx context.Context,
        userID uuid.UUID,
        page int,
        limit int,
    ) ([]*models.Notification, int64, error)

    FindUnreadByUser(
        ctx context.Context,
        userID uuid.UUID,
    ) ([]*models.Notification, error)

    CountUnread(
        ctx context.Context,
        userID uuid.UUID,
    ) (int64, error)

    MarkAsRead(
        ctx context.Context,
        userID uuid.UUID,
        notificationID uuid.UUID,
    ) error

    MarkAllAsRead(
        ctx context.Context,
        userID uuid.UUID,
    ) error

    Delete(
        ctx context.Context,
        userID uuid.UUID,
        notificationID uuid.UUID,
    ) error

    DeleteOldNotifications(
        ctx context.Context,
        before time.Time,
    ) error
}
```

## Preference repository

Implement:

```go
type NotificationPreferenceRepository interface {
    FindByUser(
        ctx context.Context,
        userID uuid.UUID,
    ) (*models.NotificationPreference, error)

    Upsert(
        ctx context.Context,
        preferences *models.NotificationPreference,
    ) error

    CreateDefault(
        ctx context.Context,
        userID uuid.UUID,
    ) (*models.NotificationPreference, error)
}
```

## Ownership

For user mutations, enforce ownership in the repository query.

Example:

```sql
WHERE id = ? AND user_id = ?
```

Do not retrieve an object by ID and then forget to verify ownership.

## Acceptance Criteria

- All methods compile.
- Queries are context-aware.
- Pagination works.
- Ownership is enforced.
- Unread count works.
- Cleanup works.
- Repository follows investment repository style.

---

# 13. Phase 4 — DTOs and Mappers

## Estimate

1–2 hours.

Create:

```text
internal/dto/notification_dto.go
```

Add:

```text
NotificationResponse
NotificationListResponse
NotificationPreferenceResponse
UpdateNotificationPreferenceRequest
```

Add mapper functions to:

```text
internal/dto/mappers.go
```

Required:

```text
NotificationToResponse
NotificationsToResponse
PreferenceToResponse
```

## Notification response

Use API-safe fields:

```go
type NotificationResponse struct {
    ID        string                 `json:"id"`
    Type      string                 `json:"type"`
    Title     string                 `json:"title"`
    Message   string                 `json:"message"`
    Data      map[string]interface{} `json:"data,omitempty"`
    ReadAt    *time.Time             `json:"read_at,omitempty"`
    CreatedAt time.Time              `json:"created_at"`
}
```

## List response

Include:

```text
notifications
unread_count
page
limit
total
total_pages
```

## Acceptance Criteria

- API does not expose raw database models unnecessarily.
- Mapping follows existing DTO conventions.
- Validation follows existing project style.

---

# 14. Phase 5 — Notification Service

## Estimate

4–6 hours.

Create:

```text
internal/services/notification_service.go
```

## Dependencies

Inject:

- notification repository
- preference repository
- WebSocket hub
- RabbitMQ client
- logger
- any existing user repository/service needed to retrieve email/phone details

Do not introduce global state.

## Service methods

Implement:

```go
Notify(
    ctx context.Context,
    userID uuid.UUID,
    notificationType string,
    title string,
    message string,
    data map[string]interface{},
) error
```

Also:

```text
SendEmail
SendSMS
SendPush
GetNotifications
GetNotification
GetUnreadCount
MarkAsRead
MarkAllAsRead
DeleteNotification
GetPreferences
UpdatePreferences
CleanupOldNotifications
```

## Notify workflow

Implement:

```text
1. Validate notification type.
2. Create notification ID.
3. Create durable in-app notification.
4. Load user preferences.
5. Determine email eligibility.
6. Determine SMS eligibility.
7. Determine push eligibility.
8. Queue email if enabled.
9. Queue SMS if enabled.
10. Send/queue realtime push if enabled.
11. Log important failures.
12. Return according to the project's error semantics.
```

## Important failure behavior

In-app notification persistence is the durable core.

A temporary email/SMS failure must not delete or invalidate the in-app notification.

Follow the project's existing outbox/error-handling patterns where available.

## Acceptance Criteria

- `Notify()` creates a durable notification.
- Preferences are respected.
- Email is queued only when permitted.
- SMS is queued only when permitted.
- Push is only attempted when permitted.
- No external provider call occurs inside a DB transaction.
- Ownership is enforced for retrieval/mutation methods.

---

# 15. Phase 6 — Notification Preferences

## Estimate

2–3 hours.

Implement preference logic centrally.

## Email

Email requires:

```text
email_enabled == true
AND
corresponding email_<type> == true
```

Example:

```text
email_enabled = false
email_deposit = true
```

Result:

```text
Do NOT send deposit email.
```

## SMS

SMS requires:

```text
sms_enabled == true
AND
corresponding sms_<type> == true
```

Example:

```text
sms_enabled = true
sms_investment = false
```

Result:

```text
Do NOT send investment SMS.
```

## Push

Use:

```text
push_enabled
```

for the current milestone.

## Defaults

Use the specified defaults:

```text
email = enabled
SMS = enabled
push = enabled

email deposit = enabled
email withdrawal = enabled
email investment = enabled
email property = enabled

SMS deposit = enabled
SMS withdrawal = enabled
SMS investment = disabled
```

## Acceptance Criteria

- Preferences persist.
- Defaults are created correctly.
- Updates are validated.
- Global settings override per-type settings appropriately.
- Notification dispatch respects all applicable preferences.

---

# 16. Phase 7 — RabbitMQ and Worker Delivery

## Estimate

4–6 hours.

## Queue architecture

Use:

```text
Exchange:
propvest.notification
```

Routing keys:

```text
notification.email
notification.sms
```

Queues:

```text
propvest.notification.email
propvest.notification.sms
```

Realtime:

```text
propvest.realtime.push
```

Use durable queues.

Use existing DLX infrastructure:

```text
propvest.dlx
```

Do not break existing queues.

## Email consumer

Flow:

```text
RabbitMQ
→ Email Queue
→ Email Consumer
→ Template Renderer
→ Existing SMTP Sender
→ ACK
```

## SMS consumer

Flow:

```text
RabbitMQ
→ SMS Queue
→ SMS Consumer
→ Template Renderer
→ Existing SMS Provider abstraction
→ ACK
```

## Push consumer

Flow:

```text
Realtime Queue
→ Push Consumer
→ WebSocket Hub
→ ACK
```

## Retry

Implement finite retries.

Suggested behavior:

```text
Attempt 1
Attempt 2
Attempt 3
DLQ
```

Use the existing project's RabbitMQ retry/DLX conventions if they already provide a standard mechanism.

Do not invent a competing retry system unnecessarily.

## ACK behavior

- ACK only after successful processing.
- NACK/requeue according to the established retry mechanism.
- Do not ACK failed messages before processing succeeds.

## Acceptance Criteria

- Email messages reach email worker.
- SMS messages reach SMS worker.
- Email messages cannot be consumed by the SMS worker accidentally.
- Failed messages retry.
- Permanently failed messages reach DLQ.
- Successful messages are acknowledged.
- Worker starts without errors.

---

# 17. Phase 8 — Templates and Providers

## Estimate

3–4 hours.

Create:

```text
templates/
├── email/
│   ├── base.html
│   ├── deposit.html
│   ├── withdrawal.html
│   ├── investment.html
│   └── property.html
│
└── sms/
    ├── deposit.txt
    ├── withdrawal.txt
    └── investment.txt
```

Create renderer:

```text
internal/utils/templates/
```

Use Go standard:

```go
html/template
```

unless the existing project already has an established template library.

## Email variables

Support appropriate variables such as:

```text
UserName
Amount
Reference
Date
PropertyName
Units
Status
AccountNumber
```

Use the actual existing domain models/DTOs where possible.

Do not invent fields that do not exist.

## SMS

Keep SMS templates concise.

Target:

```text
<160 characters
```

when practical.

## Acceptance Criteria

- Templates load.
- Variables render correctly.
- Missing required variables are handled safely.
- Email HTML is responsive/basic transactional quality.
- SMS messages are concise.
- No sensitive data is accidentally exposed.

---

# 18. Phase 9 — Notification Handlers and Routes

## Estimate

2–3 hours.

Create:

```text
internal/handlers/notification.go
```

Handler:

```text
NotificationHandler
```

Methods:

```text
List
Get
GetUnreadCount
MarkAsRead
MarkAllAsRead
Delete
GetPreferences
UpdatePreferences
```

Every handler:

1. Requires JWT authentication.
2. Extracts user ID from authenticated context.
3. Validates parameters.
4. Calls service.
5. Uses existing response helpers.
6. Returns appropriate HTTP status/error.

## Routes

Register:

```http
GET    /api/v1/notifications
GET    /api/v1/notifications/unread-count
GET    /api/v1/notifications/:id
PATCH  /api/v1/notifications/:id/read
PATCH  /api/v1/notifications/read-all
DELETE /api/v1/notifications/:id

GET    /api/v1/users/me/notification-preferences
PATCH  /api/v1/users/me/notification-preferences
```

Update:

```text
internal/routes/v1/routes.go
```

## Acceptance Criteria

- All 8 endpoints work.
- JWT required.
- User ownership enforced.
- Pagination works.
- Invalid IDs handled.
- Invalid requests handled.
- Existing response format preserved.

---

# 19. Phase 10 — Dependency Injection and Domain Integration

## Estimate

2–3 hours.

## `cmd/api/main.go`

Create:

```go
notificationRepo :=
    repositories.NewNotificationRepository(database.DB)

notificationPrefRepo :=
    repositories.NewNotificationPreferenceRepository(database.DB)

notificationService :=
    services.NewNotificationService(
        notificationRepo,
        notificationPrefRepo,
        hub,
        mq,
        logger,
    )
```

Replace the current nil notification service.

Pass the real service into Wallet Service.

Initialize Notification Handler.

Register Notification routes.

## Worker

Update:

```text
cmd/worker/main.go
```

Register:

```text
email consumer
sms consumer
realtime push consumer
```

Follow existing worker initialization conventions.

---

# 20. Wallet Integration

The Wallet Service already has notification call sites.

Do not duplicate them.

Ensure they now use the real Notification Service.

Required events:

## Deposit

```text
Deposit successful
Your wallet was credited with ₦X.
```

## Withdrawal

```text
Withdrawal requested
Your withdrawal of ₦X to ACCOUNT is being processed.
```

Preserve existing business logic and formatting conventions.

---

# 21. Investment Integration

Add:

## Investment created

Notify investor:

```text
Investment confirmed
```

Include useful metadata such as:

```text
investment_id
property_id
amount
```

where the actual domain model supports these fields.

## Property fully funded

Notify all relevant investors:

```text
Property funded
```

Do not implement an inefficient N+1 pattern if the existing repository offers a suitable bulk investor lookup.

For the initial milestone, prioritize correctness and clean architecture.

---

# 22. Property Integration

When property approval succeeds:

```text
Property approved
```

Notify the developer/property owner.

Only notify after the approval operation succeeds.

---

# 23. Phase 11 — Testing and Hardening

## Estimate

6–9 hours.

Testing is mandatory.

## Repository tests

Create:

```text
internal/repositories/notification_repository_test.go
```

Test:

- [ ] Create.
- [ ] Find by ID.
- [ ] Find by user.
- [ ] Pagination.
- [ ] Total count.
- [ ] Unread query.
- [ ] Unread count.
- [ ] Mark one as read.
- [ ] Mark all as read.
- [ ] Delete.
- [ ] Ownership.
- [ ] Delete old notifications.
- [ ] Empty results.
- [ ] Boundary conditions.

Use the existing repository test strategy, including SQLite in-memory where appropriate.

Target:

```text
20+ repository tests
```

---

# 24. Service Tests

Create:

```text
internal/services/notification_service_test.go
```

Test:

- [ ] Notify creates notification.
- [ ] Default preferences.
- [ ] Email enabled.
- [ ] Email disabled.
- [ ] Per-type email disabled.
- [ ] SMS enabled.
- [ ] SMS disabled.
- [ ] Per-type SMS disabled.
- [ ] Push enabled.
- [ ] Push disabled.
- [ ] Notification retrieval.
- [ ] Ownership.
- [ ] Mark read.
- [ ] Mark all read.
- [ ] Delete.
- [ ] Preference retrieval.
- [ ] Preference update.
- [ ] Cleanup.

Mock:

- repositories
- RabbitMQ
- WebSocket hub
- external delivery dependencies where appropriate

---

# 25. Handler Tests

Test all endpoints.

Include:

- [ ] JWT required.
- [ ] Valid authenticated user.
- [ ] Invalid UUID.
- [ ] Missing notification.
- [ ] Other user's notification.
- [ ] Invalid pagination.
- [ ] Limit > 100.
- [ ] Mark as read.
- [ ] Mark all as read.
- [ ] Delete.
- [ ] Get preferences.
- [ ] Update preferences.
- [ ] Invalid preference payload.
- [ ] Correct status codes.
- [ ] Correct response structure.

---

# 26. RabbitMQ/Worker Tests

Test:

- [ ] Email message published.
- [ ] SMS message published.
- [ ] Correct routing key.
- [ ] Correct queue.
- [ ] Consumer successfully processes email.
- [ ] Consumer successfully processes SMS.
- [ ] Consumer successfully processes push.
- [ ] Failure causes retry.
- [ ] Permanent failure reaches DLQ.
- [ ] Successful processing ACKs.
- [ ] Failed processing is not prematurely ACKed.

---

# 27. Integration Tests

At minimum:

## Deposit

```text
Deposit succeeds
→ notification created
→ email queued when enabled
```

## Investment

```text
Investment succeeds
→ notification created
```

## Property

```text
Property approved
→ developer notification created
```

## Preferences

```text
User disables email
→ deposit notification still exists
→ email is not queued
```

## WebSocket

```text
Notification created
→ connected user receives realtime event
```

---

# 28. IDOR Security Tests

These are mandatory.

Test:

```text
User A
   ↓
tries to access User B notification
   ↓
request rejected
```

Test this for:

- GET single
- MARK read
- DELETE

Also ensure list endpoints only ever query using authenticated `userID`.

---

# 29. Phase 12 — Notification Cleanup

Use:

```text
90 days
```

as the initial retention policy.

Implement:

```text
CleanupOldNotifications(ctx, daysOld)
```

Equivalent behavior:

```sql
DELETE FROM notifications
WHERE created_at < NOW() - INTERVAL '90 days';
```

Run cleanup through an appropriate background worker/job.

Do not run cleanup during an HTTP request.

Preserve the existing background job conventions.

---

# 30. Documentation

Create/update:

```text
docs/NOTIFICATION_SYSTEM.md
docs/NOTIFICATION_API.md
docs/NOTIFICATION_TESTING.md
NOTIFICATION_MODULE_COMPLETION.md
```

Document:

- architecture
- database schema
- notification types
- preference behavior
- API
- RabbitMQ topology
- retry policy
- DLQ
- templates
- worker behavior
- WebSocket behavior
- cleanup
- testing
- configuration
- operational considerations

---

# 31. CI/CD

Create:

```text
.github/workflows/test-notification-module.yml
```

It should:

- install dependencies
- run formatting checks as appropriate
- run tests
- run vet
- build API
- build worker
- report coverage

Target:

```text
≥80% coverage
```

for the module/project according to the existing CI conventions.

Do not create a conflicting CI strategy if the repository already has a suitable workflow. Extend/reuse existing workflows where appropriate.

---

# 32. Final Verification

Run:

```bash
go build ./cmd/api
```

Then:

```bash
go build ./cmd/worker
```

Then:

```bash
go test ./...
```

Then:

```bash
go vet ./...
```

Also run the project's existing formatting/lint commands if defined.

Verify:

```text
Migration up
Migration down
Migration up again
API startup
Worker startup
RabbitMQ queues
Notification creation
Notification API
Preferences
Email queue
SMS queue
Realtime push
Retries
DLQ
```

---

# 33. Definition of Done

The milestone is complete only when all of the following are true.

## Database

- [ ] `000022_create_notifications` exists.
- [ ] Up migration works.
- [ ] Down migration works.
- [ ] Tables exist.
- [ ] Foreign keys work.
- [ ] Required indexes exist.

## Repository

- [ ] CRUD complete.
- [ ] Pagination works.
- [ ] Unread count works.
- [ ] Ownership is enforced.
- [ ] Cleanup works.
- [ ] Context-aware.

## Service

- [ ] Notify works.
- [ ] In-app notification persists.
- [ ] Email preference respected.
- [ ] SMS preference respected.
- [ ] Push preference respected.
- [ ] Email queued.
- [ ] SMS queued.
- [ ] Push dispatched.
- [ ] Retrieval works.
- [ ] Mark read works.
- [ ] Mark all works.
- [ ] Delete works.
- [ ] Preferences work.
- [ ] Cleanup works.

## API

- [ ] All 8 endpoints implemented.
- [ ] JWT authentication.
- [ ] Validation.
- [ ] Ownership protection.
- [ ] Pagination.
- [ ] Correct response/error format.

## RabbitMQ

- [ ] Notification exchange.
- [ ] Email queue.
- [ ] SMS queue.
- [ ] Routing keys.
- [ ] Realtime queue.
- [ ] ACK.
- [ ] Retry.
- [ ] DLQ.

## Templates

- [ ] Base email.
- [ ] Deposit email.
- [ ] Withdrawal email.
- [ ] Investment email.
- [ ] Property email.
- [ ] Deposit SMS.
- [ ] Withdrawal SMS.
- [ ] Investment SMS.
- [ ] Template rendering works.

## Integration

- [ ] Wallet deposit notification.
- [ ] Wallet withdrawal notification.
- [ ] Investment notification.
- [ ] Property funded notification.
- [ ] Property approval notification.
- [ ] API dependency injection.
- [ ] Worker dependency injection.
- [ ] Routes registered.

## Testing

- [ ] Repository tests.
- [ ] Service tests.
- [ ] Handler tests.
- [ ] Queue tests.
- [ ] Integration tests.
- [ ] WebSocket tests.
- [ ] IDOR tests.
- [ ] Coverage target achieved.

## Build

```bash
go build ./cmd/api
go build ./cmd/worker
go test ./...
go vet ./...
```

All must pass.

---

# 34. AI Coding Agent Execution Rules

This section is specifically for the coding agent.

## Rule 1 — Work incrementally

Do NOT attempt to implement the entire milestone in one uncontrolled change.

Implement one phase at a time.

After each phase:

1. Inspect changes.
2. Format code.
3. Compile affected packages.
4. Run relevant tests.
5. Fix errors.
6. Only then continue.

---

## Rule 2 — Inspect before editing

Before modifying a file:

- Read the relevant existing implementation.
- Identify its conventions.
- Follow those conventions.

Do not blindly replace existing code.

---

## Rule 3 — Do not rewrite unrelated code

Only modify files necessary for Milestone 6.

Do not refactor unrelated modules merely because you notice improvements.

If an unrelated issue blocks implementation, document it first.

---

## Rule 4 — Reuse existing infrastructure

Before creating:

- RabbitMQ helpers
- logging helpers
- response helpers
- error types
- database helpers
- test helpers
- WebSocket helpers
- email functionality

search the repository for an existing implementation.

Reuse it.

---

## Rule 5 — Do not invent APIs

Do not invent nonexistent:

- repository methods
- configuration fields
- database columns
- user fields
- provider APIs
- helper functions

Inspect the codebase first.

If a required dependency does not exist, implement the smallest compatible abstraction following existing patterns.

---

## Rule 6 — Preserve compatibility

Do not break:

- Wallet Service
- Investment Service
- Property Service
- Authentication
- existing worker consumers
- existing queues
- existing API response conventions
- existing migrations

---

## Rule 7 — Security first

Before declaring completion, specifically test:

```text
IDOR
authentication bypass
cross-user notification access
unsafe SQL
sensitive information leakage
```

---

## Rule 8 — Never claim success without verification

Do not say:

```text
Implemented successfully
```

unless:

- code compiles,
- tests pass,
- relevant runtime paths were verified,
- and the final diff has been inspected.

---

## Rule 9 — Keep the implementation production-oriented

Do not implement fake success paths merely to satisfy tests.

For provider integrations that are intentionally stubbed, preserve the abstraction and clearly document the provider limitation.

---

# 35. Agent Progress Tracking

Maintain a checklist like:

```text
[ ] Phase 1 — Audit
[ ] Phase 2 — Database + Models
[ ] Phase 3 — Repository
[ ] Phase 4 — DTOs
[ ] Phase 5 — Service
[ ] Phase 6 — Preferences
[ ] Phase 7 — RabbitMQ + Workers
[ ] Phase 8 — Templates
[ ] Phase 9 — Handlers + Routes
[ ] Phase 10 — Integration
[ ] Phase 11 — Tests
[ ] Phase 12 — Documentation
[ ] Final Verification
```

At the end of each phase, report:

```text
Phase:
Status:
Files changed:
Tests run:
Tests passed:
Known issues:
Next phase:
```

---

# 36. Final Expected Architecture

The final system should behave as follows:

```text
                   ┌───────────────────┐
                   │   Domain Service  │
                   │ Wallet/Investment │
                   │     Property      │
                   └─────────┬─────────┘
                             │
                             ▼
                 ┌───────────────────────┐
                 │ Notification Service  │
                 └───────────┬───────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
              ▼              ▼              ▼
         PostgreSQL       RabbitMQ       Realtime
         Notification     Email/SMS        Push
              │              │              │
              │         ┌────┴────┐         │
              │         ▼         ▼         ▼
              │       Email      SMS    WebSocket
              │       Worker    Worker      Hub
              │
              ▼
        Notification API
              │
              ▼
           Frontend
```

The fundamental guarantee is:

> **Every notification that matters to the user is durably stored first; asynchronous channels are delivery mechanisms, not the source of truth.**

---

# 37. Final Agent Instruction

Start with **Phase 1 only**.

Inspect the actual repository and relevant files before writing implementation code.

Then proceed sequentially through Phases 2–12.

Do not skip phases.

Do not ask for confirmation after every phase unless you encounter a genuine architectural ambiguity or a potentially destructive change.

If a decision is already specified in this document, follow it.

If the existing codebase conflicts with this document, inspect the existing implementation and report the conflict before making a potentially breaking change.

At the end, provide a concise completion report containing:

1. What was implemented.
2. Files added.
3. Files modified.
4. Database migrations.
5. RabbitMQ changes.
6. API endpoints.
7. Notification integrations.
8. Tests added.
9. Test/build/vet results.
10. Any remaining limitations.

**Do not mark Milestone 6 complete until the Definition of Done and Final Verification sections pass.**

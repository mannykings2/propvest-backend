# 00 — TAKEOVER NOW (read first)

> Supersedes the "next up = Wallet stub" framing in `README.md`/`02-CURRENT-STATE.md`.
> Since those were written, an agent scaffolded **all remaining milestones (M3–M7)** in one uncommitted pass and rewrote `cmd/api/main.go` to wire the full stack. **It does NOT compile yet.** Your job: finish the missing files, make it build, then test.

Status snapshot: git branch has 6 commits (last `66372c1 users v2`). Everything below is **uncommitted** (`git status` = large WIP). Nothing here is committed.

---

## 1. What the scaffolding agent ADDED (present, uncommitted)

- **Models** (all present): audit_log, investment, notification, payment, property, property_document, verification_token (+ existing user/wallet/otp/refresh).
- **Migrations** `000007`–`000011` (up+down): extend_wallet_transactions, add_verification_tokens, add_payments, add_investments, add_notifications_documents_audit.
- **DTOs** (all present): admin, investment, notification, property, wallet, auth, user, mappers, error.
- **Repositories** (all present): investment, notification, payment, property, verification_token (+ user/wallet/refresh/otp/base).
- **New infra packages** (present, compile):
  - `internal/queue` — RabbitMQ client, `queue.New(url) *Client`, `.Close()`. Degrades gracefully w/o RabbitMQ.
  - `internal/mailer` — `mailer.New(cfg) Sender` (interface `Sender`).
  - `internal/payments` — `payments.NewProvider(cfg) Provider` (interface `Provider`).
  - `internal/audit` — `audit.NewLogRecorder() Recorder` (interface `Recorder`).
  - `internal/realtime` — `realtime.NewHub()` + `hub.Run()` (WebSocket hub).
  - `internal/logger` — structured logger.
- **Services present & IMPLEMENTED**: `wallet_service.go` (interface + 6 methods), `notification_service.go` (9 methods, has `StartRealtimeConsumer`), plus existing `auth_service.go`, `user_service.go`.
- **`cmd/api/main.go`**: fully rewritten. Constructs mq, hub, mailer, paymentProvider, auditRecorder, all repos, and wires: Auth, User, **Wallet, Property, Investment, Notification, Admin, Webhook, WebSocket** handlers via `v1.RegisterRoutes(apiV1, &v1.Handlers{...})`. Adds graceful shutdown + `notificationService.StartRealtimeConsumer`.

---

## 2. What is MISSING → the compile blockers (do these, in order)

### BLOCKER A — `internal/services/auth_service.go` (4 fixes)
- `:8` remove unused import `fmt`.
- `:24` remove unused import `.../internal/utils/token`.
- Interface `AuthService` (line 45) now declares 4 new methods; impl is missing **`ForgotPassword`** and **`ResetPassword`** (also declares `VerifyEmail`, `ResendVerification` — verify those exist).
- `:313` calls `s.issueVerificationEmail(...)` which is **undefined** — implement this helper.
- Struct already has deps for this: `tokenRepo repositories.VerificationTokenRepository`, `mailer mailer.Sender`, `audit audit.Recorder`. Use `verification_token` repo/model + mailer.

### BLOCKER B — Missing SERVICE files (referenced by main.go, do not exist)
main.go calls constructors that aren't defined anywhere:
- `services.NewPropertyService(propertyRepo, cloudinaryService, auditRecorder)` → **create `property_service.go`**
- `services.NewInvestmentService(investmentRepo, walletRepo, propertyRepo, notificationService, cfg, database.DB)` → **create `investment_service.go`**
- `services.NewAdminService(userRepo, propertyRepo, investmentRepo, walletRepo, refreshTokenRepo, auditRecorder)` → **create `admin_service.go`**

### BLOCKER C — Missing HANDLER files (only auth.go, health.go, user.go exist)
main.go calls these undefined constructors:
- `NewWalletHandler(walletService)` → `handlers/wallet.go`
- `NewPropertyHandler(propertyService)` → `handlers/property.go`
- `NewInvestmentHandler(investmentService)` → `handlers/investment.go`
- `NewNotificationHandler(notificationService)` → `handlers/notification.go`
- `NewAdminHandler(adminService)` → `handlers/admin.go`
- `NewWebhookHandler(walletService, cfg)` → `handlers/webhook.go` (Paystack webhook → `walletService` deposit credit)
- `NewWebSocketHandler(hub, cfg)` → `handlers/websocket.go`

### BLOCKER D — `internal/routes/v1/routes.go` is STALE
Current signature: `RegisterRoutes(router, authHandler, userHandler, cfg)` with **no `Handlers` struct**.
main.go calls: `RegisterRoutes(apiV1, &v1.Handlers{Auth,User,Wallet,Property,Investment,Notification,Admin,Webhook,WebSocket}, cfg?)`.
→ **Rewrite routes.go**: add `type Handlers struct{...}` with those fields, change signature, and add all route groups (wallet, properties, investments, notifications, admin behind RBAC; `/webhooks/paystack` public; `/ws` websocket). Auth routes already written; forgot/reset routes are commented out — enable them once Blocker A is done.

---

## 3. Make-it-build loop

```
go build ./...        # fix until clean; services fails first (Blocker A)
go vet ./...
```
Order: A → B → C → D → `go build ./...` clean. There are currently **0 `_test.go` files** — add tests after it compiles (docs mandate 85%; see `05-COMPLETION-PLAN.md` §0.6).

Run locally: `make up` (Postgres host port **5435**, not 5432), `make migrate-up`, `make run` (:8080). Ensure `.env DATABASE_URL` uses 5435. New env needed by infra: `RABBITMQ_URL`, Paystack keys, SMTP/mailer vars — check `config.Load()`/`cfg.Validate()` and add to `.env`.

---

## 4. Business rules you MUST honor when filling in B/C (from docs)

- **Money = kobo (int64)**. Every balance change = 1 `wallet_transactions` ledger row w/ `balance_before/after`.
- **Wallet deposit**: never credit on init; credit **only** in `ProcessDepositWebhook` after Paystack **signature verify + idempotency** (dedupe by provider reference), inside **one DB tx** with `SELECT ... FOR UPDATE` on wallet. (`wallet_service.go` likely already does this — verify.)
- **Investment (M5, highest risk)**: single DB tx → lock wallet FOR UPDATE → assert funds → debit + ledger → create investment → `UPDATE properties SET slots_sold=slots_sold+n WHERE slots_sold+n<=total_slots` (prevent oversell) → commit → post-commit notify. Idempotent on client key.
- **RBAC**: property create = developer/admin; approve/reject = admin only; investments = investor. Middleware `internal/middleware/rbac.go` (`RequireRole`) already exists — use it.
- **Property status**: reconcile enum (`04-DECISIONS-TO-RESOLVE.md` D-property). Model vs domain differ.

---

## 5. Deeper context (unchanged, still valid)

- `01-PROJECT-OVERVIEW.md` — stack/architecture.
- `03-ISSUES-AND-CORRECTIONS.md` — P1 auth bugs (some may now be addressed — re-verify against current code).
- `04-DECISIONS-TO-RESOLVE.md` — D4 kobo / D5 payments schema / D6 error shape / property enum. **Confirm these are settled**, the scaffolding assumed answers.
- `05-COMPLETION-PLAN.md` — full milestone plan + tests + DoD per module. Your B/C work implements M3–M7 from it.

**Do first:** Blocker A (unblocks all builds), then B, C, D. Then `go build ./...` clean, then tests. Don't commit until it compiles.

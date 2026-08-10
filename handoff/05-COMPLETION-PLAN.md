# 05 — Completion Plan

> A milestone-by-milestone plan to take PropVest from ~35% to production-ready, staying aligned with `docs/` (and with the doc-conflict decisions in `04-DECISIONS-TO-RESOLVE.md`).
>
> Sequencing follows the roadmap (`08-Roadmap/8.1`) with one addition: a **Phase 0 remediation + foundation** pass inserted before new features, because the missing cross-cutting pieces (logging, tests, error shape) get exponentially more expensive to retrofit once 5 more modules exist.
>
> Each milestone lists: Goal · New/changed files · DB migrations · Endpoints · Business rules · Tests · Definition of Done. "DoD" mirrors the roadmap §15 (acceptance criteria + tests + migrations + docs updated).

Legend for effort: **S** ≤1 day · **M** 2–4 days · **L** ~1 week · **XL** >1 week (for one focused engineer).

---

## Phase 0 — Remediation & Foundations  (do first)  · effort **L**

**Why first:** fixes the P1 correctness/security bugs and installs the cross-cutting infrastructure every later milestone depends on. Skipping this means re-touching every service later.

### 0.1 Fix the P1 bugs (from `03-ISSUES-AND-CORRECTIONS.md`)
- Password DTO `min=8`→`min=12` (or align to policy). 
- Register: pre-check phone uniqueness + map Postgres `23505` → 409 (`ErrPhoneAlreadyExists`/`ErrEmailAlreadyExists`).
- Stop swallowing errors: propagate the security-critical ones (session revoke on password change; revoke-old during rotation).
- `getUserIDFromContext` returns error when user missing.
- Validate `ACCESS_TOKEN_TTL`/`REFRESH_TOKEN_TTL` at boot (don't ignore `ParseDuration` error).
- Delete dangling comment `auth_service.go:616-618`; remove empty `internal/auth/`.

### 0.2 Structured logging  (`internal/logger`)
- Add `log/slog` JSON logger (or Zap). Package-level logger set at boot + injected where practical.
- Rewrite `middleware/logger.go`: one structured line/request with `request_id, method, path, status, duration_ms, ip, user_agent, user_id`.
- Ensure `X-Request-ID` is **echoed as a response header** (currently only in body). 
- Add an **audit log** path (start with a structured `audit` logger; add an `audit_logs` table in M7). Log: login success/failure, token refresh/revoke, password change.

### 0.3 Error-envelope convergence  (`internal/response`)  — decision D6
- One success shape `{success,message,data}`; one error shape `{success,message,errors,request_id}` (field-map for validation, single message for domain errors). Route all handlers through it. Delete the dead `dto/error_dto.go` duplicates.

### 0.4 Config hardening  (`internal/config`)
- Add missing mapped fields as needed later; **now**: read `BCRYPT_COST`, DB pool sizes from env; sanitize `.env.example` (placeholder secrets, de-dupe Cloudinary).

### 0.5 Graceful shutdown + server timeouts  (`cmd/api/main.go`)
- Explicit `http.Server` with timeouts; `signal.NotifyContext`; `srv.Shutdown`; close DB pool.

### 0.6 Test + CI harness  (unblocks the 85% coverage mandate)
- Add `tests/` layout per `6.3` §5 **or** colocated `_test.go` (pick one; colocated is idiomatic Go — recommend colocated unit tests + a `tests/integration` package using a real Postgres via Docker/Testcontainers).
- Write the **first** tests to prove the harness: `validators` (password/phone — pure, easy 95%), `utils/jwt`, `utils/otp`, and one repository integration test (user create/find) against a throwaway Postgres.
- Add `.github/workflows/ci.yml`: `go vet`, `golangci-lint`, `go test ./... -race -cover`, build. Gate merges.
- Add `golangci-lint` config.

### 0.7 Finish Milestone 1 auth (email verification + password reset)
The roadmap files these under **M1**; they're missing. Build now while auth is fresh:
- New tables: `email_verifications`, `password_reset_tokens` (both: id, user_id, token_hash, expires_at, used_at, created_at). Use the existing unused `internal/utils/token` package for generation.
- Endpoints: `POST /auth/verify-email`, `POST /auth/resend-verification`, `POST /auth/forgot-password`, `POST /auth/reset-password`.
- Rules (`4.2` §9/§10): tokens single-use + expiring; resend invalidates prior; **password reset revokes all refresh tokens**; email-verify may gate certain actions (feature-flag `REQUIRE_EMAIL_VERIFICATION`).
- Email delivery: create `internal/notifications` (or `internal/mailer`) with an SMTP sender + a **mock** for dev (log link). Wire SMTP config into `Config`.

**DoD Phase 0:** P1s fixed; JSON logs with request IDs; single error shape; graceful shutdown; CI green with a real (small) test suite; email verify + password reset working end-to-end with tests.

---

## Milestone 3 — Wallet System  · effort **L**  · roadmap M3

**Goal:** secure wallet retrieval, deposit (Paystack), webhook credit, withdrawal request, transaction history — all transactional and idempotent.

**Lock decisions first:** D4 (kobo), D5 (extend `wallet_transactions` + add `payments`), D6 (error shape) must be settled.

### DB migrations
- `00000X_extend_wallet_transactions`: add `user_id` (FK), `external_reference`, `idempotency_key` (unique, nullable) or formalize `reference` as idempotency key, `metadata JSONB`; widen `type`/`status` CHECK sets to the `2.3` enums.
- `00000X_add_payments`: `payments`(id, user_id, provider, provider_reference unique, amount kobo, currency, status, raw_payload JSONB, created_at, updated_at).
- (Optional) `bank_accounts` for withdrawals (id, user_id, bank_code, account_number, account_name, verified, created_at) — or defer to a later milestone and stub withdrawal as "request recorded, manual settlement."

### Code
- `internal/services/wallet_service.go` — implement the interface (currently commented out):
  - `GetWallet(ctx, userID)` → `dto.WalletResponse` (use existing `dto.WalletToResponse` mapper — stop building inline).
  - `InitiateDeposit(ctx, userID, amountKobo, idemKey)` → creates a `payments` row (status=PENDING) + calls Paystack initialize → returns `{authorization_url, reference}`. **Never credits wallet here.**
  - `ProcessDepositWebhook(ctx, signature, rawBody)` → verify Paystack signature (HMAC SHA512 of body with secret), parse, **idempotency check** (reference already processed? return original), then in **one DB transaction**: lock wallet (`FindByUserIDForUpdate` — add it to the interface, see P3), credit `main_balance`, append `wallet_transactions` (DEPOSIT, balance_before/after), mark `payments` COMPLETED. Respond 200.
  - `InitiateWithdrawal(ctx, userID, amountKobo, bankAccountID)` → validate KYC/limits/**sufficient balance**, lock wallet, debit, append WITHDRAWAL txn (status PENDING/PROCESSING), record payout intent. (Actual bank payout can be manual/stubbed initially.)
  - `GetTransactionHistory(ctx, userID, filter, page, limit)` → paginated, filter by type/status/date, sort — use `response.Paginated`.
- `internal/repositories/wallet_repository.go` — **add `FindByUserIDForUpdate` and `TransactionExists` to the interface** (they already exist on the impl; P3).
- `internal/payments/paystack/` — new package: `Initialize`, `VerifySignature`, `VerifyTransaction` (fallback verify via API). Add a `PaymentProvider` interface in `payments/shared` so Flutterwave can slot in later. Mock provider for tests/dev.
- `internal/handlers/wallet.go` — new `WalletHandler`; wire into `main.go` (replace `_ = walletService`).
- `internal/routes/v1/routes.go` — uncomment/enable wallet group (all behind `Auth`), and add a **public but signature-verified** `/webhooks/paystack` (NOT behind Auth).
- Config: add `PAYSTACK_SECRET_KEY`, `PAYSTACK_PUBLIC_KEY`, `PAYSTACK_WEBHOOK_SECRET` to `Config`.

### Endpoints (`3.2` §17)
`GET /wallet` · `POST /wallet/deposit` · `POST /wallet/withdraw` · `GET /wallet/transactions` · `POST /webhooks/paystack`.

### Business rules (`2.3`, `5.1`, `1.4` §13)
- Balance never negative (DB CHECK + service guard).
- Every balance change writes exactly one ledger row with correct `balance_before/after`.
- **Idempotency mandatory** on deposit init (client `Idempotency-Key` or provider reference) and webhook (dedupe by provider reference). Duplicate → return original, never double-credit.
- Never credit before signature-verified webhook (or server-side verify call).
- Row-lock wallet (`SELECT … FOR UPDATE`) inside the transaction to prevent races.

### Tests (`6.3`)
- Unit (service, mocked repo + mocked provider): deposit init happy/invalid amount; webhook credit happy; **webhook idempotency (double delivery credits once)**; withdrawal insufficient funds; signature-verification failure rejected.
- Integration (real Postgres): concurrent deposits to same wallet keep ledger consistent; balance == sum of ledger.

**DoD:** wallet endpoints live & authed; deposits credit only via verified idempotent webhook inside a transaction; history paginates; ≥90% service coverage on wallet; docs `2.3`/`3.2` updated to match built schema.

---

## Milestone 4 — Property Module  · effort **L**  · roadmap M4

**Goal:** property lifecycle (create → submit → approve/reject → funding → funded/completed), media/doc upload, public browse + filters.

### DB migrations
- `properties` table already exists. Add: `property_documents`(id, property_id, type[image|legal|agreement|other], url, mime, size, uploaded_by, created_at). 
- Reconcile the **status enum**: model uses `draft|funding|live|sold|closed`; domain model (`1.2` §8) wants `Draft→Submitted→Approved→Funding→Funded→Completed(+Rejected)`. **Decide** and migrate (add `submitted`, `approved`, `rejected`, `funded`, `completed`; keep `funded_pct`/`slots_sold`). This gap is called out in the audit.

### Code
- `internal/repositories/property_repository.go` (CRUD, filters: status/location/state/developer, pagination, sort).
- `internal/services/property_service.go` (create as Draft; submit; approve/reject = admin; ownership checks; only Draft is editable per `3.2` §PATCH; funding recalculation).
- `internal/handlers/property.go`; enable property routes (public GET list/detail; developer/admin POST/PATCH/DELETE via `RequireRole` — the RBAC middleware finally gets used).
- `internal/dto/property/*` (Create/Update/Response; consider the `1.3` feature-subfolder DTO layout now).
- Reuse `utils/cloudinary` `UploadPropertyImage` (already written, currently unused) via a proper `internal/storage` package (`1.3`).

### Endpoints (`3.2` §18)
`GET /properties` · `GET /properties/{id}` · `POST /properties` (dev/admin) · `PATCH /properties/{id}` · `DELETE /properties/{id}` (soft) · `PATCH /admin/properties/{id}/approve` · `.../reject` (reason required).

### Business rules
- Only Developer/Admin create; only owner/admin edit; edit only in Draft; approve/reject admin-only; funding cannot exceed goal; only Approved/Funding accept investment (enforced in M5); media stored externally (Cloudinary), DB stores URL only.

### Tests
- RBAC: investor forbidden from POST; developer can't approve; admin can. Filter/pagination correctness. Status-transition guard (can't edit after approval).

**DoD:** full property CRUD + approval workflow; RBAC enforced in middleware **and** service (`4.2` §12); public browse works for the React landing page; tests per `6.3`; `5.2`/`3.2` updated.

---

## Milestone 5 — Investment Engine  · effort **XL**  · roadmap M5 (highest complexity)

**Goal:** the core workflow — buy slots/shares in a property, atomically debiting the wallet, creating the investment, updating property funding, writing the ledger, and notifying the user.

### DB migrations
- `investments`(id, user_id, property_id, slots, amount kobo, status[created|confirmed|active|completed|cancelled], created_at, updated_at). 
- Ensure `properties.slots_sold`/`funded_pct` update path; consider a `property_id, user_id` composite index.

### Code
- `internal/repositories/investment_repository.go`.
- `internal/services/investment_service.go` — the atomic flow (`2.3` §Investment, `1.4` §14):
  1. Validate property exists & accepting investment; validate amount/slots; check slots availability.
  2. **Begin transaction**; lock wallet (`FOR UPDATE`); assert sufficient `main_balance`.
  3. Debit wallet; append `wallet_transactions` (INVESTMENT, balance_before/after).
  4. Create `investments` row.
  5. Update `properties.slots_sold` + `funded_pct` (guard against overselling; lock or use atomic `UPDATE ... WHERE slots_sold + :n <= total_slots`).
  6. Commit. On any failure → rollback (no partial state).
  7. Post-commit: publish event → notification (M6).
- `internal/handlers/investment.go`; enable investment routes (investor-only via RBAC).
- Portfolio read endpoints: aggregate holdings, ROI, history (this covers the frontend `INVESTMENTS`/`HISTORY`/portfolio screens from `1.5`).

### Endpoints (`3.2` §19)
`POST /investments` · `GET /investments` · `GET /investments/{id}` · `GET /portfolio/summary` · `GET /portfolio/history`.

### Business rules & invariants (`1.2` §19)
- Amount > 0; investor must have sufficient balance; only approved/funding properties; **cannot exceed funding goal / total slots**; every investment has exactly one INVESTMENT transaction; idempotency (client key) to prevent double-buy on retry.

### Tests (the most important in the app)
- Integration, real DB, concurrency: **N concurrent investments on the last available slots never oversell**; wallet debit == investment amount; property funding == sum of confirmed investments; rollback on forced failure leaves wallet+property+ledger unchanged.
- Unit: insufficient funds; property closed; amount invalid; idempotent retry.

**DoD:** investment purchase is atomic, race-safe, idempotent; portfolio endpoints back the React dashboard; ≥90% service coverage; `5.3`/`2.3`/`3.2` updated.

---

## Milestone 6 — Notification System  · effort **M**  · roadmap M6

**Goal:** in-app notifications + email (SMS already stubbed), read/unread, preferences; async delivery.

### DB migrations
- `notifications`(id, user_id, type, title, body, read, metadata JSONB, created_at). 
- (Optional) `notification_preferences`(user_id, channel flags).

### Code
- `internal/notifications` package: channel senders (in-app = DB row; email = SMTP from Phase 0.7; SMS = existing `utils/sms`). Template support.
- Wire **domain events** (`internal/events`) so services publish (UserRegistered, WalletFunded, InvestmentCreated, PropertyApproved, WithdrawalCompleted) and notification handlers consume.
- Async: introduce the **worker** (`cmd/worker`) + a queue. Minimal viable = Redis + Asynq (both named in `starter-guide`), or a simple DB-backed outbox + polling worker to avoid new infra. Recommend **Asynq** since Redis is already in compose. Retry with backoff (`5.5`, `7.3`).
- `internal/handlers/notification.go`; enable routes.

### Endpoints (`3.2` §20)
`GET /notifications` · `PATCH /notifications/{id}/read` · `PATCH /notifications/read-all` · `DELETE /notifications/{id}` (soft).

**DoD:** business events produce in-app + email notifications via the worker; retries on failure; read-state endpoints; tests mock external senders (`6.3` §17).

---

## Milestone 7 — Administration  · effort **L**  · roadmap M7

**Goal:** admin dashboards, user management, property approvals (from M4), audit-log viewer.

### DB migrations
- `audit_logs`(id, actor_id, action, target_type, target_id, metadata JSONB, ip_address, user_agent, created_at) — **append-only** (`1.2` §15, `4.2` §17). Start writing to it from Phase 0.2's audit path.

### Code
- `internal/services/admin_service.go`, `internal/handlers/admin.go`; all routes behind `Auth + RequireRole("admin")`.
- Dashboard aggregations (users, developers, wallet volume, active properties, pending approvals — `3.2` §22).
- User management: list/search/filter; suspend/activate (→ `LogoutAllDevices`); promote/demote role.

### Endpoints (`3.2` §22)
`GET /admin/dashboard` · `GET /admin/users` · `PATCH /admin/users/{id}` · `GET /admin/properties` · approve/reject (M4) · `GET /admin/audit-logs`.

**DoD:** admin-only enforced; suspend forces logout everywhere; audit log captures all sensitive admin actions; tests for RBAC + audit emission.

---

## Milestone 8 — Production Hardening  · effort **XL**  · roadmap M8

**Goal:** meet the production success metrics (`8.1` §Success Metrics, `7.1`, `7.2`).

- **Testing to targets** (`6.3` §25): services 90%, repos 85%, handlers 80%, middleware 80%, validators 95%, overall **85%**. Add E2E happy-path (register→verify→login→deposit→invest→portfolio).
- **Rate limiting**: implement the commented-out middleware (`main.go:164`) — anonymous 100/min, auth 300/min, login 10/min, reset 5/hr (`4.2` §16), Redis-backed, configurable.
- **Security headers** + HTTPS assumptions (behind Nginx per `7.1`), payload size limits, MIME validation on uploads (the `ValidateImageFile` MIME sniff is currently commented out in `utils/cloudinary` — enable it).
- **Observability** (`7.2`): `/metrics` (Prometheus: `http_requests_total`, `http_request_duration_seconds`, `database_query_duration_seconds`, business counters), `/health` + `/health/ready`, OpenTelemetry spans (target maturity Level 3 before launch).
- **Swagger/OpenAPI** (`6.1` §23) — generate from annotations; keep `3.2` in sync.
- **CI/CD** (`7.1` §15): lint → unit → integration → security scan → build → docker → deploy staging → manual approval → prod. Multi-stage Dockerfile (non-root, small image); the `docker/Dockerfile.api` referenced by `starter-guide` does not exist yet — create it.
- **DB ops**: backups + restore drills; migrations run as a discrete deploy step (not silently on boot in prod — currently `main.go` runs migrations at startup, which is convenient in dev but risky in prod; gate behind env or a separate `migrate` job per `7.1` §17).
- **Reconciliation job** (`2.3` §17): daily wallet-balance vs ledger vs provider.

**DoD:** coverage ≥85%; zero critical security findings; automated deploys; dashboards + alerts live; OpenAPI published; docs synchronized.

---

## Cross-cutting backlog (thread through every milestone)

- Keep the **docs in sync** as you build (the genesis prompt and roadmap DoD both require it). Every milestone updates `3.2` (API), `2.2`/`2.3` (schema), and the relevant `05-Modules` stub — which are currently near-empty and should be fleshed out from the code you write.
- Use **feature-subfolder DTOs** (`1.3`) from M4 onward.
- Every financial mutation: transaction + row lock + idempotency + ledger entry. No exceptions (`2.3` §24 anti-patterns).
- Every new endpoint: DTO validation, RBAC where relevant, structured log line, audit entry if sensitive, and tests before merge (`6.3` DoD).

---

## Suggested sequencing & rough timeline (one focused engineer)

| Order | Milestone | Effort |
|---|---|---|
| 1 | Phase 0 (remediation + logging + tests/CI + finish M1 auth) | ~1.5 wk |
| 2 | M3 Wallet (+Paystack) | ~1 wk |
| 3 | M4 Property (+RBAC in anger) | ~1 wk |
| 4 | M5 Investment engine | ~1.5 wk |
| 5 | M6 Notifications (+worker/Asynq) | ~3–4 d |
| 6 | M7 Admin (+audit logs) | ~1 wk |
| 7 | M8 Hardening (tests to target, rate limit, metrics, CI/CD, Swagger, Docker) | ~2 wk |

Releases map to roadmap §17: 0.1 (Foundation+Auth) → 0.2 (User+Wallet) → 0.3 (Property) → 0.4 (Investment) → 0.5 (Notifications+Admin) → 1.0 (Hardening).

**First thing the next session should do:** get owner sign-off on the D4/D5/D6 schema+contract decisions (they block M3), then start Phase 0.1–0.3. Everything else follows cleanly from there.

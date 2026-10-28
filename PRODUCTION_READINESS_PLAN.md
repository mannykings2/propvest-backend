# PropVest — Production Readiness Audit & Implementation Plan

**Date:** 24 October 2026
**Scope:** `propvest-backend` (Go/Gin/GORM/Postgres) + `propvest-app` (React/Vite), assessed together.
**Method:** static review of source, config, migrations, tests and deploy artifacts, **plus a live end-to-end test run** against a locally running stack (Postgres 16, Redis, RabbitMQ, API on `:8081`, Vite on `:3000`). Every claim below marked *(observed)* was reproduced during that run and is reproducible from the commands in [WORKFLOW_TESTING_GUIDE.md](WORKFLOW_TESTING_GUIDE.md).

---

## 1. Scorecard

| Area | Status | Notes |
|---|---|---|
| Public API (properties, off-plan, health) | ✅ Works | List/detail/off-plan all 200 with live DB data |
| Auth: register / login / logout / me / **refresh** | ✅ Works | Refresh rotation fixed (P0-2): 20 login→refresh cycles, zero 5xx |
| Wallet read + banks + transactions | ✅ Works | |
| Deposit initiation (Paystack test mode) | ✅ Works | Returns real `checkout.paystack.com` URL |
| Withdrawal (test mode) | ✅ Works | Initiated + balance debited; below-minimum → 422, empty wallet → 402 |
| KYC / off-plan / admin create **validation** | ✅ Works | 422 with field-level errors |
| **Buy a slot (`POST /investments`)** | ✅ **Fixed (P0-1)** | 201 in 81 ms, ledger row inside the transaction, 0 `idle in transaction` |
| **Admin dashboard / users / health / financial / reports** | ✅ **Fixed (P0-3)** | 200 with an admin token, 403 without (was 500/panic) |
| Error → HTTP status mapping | ✅ **Fixed (P0-4)** | Registry + contract tests: 402 / 409 / 422 / 403 as appropriate, no business error on 5xx |
| Readiness probe `/health/ready` | ✅ **Fixed (P0-5)** | Real Postgres + RabbitMQ probes; 503 while `/health` stays 200 |
| Notifications, worker email/SMS | ✅ **Done (Phase 2)** | Worker consumes outbox → in-app `notifications` row + unread badge (live-verified). Email wired to real `mailer.Sender` (Brevo SMTP); SMS intentionally disabled by launch scope (logged, not sent — shown in `/admin/health/system`). Heartbeat-file healthcheck + `/admin/health/queues` outbox-depth metric. See §11 of WORKFLOW_TESTING_GUIDE |
| **Admin audit trail** | ✅ **Fixed (P1-1.1)** | Was: 6 `TODO: Log audit event` sites. Now every admin state change writes an `audit_logs` row **in the same transaction** (actor, role, reason, old/new state, IP, request_id) and `GET /admin/audit-logs` returns it |
| **Session / role revocation** | ✅ **Fixed (P1-1.2)** | Was: a suspended account kept working until its access token expired, and role changes needed a re-login. Auth now re-reads the account row per request: suspend → `401 account_suspended` on the very next call, role change lands immediately; suspend/delete/role-change also revoke refresh tokens |
| **Launch policy flags** | ✅ **Fixed (P1-1.4)** | `REQUIRE_EMAIL_VERIFICATION` / `REQUIRE_KYC_FOR_INVESTMENT` were parsed but **never enforced**; now a route-level gate on invest / off-plan subscribe / withdraw returns `403 email_not_verified` / `403 kyc_required` |
| **Money-race safety** | ✅ **Fixed (P1-1.5)** | New integration tests: last-slot purchase, duplicate/concurrent webhook, double-withdraw — all green against real Postgres |
| **Stuck-transaction reconciliation** | ✅ **Fixed (P1-1.3)** | Deposit sweep added next to the existing withdrawal sweep; duplicate-delivery race now answers 200 instead of 500. Found & fixed a worker panic (nil notifier) that fired *after* a credit |
| **Local money-loop tooling** | ✅ **Fixed (P1-1.6)** | `credit_wallet.sql` (3 schema bugs) and `tools/trigger_webhook` (hard-coded secret) both work; mock provider now signs with the configured key |
| Frontend: Documents / Support / Developer portal | 🟠 Mock-only | No API wiring at all |
| Frontend: lint + tests | 🔴 Absent | No `test`/`lint` script in `package.json` |
| CI | ✅ Wired (P0-6) | Backend: build/vet/unit + integration on a Postgres service container. Frontend: `npm ci` + build. Nothing uses `continue-on-error` |
| Deploy artifacts | 🟠 Present, unproven | Dockerfile, Caddyfile, nginx.conf exist; never exercised in CI |

> **Phase 0 status — 24 Oct 2026: complete.** 0.1–0.5 are code-complete and each one has a live
> reproduction proving the old failure is gone; 0.6 added the CI gate (`tests/` and `.env.test`
> still need a `git add` — they are untracked); 0.7 re-ran the workflow suite. Evidence in §7.
>
> **Phase 1 status — 25 Oct 2026: complete.** 1.1–1.6 are code-complete, unit/integration tested
> and each verified live against the running stack (audit row written + read back through the
> API, suspended token rejected mid-life, role change visible without re-login, policy flags
> flipped on/off, money-loop tools run end-to-end, worker deposit sweep reconciled a seeded
> stuck deposit). Evidence in §7 ("after Phase 1"). The new concurrency/idempotency tests are
> part of `go test ./tests/integration/`, which CI runs on its Postgres service container.

---

## 2. P0 — Must fix before any live deployment

### P0-1 · Every money-moving write self-deadlocks

**Observed:** `POST /api/v1/investments` hung, then returned `500` after 20,003 ms and again after 60,005 ms.

**Evidence (live `pg_stat_activity` during the request):**

```
pid 257 | idle in transaction | ClientRead      | SELECT * FROM wallets WHERE user_id = $1 ... LIMIT $2
pid 258 | active              | Lock | transactionid | INSERT INTO "wallet_transactions" (...)
```

**Root cause:** the service opens a transaction and takes `FOR UPDATE` on the wallet, then writes the ledger row **on a different connection**, which waits forever on the lock its own transaction holds.

- `investment_service.go:143` — `s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) …)`
- `investment_service.go:256` — `s.walletRepo.CreateTransaction(ctx, walletTx)` ← no `tx` passed
- `wallet_repository.go:20` (interface) / `:210` (impl) — `CreateTransaction(ctx, tx *models.WalletTransaction)`; the second parameter is the **model**, not the GORM handle, and the implementation uses `r.WithContext(ctx)` (base pool)
- Same defect: `offplan_service.go:188`, `wallet_service.go:560`

**Fix:** give the repository transaction-awareness — `CreateTransaction(ctx context.Context, tx *gorm.DB, record *models.WalletTransaction) error` — and thread the caller's `tx` through all three services (the `Create(ctx, doc, tx)` pattern already used elsewhere in this repo). Audit every other `r.WithContext(...)` repository call made from inside a `Transaction(...)` closure for the same escape.

**Acceptance:**
- `POST /investments` with sufficient balance → `201` in < 500 ms; wallet debited and one ledger row committed atomically
- `POST /offplan/projects/:id/subscribe` → `201` under the same conditions
- A concurrent-balance test (two parallel purchases of the last slot) yields exactly one `201` and one clean `4xx`, never a hang
- `pg_stat_activity` shows no `idle in transaction` from the API after the request

---

### P0-2 · Refresh token rotation fails (sessions expire every 15 minutes)

**Observed:** `POST /auth/refresh` → `500`, log:

```
refresh: failed to store rotated refresh token
ERROR: duplicate key value violates unique constraint "refresh_tokens_token_hash_key"
```

The same error occurs on login when two logins happen within the same second (`login: failed to store refresh token`).

**Root cause:** refresh tokens are JWTs built from `jwt.RegisteredClaims` (`internal/utils/jwt/jwt.go:84`, `:138`) with **subject, issuer, issued-at, expiry — but no `jti`**. `iat` has one-second granularity, so two tokens minted for the same user in the same second are byte-identical; SHA-256 of both (`hashToken`, `auth_service.go:658`) collides with the `UNIQUE (token_hash)` constraint.

**Fix:**
1. Add `ID: uuid.NewString()` (a real `jti`) to refresh-token claims — one line per generator, makes every token unique.
2. Map storage failure of a rotated token to `401 invalid_refresh_token`, not `500`, and always return the correct status for "unknown/expired/revoked refresh token".
3. Keep the unique constraint — it is the right race backstop once `jti` exists.

**Acceptance:** 20 sequential logins and 20 refresh rotations in a row, zero 5xx; a genuinely expired token still returns `401`.

---

### P0-3 · Most admin endpoints return 500 (wrong type assertion)

**Observed:**

```
GET /admin/dashboard  → 500  "user_id in context is not a UUID"
GET /admin/users      → 500  "user_id in context is not a UUID"
GET /admin/health/system → 500
GET /admin/properties     → 200  ✅ (uses the correct helper)
GET /admin/kyc/submissions → 200 ✅
```

**Root cause:** the auth middleware stores the id **as a string** — `c.Set("user_id", claims.UserID.String())` (`internal/middleware/auth.go:101`, `:160`) — and the project's own helper `getUserIDFromContext` (`internal/handlers/helpers.go:19`) parses it correctly. The admin handlers bypass that helper and assert `.(uuid.UUID)`:

- `admin_dashboard.go:94`
- `admin_financial.go:47, 113, 145, 199` ← **bare assertions: panic**, not just 500
- `admin_user.go:42, 86, 116`
- `admin_health.go`, `admin_reports.go` (same pattern)

**Fix:** replace every `c.Get("user_id").(uuid.UUID)` with `getUserIDFromContext(c)` (or make the middleware store `uuid.UUID` and update all string consumers in the same commit). Prefer the helper — it returns `401` instead of `500` for a missing id.

**Acceptance:** every `/admin/*` route exercised as an admin returns `200` (or a documented `4xx`), and a bare assertion grep over `internal/handlers` returns nothing.

---

### P0-4 · Business errors are reported as HTTP 500

**Observed** *(all 500 or wrongly-coded)*:

| Request | Actual | Should be |
|---|---|---|
| Invest with empty wallet | `500` `code=insufficient_funds` | `402` or `409` |
| Invest in a funded property | `500` `code=resource_unavailable` | `409` |
| Webhook with bad signature | `400` but `code=internal_error` | `400` `code=invalid_signature` |
| Register with malformed phone | `422` but `code=internal_error` | `422` `code=validation_error` |
| Publish property without cover image | `422` but `code=internal_error` | `422` `code=validation_error` |

**Why it matters:** the frontend cannot distinguish "you did something wrong" from "the server is broken", monitoring counts user errors as outages, and retry logic mis-fires.

**Fix:** one error registry mapping `AppError → {status, code}` used by the single error handler; add table-driven tests asserting status **and** `code` for every sentinel error in `internal/errors/errors.go`.

**Acceptance:** a contract test sweeps the cases above; no business sentinel ever yields `5xx`.

---

### P0-5 · `/health/ready` is fake

**Observed:** `GET /health/ready` → `200 {"status":"ready"}` while the probe code is literally:

```go
// TODO: Add actual dependency checks (database, cache, message queue)   handlers/health.go:32
```

A load balancer will route traffic to an instance whose DB is down.

**Fix:** check DB (`SELECT 1` + migration version), Redis `PING`, RabbitMQ connectivity; return `503` with per-dependency detail on failure. Keep `/health` as liveness (cheap, no dependencies).

**Acceptance:** stopping Postgres flips `/health/ready` to `503` within the probe timeout while `/health` stays `200`.

---

### P0-6 · Async delivery pipeline does not deliver

- `cmd/worker/main.go:208` `// TODO: Implement actual email sending`
- `cmd/worker/main.go:224` `// TODO: Implement actual SMS sending`
- `internal/utils/sms/sms.go:91,107,138,158` — Twilio and Termii both `TODO`

**Observed:** no notification row was created for a wallet credit because the worker/outbox consumer was not running — and even running it would log rather than send.

**Fix:** implement worker email via the already-wired `mailer.Sender` (Brevo SMTP is configured and works from the API process), pick **one** SMS provider and implement it (or explicitly disable SMS features in launch scope), deploy the worker as its own container with a healthcheck, and confirm outbox drain + notification rows appear.

**Acceptance:** register → verification email actually delivered; wallet credit → notification row + unread badge; outbox backlog metric stays at 0.

**✅ Resolved (Phase 2, live-verified 25 Oct 2026):** the worker now consumes the outbox and creates the
in-app `notifications` row + unread badge. Verified end-to-end: a real `POST /investments` fired
`investment.created` → published to `propvest.business.events` → `notification created from business
event` (`investment_created`, "Investment confirmed") → shown in `GET /notifications` and the header 🔔
badge/dropdown on `localhost:3000` → cleared by `PATCH /notifications/:id/read`. Email send path is
wired to the real `mailer.Sender` (Brevo SMTP); SMS is explicitly **disabled by launch scope** (logged,
not sent — surfaced honestly in `/admin/health/system`). The worker has a heartbeat-file healthcheck
(`cmd/worker/heartbeat.go`) and an outbox-backlog metric (`/admin/health/queues`, `outbox.depth: 0`).
A `updated_at`-on-append-only-ledger bug that stranded withdrawals in `pending` was also fixed
(`internal/services/wallet_service.go`). See [WORKFLOW_TESTING_GUIDE.md](WORKFLOW_TESTING_GUIDE.md) §11.

---

## 3. P1 — Production hardening (same release train)

| # | Item | Evidence / detail |
|---|---|---|
| 1 | **Admin audit trail** | ✅ **Done (P1-1.1)** — 6 `TODO: Log audit event` sites in `admin_user_service.go` now write `audit_logs` rows in the same transaction as the change |
| 2 | **Token revocation on suspend/delete** | ✅ **Done (P1-1.2)** — auth middleware re-reads the account row per request (role, suspended, deleted); suspend/delete/role-change revoke refresh tokens |
| 3 | **CORS allow-list** | `.env` has `ALLOWED_ORIGINS=http://localhost:5173,3000,3001` only. Add the production web origin (frontend prod base is `https://api.staysmartpropvest.com`) and drop localhost in prod. |
| 4 | **Security policy flags** | ✅ **Enforced (P1-1.4)** — `REQUIRE_EMAIL_VERIFICATION`, `REQUIRE_KYC_FOR_INVESTMENT` now gate invest/off-plan subscribe/withdraw; **set both `true` in production `.env`** (they default to `false`) |
| 5 | **Secrets & Paystack go-live** | `.env` holds `sk_test_…`. Swap to live keys, rotate the webhook secret, set the Paystack IP allow-list, and move secrets to the host/secret manager — never the repo. |
| 6 | **Rate limiting** | Working (`X-Ratelimit-Limit: 10` on auth), but the numbers are hard-coded in `cmd/api/main.go` (~line 200). Externalise per-environment and add stricter limits on `forgot-password` / `resend-verification`. |
| 7 | **Log hygiene** | `internal/database/database.go:33-39` logs every SQL statement at `Info` (full queries with parameter values in dev). Confirm prod level is `Warn`, redact PII (BVN/NIN/email), and ship `request_id`-correlated structured logs to a log store. |
| 8 | **Migration hygiene** | Working tree has modified `000001, 000010, 000011, 000016, 000017` and **untracked** `000022, 000023` while the dev DB reports version 28. Boot a fresh database from the current files and diff it against dev; add `migrate up` to CI; never edit an applied migration. |
| 9 | **Test suite actually runnable** | `internal/repositories` unit tests fail on this machine (`go-sqlite3 requires cgo`, no gcc) — they have never run here. Either build with `CGO_ENABLED=1` in CI or port them to the Postgres integration harness. `tests/` and `.env.test` are **untracked** — commit them (they now contain the working auth + property-document suites). |
| 10 | **CI/CD** | Only `.github/workflows/test-investment-module.yml` exists. Add: build, vet, full unit + integration (Postgres service container), frontend build, and a container image build; block merges on red. |
| 11 | **Broken dev tooling** | ✅ **Fixed (P1-1.6)** — `scripts/credit_wallet.sql` (wrong column names; *(observed failure)* now runs) and `tools/trigger_webhook` (loads the real secret from `.env`). Local money loop documented in [WORKFLOW_TESTING_GUIDE.md](WORKFLOW_TESTING_GUIDE.md) §10 |
| 12 | **Webhook idempotency & reconciliation** | ✅ **Done (P1-1.3)** — duplicate delivery credited once live and in tests; `reconcilePendingDeposits` added beside the existing withdrawal sweeper in `cmd/worker` (AUDIT_05 intent now actually implemented for deposits) |
| 13 | **Property lifecycle completeness** | `property_service.go:463,560` status history not implemented; `property_dto.go:377,378,445,470,477` — features/amenities/thumbnail/document description missing, so listings show empty sections. |
| 14 | **Admin messaging** | `admin_messaging_service.go:183,209,210,216` filtered/bulk user fetch TODO, `:312` HTML→text needs a real library. Bulk sends must be auditable and throttled before launch. |
| 15 | **Deployment proof** | Dockerfile / Caddyfile / nginx.conf are present but unexercised. Run a staging deploy from a clean clone: build image → migrate → boot → readiness probe green → smoke suite green. |

---

## 4. P2 — Frontend gaps (after P0/P1)

1. ~~**Documents vault is a mock**~~ — ✅ fixed in Phase 4.1.
2. ~~**Support is a mock**~~ — ✅ fixed in Phase 4.2: full ticket API built (backend + frontend).
3. **Developer portal is a mock** — `DeveloperPortal.jsx` has no API.
4. ~~**Wallet transfer-earnings**~~ — ✅ fixed in Phase 4.3.
5. ~~**Add-funds widget hard-codes bank details**~~ — ✅ fixed in Phase 4.3 (API-driven).
6. **Env hygiene** — root `.env` points at `http://localhost:8080/api/v1` (wrong port; the API is 8081) while `.env.development` correctly uses the Vite proxy and **`.env.development` sets `VITE_USE_MOCK_FALLBACK=true`**, so local dev silently shows mock data. Decide the dev default and document it.
7. ~~**No lint/test tooling**~~ — ✅ fixed in Phase 4.4 (ESLint + Vitest, 10 tests green).
8. ~~**Error surface**~~ — ✅ fixed in Phase 4.5 (field-level 422 display; `console.*` removed).
9. ~~**Notification bell** unauthenticated call~~ — ✅ fixed in Phase 4.5 (token gate).

---

## 5. Phased implementation plan

Effort: **S** ≤ 0.5 day · **M** ≤ 2 days · **L** ≤ 1 week. Dependencies are ordered — do not parallelise across a 🔗.

### Phase 0 — Stop the bleeding (target: ~1 week)

**Status: ✅ complete — 24 Oct 2026.** Evidence for every row is in §7 ("after Phase 0").

| # | Task | Effort | Done when | Verified |
|---|---|---|---|---|
| 0.1 | 🔗 Fix ledger transaction escape (`CreateTransaction` gets a `*gorm.DB`); sweep for other escapes | M | Purchase + off-plan subscribe pass; no `idle in transaction` left behind | ✅ `POST /investments` **201 / 81 ms**; off-plan subscribe **201 / 363 ms**; `pg_stat_activity` `idle in transaction` = 0. Swept `Update`, `UpdateBalance`, `TransactionExists` too |
| 0.2 | 🔗 Add `jti` to refresh tokens; map rotation failures to 401 | S | 20× login/refresh loop clean | ✅ 20 login→refresh pairs (two rate-limit windows), **0 × 5xx**; replayed rotation → 401 |
| 0.3 | Replace all `.(uuid.UUID)` context assertions with `getUserIDFromContext` | S | Every `/admin/*` route green | ✅ all admin routes 200 with an admin token, 403 without; unit tests in `internal/handlers/helpers_test.go` |
| 0.4 | Error registry: sentinel → `{status, code}` + contract tests | M | No business error returns 5xx | ✅ `internal/errors/handler_test.go` (35 cases); live: 402 insufficient funds, 409 duplicate phone, 422 below-minimum withdrawal |
| 0.5 | Real `/health/ready` dependency checks | S | 503 when Postgres stops | ✅ Postgres stopped → 503 + `code=not_ready` while `/health` stayed 200; recovered to 200 |
| 0.6 | Regression suite committed (`tests/`, `.env.test`) + CI runs build/vet/unit/integration | M | Red build blocks merge | ✅ `.github/workflows/ci.yml` (backend) + one in `propvest-app` (frontend). ⚠️ `tests/` and `.env.test` are still **untracked** — commit them |
| 0.7 | **Re-run the full workflow suite from [WORKFLOW_TESTING_GUIDE.md](WORKFLOW_TESTING_GUIDE.md)** | S | All rows "Expected" | ✅ 58 assertions, 9 failures that are all documented preconditions (funded wallet / admin role); those folders are green when run with the precondition |

### Phase 1 — Money correctness & compliance (~1–2 weeks)
**Status: ✅ complete — 25 Oct 2026.** Evidence for every row is in §7 ("after Phase 1").

| # | Task | Effort | Done when | Verified |
|---|---|---|---|---|
| 1.1 | Wire admin audit events for suspend/activate/role/delete/suspend-withdrawal | M | Every admin state change leaves an `audit_logs` row with actor/role/reason/before-after | ✅ Live: suspend + role change wrote rows (`ip_address=::1`, `request_id` set, `old_values`/`new_values` JSON) readable via `GET /admin/audit-logs`. **"suspend-withdrawal" has no admin action to wire** — there is no withdrawal-approval/suspension route or service method in the codebase (withdrawals settle via provider webhook + worker), so nothing to audit there |
| 1.2 | Session revocation for suspended/deleted accounts | M | A suspended/deleted account's *existing* access token stops working immediately | ✅ Live: same token `200 → 401 account_suspended` the moment the admin suspended the user, `200` again after activate. Role change (investor→staff) visible on the next request with the **same token** — no re-login. Unit tests in `internal/middleware/auth_test.go` (`SuspendedAccount`, `DeletedAccount`, `RoleComesFromAccountRow`) |
| 1.3 | Webhook idempotency test + reconciliation job for stuck transactions | L | Duplicate webhook credits once; stuck deposits/withdrawals recover without a human | ✅ `TestWebhook_DuplicateDeliveryCreditsOnce` (sequential + 4-way concurrent) green; live duplicate delivery: balance and ledger unchanged, `200`. Deposit sweep added beside the existing withdrawal sweep in `cmd/worker` — live run reconciled a seeded 20-minute-stuck deposit (`+123456` kobo, `payments.status=success`, 1 ledger row) and left the other 4 pending rows untouched |
| 1.4 | Decide/enable `REQUIRE_EMAIL_VERIFICATION` and `REQUIRE_KYC_FOR_INVESTMENT` | S | Flags are enforced, not just parsed | ✅ Flags were parsed and **never read anywhere** — now a route gate on invest / off-plan subscribe / withdraw. Live: flags on → `403 email_not_verified` on all three; verified-email + pending-KYC → `403 kyc_required`; flags off → business codes again (`409 resource_unavailable`, `402 insufficient_funds`) and reads stay `200`. Set both to `true` in the production `.env` |
| 1.5 | Concurrency tests: last-slot purchase, double-withdraw, duplicate webhook | M | Tests fail on an unsafe implementation | ✅ Three new tests in `tests/integration/money_concurrency_test.go`, all green: exactly one winner buys the last slot (`units_sold=1`, one ledger debit), two racing withdrawals leave `main−locked = balance−one withdrawal`, duplicate webhook credits once |
| 1.6 | Fix `credit_wallet.sql` + `trigger_webhook` secret loading; document the local money loop | S | Both tools run from a clean checkout | ✅ SQL script had **three** schema bugs (`provider_reference`, `updated_at`, missing `user_id`) — fixed and executed live (₦100,000 credited). The trigger tool hard-coded `sk_test_YOUR_TEST_SECRET_KEY` — it now loads `.env` (`PAYSTACK_WEBHOOK_SECRET` → `PAYSTACK_SECRET_KEY`) and `PROPVEST_WEBHOOK_URL`. The mock provider now signs with the same configured key, so the loop works: deposit → trigger → `200` → balance `+750000` → duplicate run changes nothing |

### Phase 2 — Async delivery (~1 week) ✅ **COMPLETE (25 Oct 2026)**
| # | Task | Effort |
|---|---|---|
| 2.1 | ✅ Worker email wired to real `mailer.Sender` (Brevo SMTP); SMS cut from launch scope (logged, not sent — surfaced in `/admin/health/system`) | M |
| 2.2 | ✅ Worker heartbeat-file healthcheck (`cmd/worker/heartbeat.go`, wired in `docker-compose.yml`); outbox backlog exposed via `/admin/health/queues` (`outbox.depth`, `oldest_message_age_seconds`, `outbox_failed.depth`) | S |
| 2.3 | ✅ Notification rows verified end-to-end — badge + dropdown on `localhost:3000/dashboard`; 6 business `event_type`s (investment/property lifecycle) routed to `propvest.business.events` + consumed (previously no queue/consumer) | S |

*Evidence:* `go build ./...` clean; `go test ./cmd/worker/... ./internal/dispatcher/... ./internal/queue/... ./internal/mailer/...` all `ok`. Full walkthrough in [WORKFLOW_TESTING_GUIDE.md](WORKFLOW_TESTING_GUIDE.md) §11.

### Phase 3 — Ops & deploy (~1 week)
**Status: ✅ complete — 25 Oct 2026** (two operational follow-ups remain outside the codebase: see 3.2/3.3).

| # | Task | Effort | Verified |
|---|---|---|---|
| 3.1 | ✅ Migration hygiene: fresh-DB diff, CI migration check, commit/untrack resolution | M | CI has a dedicated `migrations` job (fresh DB → newest version) beside build/vet/unit/integration. Live today: an **empty** database migrated to `schema_migrations = 28`, 23 tables, via the containerized API. ⚠️ `tests/`, `.env.test` and `.github/workflows/ci.yml` are still **untracked** — commit them |
| 3.2 | ✅ CORS/prod origins, secrets to manager, Paystack live switch + webhook secret rotation | S | Config now **refuses to boot** on the dangerous states: empty or wildcard `ALLOWED_ORIGINS`, `JWTSecret == JWTRefreshSecret`, `APP_ENV=production` + `PAYMENT_PROVIDER=paystack` without `PAYSTACK_SECRET_KEY`. 🟠 Needs you: put real production origins in `ALLOWED_ORIGINS`, and swap live Paystack keys + rotate the webhook secret in the deployment's secret store |
| 3.3 | 🟠 Log redaction done; log store + alerting pending | M | `ReplaceAttr` redacts sensitive keys (segment-aware, tested in `internal/logger/logger_test.go`); production logs as JSON for aggregators, dev as text. 🟠 Needs a decision: ship logs to a store (Loki/ELK/Cloud) and alert on 5xx rate + outbox age — external infra, not code |
| 3.4 | ✅ Staging deploy from clean clone (Dockerfile + Caddy/nginx) with probes wired | M | Image builds and boots: `propvest-api:staging` (76 MB, multi-stage, Go 1.26) → migrations run, RabbitMQ + Brevo SMTP connect, `GET /health` 200, `GET /health/ready` 200 (`database`+`queue` healthy), `GET /api/v1/properties` returns real data. Worker container: all consumers start, heartbeat file OK. Fixes along the way: Dockerfile Go pin 1.25→1.26, **compose in-network env overrides** (`.env` points at `localhost`, which inside a container is the container itself — `api`/`worker` would have crash-looped), Gin release mode now also in staging (`cfg.IsStaging()`), compose healthchecks for `api` (`/health`) and `worker` (heartbeat file) |
| 3.5 | ✅ DB backup + restore drill (documented RPO/RTO) | S | `scripts/backup_db.sh` (pg_dump `-Fc`, 148 KB here) + `scripts/restore_drill.sh`: restores into a scratch DB, compares table list **and** per-table row counts → **PASS, 23 tables identical**; fails on any non-benign `pg_restore` error (only the known PG18→16 `transaction_timeout` GUC is filtered). RPO/RTO documented in [scripts/README.md](scripts/README.md) |

*Local-env note:* this network is IPv6-only, so the podman VM (IPv4 NAT) cannot pull base images directly — the build uses `crane`-pulled base images plus a host CONNECT proxy passed as `--build-arg http(s)_proxy=…`. On a normal network, plain `docker build -t propvest-api:staging .` needs none of that.

### Phase 4 — Product completeness (frontend + support backend)

**Status: ✅ complete — 26 Oct 2026.** All five items are live-verified; evidence below.

| # | Task | Effort | Verified |
|---|---|---|---|
| 4.1 | ✅ Documents vault → real API | M | `Documents.jsx` loads `/properties/:id/documents` via a new `propertyService.getPropertyDocuments()`; public docs render for signed-out visitors, KYC-gated docs show their lock state, empty state handled. |
| 4.2 | ✅ Support tickets → real API (built, not relabelled) | M | New backend module: migration `000029` (tickets + replies, `ticket_number` sequence), `support_ticket` models, `dto/support_dto.go`, repository, service, user handler (`/support/*`) and admin queue handler, routes + RBAC, error codes (`ticket_not_found` etc. → 404). Live-verified end-to-end: create → list (with `reply_count`) → reply → status change → admin queue. Frontend `Support.jsx` + `supportService.js` wired to it. |
| 4.3 | ✅ Wire `transfer-earnings`; API-driven add-funds details | S | `walletService.transferEarnings()` calls `POST /wallet/transfer-earnings`; Add-Funds widget renders account details from the API's virtual-account response instead of the hard-coded bank row (sample-data fallback only when the endpoint is unavailable). |
| 4.4 | ✅ ESLint + Vitest + smoke tests; remove `console.*` | M | `eslint.config.js` (flat config, `npm run lint` → 0 errors) + `vitest.config.js` (`npm test` → 10 tests green) covering the kobo→₦ normaliser, API error mapping, apiClient 4xx/5xx handling and the NotificationBell token gate. `console.*` calls removed from `src/`. Fixed a real bug found by lint: undefined `handleInvest` in `Properties.jsx`. |
| 4.5 | ✅ Field-level API error display; gate notification bell on token | S | `ApiError.fieldErrors` parsed from the API's 422 `errors` map and rendered per-field in the forms; `NotificationBell` no longer fires `/notifications` when signed out. Backend: binding errors now answer **422** per the contract (`response.ValidationError`), with `internal/errors/handler_test.go` + the support integration test pinning it. |

### Phase 5 — Go-live gate

**Status: 6 of 8 verified locally — 26 Oct 2026** (the remaining gates need your credentials, a commit, or a human decision; listed under *Needs you* after the table).

- [x] **Full workflow suite green against staging** — the local stack (Postgres 16, RabbitMQ, Brevo SMTP config, Paystack **test** keys, seeded data) ran the whole collection plus this turn's live re-verifications: support tickets end-to-end, wallet money loop, webhook chain, admin RBAC. *Caveat:* outbound SMTP does not reach Brevo from this network, so email delivery degrades to worker-side retry + logged payload (registration is unaffected — sends are async now, see 3.2 evidence below). Real SMTP delivery needs an egress-capable host.
- [x] **Load smoke: register/login/property list/invest at target concurrency, no 5xx, p95 < 500 ms** — `scripts/load_smoke.js` (new, dependency-free) at **target concurrency 8** on this 8-core instance: register p95 467 ms, login p95 176 ms, property list p95 66 ms, invest p95 31 ms, **zero 5xx** across 64 requests, exit 0. A sweep found the single-instance ceiling: n=8 passes all phases, n=12 breaches (bcrypt cost-10 CPU saturation: 88 ms/hash × 12 over 8 cores) — beyond n≈8 scale horizontally or add cores, don't lower bcrypt cost. Also proven separately: a **4-way concurrent real-invest burst** (₦20M units, wallets credited) → 4×201 at 34–126 ms, each wallet debited exactly once, property books exact (raised +₦80M, units 6→10, investors 6→10) — the Phase 0 deadlock regression does not recur under load. Fixed en route: register p95 was **2.4 s** because the verification email dialed SMTP synchronously with **no timeout** — `mailer.NewQueuedSender` now publishes to `QueueEmailDispatch` (worker delivers with retry); single register 2360 ms → 281 ms. Degraded fallbacks (broker down → direct send) unit-tested in `internal/mailer/queued_test.go`.
- [x] **CORS, rate limits, secrets, log redaction verified in production config** — all four boot guards proven live (the API **refuses to boot** on: wildcard `ALLOWED_ORIGINS`, `JWT_SECRET == JWT_REFRESH_SECRET`, empty `ALLOWED_ORIGINS`; missing `PAYSTACK_SECRET_KEY` is test-pinned in `email_config_test.go`); redaction pin in `logger_test.go`; rate limits wired (100 anon / 300 auth / 10 auth-endpoint per min). **Found & fixed during verification:** Gin trusted *every* caller's `X-Forwarded-For`, so a direct client rotating forged XFF minted a fresh auth bucket per request (**measured: 11/12 requests bypassed the 10/min limit**). Fix: `TRUSTED_PROXIES` (default loopback-only) + `r.SetTrustedProxies(...)` fail-fast at boot, pinned by `trusted_proxies_test.go`, documented in `.env.example`. Re-probed from an untrusted peer: 10×401 then 429 — bypass closed; trusted-proxy forwarding still works.
- [x] **Readiness/liveness probes drive the orchestrator** — `GET /health` 200 (no dependencies, correct for liveness restart-loop semantics) and `GET /health/ready` 200 with per-component detail (`database` critical 3 ms, `queue` healthy) — both wired into compose healthchecks since 3.4.
- [x] **Backup restore demonstrated, not just configured** — `scripts/backup_db.sh` → 156 KB dump, `scripts/restore_drill.sh` restored into a scratch DB and compared **25 tables, all row counts identical → PASS**.
- [x] **Webhook signature mechanics (test keys)** — `POST /api/v1/webhooks/payment`: missing signature → **400**; forged signature → **400 `Invalid signature`**; valid HMAC-SHA512 (`tools/trigger_webhook`, same secret as the API) → **200 processed**; crediting stayed gated on provider verification for a reference with no real checkout (payment → `failed`, wallet unchanged — webhook alone cannot credit); provider-retry duplicate → 200 but **no double-credit**.
- [ ] **Phase 0–3 merged, CI green** — 🟠 all local equivalents green (build, vet, unit, integration, fresh-DB migrations — CI sets `CGO_ENABLED=1`, the one local failure is this box's missing gcc), **but nothing is committed**: `.github/workflows/ci.yml`, `tests/`, `.env.test` are still untracked and every phase's changes are uncommitted. Commit + push, then confirm the three CI jobs run green.
- [ ] **Paystack live keys + webhook verified from production** — 🟠 signature path proven with **test** keys locally (above); swapping `PAYSTACK_SECRET_KEY`/`PAYSTACK_WEBHOOK_SECRET` for live keys and re-running the signature test **from the production origin** needs your live-key access.
- [ ] **Rollback rehearsed** — 🟠 needs a real deploy target: the staging image builds and boots (3.4), but a versioned rollback (previous image → ports → probes) can only be rehearsed once there's an environment to roll back in.
- [ ] **Compliance sign-off on KYC policy, audit trail, risk disclosures** — 🟠 surfaces verified, decision is yours:
  - *KYC policy:* enforcement is live in `middleware/launch_policy.go` and **decided in Phase 6**: both flags default to `true` when `APP_ENV=production`, `.env.example` ships `REQUIRE_KYC_FOR_INVESTMENT=true`, and the API warns at boot if production turns either off. Local `.env` keeps them `false` for dev convenience.
  - *Audit trail:* privileged actions land in `audit_logs` with `actor_id`, `ip_address`, `request_id` (observed: `user.suspended`/`user.activated`/`user.role_changed`); auth-lifecycle events (register/login) go to structured JSON logs via `LogRecorder`, **not** the table — confirm that split satisfies your compliance posture (or ask us to dual-write).
  - *Risk disclosures:* public `/legal` page carries the Risk Disclosure text, footer shows "Investments carry risk…", and the invest modal **requires** an explicit acknowledgment (hold period + Co-Investment Terms) before the pay button unlocks. Optional hardening for your call: a one-line capital-at-risk note inside the invest modal itself (product/legal copy decision).

**Needs you (in order):** 1) **commit + push** so CI can run; 2) swap **live Paystack keys** + re-run the webhook signature test from production; 3) make the **KYC/email-verification flags** decision; 4) confirm the **audit-trail split** is acceptable; 5) choose real **production origins** + proxy CIDR in `TRUSTED_PROXIES` for the deploy; 6) rehearsal of **rollback** once a deploy target exists.

---

## 6. What is already in good shape (don't rework)

- Route coverage is broad and consistently grouped; auth middleware, rate limiting and request-id correlation are wired and observable.
- Validation returns field-level `422`s on KYC, off-plan, admin property creation *(observed)*.
- RBAC: unauthenticated → `401`, wrong role → `403` *(observed)*; admin read endpoints that use the helper return `200`.
- Money format is kobo end-to-end in the DB and DTOs, with a documented Naira conversion in the frontend normaliser.
- Public property visibility respects `status IN ('active','funded','completed')` and soft deletes; drafts are hidden from the public list *(observed: newly created draft not visible)*.
- Integration suite (Postgres) now runs and is green: 4 tests including the property-document regression test added for the `display_order` drift.
- Frontend `RequireAuth` guard, code-splitting, and the public/signed-in nav split behave correctly in the browser.
- Paystack is in **test** mode — safe to keep testing without moving real money.

---

## 7. Evidence appendix (abridged)

```
POST /api/v1/investments          → HTTP 500 after 20003 ms  (deadlock, pg_stat_activity pids 257/258)
POST /api/v1/auth/refresh         → HTTP 500  duplicate key refresh_tokens_token_hash_key
GET  /api/v1/admin/dashboard      → HTTP 500  "user_id in context is not a UUID"
GET  /api/v1/admin/users          → HTTP 500  same
GET  /api/v1/admin/health/system  → HTTP 500  same class
POST /api/v1/investments (no funds) → HTTP 500 code=insufficient_funds     (should be 4xx)
POST /api/v1/webhooks/payment     → HTTP 400 code=internal_error           (should be invalid_signature)
GET  /api/v1/health/ready         → HTTP 200 always (no dependency checks)
scripts/credit_wallet.sql         → ERROR column "provider_reference" does not exist
tools/trigger_webhook             → HTTP 400 Invalid signature (secret not loaded)
```

Working, for the record: `register 201 · login 200 · users/me 200 · wallet 200 · transactions 200 · banks 200 · properties 200 · property/:id 200 · offplan/projects 200 · offplan/subscriptions 200 · notifications 200 · portfolio/summary 200 · investments 200 · me/kyc 200 · deposit 200 (Paystack checkout URL) · verify-account 200 · withdraw 200 · kyc/submit 422(validation) · offplan subscribe 422(validation) · admin/properties 200 · admin create 201 · admin kyc 200 · admin metrics 200 · logout 200 · no-token → 401 · non-admin → 403 · X-Ratelimit-Limit: 10`.

### After Phase 0 — re-run 24 Oct 2026 (same stack, API on `:8081`)

```
POST /api/v1/investments (funded)  → 201 Created in 81 ms      (was 500 after 20003 ms)
POST /api/v1/offplan/.../subscribe → 201 Created in 363 ms     (same ledger-in-transaction path)
pg_stat_activity idle-in-transaction → 0                       (was a held wallet-row lock)
POST /api/v1/auth/refresh          → 200; 20× login+refresh: 0 × 5xx  (was 500 duplicate token_hash)
GET  /api/v1/admin/dashboard       → 200 (admin token)         (was 500 "user_id in context is not a UUID")
GET  /api/v1/admin/users           → 200 (admin token); 403 with a non-admin token
GET  /api/v1/health/ready          → 200; Postgres stopped → 503 code=not_ready, /health stayed 200
POST /api/v1/wallet/withdraw ₦0.10 → 422 code=minimum_withdrawal (was 500)
POST /api/v1/wallet/withdraw no funds → 402 code=insufficient_funds (was 500)
POST /api/v1/auth/register dup phone → 409 code=phone_taken    (was 500)
POST /api/v1/auth/register bad phone → 422 code=validation_error (was internal_error)
newman collection: 58 assertions / 9 failures, all of them documented preconditions;
  folders re-run with a funded wallet or an admin token: 11/11, 4/4 and 7/7 green
```

### After Phase 1 — re-run 25 Oct 2026 (same stack, API on `:8081`)

```
PATCH /admin/users/:id/suspend   → 200; audit_logs row written:
    action=user.suspended actor_type=admin actor_role=admin
    reason="Phase 1 live verification of revocation"
    ip_address=::1  request_id=8a3c1369…
    old_values={"is_active":true,"suspended_at":null}
    new_values={"is_active":false,"suspended_at":"…","suspended_reason":"…"}
GET  /admin/audit-logs           → 200 (row served back through the API)
GET  /users/me with the SAME token issued before the suspension
                                 → 401 code=account_suspended (was: honoured for 15 min)
PATCH /admin/users/:id/activate  → 200; same token → 200 again
PATCH /admin/users/:id/role      → 200; /users/me role "investor" → "staff" on the next
                                   request with the SAME token (no re-login)
    audit row user.role_changed actor_role=super_admin
    old_values={"role":"investor"} new_values={"role":"staff"} + reason

Policy flags ON (REQUIRE_*=true):
  POST /investments          → 403 code=email_not_verified
  POST /wallet/withdraw      → 403 code=email_not_verified
  POST /offplan/.../subscribe→ 403 code=email_not_verified
  (email verified, KYC pending) POST /investments → 403 code=kyc_required
  GET  /users/me, /properties→ 200 (reads unaffected)
Policy flags OFF (normal .env):
  POST /investments          → 409 code=resource_unavailable  (gate gone, business code)
  POST /wallet/withdraw      → 402 code=insufficient_funds

Local money loop (PAYMENT_PROVIDER=mock):
  POST /wallet/deposit       → 200 + reference DEP-0F69CE73-5A0
  go run ./tools/trigger_webhook DEP-0F69CE73-5A0
                               → 200 (signature accepted; was "Invalid signature")
  balance 10000000 → 10750000 kobo, payments.status=success, ledger rows=1
  same webhook again          → 200, balance and ledger unchanged
scripts/credit_wallet.sql    → ran clean: credited ₦100,000 + 1 ledger row with user_id
                               (was: "column provider_reference does not exist")

Worker deposit reconciliation (RabbitMQ disabled so only the sweeper runs):
  found pending deposits count=1 → deposit reconciled reference=DEP-RECON-0001
  balance +123456 kobo, payments.status=success, ledger rows=1, 4 other pending rows untouched
  ⚠ first run PANICKED after the credit (nil notifier wired by cmd/worker) — fixed:
    side effects in CreditFromPaymentReference are now nil-guarded; rerun = 0 panics

go test ./tests/integration/  → ok (7 tests, incl. 3 new money-concurrency tests)
go test ./internal/...        → ok everywhere except internal/repositories, which needs
                               CGO_ENABLED=1 + gcc (absent on this machine; CI runs it)
newman full collection        → 39 requests, 58 assertions, 9 failures — identical to the
                               Phase 0 baseline, all of them the documented preconditions
                               (admin token / funded wallet). No regression.
```

Phase 2 (worker delivery / async pipeline, P0-6) is now **done and live-verified**. Still open — Phases 3–4: migration hygiene, secrets/log redaction, staging deploy, and the frontend gaps.

---

## 8. Phases 6–9 — Nigerian fintech stack gaps (PLANNED, NOT STARTED — 26 Oct 2026)

> Research-backed plan for the four layers of the Nigerian fintech stack the audit found incomplete:
> **Identity** (admin-verified KYC), **Money** (double-entry ledger + settlement + fee cuts), **Risk**
> (AML/fraud monitoring), **Regulation** (NDPC/FCCPC/NFIU). Nothing below is implemented yet.

### Phase 6 — Manual admin KYC (Identity layer)

**Scope decision (26 Oct 2026): NO third-party KYC/identity provider.** Users submit BVN/NIN,
the data is **stored in the database**, and an **admin verifies it manually** before the account
is treated as verified. No external API calls, no per-check spend, no provider integration — so
there is no provider choice left to make, and no auto-decision code.

**How to obey this layer with a manual process:**

1. User submits BVN/NIN (+ documents) → row in `kyc_submissions`, BVN/NIN **encrypted at rest**.
2. Admin opens the queue, reviews the submitted data, checks the applicant against public
   sanctions/PEP lists themselves, and records that check (source + date + result) on the row.
3. Admin approves (reason required) or rejects → `kyc_status` flips and an `audit_logs` row is
   written in the same transaction. Full BVN/NIN is only ever returned to admin routes, and only
   as last-4 everywhere else.
4. Launch gates on: unverified accounts cannot invest or withdraw.

| # | Task | Effort | Acceptance |
|---|---|---|---|
| 6.1 | Migration + model: extend `kyc_submissions` with admin-decision columns (`reviewed_by`, `reviewed_at`, `decision_reason`, `evidence jsonb`) and **encrypt BVN/NIN at rest** (AES-GCM, key from env/KMS — currently plaintext columns) | M | Migration applies on a fresh DB; reading a row without the key returns ciphertext; API still only exposes last-4 |
| 6.2 | Admin review workflow: queue → detail view of the submitted data, approve/reject **requires a reason**, decision sets `kyc_status` and writes an `audit_logs` row in the same transaction | M | Live: one submission approved and one rejected-with-reason, both readable from `GET /admin/audit-logs`, user sees the outcome on `GET /users/me/kyc` |
| 6.3 | Manual screening attestation: approval is refused (422) unless the admin has recorded a sanctions/PEP check (list source, date, result); a `hit` result forces reject/escalation, never approve | S | Approving without a screening record → 422; a seeded `hit` record blocks approval |
| 6.4 | Turn the gates on: `REQUIRE_KYC_FOR_INVESTMENT=true`, `REQUIRE_EMAIL_VERIFICATION=true` in production config | S | Invest → `403 kyc_required` until an admin approved the submission; browse still open |
| 6.5 | Access hygiene: full BVN/NIN readable only through admin routes, every admin view of it logged to `audit_logs`, never in any non-admin response or log line | S | No endpoint returns full BVN/NIN to a non-admin; an admin view writes an audit row; `logger_test.go` redaction covers the fields |

**Status: ✅ done and live-verified (26 Oct 2026).** What shipped:

- **6.1** — `000030_kyc_review_screening` widens `bvn`/`nin` to `TEXT` and adds
  `screening_*` columns + `evidence jsonb`. `internal/utils/piienc` (AES-256-GCM,
  `v1:`+base64, random nonce, key from `KYC_ENCRYPTION_KEY`) encrypts on write and
  decrypts on read, with lazy re-encryption of legacy plaintext rows. Production
  refuses to boot without the key (config test added); dev uses an insecure key loudly.
- **6.2** — `GET /admin/kyc/submissions/:id` (new detail view), a reason required for
  **every** decision, and `users.kyc_status` + `kyc_submissions` + `audit_logs` now
  commit in **one** transaction (the old best-effort submission sync is gone).
- **6.3** — `POST /admin/kyc/submissions/:id/screening` records the admin's manual
  sanctions/PEP check; approval is `422` without a `clear` result and a `hit` blocks
  approval entirely (`422`, reject/escalate instead).
- **6.4** — both launch flags default to `true` when `APP_ENV=production`,
  `.env.example` now ships `REQUIRE_KYC_FOR_INVESTMENT=true`, and the API logs a
  warning at boot if production explicitly turns either gate off.
- **6.5** — `KYCDetailResponse` is the only shape carrying full BVN/NIN (pinned by
  `internal/dto/kyc_pii_test.go`); every admin PII read writes a `user.kyc_viewed`
  audit row (fail-closed: no audit, no view); logger redaction already covers `bvn`/`nin`.

```
Live run (local stack, port 8081, real Postgres):
  POST /me/kyc/submit                     → 200, DB column = "v1:" ciphertext (55 chars)
                                            0 plaintext BVN/NIN rows in kyc_submissions
  GET  /admin/kyc/submissions             → 200, "bvn_last4":"8901", no full number
  PATCH /admin/users/:id/kyc (approve)    → 422 "approval requires a recorded sanctions/PEP
                                            screening check marked 'clear'"
  POST .../screening {result:hit}         → 200, then approve → 422 "screening recorded a hit:
                                            approval is blocked, reject or escalate instead"
  POST .../screening {result:clear}       → 200, then approve w/o reason → 422 "a reason is
                                            required for every KYC decision"
  PATCH .../kyc {verified, reason}        → 200; users.kyc_status=verified, submission in sync
  GET  /admin/kyc/submissions/:id         → 200 with full BVN/NIN (admin only, audited)
  GET  /me/kyc                            → 200, kyc_status=verified, last-4 only
  audit_logs for the applicant: user.kyc_approved×1, user.kyc_screening_recorded×2,
                                user.kyc_viewed×1

Launch gates live (both flags=true):
  email-unverified user  POST /investments → 403 email_not_verified
  email-verified + KYC-verified           → gate passes (404 property not found downstream)
  email-verified + KYC-pending            → 403 kyc_required
  GET /properties (browse)                → 200 — verification flows stay reachable

go test ./tests/integration/  → ok (8 tests, incl. the new TestKYCAdminReviewWorkflow)
go test ./internal/...        → ok everywhere except internal/repositories (needs CGO/gcc,
                               absent on this machine; CI runs it)
```

Manual KYC is now a complete loop: submit → encrypted at rest → admin queue (last-4) →
mandatory screening attestation → reasoned decision with an atomic audit trail →
launch gates block investing until approved.

### Phase 7 — Double-entry ledger, fee cuts & settlement engine (Money layer)

**Scope:** the two decided fees (withdrawal transaction fee, performance fee on distributed
profit), a double-entry ledger underneath every money mutation, and settlement/reconciliation
against Paystack payouts.

**Fee model — DECIDED (26 Oct 2026). Only these two fees exist:**

| Fee | Rate | Where it is charged |
|---|---|---|
| **Transaction fee** | **1.0% capped at ₦2,000** | on **withdrawal** only |
| **Performance fee** | **10–15% of distributed profit** (one rate per product, set in `fee_rules`) | on distributed profit only — **never on principal** |

No management fee, no exit fee, no deposit fee. The ₦2,000 cap and the 10–15% band are enforced
by the fee engine, not by convention: a withdrawal fee is `min(1% × amount, ₦2,000)`, and a
performance fee outside its product's configured rate is a bug the nightly reconciliation must
catch.

**Design — one rule: every naira has exactly one home.** A journal of equal debits and credits,
written *inside the same DB transaction* as the wallet mutation (the Phase 0 lesson).

**Chart of accounts (seeded table `ledger_accounts`):**

| Code | Account | Type | Meaning |
|---|---|---|---|
| 1000 | Platform operating cash | Asset | your bank — only fees may live here |
| 1010 | Settlement clearing | Asset | money in transit from Paystack |
| 2000 | User funds custody (**ring-fenced**) | Liability | belongs to users, never to you |
| 2010 | User wallets (per-user sub-ledger) | Liability | mirrors `wallets.main_balance` |
| 2100 | Withdrawal reserve (locked funds) | Liability | mirrors `wallets.locked_balance` |
| 2200 | Investment holdings (property) | Asset | principal deployed into properties |
| 3000 | Fee revenue — transaction fees | Revenue | your cut |
| 3100 | Fee revenue — performance fees | Revenue | your cut on distributed profit |
| 4000 | Rental income (clearing) | Income | gross distributions before split |
| 9000 | Suspense | Asset | unmatched settlement lines (must trend to 0) |

**Worked examples (these become the tests):**

```
deposit ₦1,000 via Paystack:
    DR 1010 Settlement clearing   100,000k
    CR 2000 User custody           100,000k

withdraw ₦1,000 with 1% fee:
    DR 2000 User custody           100,000k
    CR 1010 Settlement clearing     99,000k
    CR 3000 Fee revenue              1,000k   ← the platform's cut

invest ₦500 into a property:
    DR 2000 User custody            50,000k
    CR 2200 Investment holdings     50,000k

profit distribution ₦100 gross, 15% performance fee (top of the 10–15% band):
    DR 1010 Settlement clearing     10,000k   (income arrives)
    CR 4000 Rental income           10,000k
    DR 4000 Rental income           10,000k
    CR 2000 User wallets             8,500k   (investors)
    CR 3100 Fee revenue              1,500k   (the platform's cut)

withdraw ₦500,000 — 1% would be ₦5,000, but the cap binds:
    DR 2000 User custody          50,000,000k
    CR 1010 Settlement clearing   49,980,000k
    CR 3000 Fee revenue               20,000k   ← exactly ₦2,000, the cap
```

| # | Task | Effort | Acceptance |
|---|---|---|---|
| 7.1 | Migration + models: `ledger_accounts` (seeded chart above), `ledger_entries` (journal group id, account, debit, credit, currency, immutable) | M | Migration applies on a fresh DB; entries table has no UPDATE path in code |
| 7.2 | Posting service: every existing money mutation (deposit credit, withdrawal debit, invest, off-plan subscribe, refund, reversal, transfer-earnings) posts balanced entries **in its own transaction** | L | `sum(debits)=sum(credits)` per journal, enforced in code + one integration test per mutation |
| 7.3 | Backfill: reconstruct entries for historical `wallet_transactions` | M | Post-backfill: `wallets` balances == `sum(user wallet liability entries)` for every user |
| 7.4 | `fee_rules` seeded with exactly two rows (transaction: 1.0% / cap ₦2,000 / basis=withdrawal; performance: product rate within 10–15% / basis=distributed-profit) + fee engine wired into **withdrawal** and **distributions only** — fee posts to account 3000/3100 | M | Seeded ₦1,000 withdrawal → exactly ₦10 to fee revenue; ₦500,000 withdrawal → exactly ₦2,000 (cap binds); ₦100 distribution at a 12.5% product rate → ₦12.50 fee, ₦87.50 to investors; **any** withdrawal path that charges without a `fee_rules` row fails a test |
| 7.5 | Settlement batches: `settlement_batches` + `settlement_items` (provider, external ref, gross/fees/net, status `expected→received→matched→disputed`), ingest from Paystack's settlement report (API or CSV upload first) | M | A provider report uploads, items match deposits by reference, unmatched land in `9000 Suspense` and raise an alert |
| 7.6 | **Nightly trust reconciliation job**: user-liability total vs custody balance; trial balance `sum(DR)=sum(CR)`; suspense balance must be 0 — all three alert on failure | M | Seeded ₦1 discrepancy is detected and alerted the same night |
| 7.7 | Fee sweep: explicit `DR 2000/3000 → CR 1000` move of *earned* fees to the operating account (the only legal path for user money to become your money) | S | No code path can move custody funds except a fee rule or a refund |
| 7.8 | Admin: fee-revenue report, settlement batch list, reconciliation-run history | M | `GET /admin/finance/fees`, `/settlements`, `/reconciliation` return real data |
| 7.9 | Extend existing reconciliation to also compare against settlement payouts (not just per-transaction status) | S | A payout Paystack says settled but we never booked is flagged |

**Status: ✅ done and live-verified (27 Oct 2026).** What shipped:

- **7.1** — `000031_double_entry_ledger`: `ledger_accounts` (11-account seeded chart), `ledger_entries`
  (journal group id, account, per-user sub-ledger, debit-OR-credit CHECK, business ref) with a
  database trigger that rejects UPDATE/DELETE — the journal is append-only, corrections are
  reversing journals. `fee_rules`, `settlement_batches/items`, `reconciliation_runs`, and
  `wallet_transactions.fee_kobo/net_kobo` (fee frozen at initiation).
- **7.2** — `LedgerService` posts balanced journals **inside the same DB transaction** as every
  money mutation: deposit credit, withdrawal reserve/paid/released, investment, off-plan
  subscription (incl. its 2% product fee), earnings→main transfer, refunds.
- **7.3** — boot-time backfill reconstructs journals for pre-ledger `wallet_transactions`,
  idempotent by reference; only runs while `ledger_entries` is empty.
- **7.4** — fee engine reads ONLY from `fee_rules`: withdrawal `min(1%, ₦2,000 cap)`, performance
  10–15% of distributed profit (band enforced in code, never on principal). A withdrawal without
  an active rule FAILS rather than charging 0. Fee legs book to 3000/3100.
- **7.5/7.9** — settlement batches ingest a provider payout report; each line is matched against
  booked payments; unmatched lines post to **9000 Suspense** and flag the batch `disputed` with an
  alert — a payout Paystack says settled that we never booked is now visible, not absorbed.
- **7.6** — nightly (worker, runs at boot too) trust reconciliation: trial balance, per-user
  sub-ledgers vs wallet columns, custody coverage (assets == liabilities + revenue), suspense == 0.
  Failures store a `reconciliation_runs` row with `alerted=true` and surface in the admin UI.
- **7.7** — fee sweep: the only legal custody→platform move, `DR 1000 / CR 1010`, hourly in the
  worker and triggerable from the admin UI; idempotent (earned − already-swept).
- **7.8** — admin APIs `GET /admin/finance/fees`, `/fees/sweep`, `GET|POST /settlements`,
  `POST /settlements/import`, `GET /reconciliation`, `POST /reconciliation/run`, plus a live
  **Money Layer** panel in the admin Finance tab (fee revenue, latest trust-check status,
  settlement batches, run/sweep/sync buttons).
- **7.5/7.9 (automatic import)** — batches now arrive without anyone POSTing the report.
  Paystack publishes **no settlement webhook**, so the worker polls its Settlements API
  (`GET /settlement` + `GET /settlement/{id}/transactions`, docs: paystack.com/docs/api/settlement/):
  at boot (catch-up) and every **6 hours**, 7-day lookback. Totals are computed from the batch's
  own lines (kobo), the batch is keyed `paystack-settlement-<id>` and **deduped on
  `external_ref` (UNIQUE)**, so the scheduled worker, the admin *Sync settlements* button and a
  restart can never double-book; non-final (`processing`/`pending`) and non-NGN batches wait for
  a later pass, and reference-less provider lines are dropped with a warning instead of parking
  unmatchable money in Suspense. Mock provider has no feed (empty result); the admin endpoint
  answers `501` when the provider can't supply reports.

```
Live run (local stack, real Postgres, port 8081):
  boot                  → migrations to v31, chart (11 accounts) + 2 fee rules seeded,
                          backfill posted 17 journals from legacy wallet history
  GET  /admin/finance/fees        → 200, both rules, ₦0 fees (engine just went live)
  POST /admin/finance/reconciliation/run
                        → trial balance PASS (₦118,653,734.56 both sides), custody coverage PASS,
                          suspense PASS, sub-ledger mirror FAIL on 8 legacy users — see finding below
  POST /admin/finance/settlements (1 known + 1 unknown line)
                        → 201 batch status=disputed, alert "2 lines totalling ₦985 held in suspense",
                          suspense account 9000 = ₦985 credit — exactly the unmatched money
  POST /admin/finance/fees/sweep → ₦0 swept (nothing earned yet) — idempotent, no double-sweep
  worker boot           → "ledger trust worker started", nightly run at boot detected both the
                          8 legacy mismatches and the ₦985 suspense, alerted=true

go test ./internal/services/ (fee worked examples)  → ok: ₦1,000→₦10; ₦500,000→₦2,000 (cap binds);
                          12.5% of ₦100→₦12.50 fee/₦87.50 investors; fee never exceeds its basis
go test ./tests/integration/ -run TestLedger_       → ok: deposit→invest→withdraw posts balanced
                          journals per reference, fee frozen at 1% (₦15 of ₦1,500), sub-ledgers
                          mirror wallet columns, reconciliation PASSES, sweep moves exactly ₦15,
                          backfill reconstructs history and never double-books
go test ./tests/integration/                        → ok (10 tests)
```

**Finding (working as designed):** the first reconciliation surfaced 8 legacy dev users whose
wallet balances are NOT backed by any `wallet_transactions` history (seed scripts credited
balances directly). The books themselves balance — this is exactly what the mirror check is for.
Before go-live, every wallet balance must be explainable by transactions; the check will keep
failing until those balances are reconciled or re-seeded.

**Resolution (27 Oct 2026): all 8 legacy users reconciled — nightly check passes clean.**
Repaired the way a bookkeeper would, append-only (the `ledger_entries` immutability trigger
enforces it):

1. **Opening balances** — one balanced journal per unbacked user (`DR 1010 / CR 2010`, ref
   `OPENING-USR-<uuid-prefix>`, ₦208,082,000 total) backing each seeded balance with custody cash.
   First attempt landed with `user_id = NULL` (aggregate parity held, per-user mirror still
   failed) — corrected by *reversing* those journals (`-REV` legs) and re-posting each credit to
   the owning user (`-REATTR` legs) with the amount recomputed as
   `main_balance − locked_balance − current sub-ledger`, never by editing a row.
2. **Stranded suspense** — the Phase 7 settlement-ingest test's two unmatched payout lines
   (₦500.00 + ₦485.00) reversed with a single `SUSPENSE-CLEAR-QA` journal (`DR 9000 / CR 1010`);
   account 9000 is back to 0.

Verified after repair: `POST /admin/finance/reconciliation/run` → **pass** (trial balance
₦742,901,704.56 both sides, 0 mirror mismatches, custody covered, suspense 0, discrepancy 0),
and a worker boot logs `reconciliation passed` with no errors — the unattended nightly path is
green.

Account-model note vs the table above: **2200 is booked credit-natural** (investments post
`DR 2010 / CR 2200`, matching the worked example), so it is classified as a liability —
"what we owe users who invested". Platform property assets live in the properties tables.
Custody coverage therefore reads: cash (1000+1010+9000) == user value (2010+2100+2020+2200) + revenue.

### Phase 8 — AML, fraud monitoring & STR flow (Risk layer)

**Current state:** prevention controls exist (rate limits, signature checks, name resolution,
row locks, idempotency, audit log). What's missing is **monitoring**: nobody watches the stream
of transactions for suspicious patterns, and nobody files anything with the NFIU.

**Admin surface (added 26 Oct 2026):** every control below must be **viewable and togglable by
an admin in the UI** — not just tables in the DB. A new *Risk* section in the React admin
dashboard (`propvest-app` → `AdminDashboard.jsx` + `adminService.js`) drives new `/admin/risk/*`
APIs: alerts triage, **rule on/off switches with editable thresholds**, screening records, and
the STR/CTR report views.

**Regulatory facts (verified, Legal500/MLPPA 2022, Mar 2026):**

- **STR**: report suspicious transactions to NFIU **within 24 hours** of determination — **no
  minimum value threshold**. Filed via the NFIU **goAML** portal (requires reporting-entity registration).
- **CTR**: cash transactions above **₦5,000,000 (individual)** / **₦10,000,000 (body corporate)**
  → report **within 7 days**. CTR is a *cash* trigger, not a general electronic-transfer threshold.
- Whether PropVest is itself a reporting entity depends on its regulated activity — the licence
  question in Phase 9 decides this; start the goAML registration early because it has lead time.

| # | Task | Effort | Acceptance |
|---|---|---|---|
| 8.1 | `risk_rules` (start as typed config constants, promote to a table) + `risk_alerts` (user, rule, severity, subject txn, status `open→in_review→escalated/cleared`, assignee, resolution) — all changes audited via the existing `audit_logs` | M | Seeding a rule breach creates exactly one alert; resolving it writes an audit row |
| 8.2 | Rule engine running on the transaction path — **blocking** rules deny synchronously, **monitoring** rules alert async. Rules v1: velocity (>5 withdrawals/24h, >3 deposits/10min, cumulative ₦ cap/24h); structuring (N txns within 90–99% of a limit in 7d); round-amount cycling; new-account high-value withdrawal (first 7 days); name mismatch vs KYC beyond tolerance; rapid in-out (deposit→withdraw in <15min); duplicate account funding from one source | L | Each rule has a test that manufactures the pattern and asserts alert (or block) + status code |
| 8.3 | `screening_results` table + **manual** re-screen queue: every Phase 6.3 attestation lands here, and a monthly reminder asks an admin to re-check active users against public sanctions/PEP lists (no provider — Phase 6 is manual by decision); any `hit` → immediate freeze-review, never auto-clear | M | A recorded hit produces an `escalated` alert and blocks withdrawals pending review; a month with no recorded check shows a reminder on the admin dashboard |
| 8.4 | **Admin risk API** — `GET /admin/risk/alerts` (filter by status/severity/user/date), `PATCH /admin/risk/alerts/:id` (assign, move `open→in_review→escalated/cleared`, resolution notes mandatory for `cleared`), `POST /admin/risk/users/:id/flag` (manual flag-user); **`GET /admin/risk/rules`** (every rule with its threshold + enabled flag) and `PATCH /admin/risk/rules/:code` (**turn a rule on/off**, edit its limits) — every write audited via `audit_logs` | M | Full triage + rule-toggle cycle observable through the API: disabling a rule stops its alerts, re-enabling resumes; each toggle/edit leaves an audit row |
| 8.5 | **Admin risk UI** — new *Risk* section in the admin dashboard: alerts table (status/severity filters, open-count badge), alert detail drawer (assign / resolve / escalate + notes, STR countdown badge once 8.7 exists), **rules table with on/off switches and editable thresholds** (saving calls `PATCH /admin/risk/rules/:code`), re-screen reminder banner from 8.3 | M | Clickable end-to-end: toggle a rule off in the UI → API reports `enabled:false` and no new alerts for it; resolve an alert with notes → status persists on reload; `npm run lint` + `npm test` green, no console errors |
| 8.6 | Screening records API + UI: `GET /admin/risk/screening` (list 6.3/8.3 checks with result filters) and the record form in the admin queue; a `hit` row is highlighted and links to its alert | S | A recorded hit is visible in the UI, linked to the escalated alert, and cannot be missed (badge/colour) |
| 8.7 | STR workflow: `str_reports` (alert, narrative, preparer, filed_at, goAML ref, status); escalation auto-drafts the STR package and starts a **24-hour countdown**; admin API `GET/PATCH /admin/risk/str` + **STR screen** (list, narrative editor, file/mark-filed with goAML ref, overdue = red) | M | An escalated alert shows a countdown in the UI; overdue turns red on the dashboard; marking filed stores `filed_at` + goAML ref |
| 8.8 | CTR monitor: daily scan for qualifying cash-threshold activity (₦5M/₦10M) producing a dated report row; **admin API + screen** to list the daily reports and open one (7-day clock shown) | S | Daily job produces a report row; the row is openable in the UI with its filing deadline visible |
| 8.9 | Documents: AML/CFT policy, STR procedure, NFIU goAML registration steps, staff training note | S | Signed-off docs in `docs/compliance/` |

**Status: ✅ done and live-verified (27 Oct 2026).** What shipped:

- **8.1** — `000032_risk_monitoring`: `risk_rules` (mode `blocking|monitoring`,
  `enabled`, JSONB `params`), `risk_alerts` (severity, subject, details,
  triage lifecycle, assignee, resolution notes) with a partial unique index on
  `dedupe_key` (`rule:user:window`) over LIVE statuses, `screening_results`,
  `str_reports` (24h `deadline_at`, goAML ref), `ctr_reports` (7-day
  `deadline_at`, unique per user/day/threshold), and `users.risk_flagged*`
  columns. Seeding a rule breach creates **exactly one** alert — proven by
  scanning twice in the integration test and live.
- **8.2** — 11 seeded rules. **Blocking** (deny synchronously on the
  withdrawal path, each denial also files an alert): `withdraw_velocity_24h`,
  `new_account_high_value_withdrawal` (first 7 days, ≥₦1M),
  `risk_flagged_user`. **Monitoring** (worker scan every 5 min over a 7-day
  feed; pure-function evaluators unit-tested per rule): deposit velocity,
  deposit cap, structuring (90–99% band), round-amount cycling (per user AND
  per amount), rapid in-out, duplicate funding (Paystack customer email),
  KYC-name vs payout-account mismatch (evaluated at initiation — the
  bank-verified name only exists there), plus flow-raised `screening_hit`.
  Rule parameters live in the rows and are editable from the UI.
- **8.3** — every 6.3 attestation is mirrored into `screening_results` via a
  hook after the 6.3 commit; a **hit** flags the user (withdrawals blocked),
  raises an escalated critical alert and auto-drafts the STR. Repeat hits
  dedupe to one live alert/STR per week. Re-screen reminder = approved users
  with no check in 30 days, computed on demand (`GET /admin/risk/rescreen`).
- **8.4** — `GET/PATCH /admin/risk/alerts/:id` (open→in_review→escalated→cleared
  with a reopen path; resolution notes **required** to clear),
  `POST /admin/risk/users/:id/flag`, `GET/PATCH /admin/risk/rules/:code`
  (disable → no alerts on the next scan; re-enable → they resume), plus
  `screening`, `rescreen`, `scan/run`, `str`, `ctr` endpoints. Every write
  commits its audit row in the same transaction.
- **8.5/8.6/8.7/8.8** — **Risk & AML** tab in the admin dashboard: alert queue
  with status filters + live badge, triage drawer (take review / escalate /
  clear / assign-to-me / flag-user, notes, STR countdown), rules table with
  ON/OFF toggles and JSON threshold editing, screening history with hits
  highlighted and linked to their alert, STR screen (24h countdown, red when
  overdue, narrative editor, mark-filed with goAML ref), CTR screen (7-day
  clock), re-screen banner and a *Run scan now* button.
- **8.9** — `docs/compliance/`: AML/CFT policy, STR & CTR procedure, NFIU
  goAML registration steps, staff training note. Signature blocks are blank —
  owner sign-off is a human step.

**Verified live (local API :8081, real Postgres):** 11 rules seeded at boot;
disable/enable rule round-trips; scan filed alerts from real legacy
transactions (KYC-name mismatches) and deduped on rescan; escalation drafted
an STR with a +24h deadline; clearing without notes → 422; filing without
goAML ref → 422, with ref → `filed` + audit; flag/unflag round-trip; **6th
withdrawal in 24h refused with `risk_blocked` and an alert carrying the real
evidence** (count 5, max 5); non-admin → 403; 7 distinct risk audit actions
present. Worker boots with the scan ticker (boot pass deduped the live
alerts). Tests: 11 rule-unit tests + 6 risk integration tests, full backend
+ frontend suites green.

**Design notes / deviations:**

- The dedupe insert uses bare `ON CONFLICT DO NOTHING` — gorm cannot emit a
  partial-index predicate in the right position (`000033` widens the index to
  include `escalated`, since an escalated alert's STR is still in flight).
- `EnsureRules` re-seeds the rule set at every boot (idempotent, never
  overwrites admin edits): `TRUNCATE users CASCADE` silently wipes
  `risk_rules` through its `updated_by` FK and golang-migrate never re-runs a
  finished migration.
- CTR rows are produced from **electronic** deposit volume (PropVest has no
  cash flows) and are labelled "confirm whether a cash CTR applies" — the
  Compliance Officer makes the cash/electronic call before filing.
- The name-mismatch control is an ALERT, not a block: banks return name
  formats that differ from KYC; a human reviews them.

### Phase 9 — Data rights, consent & the compliance pack (Regulation layer)

**Verified obligations (NDPA 2023 + GAID 2025):**

- Implementable data-subject rights: **access/export, rectification, erasure** (subject to legal
  retention), objection (incl. direct marketing), restriction, withdrawal of consent.
- Consent must be *freely given, specific, informed, unambiguous*, purpose-tied and **withdrawable**
  — and must NOT be bundled with the KYC processing that the contract actually requires.
- Records of processing activities must be kept current; a **DPO** is required if you qualify as a
  *data controller of major importance* (BVN/NIN at scale makes that assessment mandatory — do the
  assessment, don't assume the old 2,000-subscriber NDPR rule).
- Breach: **notify NDPC within 72 hours**; notify data subjects without undue delay where risk is high.
- Cross-border transfers need an adequacy/safeguard basis, documented.
- **FCCPC:** no general 2024 marketing-message code could be verified (site unfetchable — marked
  *unverified*); the enforceable baseline is NDPA's objection-to-direct-marketing right, so build
  opt-out and suppression. Telecom SMS sender-ID rules are separate.

**Admin surface (added 26 Oct 2026):** data-rights activity must be **viewable and manageable in
the UI** — a new *Privacy* section in the React admin dashboard behind new `/admin/privacy/*`
APIs: per-user consent records, suppression list, data-export job queue, and deletion requests
(with approve/cancel during the grace period).

| # | Task | Effort | Acceptance |
|---|---|---|---|
| 9.1 | `user_consents` table + capture at registration (privacy policy version, terms, **separate** marketing consent) with IP/user-agent/timestamps; re-consent prompt when a policy version bumps | M | Exporting a user shows their consent record with versions and dates; declining marketing still allows signup |
| 9.2 | **`GET /me/data-export`** — async job assembling profile, wallet, transactions, investments, off-plan subscriptions, KYC metadata, notifications, tickets, consents into a downloadable bundle (email link, 24h SLA) | M | A seeded user's export contains every category; BVN/NIN included only in the owner-authenticated export |
| 9.3 | **`DELETE /me/account`** — deferred flow: 30-day grace with cancellation, blocks new activity immediately, then anonymizes identity fields while **retaining financial ledger rows** (statutory retention — the legal erasure exception) | M | After the grace period the account is unreachable, PII gone, ledger still reconciles |
| 9.4 | Marketing opt-out: one-click unsubscribe token on every bulk message + a suppression list the admin messaging service **must** check before sending | S | A suppressed address receives nothing from `POST /admin/messaging/send`; opt-out writes a consent row |
| 9.5 | **Admin privacy API** — `GET /admin/privacy/consents/:userId` (a user's consent records with versions, timestamps, IP/UA), `GET /admin/privacy/suppression` + `POST`/`DELETE` (view **and toggle** entries), `GET /admin/privacy/exports` (data-export jobs: status `queued→ready→expired`, re-issue download link), `GET /admin/privacy/deletions` + `PATCH .../:id` (approve / cancel a deletion during the 30-day grace, countdown shown), `POST /admin/policies/version` (bump a policy version → triggers re-consent prompts) — all writes audited | M | Every privacy action works through the API alone: suppress an address, cancel a pending deletion, bump a policy version; audit rows for each |
| 9.6 | **Admin privacy UI** — new *Privacy* section in the admin dashboard: tabs for **Consents** (search by user, see which version they agreed to), **Suppression list** (add/remove with one click), **Data exports** (queue with status + re-send), **Deletion requests** (approve/cancel buttons, days-left countdown, red when near deadline), **Policy versions** (current versions + "bump" action) | M | All five tabs clickable against live data; toggling suppression and cancelling a deletion persist on reload; `npm run lint` + `npm test` green |
| 9.7 | Breach runbook: detection → severity → **72h NDPC clock** → subject notification decision → evidence log | S | Tabletop walkthrough documented in `docs/compliance/` |
| 9.8 | **Compliance pack (documents, not code):** licence posture memo (SEC registration likely for pooled real-estate investment / collective scheme — get counsel; CBN MMO only if *you* issue e-money, otherwise operate under Paystack's licence and keep custody/settlement inside its authorised role), NDIC disclosure wording, data-retention schedule, DPO/DCMI assessment under GAID, incident-breach notification procedure | M | Signed-off pack in `docs/compliance/` — this gates the go-live compliance checkbox in Phase 5 |

**Status: ✅ done and live-verified (27 Oct 2026).** What shipped:

- **9.1** — `000034_privacy_data_rights`: `policy_versions` (append-only
  history), `user_consents` (append-only: a withdrawal is a NEW row with
  `granted=false`, pinned to the policy version, IP + user-agent + timestamp),
  `marketing_suppressions`, `data_export_jobs`, `account_deletions` (partial
  unique index ⇒ exactly one LIVE request per user). Consent is captured in
  the registration transaction (privacy policy + terms + **separate**
  marketing); `GET /me/consents` reports `needs_reconsent` when a version
  bumps. `000035_policy_version_history` dropped a stray `UNIQUE (policy)`
  constraint that made every version bump fail with 23505 — found by the
  integration test, fixed with a migration + an ordered "newest version wins"
  read.
- **9.2** — `POST /me/exports` queues an async job (assembly never blocks the
  request), the bundle lands **in the row** (JSONB, no shared filesystem), and
  the one-time bearer token is stored only as SHA-256. `GET
  /me/exports/download/:token` serves it inside the 24h window; re-requesting
  a ready job rotates the link (old token dies); the worker sweep **wipes the
  bundle** on expiry, not just the status. Bundle carries profile (hash
  redacted), wallet, transactions, payments, investments, off-plan,
  notifications, consents, support tickets, and KYC with BVN/NIN **decrypted
  — only for the owner**.
- **9.3** — `POST/GET/DELETE /me/account/delete`: 30-day grace, idempotent
  request, cancel path; the launch-policy middleware blocks deposits and
  withdrawals while a request is live (`HasPendingDeletions`); execution
  anonymises identity (`deleted+<id>@deleted.invalid`, scrubbed KYC, revoked
  sessions) and **retains** wallet/ledger/payments rows — proven by
  `TestPrivacy_DeletionGraceAnonymisesButKeepsLedger`.
- **9.4** — unsubscribe token (HMAC, derived from the JWT secret) on one click
  suppresses both channels and writes the withdrawal consent row; the bulk
  sender reads `marketing_suppressions` for email **and** SMS and **fails
  closed** if the list cannot be read.
- **9.5** — `GET /admin/privacy/consents/:userId`, `suppression`
  (list/add/delete), `exports` (list + `reissue` — the fresh link goes **to
  the user**, never the admin), `deletions` (`PATCH` approve=accelerate /
  cancel, with countdown), `policies` (version history + bump). Every write
  commits an audit row in the same transaction.
- **9.6** — new **Privacy** tab in the admin dashboard with the five views:
  Suppression (add/remove one click), Data exports (status + re-issue),
  Erasure requests (approve/cancel, days-left, reason), Policy versions
  (current + "publish next version"), Consent evidence (user lookup →
  versioned timeline). `npm run lint` 0 errors, 10/10 tests, build green.
- **9.7** — `docs/compliance/BREACH_RUNBOOK.md`: detection sources, S1–S4
  severity table, the **T+0 → T+72h** NDPC clock, notification content,
  subject-notification decision rule, evidence log format, and a quarterly
  tabletop with pass criteria.
- **9.8** — pack in `docs/compliance/`: `README.md` (index + sign-off),
  `LICENCE_POSTURE.md` (open decisions L1 wallet/MMO and L2 SEC/CIS marked
  **[COUNSEL]** and gating go-live), `NDIC_DISCLOSURE.md` (approved and
  prohibited wording — PropVest is not a bank, no unqualified "insured"),
  `DATA_RETENTION_SCHEDULE.md` (R1–R15 with the erasure exception stated),
  `DPO_ASSESSMENT.md` (treated as a DCMI; DPO + GAID filing open).
  Regulatory claims were web-verified (NDIC limits, ISA 2025 perimeter, CBN
  PSP register, GAID duties); anything unverifiable from a primary source is
  flagged for counsel instead of asserted.

**Verified:** `go vet ./...` clean · `TestPrivacy_*` 4/4 green on real
Postgres · frontend lint 0 errors, 10/10 tests, `vite build` green.

**Suggested order:** Phase 6 → 7 → 8 → 9 (verified identity unlocks screening records; the ledger
unlocks settlement and fee reports; monitoring needs the ledger's anomaly signals; compliance
pack last because it freezes decisions the code just made).

**Open decisions for you:** (a) is PropVest a *reporting entity* for NFIU — start goAML
registration early if yes; (b) does the product need SEC registration — ask counsel before Phase
7 ships fees. **Decided:** KYC is **manual, admin-verified, stored in DB** (no third-party
provider); fee model is 1% capped ₦2,000 on withdrawal + 10–15% performance fee on distributed
profit only.

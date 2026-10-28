# Integration Phase 2 — Implementation Plan

**Date:** 2026-10-21
**Scope:** Close the remaining frontend↔backend integration gaps identified by
`.agents/tasks/frontend-backend-integration-audit/INTEGRATION_AUDIT_REPORT.md` (health 65/100, ~15% of
features actually integrated), following Phase 1 (profile + notification bell, completed and live-verified).

**Method note:** Every claim below was verified against the actual code — route registrations in
`internal/routes/v1/routes.go`, service interfaces, DTOs, migrations `000001–000024`, and the frontend
services in `../propvest-app/src/services/`. Estimates assume one developer with agent assistance.

---

## 1. Verified Gap Inventory

| # | Frontend call | Backend status | Impact |
|---|---------------|----------------|--------|
| 1 | `GET /wallet/banks` (Withdraw page mount) | ❌ No route | Bank dropdown silently falls back to static `BANKS`; no bank codes exist client-side, so every withdrawal sends the hardcoded default `bank_code: '058'` |
| 2 | `POST /wallet/transfer-earnings` (`walletService`) | ❌ No route | Earnings→main transfer impossible; Withdraw page's transfer UI is a fake `setTimeout` that never calls the service |
| 3 | `POST /investments/:id/vote`, `POST /investments/:id/trigger-sale` (`portfolioService`) | ❌ No routes, no tables, no model fields | Sale-vote feature entirely cosmetic; `InvestmentResponse` also lacks `vote_*`/`can_trigger` fields and the frontend normalizer reads `amount_paid`/`current_val` which the backend never sends (portfolio shows ₦0 invested) |
| 4 | `POST /me/kyc/submit` (Profile KYC modal) | ❌ No route, no table (only `users.kyc_status` + admin `DecideKYC`) | Users cannot submit KYC at all |
| 5 | `GET /offplan/projects`, `GET /offplan/projects/:id`, `POST /offplan/projects/:id/subscribe` | ❌ No module | Whole Off-Plan page runs on static fallback; `confirm()` in `OffPlan.jsx` is a fake `setTimeout` that never calls `subscribe` |
| 6 | Admin portal (8 tabs, ~1200-line mock UI) | ✅ ~25 endpoints exist (dashboard, users, KYC, properties, investments, financial, reports, health, messaging) | Zero connection; `AdminDashboard.jsx` runs entirely on hardcoded constants; no `adminService.js` exists; `/admin` route has no client-side role guard |
| 7 | Notification preferences UI | ✅ All 7 endpoints wrapped in `notificationService` | Bell has list/actions but no preferences surface |
| 8 | Profile address/occupation/employer edits | ⚠️ `PATCH /users/me` accepts `full_name` only; `users` table has no such columns | Edits save to local state only, silently lost |
| 9 | forgot/reset-password, verify-email, resend-verification | ⚠️ DTOs + `verification_token` repo + mailer exist; service methods are `logger.Info("...not yet implemented")` stubs; no handler methods; routes commented out | No password-reset or email-verification flow |

**Important:** `apiClient.js` masks all of these behind `fallbackData` (enabled unless
`VITE_USE_MOCK_FALLBACK=false`), so failures show as silent mock data, not errors. Verification must run
with `VITE_USE_MOCK_FALLBACK=false`.

---

## 2. Workstream A — Repair Dead Endpoints (highest priority)

Repairs flows that are broken *right now*. Backend-only except where noted.

### A1. `GET /wallet/banks`

- **Backend:** add `ListBanks()` returning the Nigerian bank list. Source: `internal/payments/mock.go`
  already carries a bank-code→name map (~line 402); mirror it as a static provider-agnostic list in a new
  `dto.BankResponse{Code, Name string}` — no provider call needed (codes are stable).
- **Route:** `walletAuthenticated.GET("/banks", walletHandler.ListBanks)` → `data: [{code, name}, ...]`
- **Frontend:** `walletService.getBankList` returns objects; update `Withdraw.jsx`:
  - `<select>` renders `b.name`, value = `b.code`
  - `requestWithdrawal({ ..., bankCode: bank.code, bankName: bank.name, ... })` (today it always sends `058`)
- **Optional A1b:** `POST /wallet/verify-account` wrapping `provider.ResolveAccountNumber` to replace the
  fake `setTimeout` account-name verification in `Withdraw.jsx`. (Nice-to-have; recommended because the
  withdrawal API requires `account_name`.)

### A2. `POST /wallet/transfer-earnings`

- **Backend:** new `WalletService.TransferEarnings(ctx, userID, amountKobo)`:
  1. Txn with `SELECT ... FOR UPDATE` on wallet (same pattern as existing debit paths)
  2. Validate `amountKobo > 0 && amountKobo <= EarningsBalance`
  3. `EarningsBalance -= amount; MainBalance += amount`
  4. Single ledger row, `type = "transfer"` (already in the allowed type list), with balance_before/after
     for main balance and metadata noting the earnings side
- **DTO:** request `{amount}` (kobo, matches frontend), response `dto.WalletResponse`
- **Route:** `walletAuthenticated.POST("/transfer-earnings", ...)` + `errors` switch entries
  (`ErrInsufficientEarnings` → 400 `insufficient_earnings`)
- **Frontend:** rewire `Withdraw.jsx` `doTransfer` to `await walletService.transferEarnings(tAmt)` then
  refresh the wallet summary (currently a fake timeout that never calls the service).

### A3. Investment sale votes

**Migration `000025_sale_votes`:**

```sql
-- Model drift fix: column exists since 000001 but models.Property lacks the field
ALTER TABLE properties ADD COLUMN IF NOT EXISTS hold_years INTEGER NOT NULL DEFAULT 3;

CREATE TABLE sale_vote_campaigns (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    property_id   UUID NOT NULL REFERENCES properties(id),
    status        VARCHAR(20) NOT NULL DEFAULT 'open'
                  CHECK (status IN ('open','passed','failed')),
    triggered_by  UUID NOT NULL REFERENCES users(id),
    electors      INT  NOT NULL,      -- distinct investors at trigger time
    for_count     INT  NOT NULL DEFAULT 0,
    against_count INT  NOT NULL DEFAULT 0,
    result        VARCHAR(20),
    opened_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at     TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- At most one open campaign per property
CREATE UNIQUE INDEX idx_sale_votes_one_open
    ON sale_vote_campaigns(property_id) WHERE status = 'open';

CREATE TABLE sale_votes (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id   UUID NOT NULL REFERENCES sale_vote_campaigns(id) ON DELETE CASCADE,
    investment_id UUID NOT NULL REFERENCES investments(id),
    user_id       UUID NOT NULL REFERENCES users(id),
    property_id   UUID NOT NULL REFERENCES properties(id),
    vote          VARCHAR(10) NOT NULL CHECK (vote IN ('for','against')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (campaign_id, user_id)   -- one vote per investor
);
```

**Backend:**
- `models/sale_vote.go` (Campaign + Vote), repository, service methods on `InvestmentService`:
  - `TriggerSaleVote(ctx, userID, investmentID)` — ownership + active status check; require
    `investment.CreatedAt + property.HoldYears years <= now` (add `HoldYears` to `models.Property`);
    create campaign with `electors = COUNT(DISTINCT user_id) WHERE property_id = ? AND status='active'`;
    fan out in-app notifications to co-owners via `notificationService`; audit-log the trigger.
  - `CastVote(ctx, userID, investmentID, choice)` — voter must own an investment in that property;
    insert (unique violation → 409 `already_voted`); increment counters in the same txn; close when
    `for_count > electors/2` (passed) or all electors voted (failed); notify result.
  - `GetVoteState(ctx, investmentID)` for DTO enrichment.
- **DTO:** extend `InvestmentResponse` with `VoteActive, CanTrigger bool`,
  `VoteFor, VoteAgainst int`, `MyVote *string`, `EquityPct string`, `HoldYears int`
  (`can_trigger = hold elapsed && no open campaign`).
- **Routes:** `investments.POST("/:id/trigger-sale", ...)`, `investments.POST("/:id/vote", ...)`
  (already inside the `/investments` authenticated group; add `binding:"required,oneof=for against"` on vote).
- **Error mapping:** new `ErrSaleVoteNotEligible`, `ErrSaleVoteAlreadyOpen`, `ErrAlreadyVoted`,
  `ErrSaleVoteNotOpen` → 400/403/409 in all three switches in `internal/errors/handler.go` (status,
  client message, code) — the Phase 1 lesson.

**Frontend (`portfolioService.js` + `Portfolio.jsx`):**
- Fix `normalizeInvestment` mapping: `amount_kobo → myInvested` (`/100` when > 10,000,000 — the existing
  heuristic works), map `property.title → name`, `property.city/state → loc`, `property.roi_percent` etc.
  Without this the list renders ₦0 invested even once votes exist.
- `Portfolio.jsx` `triggerSale` currently never calls the service — it is a pure `setTimeout` with local
  state mutation. Make it await `portfolioService.triggerSaleVote(invId)` and surface backend errors; the
  vote button already calls `castInvestmentVote`. Invalidate the investments query on success instead of
  only mutating local state.

### A4. KYC submission

**Migration `000026_kyc_submissions`:**

```sql
CREATE TABLE kyc_submissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id),
    bvn         VARCHAR(11) NOT NULL,
    nin         VARCHAR(11) NOT NULL,
    doc_type    VARCHAR(60) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','verified','rejected')),
    reason      TEXT,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at   TIMESTAMPTZ,
    decided_by   UUID REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id)          -- one submission row per user (re-submit updates it)
);
CREATE INDEX idx_kyc_submissions_status ON kyc_submissions(status);
```

- **Route (exact frontend path):** frontend posts to `/me/kyc/submit`, i.e. `/api/v1/me/kyc/submit` —
  register it at the router root (authenticated group), *not* under `/users`. Optionally also
  `GET /me/kyc` for status.
- **Backend:** `KYCService` (or `UserService.SubmitKYC`): upsert submission, set `users.kyc_status`
  stays/becomes `'pending'`, return `{kyc_status, submitted_at}`. Handler fits naturally in `user.go`.
- **Admin integration:** extend `AdminUserHandler.DecideKYC` to also update `kyc_submissions.status`,
  `decided_at/decided_by/reason` in the same transaction. Add `GET /admin/kyc/submissions`
  (pending queue with user summary) so the admin KYC tab can show real documents/data.
- **Security:** BVN/NIN are PII — never log them, never return them in list endpoints (return `bvn_last4`
  only); note in code that encryption-at-rest is a follow-up decision.
- **Frontend:** no change required (Profile modal already posts `{bvn, nin, docType}` → map `doc_type` on
  the backend side via json tag `docType` or adjust `Profile.jsx` to `doc_type` — prefer fixing the
  frontend to snake_case for consistency).

---

## 3. Workstream B — Off-Plan Backend Module

New vertical slice: migration → model → repository → service → handler → routes → DI wiring.

**Migration `000027_offplan`:**

```sql
CREATE TABLE offplan_projects (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(255) NOT NULL,
    location      VARCHAR(255) NOT NULL,       -- frontend field "loc"
    property_type VARCHAR(100) NOT NULL,       -- frontend field "type"
    price_kobo    BIGINT NOT NULL,
    stage         VARCHAR(50) NOT NULL,        -- Foundation|Superstructure|Roofing|Finishing
    completion    VARCHAR(50) NOT NULL,        -- "Q4 2027"
    developer     VARCHAR(255) NOT NULL,
    down_pct      INT NOT NULL,
    img           VARCHAR(10) NOT NULL DEFAULT '🏡',
    features      JSONB NOT NULL DEFAULT '[]',
    renders       JSONB NOT NULL DEFAULT '[]',
    finish_types  JSONB NOT NULL DEFAULT '[]', -- [{id,label,price,desc,includes[]}]
    plans         JSONB NOT NULL DEFAULT '[]', -- [{id,label,months,rate}]
    status        VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE TABLE offplan_subscriptions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id),
    project_id         UUID NOT NULL REFERENCES offplan_projects(id),
    plan_id            VARCHAR(20) NOT NULL,
    finish_type_id     VARCHAR(20) NOT NULL,
    price_kobo         BIGINT NOT NULL,
    down_payment_kobo  BIGINT NOT NULL,
    fee_kobo           BIGINT NOT NULL DEFAULT 0,   -- 2% subscription fee
    reference          VARCHAR(60) UNIQUE NOT NULL,
    status             VARCHAR(20) NOT NULL DEFAULT 'active'
                       CHECK (status IN ('pending','active','completed','cancelled')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_offplan_subs_user ON offplan_subscriptions(user_id);
```

- **Seed:** `INSERT` the three projects from `../propvest-app/src/constants/data.js` (`OFF_PLAN`) inside
  the migration so the UI shows real rows immediately; `down` migration deletes them. JSONB columns store
  `plans/finish_types/renders/features` verbatim — no lossy flattening, no frontend shape changes.
- **Endpoints** (route group `/offplan`, public read + auth write):
  - `GET /offplan/projects` → **must unwrap to a bare array**: `OffPlan.jsx` does
    `setProjectsList(data || [])` and would crash on `{projects: [...]}`. Return
    `response.Success(c, 200, gin.H{"projects": ...})`? No — the client unwraps `data`, so `data` itself
    must be the array; use `response.Success(c, http.StatusOK, projects)`.
  - `GET /offplan/projects/:id` → single project or 404.
  - `POST /offplan/projects/:id/subscribe` (auth) body `{plan_id, finish_type_id}` — **decision point:**
    - *Recommended (MVP):* validate plan/finish_type exist, compute `price` from the finish type and
      `down = price * down_pct/100`, `fee = 2%`, debit `down + fee` from the user's main wallet in a
      locked txn with two ledger rows (reuses A2's debit pattern; rejects with `insufficient_funds`),
      insert subscription, return `{reference, status, down_payment_kobo, ...}`.
    - *Alternative (record-only):* insert with `status='pending'` and no wallet debit; instalment
      debiting deferred to a payment-scheduling milestone.
  - Instalment auto-debit scheduling, cancellation fees, and progress tracking are **explicitly out of
    scope** for Phase 2 (no scheduler exists; the 15% cancellation fee shown in the T&C stays cosmetic).
- **Frontend:** `offplanService.subscribe` switches to snake_case keys; `OffPlan.jsx` `confirm()` awaits
  it (real errors surfaced instead of unconditional success), then shows the existing success screen;
  invalidate wallet query on debit.

**DI wiring:** `NewOffplanRepository` → `NewOffplanService(...)` → `NewOffplanHandler(...)` → add
parameter to `v1.RegisterRoutes` in `cmd/api/main.go` + `internal/routes/v1/routes.go` (same pattern as
notifications).

---

## 4. Workstream C — Admin Portal Wiring

~25 endpoints exist; the 1236-line `AdminDashboard.jsx` uses none. Create
`../propvest-app/src/services/adminService.js` and rewire tab-by-tab (each independently shippable):

| Phase | Tab | Backend endpoints | Notes |
|-------|-----|-------------------|-------|
| C1 | overview | `GET /admin/dashboard` | `DashboardResponse` (users/properties/investments/financial metrics, all kobo → convert with existing `nfmt`); replace `ADMIN_ACTIVITY` with audit-log tail `GET /admin/reports/audit-logs?limit=10` |
| C1 | users | `GET /admin/users`, `GET /admin/users/:id`, `PATCH :id/suspend|activate`, `POST :id/logout-all`, `POST :id/reset-password`, `PATCH :id/role` | Replace `USERS_LIST`; filters (`role`, `kyc_status`, `search`) map to existing query params |
| C1 | kyc | `GET /admin/users?kyc_status=pending` + new `GET /admin/kyc/submissions` (A4) + `PATCH /admin/users/:id/kyc` | Replace `ADMIN_KYC`; approve/reject writes real decisions |
| C2 | properties | existing `/admin/properties` CRUD + `publish` + images/documents | AdminDashboard embeds `<Properties/>`; give the embedded instance an `admin` mode that calls `adminService` instead of the public list |
| C3 | finance | `GET /admin/financial/transactions`, `/payments` (+ `:id` detail) | Replace `PAYOUT_PROPS`; **rental-distribution payout processing has no backend — keep that panel labeled "demo"** |
| C3 | compliance | `GET /admin/reports/audit-logs` (+ `/export` CSV), `POST /admin/reports/generate`, `GET /admin/health/system`, `/queues` | Filings status (SEC/CAC/FIRS) stays static informational — no backend exists |
| C3 | broadcast | `POST /admin/messaging/send` | Wire the existing bulk-messaging backend (`ADMIN_BULK_MESSAGING_FEATURE.md`) |
| C4 | staff / roles | `PATCH /admin/users/:id/role` only | Staff roster, permission matrix, impersonation, charge settings have **no backend**: either hide these tabs for MVP or render with an explicit "not connected — demo data" badge. **Decision point.** |

- **Guard:** client-side redirect off `/admin` unless the profile's `role` is `admin`/`super_admin`
  (backend `RequireRole("admin")` already protects every route — this is UX only).
- **Errors:** 403 from `RequireRole` should render a clean "Admins only" state, not the generic alert.

---

## 5. Workstream D — Phase-1 Leftovers

### D1. Notification preferences UI
- Surface: gear button inside the `NotificationBell` dropdown → preferences panel (no new route needed).
- `GET /notifications/preferences` returns `{preferences: [{notification_type|null, in_app_enabled,
  email_enabled, sms_enabled, ...}]}` — render the global row (`notification_type: null`) as
  "Default for all" plus one row per type; save with the batch `PUT /notifications/preferences`
  (`{preferences:[{notification_type, in_app_enabled, ...}]}` — matches `BatchUpdatePreferences`).
- Type list: backend accepts arbitrary strings; use the same canonical list the bell already has
  (`investment_confirmed, wallet_credited, withdrawal_processed, deposit_success, kyc_approved,
  kyc_rejected, property_update, payment_received, system_announcement`) seeded as global rows, or
  simply render whatever GET returns. Optional hardening: add `ValidNotificationTypes` validation on the
  backend so typos can't create junk rows.

### D2. Profile address/occupation/employer persistence
- **Migration `000028_add_user_profile_fields`:**
  `ALTER TABLE users ADD COLUMN address TEXT, ADD COLUMN occupation VARCHAR(120), ADD COLUMN employer VARCHAR(120);`
- Extend `UpdateProfileRequest` with optional `address/occupation/employer` (keep `full_name` required —
  `Profile.jsx` always sends it), extend `UserResponse` + `mappers.go` + `UserService.UpdateProfile`.
- Frontend already sends/receives these keys — no change.
- Re-probe: PATCH with all fields → GET shows them.

### D3. Password reset + email verification (finish the commented-out routes)
Foundations exist: `verification_tokens` table (000008), `VerificationTokenRepository`,
`mailer.New(cfg)` (mock prints in dev), DTOs `ForgotPasswordRequest/ResetPasswordRequest/
VerifyEmailRequest/ResendVerificationRequest`.
1. Implement the four stubbed `authService` methods (they currently just `logger.Info`):
   - `ForgotPassword`: find user by email → `InvalidateActive(purpose=password_reset)` → create
     hashed single-use token (15–30 min TTL) → enqueue email with reset link → **always return nil**
     (no account enumeration).
   - `ResetPassword`: `FindByHash` → expiry/used checks → set password → revoke all refresh tokens
     → `MarkUsed`.
   - `VerifyEmail` / `ResendVerification`: same pattern with `purpose=email_verification` →
     `users.email_verified = true`.
2. Add the four handler methods in `internal/handlers/auth.go` (mock auth tests already declare them).
3. Uncomment the routes at `routes.go:88–91`.
4. Error mapping: token invalid/expired/used → 400 with dedicated codes (add to all three switches).
5. Frontend: add `authService.forgotPassword/resetPassword`, a "Forgot password?" flow in the auth
   modal, and a verify-email banner/notice; dev mailer surfaces the link in server logs.

---

## 6. Migration Schedule

| Migration | Contents |
|-----------|----------|
| `000025_sale_votes` | `sale_vote_campaigns`, `sale_votes`, `properties.hold_years` model-drift note |
| `000026_kyc_submissions` | KYC submission queue table |
| `000027_offplan` | `offplan_projects` (+ seed), `offplan_subscriptions` |
| `000028_add_user_profile_fields` | `users.address/occupation/employer` |

Each with a matching `.down.sql`; run via the existing auto-migration on boot, verify with
`run-migrations.ps1` / local DB.

---

## 7. Sequencing & Estimates

| Order | Workstream | Est. | Depends on |
|-------|-----------|------|-----------|
| 1 | **A1 + A2** wallet banks & transfer-earnings | 0.5–1 d | — |
| 2 | **D2** user profile fields (tiny, unblocks profile) | 0.5 d | — |
| 3 | **A4** KYC submit | 1 d | — |
| 4 | **A3** sale votes (largest design surface) | 1.5–2 d | model-drift fix |
| 5 | **B** off-plan module | 1.5–2 d | debit pattern from A2 |
| 6 | **D1** notification preferences UI | 0.5 d | — |
| 7 | **D3** password reset + email verification | 1–1.5 d | — |
| 8 | **C** admin wiring (C1→C3; C4 decision) | 2–3 d | A4 for KYC tab |

**Total: ~9–12 days.** Each workstream is independently shippable and independently verifiable.

---

## 8. Verification Strategy (per workstream, mirrors Phase 1)

1. **Backend:** `go build ./...` and `go test ./internal/...` after every workstream; add service-level
   tests for money movement (A2, B debit) and vote state machine (A3) — these are the two places where a
   bug costs real money or a governance outcome.
2. **Frontend:** `npm run build` (exit 0) after every frontend change.
3. **Live probes:** boot a throwaway instance (Phase-1 methodology), register a fresh user, and curl
   every new/changed endpoint:
   - A1: `GET /wallet/banks` → array with codes; withdrawal with the selected `bank_code`
   - A2: fund earnings (test hook/direct DB credit) → transfer → balances + ledger row; over-amount → 400
   - A3: two users on one property → trigger (after artificially aging `created_at` or `hold_years=0`) →
     vote dup → 409, threshold → campaign `passed`
   - A4: submit → admin queue → `DecideKYC` → `kyc_status` flips on both tables
   - B: list/detail/subscribe → wallet debited + subscription row; insufficient funds → 409
   - C: one probe per admin endpoint with an admin token + a 403 with a normal user token
   - D2: PATCH all profile fields → GET round-trip
   - D3: forgot-password → token from mock mailer log → reset → old password dead, sessions revoked
4. **Frontend integration pass** with `VITE_USE_MOCK_FALLBACK=false` so no 404 can hide behind mock data
   — this is the check that would have caught every gap in this document.
5. Re-run the endpoint-mapping section of the audit (or a focused re-grep of `api.` calls vs
   `routes.go`) and confirm zero frontend-only endpoints remain.

---

## 9. Open Decisions

1. **Off-plan subscribe:** wallet-debit MVP (recommended, needs A2's debit pattern first) vs
   record-only subscriptions.
2. **Admin C4 tabs (staff/roles/charges/impersonation):** hide for MVP (recommended) vs render with a
   "demo data" badge.
3. **BVN/NIN storage:** plaintext columns now + encryption follow-up (recommended for MVP; document in
   code), or encrypt-before-write immediately.
4. **Fallback policy:** after verification, strip `fallbackData` from implemented endpoints (recommended)
   so real errors reach the UI; keep mocks only for genuinely backend-less features.

---

## 10. Async machinery — status (implemented & live-verified 25 Oct 2026)

The gap inventory above (§1) covers the **request/response** surface. Phase 2 also closed the
**asynchronous** half — the worker, the transactional outbox, the queue, and the notification
fan-out — which is what makes deposits, investments, and withdrawals actually *notify* the user.

### What was built

| Piece | Where | What it does |
|---|---|---|
| Worker heartbeat | `cmd/worker/heartbeat.go` | Touches a file every 30 s; compose healthcheck fails if stale (>2 min), so a wedged consumer shows as `unhealthy` instead of silent silence |
| Business-events queue | `internal/queue` (`propvest.business.events`) | Shared channel the outbox dispatcher publishes to |
| Outbox dispatcher routing | `internal/dispatcher/outbox_dispatcher.go` | Routes the 6 business `event_type`s (investment/property lifecycle) that previously had **no queue and no consumer** — they used to retry 10× then fail permanently |
| Business-event consumer | `cmd/worker/dispatch.go` (`startBusinessEventDispatcher`) | Consumes `propvest.business.events` → writes the in-app `notifications` row → pushes the badge update to open sockets |
| Outbox backlog metric | admin health `/admin/health/queues` | Reports `outbox.depth`, `oldest_message_age_seconds`, `outbox_failed.depth` — turns the "silent black hole" failure mode into a visible `unhealthy` |
| Withdrawal-finalization fix | `internal/services/wallet_service.go` | Stopped writing `updated_at` on the append-only `wallet_transactions` ledger (no such column); that bad write used to roll back finalization and strand withdrawals in `pending` |

### Live-verified evidence

- **Build & tests:** `go build ./...` clean; `go test ./cmd/worker/... ./internal/dispatcher/... ./internal/queue/... ./internal/mailer/...` → all `ok`.
- **End-to-end:** a real `POST /investments` fired `investment.created` → published to `propvest.business.events` → `notification created from business event` (`investment_created`, "Investment confirmed", ₦20,000,000 in Wuse II Plaza) → appeared in `GET /notifications` and as the header 🔔 badge + dropdown on `localhost:3000/dashboard` → `PATCH /notifications/:id/read` cleared it.
- **Health:** `/admin/health/system` → `healthy` (postgresql latency, email=Brevo SMTP, paystack, rabbitmq, cloudinary, sms=logged-not-sent). `/admin/health/queues` → `healthy`, `outbox.depth: 0` after a real event drained.
- **Heartbeat:** `/tmp/propvest-worker-heartbeat` mtime refreshes every ~30 s while the worker runs.

**Operational rule:** the API alone does not deliver notifications. The worker (`go run ./cmd/worker`) must
be running; see [WORKFLOW_TESTING_GUIDE.md](WORKFLOW_TESTING_GUIDE.md) §11 for the hands-on Postman walkthrough.

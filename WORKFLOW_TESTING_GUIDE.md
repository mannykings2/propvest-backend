# PropVest — Workflow Testing Guide

**How to test every workflow in this app yourself, live, mostly with Postman.**

Every status code marked *observed* below was executed against a real local stack on 24 October 2026. Where today's behaviour differs from correct behaviour, both values are shown with a reference to the matching item in [PRODUCTION_READINESS_PLAN.md](PRODUCTION_READINESS_PLAN.md).

> ⚠️ **Five workflows currently fail by design of the bugs, not by misconfiguration** — buying a slot, refreshing a session, three admin endpoints, and the error codes on validation/business failures. They are marked **KNOWN BUG** in the tables below so you can tell your own mistakes apart from the app's. §9 lists them with the fix reference.

---

## 1. What you need installed

| Tool | Why | Check |
|---|---|---|
| Podman or Docker | Postgres, Redis, RabbitMQ | `podman --version` |
| Go 1.26+ | API + worker | `go version` |
| Node 18+ | Frontend | `node -v` |
| Postman (or Bruno/Insomnia) | Running the collection | — |

---

## 2. Start the stack

```bash
# 1. Infrastructure (Postgres :5435, Redis :6379, RabbitMQ :5672)
cd propvest-backend
podman compose up -d postgres redis rabbitmq

# 2. API on :8081
PORT=8081 go run ./cmd/api
```

> **Gotcha (real):** this machine exports `PORT=0`, and viper prefers OS env over `.env`, so the API will silently bind to a random port if you omit `PORT=8081`. Symptom: `curl localhost:8081/health` returns `000` while the log says `HTTP server listening addr=:0`.

```bash
# 3. Frontend on :3000 (proxies /api -> :8081)
cd propvest-app
npm run dev
```

Verify before anything else:

```bash
curl -s http://localhost:8081/health        # 200 {"status":"healthy"}
curl -s http://localhost:8081/api/v1/properties | head -c 200   # 200, real rows
```

---

## 3. Postman setup (recommended path)

1. **Import:** `postman/PropVest_Workflow_Tests.postman_collection.json`
2. **Environment:** create one (or import `postman/PropVest_Local.postman_environment.json`) with:

   | Variable | Value |
   |---|---|
   | `baseUrl` | `http://localhost:8081/api/v1` |
   | `access_token` | *(blank — filled automatically)* |
   | `refresh_token` | *(blank — filled automatically)* |
   | `user_email` | *(blank — generated automatically)* |
   | `user_password` | `QaTesting!2026` |

3. **Run:** Collection → **Run** → **Run PropVest — Live Workflow Tests**, keeping folder order 1 → 10.

Earlier folders capture `access_token`, `refresh_token`, `property_id`, `offplan_project_id` and `new_property_id` into the environment, so later folders authenticate and target real records without any manual copy/paste.

The collection has **39 requests across 10 folders**, each with `pm.test` assertions. Green = workflow works. A red test named `KNOWN P0-x` = the documented bug, not your setup.

---

## 4. Seed data

The dev database already contains **8 properties** (5 rental, 3 land) created for testing. If they are missing:

```sql
-- Minimal active property (amounts are in KOBO: ₦20,000,000 = 2000000000)
INSERT INTO properties (title, slug, description, property_type, address, city, state,
  target_amount, roi_percent, duration_months, total_units, unit_price,
  minimum_investment, status, created_by)
VALUES ('QA Property', 'qa-property', 'Seed property for workflow testing.',
  'residential', '1 Test Street', 'Abuja', 'FCT',
  20000000000, 14, 36, 10, 2000000000, 2000000000, 'active',
  (SELECT id FROM users ORDER BY created_at LIMIT 1));
```

**Credit a wallet for investment tests** (the shipped `scripts/credit_wallet.sql` is broken — see §9.7; use this):

```sql
DO $$
DECLARE v_w UUID; v_uid UUID := (SELECT id FROM users WHERE email = 'YOUR_EMAIL');
  v_cur BIGINT; v_new BIGINT;
BEGIN
  SELECT id, main_balance INTO v_w, v_cur FROM wallets WHERE user_id = v_uid;
  v_new := v_cur + 3000000000;            -- ₦30,000,000
  UPDATE wallets SET main_balance = v_new, updated_at = NOW() WHERE id = v_w;
  INSERT INTO wallet_transactions (id, wallet_id, user_id, type, amount,
    balance_before, balance_after, reference, description, status, external_reference, created_at)
  VALUES (gen_random_uuid(), v_w, v_uid, 'deposit', 3000000000, v_cur, v_new,
    'SEED-' || TO_CHAR(NOW(),'YYYYMMDD-HH24MISS'), 'Test credit', 'completed', 'SEED', NOW());
END $$;
```

Note the two columns that differ from the old script: `external_reference` (not `provider_reference`) and `user_id` — **without `user_id` the row is invisible to `GET /wallet/transactions`**, which filters on it.

---

## 5. Folder-by-folder: what to expect

### 1 · Health
| Request | Expected | Notes |
|---|---|---|
| `GET /health` | **200** *observed* | Liveness — deliberately checks no dependencies |
| `GET /health/ready` | **200** *observed* | Real probes (P0-5 fixed): Postgres ping + migration version (**critical**), RabbitMQ connection (reported, non-critical). Stop Postgres and this returns **503** with `code=not_ready` while `/health` stays 200 |

### 2 · Auth
| Request | Expected | Notes |
|---|---|---|
| `POST /auth/register` | **201** *observed* | Unique email/phone generated by the pre-request script |
| `POST /auth/login` | **200** *observed* | Captures both tokens |
| `GET /users/me` | **200** *observed* | Echoes the registered email |
| `POST /auth/refresh` | **200** *observed* | Rotates the token pair (P0-2 fixed: refresh tokens carry a `jti`). 20 consecutive login→refresh cycles returned **zero 5xx** |
| `POST /auth/logout` | **200** *observed* | |

> Tokens live 15 minutes. If later folders start returning `401 Invalid or expired token`, re-run folder 2.

### 3 · Wallet & Money
| Request | Expected | Notes |
|---|---|---|
| `GET /wallet` | **200** *observed* | `main_balance / locked_balance / earnings_balance` in kobo |
| `GET /wallet/transactions` | **200** *observed* | Empty until the wallet is credited |
| `GET /wallet/banks` | **200** *observed* | ~16 banks with `code` + `name` |
| `POST /wallet/verify-account` | **200** *observed* | Returns `account_name` (test mode resolves `0123456789`) |
| `POST /wallet/deposit` | **200** *observed* | Returns a real `https://checkout.paystack.com/...` URL + `reference` |
| `POST /wallet/withdraw` | **200** *observed* | Paystack **test** mode — debits balance and queues a test transfer. Needs a **funded** wallet; an empty wallet correctly returns **402** `code=insufficient_funds` (was 500) |
| `POST /wallet/withdraw` (amount ₦0.10) | **422** *observed* | `code=minimum_withdrawal` (was 500 — P0-4) |

**Completing a deposit for real (test cards):** open the returned `authorization_url` and pay with a Paystack *test* card — see `PAYSTACK_TEST_CARDS.md` in this repo. Paystack cannot reach `localhost`, so confirm the credit locally with:

```bash
export PAYSTACK_WEBHOOK_SECRET=your_secret_from_.env
go run ./tools/trigger_webhook DEP-XXXXXXXX-XX     # the reference from the deposit call
```

⚠️ Without exporting `PAYSTACK_WEBHOOK_SECRET` the tool fails with `400 Invalid signature` — the secret is **not** read from `.env` automatically (known issue 9.7).

### 4 · Properties (public — no token)
| Request | Expected | Notes |
|---|---|---|
| `GET /properties` | **200** *observed* | Captures the first `active` property id |
| `GET /properties/:id` | **200** *observed* | Was 500 before the `display_order` fix; includes `financial.*` |

Also worth checking in a browser: a **draft** property (just created in folder 9) must **not** appear in this public list.

### 5 · Investments
| Request | Expected | Notes |
|---|---|---|
| `POST /investments` | **201** *observed* — 81 ms with a funded wallet | P0-1 fixed: no more 20–60 s hang. Empty wallet → **402** `code=insufficient_funds`; fully funded property → **409** `code=resource_unavailable` |
| `GET /investments` | **200** *observed* | `investments: []` until a purchase succeeds |
| `GET /portfolio/summary` | **200** *observed* | `total_invested, active_count, wallet_balance, earnings_balance` |

Before blaming the bug, confirm the wallet is funded (§4) and the target property is `active` — a funded property correctly returns an error, just with the wrong status code (P0-4).

### 6 · Off-plan
| Request | Expected | Notes |
|---|---|---|
| `GET /offplan/projects` | **200** *observed* | 3 projects with plans + finish tiers |
| `POST /offplan/projects/:id/subscribe` (empty body) | **422** *observed* | Requires `plan_id` + `finish_type_id` |
| `POST /offplan/projects/:id/subscribe` (valid body) | **201** *observed* — 363 ms | Same ledger-in-transaction path as P0-1, now green; ledger row written and 0 `idle in transaction` sessions left behind |
| `GET /offplan/subscriptions` | **200** *observed* | |

### 7 · KYC & Profile
| Request | Expected | Notes |
|---|---|---|
| `GET /me/kyc` | **200** *observed* | `kyc_status: "pending"` for a new user |
| `POST /me/kyc/submit` (empty) | **422** *observed* | Requires `BVN`, `DocType`, `NIN` |
| `PATCH /users/me` | **200** *observed* | `full_name` only |

### 8 · Notifications
| Request | Expected | Notes |
|---|---|---|
| `GET /notifications` | **200** *observed* | Returns `{notifications:[...], unread_count, pagination}`. Populated **only while the worker is running** — see the note below |
| `GET /notifications/preferences` | **200** | |
| `PATCH /notifications/read-all` | **200** | |

**To see notifications appear, the worker must be running** (it is a separate process from the API).
Start it in a second terminal and keep it alive:

```bash
cd propvest-backend && go run ./cmd/worker   # log shows: consumer started queue=propvest.business.events
```

Then do a deposit, an investment, or a withdrawal. The write emits an outbox event in the same DB
transaction; within a few seconds the worker's dispatcher publishes it to RabbitMQ
(`propvest.business.events`), its consumer writes an in-app `notifications` row, and
`GET /notifications` / the header bell `unread_count` reflect it. No worker ⇒ rows never appear, no
matter how many API calls you make.

**Live-verified (25 Oct 2026):** a real investment fired `investment.created` → published to
`propvest.business.events` → `notification created from business event` (type `investment_created`,
"Investment confirmed", ₦20,000,000 in Wuse II Plaza) → showed as the bell badge and dropdown entry on
`http://localhost:3000/dashboard`, and `PATCH /notifications/:id/read` cleared it.

### 9 · Admin (needs an admin token — see §6)
| Request | Expected | Notes |
|---|---|---|
| `GET /admin/dashboard` | **200** *observed* | P0-3 fixed — the actor id is read through `getUserIDFromContext` |
| `GET /admin/users` | **200** *observed* | Same fix; a **non-admin** token now gets **403** instead of a panic/500 |
| `GET /admin/properties` | **200** *observed* | |
| `POST /admin/properties` | **201** *observed* | Creates in `draft` |
| `POST /admin/properties/:id/publish` | **422** *observed* | `cover image is required` — upload an image first (multipart, `POST /admin/properties/:id/images`) |
| `GET /admin/kyc/submissions` | **200** *observed* | |
| `GET /admin/investments/metrics` | **200** *observed* | |

### 10 · Security
| Request | Expected | Notes |
|---|---|---|
| `GET /users/me` (no token) | **401** *observed* ✅ | |
| `GET /admin/users` (no token) | **401** *observed* ✅ | |
| Register with `phone: "12345"` | **422** `code=validation_error` *observed* | P0-4 fixed — no more `internal_error` on a 4xx |
| Register with a duplicate phone | **409** `code=phone_taken` *observed* | Was 500 — the duplicate-phone sentinel was a separate error value that no mapping recognised |
| Login with wrong password | **401** + `X-Ratelimit-Limit: 10` *observed* ✅ | Rate limiting works |

**Extra checks worth doing by hand**

```bash
# A non-admin token hitting an admin route must be 403
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $USER_TOKEN" \
  http://localhost:8081/api/v1/admin/users      # expected 403
```

---

## 6. Give yourself admin access

Admin routes read the role from the **JWT claim**, so a role change in the DB only takes effect after a fresh login.

```sql
UPDATE users SET role = 'admin' WHERE email = 'qa_XXXX@example.com';
```

Then re-run folder **2 · Auth → Login** in Postman (a new token is captured automatically) and run folder 9.

> Related: because the role lives in the token, suspending or deleting a user does **not** revoke an already-issued token until it expires (P1 item 2).

---

## 7. Frontend walkthrough (browser)

Open `http://localhost:3000` and verify:

| Step | Expected |
|---|---|
| Signed out at `/` | Product-first home: stats, "Open for investment" rows showing **live** yields/prices, explore tiles, footer |
| Header nav (signed out) | Properties · Off-Plan · How It Works · Pricing · Insights · About |
| `/properties` | Lists the seeded properties; counts "5 properties · Monthly payouts" / "3 properties" |
| `/portfolio` | Sign-in gate (no protected API calls fired — check the Network tab) |
| Sign in through the gate | Page mounts in place, no reload |
| Header nav (signed in) | Dashboard · Properties · Portfolio · Off-Plan · Withdraw · Documents · Support |
| `/` while signed in | Redirects straight to `/dashboard` |
| Console | Only the expected Vite/dev messages — the API 500s from the bugs above will appear as failed `/api/v1/...` requests |

> `.env.development` sets `VITE_USE_MOCK_FALLBACK=true`, so a failing request silently serves mock data. Set it to `false` when you want to see raw API failures.

---

## 8. Automating it

**One-shot collection run (CI-friendly):**

```bash
newman run postman/PropVest_Workflow_Tests.postman_collection.json \
  --env-var baseUrl=http://localhost:8081/api/v1 --timeout-request 60000
```

Every request in the collection carries real `pm.test` assertions, so newman
exercises the same checks you would click through in Postman. Two folders have
preconditions that a brand-new user does not meet, so run them separately once
you have satisfied the precondition:

```bash
# 3 · Wallet & Money and 5 · Investments need money in the wallet (§4)
newman run postman/PropVest_Workflow_Tests.postman_collection.json \
  --folder "5. Investments" --env-var "access_token=$TOKEN" \
  --env-var "property_id=<an active property id>"

# 9 · Admin needs a token that belongs to an admin (§6)
newman run postman/PropVest_Workflow_Tests.postman_collection.json \
  --folder "9. Admin (needs an admin token — guide §6)" \
  --env-var "access_token=$ADMIN_TOKEN"
```

`--env-var` beats the collection variable, so an already-minted token wins over
whatever the auth folder captured. Access tokens live 15 minutes — if a run
suddenly turns `401`, log in again and paste a fresh token.

**Results of the automated run on 2026-10-24 (API on :8081, local Postgres):**

| Run | Assertions | Failed |
|---|---|---|
| Whole collection (fresh user, empty wallet, no admin role) | 58 | 9 — all of them are the two preconditions above, never a 5xx |
| `3 · Wallet & Money` with a funded wallet | 11 | 0 |
| `5 · Investments` with a funded wallet + active property | 4 | 0 (`POST /investments` → 201 in 81 ms) |
| `9 · Admin` with an admin token | 7 | 0 |

**Quick smoke without Postman:**

```bash
B=http://localhost:8081/api/v1
curl -s -o /dev/null -w "health      %{http_code}\n" $B/health
curl -s -o /dev/null -w "properties  %{http_code}\n" $B/properties
curl -s -o /dev/null -w "offplan     %{http_code}\n" $B/offplan/projects
curl -s -o /dev/null -w "no-token 401 %{http_code}\n" $B/users/me
```

**Go suites:**

```bash
go test ./tests/integration/... -v      # 4 tests, needs the *_test containers:
podman compose -f docker-compose.test.yml up -d
```

---

## 9. Known failures (do not debug these as your own mistakes)

**Fixed in Phase 0 (24 Oct 2026) — if you see these again, something regressed:**

| # | Symptom | Was | Now |
|---|---|---|---|
| 9.1 | `POST /investments` hangs, then `500` | **P0-1** ledger insert escaped the DB transaction | **201 in 81 ms**, 0 `idle in transaction` |
| 9.2 | `POST /auth/refresh` → `500` duplicate `token_hash` | **P0-2** refresh tokens had no `jti` | **200**, 20 login→refresh cycles with zero 5xx |
| 9.3 | `/admin/dashboard`, `/admin/users`, `/admin/health/system` → `500` | **P0-3** handlers asserted `uuid.UUID`, middleware stores a `string` | **200** with an admin token, **403** without |
| 9.4 | Business/validation failures returned `500` / `code=internal_error` | **P0-4** no error→status registry | 402 insufficient funds · 409 conflict · 422 validation · 409 duplicate phone |
| 9.5 | `/health/ready` said ready with Postgres stopped | **P0-5** no dependency checks | **503** `code=not_ready`, liveness stays 200 |

**Still open:**

| # | Symptom you will see | Fix reference |
|---|---|---|
| 9.6 | ~~Notifications stay empty~~ | ✅ **Fixed (Phase 2)** — worker email/SMS dispatch wired + `business.events` consumer creates the in-app row. Notifications stay empty **only if the worker process isn't running** (see §8); with `go run ./cmd/worker` up, an investment/deposit/withdrawal populates them within seconds |
| 9.7 | ~~`scripts/credit_wallet.sql` errors on `provider_reference`; `tools/trigger_webhook` says `Invalid signature`~~ | ✅ **Fixed (P1-1.6)** — see §10E/§10D |
| 9.8 | New property cannot be published | Not a bug: cover image is mandatory (422 is correct) |
| 9.9 | `newman` reports `assertions 0` on an older copy of the collection | The event blocks must be `{"listen":"test","script":{"exec":[...]}}` (v2.1). A flat `{"listen":"test","exec":[...]}` is v2.0 and is silently ignored — re-import the copy from this repo |

---

## 10. Phase 1 checks (audit trail, revocation, policy flags, money loop)

Added 25 Oct 2026 with Phase 1 of the readiness plan. These are the workflows that did not
exist before it; each one is quick to drive from Postman with the same collection variables
(`{{admin_token}}`, `{{user_token}}`, `{{base_url}}`).

### A · Admin audit trail + instant suspension

| # | Request | Expect |
|---|---|---|
| 1 | `PATCH /admin/users/{{user_id}}/suspend` body `{"reason":"testing the revocation path"}` (reason is **min 10 chars**), header `Authorization: {{admin_token}}` | `200` |
| 2 | `GET /users/me` with **`{{user_token}}`** — the token you already had | **`401` `code=account_suspended`**, immediately (previously it kept working until it expired) |
| 3 | `GET /admin/audit-logs?limit=5` with `{{admin_token}}` | `200`, top row `action=user.suspended`, `actor_role=admin`, your reason, `ip_address`, `request_id`, `old_values`/`new_values` JSON |
| 4 | `PATCH /admin/users/{{user_id}}/activate` then `GET /users/me` with `{{user_token}}` | `200` then `200` — state-based, nothing to re-login for |

You can also read the row straight from the database:

```sql
SELECT action, actor_role, reason, ip_address, request_id, old_values, new_values
FROM audit_logs ORDER BY created_at DESC LIMIT 5;
```

### B · Role change without a re-login (needs a `super_admin` token)

`PATCH /admin/users/:id/role` `{"new_role":"staff","reason":"..."}` → `200`, then `GET /users/me`
with **the same token** shows the new `role` on the very next request. Refresh tokens for that
user were revoked at the same time, so their old refresh token answers `401`.

### C · Launch policy flags (email verification / KYC)

Start the API with the production policy on:

```bash
REQUIRE_EMAIL_VERIFICATION=true REQUIRE_KYC_FOR_INVESTMENT=true PORT=8081 ./bin/api.exe
```

| Request (with an ordinary, unverified user's token) | Expect |
|---|---|
| `POST /investments` | `403` `code=email_not_verified` |
| `POST /wallet/withdraw` | `403` `code=email_not_verified` |
| `POST /offplan/projects/:id/subscribe` | `403` `code=email_not_verified` |
| `GET /users/me`, `GET /properties` | `200` — reads are never blocked |
| `UPDATE users SET email_verified=true` (psql), retry `POST /investments` | `403` `code=kyc_required` (KYC gate now exposed) |

Restart **without** those variables (the repo `.env` ships `false`) and the same calls return
business codes again (`409 resource_unavailable`, `402 insufficient_funds`). The gate only ever
applies to money movement: invest, off-plan subscribe, withdraw.

### D · The local money loop (deposit → webhook → credit)

```bash
PAYMENT_PROVIDER=mock PORT=8081 ./bin/api.exe   # mock verifies charges locally
```

1. `POST /wallet/deposit` `{"amount":750000}` → `200` + `reference` (e.g. `DEP-0F69CE73-5A0`).
2. `go run ./tools/trigger_webhook DEP-0F69CE73-5A0` (from the repo root — it loads `.env` and
   signs with `PAYSTACK_WEBHOOK_SECRET`) → `Status: 200`.
3. `GET /wallet` → balance up exactly `750000` kobo; `payments.status=success`; one ledger row.
4. Run the tool **again** → `200`, balance and ledger unchanged (idempotent duplicate delivery).

> With the repo's default `PAYMENT_PROVIDER=hybrid` the tool still gets `200` for the signature,
> but the credit is refused because Paystack has no such charge — that is the mandatory
> server-side verification doing its job, not a bug.

### E · Seed a wallet without the API

```bash
sed "s/YOUR-USER-ID-HERE/<uuid>" scripts/credit_wallet.sql | docker exec -i propvest_postgres psql -U propvest -d propvest
```

Credits ₦100,000 and writes a ledger row attributed to `user_id` (the script had three wrong
columns before Phase 1 and failed outright).

### F · Concurrency / idempotency suite (what CI runs)

```bash
go test ./tests/integration/ -run 'TestWebhook_DuplicateDeliveryCreditsOnce|TestConcurrentInvestment_NeverOversellsLastSlot|TestConcurrentWithdrawals_CannotOverdrawWallet' -v
```

Needs the test database from §2 (`propvest_postgres_test` on `:5433`); all three must be `PASS`.

---

## 11. Phase 2 checks (async machinery: worker, outbox, notifications, admin health)

Added 25 Oct 2026 with Phase 2 of the readiness plan. These verify the pieces that run **outside** the
API process — the worker, the transactional outbox, the queue, and the notification fan-out.

> **Rule for every check below:** the worker must be running (`go run ./cmd/worker`, log line
> `consumer started queue=propvest.business.events`). The API alone is not enough.

### A · Worker heartbeat (container healthcheck)

The worker has no HTTP port, so it touches a file every 30 s; the compose healthcheck fails if the
file is older than 2 min.

```bash
# On Windows the file resolves to C:\tmp\propvest-worker-heartbeat; on Linux/macOS it's /tmp/.
ls -la /tmp/propvest-worker-heartbeat C:/tmp/propvest-worker-heartbeat 2>/dev/null
# mtime should be within the last ~30s, continuously
```

> **Windows/Git-Bash gotcha (real):** a bare `[ -f /tmp/propvest-worker-heartbeat ]` test can report
> MISSING even while the file is being refreshed — the Go worker writes to `C:\tmp`, and the shell's
> `/tmp` may map elsewhere. Trust `ls -la /tmp/ | grep heartbeat`, which shows the live file.

If the file stops updating, the worker's main loop is wedged (or it never started) — `docker ps`
would show it `unhealthy` and `restart: unless-stopped` would recycle it.

### B · Admin health endpoints (needs an admin token — §6)

```bash
B=http://localhost:8081/api/v1
GET  $B/admin/health/system   # → 200 "status":"healthy" — components: postgresql (latency_ms),
                              #   email ("smtp via smtp-relay.brevo.com"), paystack, rabbitmq,
                              #   cloudinary, sms_provider ("disabled by launch scope … logged, not sent")
GET  $B/admin/health/queues   # → 200 "status":"healthy", queues.outbox.depth (should be 0),
                              #   outbox.oldest_message_age_seconds, queues.outbox_failed.depth
```

**Regression to watch for:** if `queues.outbox.depth` climbs and `status` flips to `unhealthy`, events
are being written but not consumed — that is exactly the P0-6 class of bug Phase 2 fixed. A fresh
system sits at `depth: 0`.

### C · Outbox → queue → notification, end to end

This is the core Phase 2 path. Do it live:

```bash
# 1. Fund the QA wallet (§10D/§10E), then invest:
POST $B/investments   {"property_id":"<uuid>","slots":1}    # → 201, same tx writes outbox event

# 2. Within ~2s the worker publishes it — worker log shows:
#    ✓ event published  event_type=investment.created queue=propvest.business.events
#    notification created from business event  type=investment_created

# 3. The in-app row now exists:
GET  $B/notifications?page=1&page_size=3   # → 200, unread_count ≥1, type=investment_created,
                                            #   title="Investment confirmed"

# 4. Mark it read and watch the badge drop:
PATCH $B/notifications/<id>/read           # → 200 "Notification marked as read"
```

Then confirm the **frontend badge** matches: sign in on `http://localhost:3000`, the header 🔔 count
equals `unread_count`, the dropdown lists the same items with relative timestamps, and marking read
decrements it. (Live-verified 25 Oct 2026: badge showed `2`, dropdown listed "Investment confirmed"
+ "Withdrawal successful".)

### D · Withdrawal finalization (the ledger `updated_at` regression)

`wallet_transactions` is an append-only ledger with **no `updated_at` column**. An earlier version
wrote one during finalization, which failed, rolled the transaction back, and left withdrawals stuck
in `pending` forever.

```bash
POST $B/wallet/withdraw   {"amount_kobo":5000000,"bank_code":"058","account_number":"..."}
# wait for the worker / webhook, then:
GET  $B/wallet/transactions   # the withdrawal row must reach status=completed (or failed)
                              # — NOT stuck at pending. Worker log: "✓ reconciliation complete"
```

If a withdrawal sits `pending` across two reconciliation cycles (~20 min), this regressed.

### E · Business-event types that must route

Every one of these writes an outbox event in the same transaction as its business write, and the
worker must route it to `propvest.business.events` (otherwise it retries 10× then fails permanently
and shows up as `outbox_failed` in §11B):

| Trigger | `event_type` | Notification created |
|---|---|---|
| `POST /investments` | `investment.created` | `investment_created` |
| Investment cancelled / completed | `investment.cancelled` / `investment.completed` | `investment_cancelled` / `investment_completed` |
| Property funded / completed | `property.funded` / `property.completed` | `property_*` |
| Property published (admin) | `property.published` | (admin fan-out) |
| Wallet deposit / withdrawal | `deposit.*` / `withdrawal.*` | `deposit_*` / `withdrawal_*` |

After any of the above, `GET /admin/health/queues` should still show `outbox_failed.depth = 0`.

---

## 12. Phase 3 checks (ops: staging container, backup & restore)

These aren't Postman requests — they verify the deploy/ops layer. Every command runs from `propvest-backend/` with the stack from §2 already up.

### a. Build the staging image

```bash
docker build -t propvest-api:staging .
```

Expected: two stages (builder → alpine runtime), `Successfully tagged propvest-api:staging`.
*(On this IPv6-only machine the build additionally needs base images loaded with `crane` and a host CONNECT proxy passed as `--build-arg http(s)_proxy=…` — see the Phase 3 note in [PRODUCTION_READINESS_PLAN.md](PRODUCTION_READINESS_PLAN.md). Normal networks need none of that.)*

### b. Boot it on the compose network and probe it

```bash
docker run -d --name probe --network propvest-backend_default --env-file .env \
  -e APP_ENV=staging -e PORT=8081 \
  -e DATABASE_URL='postgres://propvest:password@postgres:5432/propvest?sslmode=disable' \
  -e REDIS_URL='redis://redis:6379/0' \
  -e RABBITMQ_URL='amqp://propvest:password@rabbitmq:5672/' \
  propvest-api:staging
```

Expected:

| Check | Command | Expected |
|---|---|---|
| Startup | `docker logs probe` | `migrations completed successfully (current version: 28)`, `connected to RabbitMQ`, `email provider: SMTP`, **zero** `GIN-debug` lines (staging runs release mode) |
| Liveness | `docker exec probe wget -qO- http://localhost:8081/health` | `"status":"healthy"` |
| Readiness | `docker exec probe wget -qO- http://localhost:8081/health/ready` | `database` + `queue` components `healthy` |
| Real route | `docker exec probe wget -qO- http://localhost:8081/api/v1/properties` | live property list (same rows as §5) |

### c. Fresh-DB migration run

```bash
docker exec propvest_postgres psql -U propvest -d postgres -c "CREATE DATABASE propvest_probe;"
# re-run the container from (b) with DATABASE_URL ...@postgres:5432/propvest_probe?sslmode=disable
docker exec propvest_postgres psql -U propvest -d propvest_probe -tAc \
  "SELECT version FROM schema_migrations; SELECT count(*) FROM information_schema.tables WHERE table_schema='public';"
```

Expected: `28` then `23` — an empty database goes from nothing to fully migrated on boot. CI runs the same check on every push (`migrations` job in `.github/workflows/ci.yml`).

### d. Worker container + heartbeat healthcheck

```bash
docker run -d --name wprobe --network propvest-backend_default --env-file .env \
  -e APP_ENV=staging \
  -e DATABASE_URL='postgres://propvest:password@postgres:5432/propvest?sslmode=disable' \
  -e REDIS_URL='redis://redis:6379/0' \
  -e RABBITMQ_URL='amqp://propvest:password@rabbitmq:5672/' \
  propvest-api:staging ./worker
docker logs wprobe          # "consumer started" per queue, "✓ outbox dispatcher started"
docker exec wprobe find /tmp/propvest-worker-heartbeat -mmin -2   # prints the file
```

⚠️ Stop the native `worker.exe` first — two live consumers round-robin the same queues, and the container one would process against a different config.

### e. Backup + restore drill

```bash
bash scripts/backup_db.sh       # -> backups/propvest_<stamp>.dump (148 KB here)
bash scripts/restore_drill.sh   # PASS: 23 tables, all row counts identical
```

Expected exit code 0 and the last two lines `23 tables match` / `PASS … all row counts identical`. The drill fails on any table/row mismatch or any non-benign `pg_restore` error. RPO/RTO guidance: [scripts/README.md](scripts/README.md).

### f. Compose healthchecks

```bash
podman compose ps
```

Expected: `api` **healthy** (wget `/health` every 15 s), `worker` **healthy** (heartbeat file < 2 min old).

```bash
# Cleanup
docker rm -f probe wprobe
docker exec propvest_postgres psql -U propvest -d postgres -c "DROP DATABASE propvest_probe;"
```

---

## 13. Phase 4 checks (support tickets, transfer earnings, frontend quality)

Everything in Phase 4 is exercised by the API and the browser. Backend first (Postman).

### 13.1 Support tickets (new in Phase 4.2)

As a regular user (`{{user_token}}`):

| # | Request | Expect |
|---|---|---|
| 1 | `POST /support/tickets` `{"subject":"QA withdrawal stuck","category":"Withdrawal","message":"Money left my wallet 3 days ago"}` | `201`; body has `ticket_number` like `TKT-000001`, `status: "open"`, `reply_count: 1` — the opening message is stored as the first reply, so create and reply render as one thread |
| 1b | Same body but `"category":"withdrawal"` (lowercase) | `422` — the category allowlist is **display labels**: `Rental Income`, `Distribution`, `Investment / Slots`, `KYC / Verification`, `Documents`, `Withdrawal`, `Off-Plan`, `Other` (exactly the 8 options on the Support page's dropdown) |
| 2 | `POST /support/tickets` with an empty subject | `422` `validation_error` with the field map (not `400`). Note the keys are Go field names: `Subject`, `Category`, `Message` |
| 3 | `POST /support/tickets` with `"category":"nonsense"` | `422` `validation_error` — the category allowlist lives in the service |
| 4 | `GET /support/tickets` | `200`; `{tickets: [...], pagination}` — your ticket listed |
| 5 | `GET /support/tickets/<someone-elses-uuid>` | `404` `ticket_not_found` (ownership is enforced — IDs don't leak existence) |
| 6 | `POST /support/tickets/<id>/replies` `{"message":"Any update?"}` | `201`; a follow-up `GET /support/tickets/<id>` shows 2 replies |
| 7 | `PATCH /support/tickets/<id>` | `404` — **there is intentionally no user-side status change.** Users reply; agents own the lifecycle. Same for a reply to a closed ticket: `409` `ticket_closed` |
| 8 | As admin: `GET /admin/support/tickets?status=open` | `200`; sees the ticket **with the user's email** |
| 9 | As admin: `POST /admin/support/tickets/<id>/replies` `{"message":"We're on it"}` | `201`; replying auto-moves an open ticket to `in_progress` (no separate status click) — the user's next `GET /support/tickets/<id>` shows the reply and the new status |
| 10 | As admin: `PATCH /admin/support/tickets/<id>/status` `{"status":"resolved"}` | `200`; `closed_at` is stamped only on `closed`. Then close it and try replying as the user → `409 ticket_closed` |

### 13.2 Wallet transfer earnings (4.3)

`POST /wallet/transfer-earnings` `{"amount":500000}` (kobo). With earnings ≥ amount → `200` and the wallet balances return updated. With no/short earnings → `422 insufficient_earnings` *(observed)* — a business error mapped to a 4xx, never a 500. The frontend Add-Funds widget now renders account details from the API instead of the old hard-coded "PropVest Microfinance Bank" row.

### 13.3 Frontend quality gates (4.4/4.5)

```bash
cd ../propvest-app
npm run lint    # 0 errors (legacy style warnings are expected)
npm test        # 10 tests: kobo→₦ normaliser, error mapping, 4xx/5xx handling, bell token gate
```

In the browser (`npm run dev`):
1. Signed out, open the landing page → the **notification bell must not** fire a `/notifications` request (check DevTools → Network). This used to 401 on every page load.
2. Submit the login form with a bad password → the field-level error renders under the field (422 `errors` map), not just a toast.
3. Open **Support**, create a ticket, see it listed, reply, close it → all against the API (Application tab shows real `/support/*` calls, no local state only).
4. Open **Documents** → deeds/statements load per property from `/properties/:id/documents`.

---

## 14. Phase 5 checks (probes, load smoke, webhook signature, restore drill)

Everything in the go-live gate you can reproduce yourself on a running local stack:

### 14.1 Probes (liveness / readiness)

```bash
curl -i http://localhost:8081/health        # 200, no dependency checks — liveness
curl -i http://localhost:8081/health/ready # 200, per-component detail (database critical, queue) — readiness
```

### 14.2 Load smoke (no 5xx, p95 < 500 ms)

```bash
node scripts/load_smoke.js --n 8 --rounds 2   # exit 0 = PASS at target concurrency 8
node scripts/load_smoke.js --n 12 --rounds 1  # expected FAIL: bcrypt CPU ceiling (see plan §Phase 5)
```

Each virtual user sends its own `X-Forwarded-For` through the trusted loopback hop — this models distinct clients behind a production reverse proxy, **not** a bypass: direct (untrusted) peers' forged headers are ignored since the `TRUSTED_PROXIES` fix.

### 14.3 Webhook signature test

```bash
# rejected (no signature)                     → 400 Missing signature header
curl -i -X POST http://localhost:8081/api/v1/webhooks/payment -H 'Content-Type: application/json' -d '{"event":"charge.success","data":{"reference":"FAKE-1","amount":999999999}}'
# rejected (forged signature)                 → 400 Invalid signature
curl -i -X POST http://localhost:8081/api/v1/webhooks/payment -H 'Content-Type: application/json' -H 'X-Paystack-Signature: 0000…' -d '{"event":"charge.success","data":{"reference":"FAKE-1"}}'
# accepted (valid HMAC-SHA512, real reference):
go run ./tools/trigger_webhook DEP-XXXXXXXX-XXX
```

Expect: 200 for the signed one; the wallet is credited **only if** Paystack verification of that reference passes (a fabricated reference stays `failed` — webhook alone can't credit), and re-sending the same webhook must not double-credit. With **live** keys, repeat from the production origin (plan §Phase 5 needs-you #2).

### 14.4 Backup + restore drill

```bash
bash scripts/backup_db.sh       # → backups/propvest_<ts>.dump
bash scripts/restore_drill.sh   # restores into propvest_restore_drill, compares tables + row counts → PASS
```

### 14.5 Rate-limit identity (regression for the X-Forwarded-For fix)

```bash
# 12 requests with ROTATING forged XFF, arriving from a non-loopback address:
#   expect 10×401 then 429 — one shared bucket keyed on the real peer.
# Before the fix: 11/12 passed (fresh bucket per forged value).
for i in $(seq 1 12); do curl -s -o /dev/null -w '%{http_code} ' -X POST http://<LAN-IP>:8081/api/v1/auth/login \
  -H 'Content-Type: application/json' -H "X-Forwarded-For: 198.51.100.$i" \
  -d '{"email":"nobody@example.com","password":"WrongPass1!"}'; done; echo
```

---

## 15. Cleanup

```sql
-- Remove QA users and everything hanging off them
DELETE FROM support_replies    WHERE ticket_id IN (SELECT id FROM support_tickets WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa_%@example.com'));
DELETE FROM support_tickets    WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa_%@example.com');
DELETE FROM wallet_transactions WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa_%@example.com');
DELETE FROM wallets            WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa_%@example.com');
DELETE FROM refresh_tokens     WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa_%@example.com');
DELETE FROM investments        WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'qa_%@example.com');
DELETE FROM users              WHERE email LIKE 'qa_%@example.com';
DELETE FROM properties         WHERE title LIKE 'QA %';
```

```bash
# Stop infrastructure (keeps data)
podman compose stop postgres redis rabbitmq
# Stop test infrastructure
podman compose -f docker-compose.test.yml down
# API: Ctrl+C in its terminal (or kill the recorded PID)
```

---

## 16. Suggested reading order

1. Run the collection, note which `KNOWN P0-*` tests are red.
2. Open [PRODUCTION_READINESS_PLAN.md](PRODUCTION_READINESS_PLAN.md) → Phase 0 (seven tasks) clears 9.1–9.5; Phase 1 adds §10 above; Phase 2 adds §11 (async machinery — worker/outbox/notifications); Phase 3 adds §12 (staging container, backup/restore drill); Phase 4 adds §13 (support tickets, transfer earnings, frontend quality); Phase 5 adds §14 (probes, load smoke, webhook signature, rate-limit identity).
3. Re-run the collection: every folder should be green end-to-end before a staging deploy. The remaining go-live items are **needs-you** (commit/push so CI can run, live Paystack keys + production-origin webhook test, KYC flag decision, audit-trail split, production origins + `TRUSTED_PROXIES`, rollback rehearsal) — enumerated in [PRODUCTION_READINESS_PLAN.md](PRODUCTION_READINESS_PLAN.md) §Phase 5.

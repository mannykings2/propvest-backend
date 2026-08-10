# 02 — Current State (What's Actually Built)

> Based on a direct file-by-file audit of `internal/`, `cmd/`, and `internal/database/migrations/` as of this handoff. Line references are `path:line`.

## Progress vs. the roadmap

| Milestone | Roadmap says | Reality | Real % |
|---|---|---|---|
| 0 — Foundation | ✅ | Layering, DI, repos, services, errors, response envelope all present | **~95%** |
| 1 — Authentication | ✅ | register/login/refresh(+rotation)/logout/logout-all done. **Email verify, password reset, forgot-password NOT built** (roadmap lists them under M1). RBAC middleware exists but is **unused**. | **~70%** |
| 2 — User Management | ✅ | profile get/update, avatar upload, password change, phone-change via OTP done | **~90%** |
| 3 — Wallet | 🔜 next | **Stub only.** Service methods commented out; no handler; routes commented out | **~5%** |
| 4 — Property | ⬜ | Model + migration + DB indexes exist. No repo/service/handler/routes/DTO | **~10%** |
| 5 — Investment | ⬜ | Nothing beyond references | **0%** |
| 6 — Notifications | ⬜ | Nothing (SMS util exists; email/in-app absent) | **~2%** |
| 7 — Admin | ⬜ | Nothing (RBAC middleware ready) | **0%** |
| 8 — Hardening | ⬜ | No tests, no CI, no structured logs, no metrics, no rate limiting wired | **~0%** |

Root status files (`MILESTONE_2_COMPLETE.md`, `SCHEMA_FIXES_COMPLETE.md`) claim "~35% complete, M0–M2 100%." The **M0–M2 features that exist do work**, but "100%" overstates M1 (missing email verification & password reset) and ignores the missing cross-cutting foundations (logging, tests, CI).

## Directory reality (verified on disk)

```
cmd/
├── api/main.go          # composition root — real DI wiring
└── worker/main.go       # EMPTY STUB: func main() {}

internal/
├── auth/                # EMPTY directory (leftover from an older doc layout)
├── config/              # Viper config loader
├── database/            # GORM connect + migration runner + migrations/*.sql
├── dto/                 # FLAT files (auth_dto, user_dto, wallet_dto, error_dto, mappers)
├── errors/              # AppError + sentinal errors + HTTP mapping
├── handlers/            # auth.go, user.go, health.go
├── middleware/          # auth, rbac, cors, logger, recovery, request_id
├── models/              # user, wallet, refresh_token, otp_verification, property
├── repositories/        # base, user, wallet, refresh_token, otp_verification
├── response/            # StandardResponse envelope + helpers
├── routes/v1/           # routes.go (auth + users active; rest commented out)
├── services/            # auth_service, user_service, wallet_service(STUB)
├── utils/
│   ├── cloudinary/  jwt/  otp/  password/  sms/  token/
└── validators/          # password, phone, generic
pkg/
└── utils/               # EMPTY
```

## What works (implemented and wired)

**Auth (`/api/v1/auth`)**
- `POST /register` — validates email+phone+password, bcrypt-hashes, creates **user+wallet in one DB transaction**, issues JWT access+refresh, stores hashed refresh token. `auth_service.go:117`.
- `POST /login` — generic invalid-credentials error, active-account check, bcrypt verify. `auth_service.go:303`.
- `POST /refresh` — **real refresh-token rotation**: validate JWT → look up hash → check validity → check owner → revoke old → issue new pair → store new hash. `auth_service.go:413`.
- `POST /logout`, `POST /logout-all` — revoke one / all refresh tokens.

**Users (`/api/v1/users`, all behind `middleware.Auth`)**
- `GET /me`, `PATCH /me`, `PATCH /avatar` (Cloudinary), `PATCH /password` (revokes all sessions), `POST /phone/request` + `POST /phone/verify` (OTP via SMS mock).

**Foundation**
- Config via Viper; DB connect with pool tuning; migrations run on boot via golang-migrate; middleware stack (Recovery → RequestID → Logger → CORS) wired in `main.go:155`; standard response envelope; centralized `AppError` + `HTTPStatusFromError` mapping; repository pattern with a `BaseRepository.Transaction` helper; JWT/bcrypt/OTP utils.

**Health:** `GET /api/v1/health` returns `{status, service, version}`.

## What is stubbed or missing

| Area | State | Evidence |
|---|---|---|
| Wallet service | All methods commented out; impl has no methods | `internal/services/wallet_service.go:9-29` |
| Wallet handler / routes | Do not exist; service discarded | `cmd/api/main.go` (`_ = walletService`), `routes.go:107-114` |
| Worker binary | Empty `func main(){}` | `cmd/worker/main.go` |
| Email verification / password reset / forgot-password | Not implemented (routes commented) | `routes.go:67-71` |
| RBAC on routes | Middleware exists, never applied | `internal/middleware/rbac.go` vs commented routes |
| Property feature | Model + migration only; no repo/service/handler/DTO/routes | `internal/models/property.go` (no consumers) |
| Investment / Notification / Admin | Absent | — |
| Payments (Paystack) | Referenced in comments/env only; no code, no config struct fields | `routes.go:183`, `.env.example` |
| Redis / Asynq / jobs / cache | Config field `REDIS_URL` only; no client, no worker | `config.go` |
| Real SMS (Twilio/Termii) | Placeholder `log.Printf` + TODO | `internal/utils/sms/sms.go:90-160` |
| Structured logging | Uses stdlib `log.Printf` | `internal/middleware/logger.go:29` (`// TODO: Zap`) |
| Tests | **Zero** `*_test.go` in repo | — |
| CI/CD | No `.github/` or any pipeline | — |
| Swagger/OpenAPI | None | — |
| Graceful shutdown | `r.Run()` only; no `http.Server` timeouts | `cmd/api/main.go` |

## Database schema (as of migration 000006)

Tables that exist: `users`, `wallets`, `wallet_transactions`, `properties`, `refresh_tokens`, `otp_verifications` (+ golang-migrate's `schema_migrations`).

Migration history:
- `000001_init_schema` — users, wallets (incl. `currency`), wallet_transactions, properties, `update_updated_at_column()` trigger + triggers, CHECK constraints, indexes.
- `000002_add_refresh_tokens` — refresh_tokens + FK + indexes.
- `000003_add_otp_verifications` — otp_verifications (note: uses plain `TIMESTAMP`, not `TIMESTAMPTZ` — inconsistent with other tables).
- `000004_add_user_avatar_and_email_verified` — adds `users.avatar_url`, `users.email_verified`.
- `000005_add_wallet_currency` — idempotent reconciliation (currency already in 000001 for fresh DBs).
- `000006_add_refresh_tokens_timestamps` — adds `refresh_tokens.updated_at` + `deleted_at` + trigger (GORM soft-delete/timestamps).

**Note on 000005/000006:** these are "catch-up" migrations created because the model was edited but the *already-applied* migration wasn't. On a **fresh** database they are near-no-ops. They're harmless but signal the schema-drift workflow problem documented in `03-ISSUES-AND-CORRECTIONS.md`.

## Money & schema modeling note (important for Wallet/Investment work)

The **code** models money as `int64` **kobo** across `wallets.main_balance`, `wallets.earnings_balance`, `wallet_transactions.amount`, and property prices. The **docs** are split:
- `2.2-DATABASE_DESIGN.md` shows `wallet.balance DECIMAL(18,2)` and a single `balance` field.
- `2.3-TRANSACTION_MODEL.md` and `starter-guide.md` and the actual code use **integer minor units** and **two balances** (`main` + `earnings`).

The **code's integer-kobo, two-balance, append-only ledger design is the better one** (no float rounding, matches the transaction-model anti-patterns section) and is already in the DB. Keep it. Treat `2.2`'s `DECIMAL`/single-`balance` as outdated. This is logged in `04-DECISIONS-TO-RESOLVE.md`.

## The `wallet_transactions` vs `transactions` question

- Code/DB has **`wallet_transactions`** (wallet_id, type, amount, balance_before, balance_after, reference (unique), description, status, created_at).
- `2.3-TRANSACTION_MODEL.md` describes a richer **`transactions`** table (adds user_id, direction, currency, source, external_reference, idempotency_key, metadata) and a separate **`payments`** table (roadmap M3 also lists `payments` + `transactions`).

You do **not** need to rename the table. The pragmatic path is to **extend `wallet_transactions`** with the fields the transaction model and idempotency require (`user_id`, `external_reference`, `idempotency_key`/reuse `reference`, `metadata`, richer `type`/`status` enums) via a new migration, and add a `payments` table for provider-side records. See `05-COMPLETION-PLAN.md` M3.

# 04 — Decisions To Resolve (Doc/Code Contradictions)

> The docs are the "source of truth," but they contradict each other and the code in specific places. The genesis prompt says: when docs conflict, **stop, explain, recommend, get approval**. This file does the "explain + recommend" so the next session can get a quick decision from the product owner.
>
> Each item: the conflict, the options, and a **recommended canonical answer** to standardize on (and then update the losing doc so the set is consistent again).

---

## D1 — HTTP framework: chi vs Gin  →  **Gin (settled)**

- `docs/starter-guide.md` (Steps 5, 11) says **chi**.
- Entire codebase + `docs/01-Architecture/1.1` §44 use **Gin**.

**Recommendation:** Gin. It's built, it works, `1.1` ratifies it. **Action:** mark `starter-guide.md` as historical (add a note at the top) so no one re-introduces chi.

---

## D2 — `docs/CLAUDE.md` is for the wrong stack  →  **Replace with a Go agent guide**

`docs/CLAUDE.md` is a TypeScript/Fastify/Next.js/vitest/prettier/turbo/Kysely ruleset (`packages/api`, `*.spec.ts`, `import type`, branded TS types). **None of it applies** to this Go project. An agent that follows it will do the wrong thing (e.g., "colocate `*.spec.ts`", "run `turbo typecheck`").

**Recommendation:** Do not follow `docs/CLAUDE.md` for Go work. Keep it only if some sibling TS repo uses it. Create a **Go-specific** agent guide (this handoff folder is a start) covering: `go test ./... -race -cover`, table-driven tests, `golangci-lint`, error wrapping with `%w`, `slog`, the layer rules from `1.3`. **Action:** get owner confirmation, then write `AGENTS.md`/a Go `CLAUDE.md` at repo root.

---

## D3 — Folder layout: which doc is authoritative?  →  **`1.3-BACKEND_FOLDER_STRUCTURE.md` (v2.0)**

Three layouts exist: `starter-guide.md` (`internal/db/models`, per-domain `services/auth` etc.), `1.1` §9 (lists `internal/auth/`), and `1.3` v2.0 (cross-cutting `handlers/services/repositories/...`, **"supersedes every previous folder organization proposal"**).

The **code follows `1.3`** (cross-cutting) — correctly — with three deviations:
1. `internal/auth/` exists but is **empty** (tracks the older `1.1`; `1.3` has no `auth` package). → **delete it.**
2. `dto/` is **flat**; `1.3` wants feature subfolders (`dto/auth/`, `dto/wallet/`, …). → **acceptable to keep flat while small; migrate to subfolders when DTOs grow** (M4+). Low priority.
3. `routes/v1/` subfolder isn't in `1.3` but matches the `/api/v1` versioning mandate in `1.1` §15. → **keep** `routes/v1/`.
4. Missing packages `1.3` prescribes: `logger, events, jobs, notifications, payments, storage, cache, constants`. → **create them as their features land** (logger first — see M0.5).
5. `cmd/worker` exists but `1.3` says only `cmd/api`. → **keep worker** (background jobs need it; `1.3` simply didn't anticipate it). Update `1.3` to allow `cmd/worker`.

**Recommendation:** Treat `1.3` as the folder authority, with the pragmatic exceptions above. **Action:** reconcile `1.1` §9 to point at `1.3`.

---

## D4 — Money type: `DECIMAL(18,2)` vs integer kobo  →  **Integer kobo (keep code)**

- `2.2-DATABASE_DESIGN.md` §12 shows `wallet.balance DECIMAL(18,2)` and a **single** `balance`.
- Code + DB + `2.3-TRANSACTION_MODEL.md` §24 (anti-patterns: "never use floating-point for money") + `starter-guide.md` use **`int64` kobo** with **two** balances (`main_balance`, `earnings_balance`).

**Recommendation:** **Integer minor units (kobo).** It avoids float/decimal rounding, is already in the schema with CHECK constraints, and matches the transaction-model anti-patterns. The two-balance split (spendable vs earnings) is a real product requirement. **Action:** update `2.2` to `BIGINT` kobo + two balances so the schema doc stops contradicting reality. When you serialize to the frontend, decide whether the API returns kobo (integer) or naira (string/decimal) — recommend returning **integer kobo** + a `currency` field and letting the client format; document it in `3.2`.

---

## D5 — Transaction table: `wallet_transactions` vs `transactions` + `payments`  →  **Extend `wallet_transactions`, add `payments`**

- Code/DB: `wallet_transactions` (append-only ledger, unique `reference`).
- `2.3` and roadmap M3: a richer `transactions` table (+`user_id, direction, currency, source, external_reference, idempotency_key, metadata`) **and** a `payments` table.

**Recommendation:** Don't rename. **Extend `wallet_transactions`** with `user_id`, `external_reference`, `idempotency_key` (or formally treat `reference` as the idempotency key), `metadata JSONB`, and widen the `type`/`status` enums to the `2.3` set (DEPOSIT/WITHDRAWAL/INVESTMENT/REFUND/REVERSAL/FEE/ADJUSTMENT/… and PENDING/PROCESSING/COMPLETED/FAILED/REVERSED). Add a **`payments`** table for provider-side records (provider, reference, amount, status, raw payload). **Action:** capture this in a new migration during M3 and update `2.3` to name the table `wallet_transactions`.

---

## D6 — Error-response envelope shape  →  **Adopt the documented `{success,message,errors,request_id}`**

Covered in `03-ISSUES-AND-CORRECTIONS.md` P2. The code's `{...,error,code,...}` diverges from `6.2`'s `{...,errors,...}` and the code has two error shapes.

**Recommendation:** Converge on the doc shape now (few endpoints exist). `errors` = field→[]string for validation; single-message form for domain errors. Keep an optional internal `code` only if applied consistently. **Action:** refactor `internal/response` + update `3.2`/`6.2` if you keep `code`.

---

## D7 — Password hashing: bcrypt vs Argon2id  →  **Keep bcrypt now, document it**

Docs (`4.2` §8, `6.1` §28) **prefer Argon2id**, list bcrypt as "acceptable." Code uses bcrypt cost 10.

**Recommendation:** bcrypt is fine and built; **keep it** to avoid churn, but (a) read the cost from `BCRYPT_COST` env (documented, currently unused), and (b) record the decision. Migrating to Argon2id later is possible via `NeedsRehash`-on-login. **Action:** none required beyond wiring `BCRYPT_COST`; optionally note the deviation in `4.2`.

---

## D8 — Access/refresh token TTLs & the two-secret scheme  →  **Confirm values**

- `4.2` recommends access **15 min**, refresh **30 days**; `1.1` says access 15–30 min, refresh 7–30 days.
- Code reads `ACCESS_TOKEN_TTL` / `REFRESH_TOKEN_TTL` from env and uses **two separate secrets** (`JWT_SECRET`, `JWT_REFRESH_SECRET`). `starter-guide.md` used a single secret and `REFRESH_TOKEN_TTL=720h` (30 days).

**Recommendation:** Keep the **two-secret** scheme (it's stronger — a leaked access-secret can't mint refresh tokens). Standardize TTLs at **15m / 720h (30d)**. **Action:** ensure `.env`/`.env.example` set both; note that `time.ParseDuration` errors are currently ignored in `auth_service.go` (`accessTokenTTL, _ := ...`) → validate these at startup so a malformed TTL fails fast instead of silently becoming 0.

---

## D9 — Health/readiness endpoint paths  →  **Pick one set**

- `1.1` §28: `/health`, `/ready`, `/metrics`.
- `7.1` §23: `/health`, `/health/live`, `/health/ready`.
- Code: only `/api/v1/health`.

**Recommendation:** Provide **`/health` (liveness)**, **`/health/ready` (readiness: checks DB, and Redis once added)**, and **`/metrics`** (Prometheus). Note these should arguably live at the **root**, not under `/api/v1`, so orchestrators hit `/health` not `/api/v1/health`. **Action:** decide root vs versioned; update whichever doc loses.

---

## D10 — Register request fields: `first_name/last_name/role` vs `full_name`  →  **`full_name`, no client role**

- `3.2` §15 register example uses `first_name`, `last_name`, and a client-supplied `role`.
- Code/DB uses a single **`full_name`** and **ignores any client role** (always `investor`; role is admin-assigned).

**Recommendation:** Keep **`full_name`** (simpler, matches DB and `user` model) and **never accept `role` from the client** (privilege-escalation risk — the code is correct to hardcode `investor`). **Action:** fix the `3.2` register example to match (`full_name`, `phone`, no `role`). Also `3.2`'s `GET /users/me` shows `first_name/last_name` — update to `full_name`.

---

## D11 — DB port 5432 vs 5435  →  **5435 (compose reality)**

`docker-compose.yml` maps Postgres to host **5435**; `Makefile` migrate targets use 5435; `starter-guide.md` and some docs say **5432**.

**Recommendation:** Standardize on **5435** for local (avoids clashing with a native Postgres on 5432). **Action:** ensure `.env`/`.env.example` `DATABASE_URL` uses 5435; fix stale 5432 references in docs.

---

## Summary table

| # | Conflict | Recommended canonical | Doc to update |
|---|---|---|---|
| D1 | chi vs Gin | **Gin** | starter-guide (mark historical) |
| D2 | CLAUDE.md wrong stack | **Write Go agent guide** | replace CLAUDE.md |
| D3 | folder authority | **1.3 v2.0** (+worker, +routes/v1) | 1.1 §9 |
| D4 | money type | **int64 kobo, 2 balances** | 2.2 |
| D5 | tx table shape | **extend wallet_transactions + add payments** | 2.3 |
| D6 | error envelope | **{success,message,errors,request_id}** | response pkg + 6.2/3.2 |
| D7 | bcrypt vs argon2 | **bcrypt (wire BCRYPT_COST)** | 4.2 (note) |
| D8 | token TTLs/secrets | **15m/30d, two secrets, validate at boot** | .env(.example) |
| D9 | health paths | **/health, /health/ready, /metrics at root** | 1.1 or 7.1 |
| D10 | register fields | **full_name, server-set role** | 3.2 |
| D11 | DB port | **5435** | starter-guide + docs |

None of these should block progress for more than a quick owner confirmation. D4/D5/D6 should be locked **before** Wallet M3 because they shape the schema and the API contract.

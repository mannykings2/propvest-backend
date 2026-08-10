> ⚠️ START WITH `00-TAKEOVER-NOW.md`. It reflects the current uncommitted state (M3–M7 scaffolded, does NOT compile). The TL;DR below is now historical.

# PropVest Backend — Handoff Package

> Prepared as a takeover briefing for the next engineer/agent.
> This folder is **observation and planning only**. No application code or existing docs were modified to produce it.

---

## What this folder is

A previous developer/agent built PropVest up through **User Management** and stopped just before the **Wallet System**. This folder captures:

1. What the project is and how it is meant to be built (`01-PROJECT-OVERVIEW.md`)
2. Exactly what has been built and what is stubbed/missing (`02-CURRENT-STATE.md`)
3. Bugs, risks, and bad practices found in the current code, with concrete fixes (`03-ISSUES-AND-CORRECTIONS.md`)
4. Contradictions between docs and code (and between docs), with a recommended canonical decision for each (`04-DECISIONS-TO-RESOLVE.md`)
5. An extensive, milestone-by-milestone plan to finish the project (`05-COMPLETION-PLAN.md`)

Read them in order. If you only read one thing first, read the TL;DR below.

---

## TL;DR — where the previous developer left off

- **Milestones 0, 1, 2 are reported "100% complete"** in the root status files (`MILESTONE_2_COMPLETE.md`). In reality:
  - **Milestone 0 (Foundation):** done and solid — clean layering, DI, repositories, services, centralized errors, response envelope.
  - **Milestone 1 (Authentication):** register / login / refresh (with rotation) / logout / logout-all are done. **But email verification, password reset, and forgot-password — which the roadmap lists under Milestone 1 — are NOT built.** So M1 is closer to **~70%**, not 100%.
  - **Milestone 2 (User Management):** profile, avatar (Cloudinary), password change, phone-change via OTP are done.
- **Next up is Milestone 3 (Wallet System).** The scaffolding exists but is a **stub**: `internal/services/wallet_service.go` has every method commented out, `WalletService` is constructed in `main.go` then thrown away (`_ = walletService`), there is no `WalletHandler`, and all wallet routes are commented out in `internal/routes/v1/routes.go`.
- **There are zero automated tests and no CI**, despite the docs mandating 85% coverage.
- **Cross-cutting foundations are missing/incorrect:** logging is `log.Printf` (docs require structured JSON logs), the error-response envelope does not match the documented shape, and several `error` returns are silently swallowed.

**Recommendation:** do a short **Phase 0 remediation pass** (fix the auth bugs, add structured logging, add the test/CI harness, finish M1 auth) *before* piling on Wallet/Property/Investment features. These are cross-cutting and far cheaper to fix now than later. Details in `05-COMPLETION-PLAN.md`.

---

## Project at a glance

| Item | Value |
|---|---|
| Module | `github.com/mannykings2/propvest-backend` |
| Language | Go 1.25.0 |
| HTTP framework | Gin (`github.com/gin-gonic/gin`) |
| ORM | GORM + `gorm.io/driver/postgres` |
| Migrations | `golang-migrate/migrate/v4` (SQL files in `internal/database/migrations`) |
| Config | Viper (`.env` + env) |
| Auth | `golang-jwt/jwt/v5`, access + refresh with rotation, bcrypt (`x/crypto`) |
| Validation | `go-playground/validator/v10` (via Gin binding) + custom `internal/validators` |
| Media | `cloudinary-go/v2` |
| Not yet present | Redis client, Asynq, Paystack SDK, structured logger, tests, CI |

---

## How to run (verified from repo config, not executed here)

```bash
# 1. Start infrastructure (Postgres on host port 5435, Redis on 6379)
make up           # docker compose up -d

# 2. Apply DB migrations (uses standalone migrate CLI against localhost:5435)
make migrate-up

# 3. Run the API (defaults to :8080)
make run          # go run ./cmd/api
```

Then `GET http://localhost:8080/api/v1/health` should return a success envelope.

> Note: `docker-compose.yml` maps Postgres to host port **5435** (not 5432), and the `Makefile` migrate targets hardcode `postgres://propvest:password@localhost:5435/propvest`. Make sure your `.env` `DATABASE_URL` uses **5435**. The `docs/starter-guide.md` still says 5432 — that is stale.

---

## Important caveat about the docs

The `docs/` set is large and mostly high quality, but it is **internally inconsistent in places** and, in a few spots, **contradicts the code that was actually built**. Treat `docs/` as the intended direction, but resolve the specific conflicts listed in `04-DECISIONS-TO-RESOLVE.md` before implementing against any single doc. Two examples:

- `docs/starter-guide.md` says use **chi**; the whole codebase uses **Gin** (and `docs/01-Architecture/1.1` confirms Gin as the chosen framework). Gin wins.
- `docs/CLAUDE.md` is a **TypeScript / Fastify / Next.js** rulebook (mentions `packages/api`, vitest, prettier, turbo, Kysely). It does **not** apply to this Go project and should be replaced with a Go-specific agent guide.

# 01 — Project Overview

## What PropVest is

PropVest is a **real-estate investment platform backend**. Users ("Investors") fund an in-app **wallet**, then buy **slots/shares** in **properties** (residential, commercial, land, etc.). Properties are listed by **Developers** and approved by **Administrators**. Investors earn returns (rental income / resale gains) that accrue to an **earnings balance**. The platform tracks every money movement as an immutable **transaction ledger**.

It is a Nigerian fintech-style product:
- Money is stored in **kobo** (integer minor units; ₦1 = 100 kobo). No floats for money.
- Phone numbers are Nigerian E.164 (`+234...`).
- Intended payment provider is **Paystack** (Flutterwave later).
- SMS via Termii/Twilio; email via SMTP/Resend; media via Cloudinary.

The project has a **dual goal** (from `docs/README.md` and `docs/genesis_prompt.md`):
1. Ship a secure, scalable, production-ready backend.
2. Serve as a **teaching project** — the genesis prompt asks the agent to explain everything like a mentor to a junior engineer, and to challenge/ђdisagree rather than blindly follow.

## The actors (domain roles)

| Role | Can do |
|---|---|
| **Investor** | Fund wallet, invest in properties, view portfolio, manage profile, upload KYC |
| **Developer** | Create/list properties, upload property docs, monitor funding |
| **Administrator** | Approve/reject properties, manage users, view audit logs, dashboards |

(Future roles named in docs: Support, Finance, Compliance, Super Admin.)

## Core domain aggregates (from `docs/01-Architecture/1.2-DOMAIN_MODEL.md`)

- **User** — one user owns exactly **one Wallet**. Lifecycle: Registered → Email Verified → Active → Suspended → Archived.
- **Wallet** — balance never negative; balance changes **only** by creating a Transaction.
- **Property** — lifecycle: Draft → Submitted → Approved → Funding → Funded → Completed (or Rejected). Only approved properties accept investment.
- **Investment** — links a user to a property; amount > 0; requires sufficient wallet balance. Every investment produces a Transaction.
- **Transaction** — immutable append-only ledger. Corrections are *new* compensating entries, never edits/deletes.
- Supporting: **Notification, Document, Payment, RefreshToken, AuditLog**.

## Architecture (the intended shape)

Strict layered / clean architecture. Request flow:

```
Client (React)
   │  HTTP
   ▼
Gin Router
   ▼
Middleware:  Recovery → RequestID → Logger → CORS → RateLimit → Auth(JWT) → RBAC → Validation
   ▼
Handler   (parse DTO, call service, format response — NO business logic, NO SQL)
   ▼
Service   (business rules, orchestration, transactions — NO HTTP, NO SQL strings)
   ▼
Repository (GORM CRUD, queries, locking — NO business rules)
   ▼
PostgreSQL
```

Cross-cutting packages the docs expect: `logger`, `events`, `jobs`, `notifications`, `payments`, `storage`, `cache`, `response`, `errors`, `constants`, `validators`.

Rules that matter most (from `1.3`, `1.4`, `6.1`):
- Lower layers never import higher layers.
- Handlers never touch the DB; services never touch HTTP.
- Never return GORM models directly to clients — always map to a DTO.
- All money in integer minor units; all financial mutations inside a DB transaction with row locking.
- Every schema change is a versioned migration (no AutoMigrate in production).

## The authoritative documents (and which to trust)

The `docs/` are numbered by area. Not all are equal — several are ~30-line stubs. Ranked by usefulness for implementation:

**Detailed and authoritative:**
- `01-Architecture/1.1-SYSTEM_ARCHITECTURE.md` (~2,468 lines) — master architecture; confirms Gin, GORM, UUID PKs, layered design, `/api/v1`, rate limits, health/ready/metrics.
- `01-Architecture/1.3-BACKEND_FOLDER_STRUCTURE.md` (v2.0, "**supersedes every previous folder proposal**") — the canonical package layout. **This is the folder authority**, above `1.1` and above `starter-guide.md`.
- `01-Architecture/1.2-DOMAIN_MODEL.md` — aggregates, invariants, lifecycles.
- `01-Architecture/1.4-REQUEST_FLOW.md` — end-to-end flows incl. deposit/investment.
- `01-Architecture/1.5-FRONTEND_BACKEND_MAPPING.md` — every FE feature → endpoint → table → status. Great backlog.
- `02-Database/2.2-DATABASE_DESIGN.md` — full schema field tables per entity.
- `02-Database/2.3-TRANSACTION_MODEL.md` — the financial backbone: transaction entity, statuses, idempotency, atomicity, double-entry (future), anti-patterns.
- `03-API/3.2-API_SPECIFICATION.md` — the REST contract (endpoints, request/response, error codes).
- `04-Security/4.2-AUTHENTICATION_AND_AUTHORIZATION.md` — JWT/refresh/rotation, password policy, RBAC matrix, middleware order, rate limits, audit fields.
- `06-Engineering/6.1-CODING_STANDARDS.md`, `6.2-ERROR_HANDLING_AND_LOGGING.md`, `6.3-TESTING_STRATEGY.md` — the "how to write it" rules.
- `07-Operations/7.1-DEPLOYMENT_AND_DEVOPS.md`, `7.2-OBSERVABILITY_AND_MONITORING.md` — prod topology, CI/CD, metrics, SLOs.
- `08-Roadmap/8.1-BACKEND_IMPLEMENTATION_ROADMAP.md` — the milestone plan the previous dev followed.

**Stubs (intent only — do not expect implementable detail here):**
- `02-Database/2.1-DATABASE_ENGINEERING.md`, `03-API/3.1-API_DESIGN.md`, `07-Operations/7.3-BACKGROUND_JOBS.md`, `07-Operations/7.4-CACHE_STRATEGY.md`, and **all of `05-Modules/*` except `5.4` is barely longer** — `5.1-PAYMENT_ARCHITECTURE`, `5.2-PROPERTY_MODULE`, `5.3-INVESTMENT_MODULE`, `5.4-WALLET_DESIGN`, `5.5-NOTIFICATION_SYSTEM` are all ~30-line scope/principles scaffolds. The real detail for those modules lives in `1.2`, `2.2`, `2.3`, `3.2`, and `1.4`.

**Not applicable / misleading:**
- `docs/starter-guide.md` — an early bootstrap guide. Uses **chi**, `internal/db/models`, port 5432, `WalletAccount` naming. The real project diverged from all of these. Historical only.
- `docs/CLAUDE.md` — a **TypeScript/Fastify/Next.js** ruleset. Wrong stack entirely. Ignore for Go work; see `04-DECISIONS-TO-RESOLVE.md`.

## The development process the genesis prompt asks for

`docs/genesis_prompt.md` sets expectations for whoever works here:
- Read all docs first; treat docs as source of truth **but disagree when they're wrong** (explain why, propose better, wait for approval before large refactors).
- Teach as you go (beginner/intermediate/production explanations).
- Don't run terminal commands without explaining them / getting consent.
- Maintain continuity: reuse abstractions, preserve naming, avoid duplication.
- Every feature: explain goal → architecture → files → code → security → performance → edge cases → tests → summary → next milestone.

This is a **mentorship-style** engagement, not a code-dump. Keep changes explained and incremental.

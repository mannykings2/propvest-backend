# PropVest Milestone 7 — AI Coding Agent Execution Plan

## Mission

Implement the Administration Module for the PropVest Go/Gin backend using the repository's existing Repository → Service → Handler architecture. Deliver only behavior that is grounded in the actual codebase, tested, documented, and safe for production.

**Read first:** `MILESTONE_7_ADMINISTRATION_IMPLEMENTATION_BLUEPRINT.md` (the companion architecture plan). Use this file as the execution contract and the blueprint as the detailed technical reference.

## Non-negotiable rules

1. **Inspect before editing.** The supplied design prompt describes the intended system but may not exactly match the current repository. Verify actual files, migrations, model fields, routes, error helpers, service interfaces, and tests.
2. **Do not invent existing functionality.** If a feature such as login history, notification delivery analytics, admin withdrawal approval, refunds, investment cancellation, or returns distribution does not exist, report the gap and design it before implementing.
3. **Do not duplicate domain logic.** Admin code must call existing user/property/investment/wallet/payment/withdrawal/notification services. Do not directly change wallet balances or bypass ledger, locked-balance, provider, webhook, reconciliation, or state-machine logic.
4. **Use incremental phases.** Complete the discovery phase and report findings before making broad changes. Do not attempt the entire milestone in one large patch.
5. **Preserve compatibility.** Existing endpoints, particularly property approval and existing admin bulk notification functionality, must keep working. Extend existing patterns instead of replacing them.
6. **Audit is mandatory.** A consequential admin mutation and its audit row must commit atomically. External notifications are asynchronous through an existing outbox/queue pattern, never inside a DB transaction.
7. **Security first.** Every admin route must be authenticated and authorized. Enforce actor/target hierarchy, reason requirements, pagination bounds, allowlisted sort fields, redaction and rate limits.
8. **Financial safety.** Idempotency, transaction boundaries, concurrency behavior and reconciliation must be tested before enabling any financial mutation. If the current domain cannot support a safe action, leave it read-only or feature-disabled and report why.
9. **No false completion claims.** Never say tests/build/migrations passed unless you ran them and captured the result. Do not mark a phase complete when required tests fail.
10. **Keep changes focused.** Do not rewrite unrelated architecture, update unrelated dependencies, or add Redis/metrics/export frameworks without evidence they are needed.

---

## Phase 0 — Repository discovery (do this first; no broad code edits)

### Inspect

- API and worker entry points, dependency injection and route registration.
- Auth middleware, `RequireRole`, current role claims, session/refresh token revocation and access-token invalidation behavior.
- User, property, investment, wallet, transaction, payment, withdrawal, notification, outbox and audit models/repositories/services.
- Existing property approval/rejection endpoints and tests.
- Notification delivery attempts/events and admin bulk messaging implementation.
- Migration numbering and current schema; do not assume `000022` is the last migration.
- Error types, response envelope, logger, config, pagination DTOs, transaction conventions and CI commands.

### Produce before coding

Create/update a short `docs/MILESTONE_7_DISCOVERY.md` (or report in the agent response if project docs should not be modified yet) containing:

- Actual relevant files and interfaces.
- Existing endpoints that overlap with this milestone.
- Existing versus missing database fields/tables.
- Existing financial state machines and safe service entry points.
- Whether login history, delivery analytics, queue depth, MFA, export jobs, metrics and idempotency are available.
- Migration next-number confirmed from repository.
- Risks, scope recommendation and implementation sequence.

### Exit criteria

Do not begin broad implementation until the findings are summarized. Resolve naming conflicts by adapting the plan to the repository, not by creating duplicate systems.

---

## Phase 1 — Audit foundation

### Implement

- Audit log model and migration, using the next unused migration number.
- Repository methods for create and paginated/filterable reads. Do not implement update/delete repository methods.
- Audit service/helper with actor, role, action, target, reason, request ID, IP, user agent and redacted allowlisted before/after state.
- A transaction-aware mechanism so audit insertion shares the transaction with each domain mutation. Use existing transaction abstractions if present; avoid unnecessary GORM coupling.
- Correlation/request ID propagation if an established mechanism exists.
- Indexes for common actor/action/target/date queries based on actual filter requirements.

### Tests

- Create and query audit rows.
- Filters and stable pagination.
- Redaction/allowlist behavior.
- Business mutation rolls back if audit insert fails.
- No successful audit event is left behind if the business mutation fails.
- Audit repository exposes no mutation/delete path.

### Exit criteria

Migration up/down/up works in a disposable DB; audit atomicity tests pass; no secrets appear in audit snapshots.

---

## Phase 2 — Admin authorization and route security

### Implement

- One consistent admin route group using existing authentication and RBAC middleware.
- Explicit admin/super-admin hierarchy checks for privileged target actions.
- Current-session/role behavior verified; role changes and logout-all revoke refresh tokens and follow the actual access-token invalidation model.
- Dedicated stricter rate limits for role changes, password reset, exports and high-risk financial actions.
- Permission checks for audit reads/exports and detailed health/queue status.

### Tests

- No token → 401.
- Regular user → 403.
- Admin attempting forbidden super-admin action → 403.
- Self-promotion and privilege escalation blocked.
- Every registered admin route is protected.
- Sensitive endpoints are rate-limited.

### Exit criteria

Route inventory reviewed; no unprotected admin endpoint; authorization tests pass.

---

## Phase 3 — Dashboard and read-only operational views

### Implement

- Dashboard metrics through bounded aggregate queries.
- User list/detail and related read endpoints.
- Property admin list/analytics.
- Investment list/detail/metrics.
- Transaction/payment/withdrawal monitoring views.
- Audit-log list/detail.

### Rules

- Document the exact definition and source for each metric.
- Use integer minor units plus currency for money.
- Never call investment principal “revenue” unless that is actually the accounting definition.
- Never double-count payment rows and ledger entries.
- Never infer login history or delivery status from unrelated tables.
- Use paginated collections and stable sort; maximum page size 100.
- Allowlist sort fields; use query parameters safely.
- Avoid N+1 queries and in-memory aggregation of all records.

### Tests

- Metric fixtures reconcile to expected values.
- Time-window boundaries/UTC handling.
- Pagination, filters, sorting and empty results.
- IDOR tests for user/property/investment/transaction/audit IDs.
- Query plans reviewed for expensive dashboard and list queries.

### Cache policy

Only add Redis caching after measuring uncached queries. If justified, use short TTL, versioned keys, `data_as_of`, safe fallback on Redis failure and best-effort invalidation after successful writes. Do not cache authorization decisions or sensitive user detail by default.

### Exit criteria

Read-only APIs are documented, bounded and pass tests. Dashboard latency is measured under a stated test dataset; do not claim the 2-second target without evidence.

---

## Phase 4 — User management

### Implement only after Phase 1–2

- Allowlisted profile updates.
- Suspend/activate with reason and audit.
- KYC decisions using the existing KYC state model.
- Force password reset by triggering the existing reset workflow; never set/return a password.
- Logout-all by revoking sessions using actual auth implementation.
- Role change, super-admin-only by default, with hierarchy validation and session revocation.
- Soft deletion with checks for active investments, unresolved balances, pending withdrawals and retention requirements.
- Admin notes only if an appropriate attributable, access-controlled model is implemented.

### Atomicity and side effects

For each state-changing operation, perform current-state validation, domain mutation, audit insertion and outbox event creation (if needed) in a transaction. Send notifications asynchronously. Do not include password hashes, tokens or sensitive KYC data in audit snapshots.

### Tests

- Repeated suspend/activate behavior.
- Concurrent state changes.
- Session revocation.
- Role hierarchy/self-elevation prevention.
- KYC valid/invalid transitions.
- Soft-delete safety constraints.
- Audit row is atomic with the mutation.

### Exit criteria

All user mutations have clear state-transition rules, reasons where required, audit records and authorization tests.

---

## Phase 5 — Property administration

### Implement

- Enhanced property listing and analytics.
- Approve/reject/request-changes only using existing property service transitions.
- Reviewer ID/time and rejection reason only if equivalent metadata is absent.
- User-facing notifications through the existing outbox/notification service.
- Bulk actions only after single-record behavior is reliable and a bounded partial-success contract is documented.

### Tests

- Invalid state transitions rejected.
- Concurrent approve/reject race is safe.
- Required document checks enforced by service.
- Approval/rejection and audit commit atomically.
- Existing property endpoints remain backward-compatible.

### Exit criteria

Property workflow works with existing module semantics and regression tests pass.

---

## Phase 6 — Investment administration

### First deliver read-only operations

- List/detail/metrics with explicit distinction between principal, current value, expected ROI, realized returns and platform revenue.
- Related transaction/ledger records are scoped and paginated.

### Financial mutations are conditional

Implement cancel, mature or distribute-returns only if the existing domain already defines a safe, authoritative workflow. Do not invent transition rules. Do not update an investment status directly without required ledger/refund/slot operations. Return distribution must have a durable batch/reference and per-beneficiary idempotency. Require review/dual control if policy requires it.

### Tests before enabling mutations

- Same idempotency key repeated → one effective outcome.
- Same key with different payload → conflict.
- Concurrent execution is safe.
- Ledger, wallet, property slot and investment state remain consistent.
- Provider uncertainty triggers reconciliation rather than duplicate financial action.

### Exit criteria

Unsafe actions are either fully tested and enabled or explicitly deferred/feature-disabled with documentation.

---

## Phase 7 — Financial management

### Implement

- Read-only transactions, payment attempts, withdrawals and financial reports.
- Filters by state/type/user/date/amount/reference with safe allowlists and indexes.
- Report calculations based on canonical ledger/payment states.

### Approve/reject/refund constraints

Before implementing any action, inspect the existing withdrawal/payment/refund flow. In particular, determine whether withdrawals have an admin-review state at all. If transfer initiation is automatic, do not add an approval endpoint that bypasses the actual flow. Reuse the existing service, Paystack reference, locked-balance handling, webhooks and reconciliation. Refunds require durable records, provider idempotency, safe retries and reconciliation.

If the existing domain lacks these safeguards, keep this phase read-only and document the blocker.

### Tests

- Concurrent duplicate requests.
- Locked balance released exactly once on eligible rejection/failure.
- Unknown provider result reconciled before retry.
- No direct wallet balance updates.
- Ledger totals reconcile with report totals.

### Exit criteria

No financial mutation is enabled without state-machine, idempotency, ledger, concurrency and reconciliation tests.

---

## Phase 8 — Reports, exports and audit search

### Implement

- Filterable audit list/detail.
- Bounded CSV export, or asynchronous export job for large datasets.
- Financial report endpoints with bounded date ranges and explicit metric definitions.
- Audit the export/report request and completion.

### Security

- Escape CSV cells to prevent formula injection.
- Redact restricted PII.
- Enforce permissions, date ranges, row caps and download expiration.
- Do not expose an unbounded synchronous export endpoint.

### Tests

- Export permission checks and audit events.
- CSV injection payloads.
- Date-range/row-limit validation.
- Stable ordering and correct filters.
- Async job status/retry/expiry if implemented.

### Exit criteria

Exports are safe, bounded, audited and documented.

---

## Phase 9 — Notification administration and system health

### Notification administration

- Confirm whether delivery attempts/events exist. If not, implement a reliable delivery record as a separately reviewed schema change before analytics.
- Show per-channel delivery status only from actual delivery records.
- Resend through existing notification/queue infrastructure; preserve user preferences/consent and add idempotency/rate limits.
- Distinguish retrying a failed attempt from intentionally sending a new notification.

### Health/queue endpoints

- Use existing health/metrics adapters if present.
- Use bounded timeouts and sanitized error categories.
- RabbitMQ queue depth requires a real management/metrics interface; do not infer depth from a connection check.
- Do not expose secrets, DSNs, provider payloads or internal topology.

### Tests

- Correct delivery state and retry rules.
- Resend permission, idempotency and rate limits.
- Provider outage/timeouts do not leak details or block indefinitely.
- Queue metrics return unknown/unsupported if unavailable rather than fabricated values.

### Exit criteria

Notification metrics are truthful; health endpoints are protected and sanitized.

---

## Phase 10 — Performance, observability, docs and release

### Work

- Add/adjust indexes only based on real query patterns and plans.
- Add metrics/alerts only through existing instrumentation or a justified small integration.
- Write admin architecture/API/audit/security/operations/testing docs.
- Test migration up/down/up and run restore/rollback rehearsal as feasible.
- Verify API and worker builds, tests, vet and project lint.
- Smoke-test all enabled endpoints with a controlled admin account.
- Confirm no secrets or PII are present in logs, metrics labels, queues or API DTOs.
- Review feature flags and permissions for high-risk operations.

### Suggested commands (adapt to repo)

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build ./cmd/api
go build ./cmd/worker
```

Run the project's actual migration/test/lint commands as well. Do not assume these are the only checks. Record exact command outcomes and any failures.

### Exit criteria

All release-scoped tests pass; docs match implemented behavior; migration and rollback steps are documented; no critical security/financial blocker remains.

---

## Recommended file organization

Follow actual repository conventions and reuse existing files where appropriate. Potential additions only if equivalent files do not exist:

- `internal/models/audit_log.go`
- `internal/repositories/audit_log_repository.go`
- `internal/services/audit_service.go`
- `internal/services/admin_service.go`
- `internal/services/dashboard_service.go`
- `internal/handlers/admin_handler.go` (or smaller focused handlers)
- `internal/dto/admin_dto.go`
- `internal/routes/admin_routes.go`
- next unused SQL migration(s)
- `docs/ADMINISTRATION_MODULE.md`
- `docs/ADMIN_API.md`
- `docs/ADMIN_AUDIT_LOGGING.md`
- `docs/ADMIN_OPERATIONS_RUNBOOK.md`
- `docs/ADMIN_SECURITY.md`
- `docs/ADMIN_TESTING.md`
- `docs/ADMIN_RELEASE_CHECKLIST.md`

Do not create all files mechanically if the codebase groups responsibilities differently.

---

## Progress tracking

Maintain this checklist and update it as work proceeds:

- [ ] Phase 0 — Discovery complete and findings reported
- [ ] Phase 1 — Audit foundation and atomicity tests
- [ ] Phase 2 — Admin authorization and security tests
- [ ] Phase 3 — Dashboard/read-only views
- [ ] Phase 4 — User management
- [ ] Phase 5 — Property management
- [ ] Phase 6 — Investment management
- [ ] Phase 7 — Financial management
- [ ] Phase 8 — Reports/exports/audit search
- [ ] Phase 9 — Notification administration/health
- [ ] Phase 10 — Performance, docs and release verification

For each phase, report: files added/modified, migration number, endpoints, tests added, commands run, pass/fail results, known limitations and next step. Do not mark a phase complete on code generation alone.

## Final report format

At completion, report:

1. Scope delivered and deferred.
2. Files added/modified.
3. Migration numbers and schema changes.
4. Endpoints implemented with auth/permission notes.
5. Audit events and transactional behavior.
6. Financial workflows enabled versus intentionally disabled.
7. Tests/commands run with exact results.
8. Performance/security checks performed.
9. Documentation/runbooks created.
10. Remaining risks, assumptions and recommended follow-up.

**Do not declare Milestone 7 production-ready unless the Definition of Done in the companion blueprint is satisfied for the enabled scope.**

# 03 — Issues, Bugs & Corrections

> Concrete problems found by reading the code, each with severity, evidence, and a recommended fix.
> Severity: **P1** = correctness/security, fix before building on top · **P2** = should fix soon · **P3** = polish/hygiene.
> Nothing here has been changed yet — this is a review, per the genesis-prompt "review like a senior engineer" instruction.

---

## P1 — Password length validation mismatch (broken UX / inconsistent contract)

**Where:** `internal/dto/auth_dto.go:22` and `:50` vs `internal/validators/password.go:40` (called at `internal/services/auth_service.go:148`).

**Problem:** The register/login DTOs bind `password` with `min=8`:
```go
Password string `json:"password" binding:"required,min=8,max=72" ...`
```
But the service enforces **min 12** (`ValidatePasswordComplexity`). A user submitting a 9-character password passes DTO binding, reaches the service, and is rejected there — with a *different* error path than the binding validator. `user_dto.go` meanwhile uses `min=12`. So the same rule is expressed as 8 in one place and 12 in another.

**Why it matters:** Inconsistent validation is a documented anti-pattern (`6.1` §29, `6.2` §27). It produces confusing errors and means the "first line of defence" comment in the DTO is wrong.

**Fix:** Make the DTO binding match the real policy — `min=12,max=72` on register (and keep login lenient or also 12; login should not reveal policy, so `required` alone is arguably better on login). Single source of truth for the policy is `internal/validators/password.go`; the binding tag should agree with it.

---

## P1 — Register maps unique-constraint violations to HTTP 500

**Where:** `internal/services/auth_service.go:211-215`.

**Problem:** The user+wallet creation runs in `db.Transaction`. Any error from the transaction (including a **duplicate phone** unique-constraint violation, or a duplicate email that slipped past the pre-check due to a race) is flattened to `errors.ErrInternalServer` → HTTP 500:
```go
if err != nil {
    return nil, errors.ErrInternalServer   // <- even for 23505 unique_violation
}
```
Email is pre-checked (`ExistsByEmail`), but **phone uniqueness is not pre-checked**, and the DB has a unique index on `phone`. So registering a second account with an existing phone returns **500**, not **409 Conflict**.

**Why it matters:** Wrong status code for a client-correctable error; hides a conflict as a server fault; violates the error-category mapping in `6.2` §4.

**Fix:** Inspect the error. Either (a) pre-check phone with `ExistsByPhone` (a `UserRepository.ExistsByPhone` already exists per M2 notes) before the transaction, and/or (b) detect Postgres unique violations (pgconn `*pgconn.PgError` code `23505`) inside/after the transaction and return `ErrPhoneAlreadyExists` / `ErrEmailAlreadyExists` (both already defined in `internal/errors`). Prefer the pre-check for a clean message *and* the constraint mapping as a race-safe backstop.

---

## P1 — Silent error swallowing (no logging wired)

**Where (all in services):**
- `auth_service.go:256-261` — refresh-token store failure on register: empty `if err {}` block, comment only.
- `auth_service.go:365-368` — same on login.
- `auth_service.go:456-459` — refresh-token revoke failure during rotation: ignored.
- `auth_service.go:531-536` and `:567-572` — `Logout` / `LogoutAllDevices` return `nil` even when the DB revoke errored.
- `internal/services/user_service.go:199` — `_ = revoke all sessions` after password change.
- `user_service.go:314, :335` — OTP `Update` errors ignored.

**Problem:** Every one of these has a comment like `// In production: log this for monitoring` but **there is no logger**, so the errors vanish. Two real consequences:
1. `LogoutAllDevices` after a password change can **silently fail to revoke sessions** and still report success — a security-relevant outcome (a stolen refresh token survives a password change).
2. Refresh-token rotation: if the *revoke* of the old token fails silently (`:456`), the old token remains valid alongside the new one, weakening the "single-use" guarantee.

**Why it matters:** `6.1` §9 and `6.2` §27 explicitly forbid ignoring errors. Silent failure of session revocation is a security issue, not just hygiene.

**Fix:**
1. Introduce the structured logger (see next item) and log every one of these with `request_id`/`user_id`/`operation`/`error`.
2. For the **security-critical** ones (`LogoutAllDevices` after password change, revoke-old during rotation), **propagate the error** (return it) rather than swallowing — the caller should know the session wasn't killed. At minimum, password-change must fail closed if it cannot revoke sessions.

---

## P1 — No structured logging (docs mandate JSON logs)

**Where:** `internal/middleware/logger.go:29` (`// TODO: Replace with structured logger (Zap)`), and the many `// In production: log...` comments.

**Problem:** The app uses stdlib `log.Printf`. `6.1` §10/§30, `6.2` §14/§25, `4.2` §14/§17, and `7.1`/`7.2` all mandate **structured JSON logging** with fields `request_id, user_id, method, path, status, duration_ms, ip, user_agent, error`, plus immutable **audit logs** for security events.

**Fix:** Add `internal/logger` (the docs even name the package). Recommended: Go stdlib **`log/slog`** with a JSON handler (zero extra deps, Go 1.25 has it) — or Zap if you want the ecosystem. Wire it:
- Replace `middleware/logger.go` with a slog-based request logger that pulls `request_id` from context and logs one structured line per request.
- Inject the logger into services (or use a package-level `slog.Default()` set at boot) so the swallowed errors above become real log lines.
- Add an `audit` logger/table for login success/failure, password reset, token revocation, withdrawals, property approvals (`4.2` §17).

---

## P2 — Error-response envelope does not match the documented shape

**Where:** `internal/response/response.go` (StandardResponse) vs `6.2` §5/§26, `1.4` §10, `3.2` §8.

**Problem:** Code envelope is `{success, message, data, error, code, request_id}`. Docs mandate:
- Success: `{success, message, data}` ✅ (matches).
- Error: `{success, message, errors, request_id}` where **`errors` is an object mapping field → array of strings**. The code emits a singular `error` string and a `code`, and only emits a field-map (`fields`) via a separate `ValidationError` path using inline `gin.H`. So there are effectively **two different error shapes** in the code, and **neither exactly matches** the doc's `errors` field.

**Why it matters:** The frontend contract (`3.2`) and `6.2` §27 ("inconsistent response formats" is an anti-pattern) both assume one stable shape. The React app will branch on `errors`.

**Fix:** Converge on the documented shape. Recommended envelope:
```json
// success
{ "success": true, "message": "...", "data": { } }
// error (validation)
{ "success": false, "message": "Validation failed.", "errors": { "email": ["Email is required."] }, "request_id": "req_..." }
// error (single business/domain error)
{ "success": false, "message": "Insufficient wallet balance.", "request_id": "req_..." }
```
Keep an internal machine `code` if you want, but expose it consistently (either always or never). Make `response.Error` and `response.ValidationError` both produce this shape. This is a small, high-leverage change — do it **before** more endpoints exist and multiply the inconsistency.

---

## P2 — `/auth/logout` route contract mismatch

**Where:** `internal/routes/v1/routes.go:61`.

**Problem:** The comment says "(requires auth)" but no `middleware.Auth` is attached. It happens to work because `Logout` reads the refresh token from the request body, not from the access-token context. But `logout-all` *is* gated by `Auth`, so the two are inconsistent, and the API spec (`3.2` §"POST /auth/logout") says **Authentication: Required**.

**Fix:** Decide the contract and make code + comment + spec agree. Reasonable options:
- (a) Keep logout unauthenticated but body-driven (revokes a specific refresh token) — then fix the misleading comment and the spec.
- (b) Require `Auth` and revoke the current session — more consistent with the spec. Given rotation, (a) is common and fine; just make it consistent.

---

## P2 — `getUserIDFromContext` returns a nil error when the user is missing

**Where:** `internal/handlers/user.go:158-170`.

**Problem:** When `user_id` is absent or the wrong type, the helper returns `(uuid.Nil, nil)` — **no error**. Handlers then check `if err != nil` and proceed with `uuid.Nil`. Today `middleware.Auth` guarantees the value so it's not exploitable, but the helper's contract is a latent bug: any future route that forgets the middleware would operate on `uuid.Nil` silently.

**Fix:** Return a non-nil error (e.g. `errors.ErrUnauthorized`) on the missing/þwrong-type case so callers fail closed.

---

## P2 — No graceful shutdown; server has no timeouts

**Where:** `cmd/api/main.go` (`r.Run(":"+port)`).

**Problem:** `r.Run` uses `http.ListenAndServe` with no `ReadTimeout`/`WriteTimeout`/`IdleTimeout` and no signal handling. On SIGTERM (Docker stop, deploy) in-flight requests and DB transactions are cut. `7.1` expects health/readiness and clean deploys; Slowloris-style abuse is possible without header timeouts.

**Fix:** Build an explicit `&http.Server{Addr, Handler: r, ReadTimeout, ReadHeaderTimeout, WriteTimeout, IdleTimeout}`, run it in a goroutine, and block on `signal.NotifyContext` for `SIGINT`/`SIGTERM`, then `srv.Shutdown(ctx)` with a timeout. Also close the DB pool.

---

## P3 — Dangling doc comment with no function (dead code from a refactor)

**Where:** `internal/services/auth_service.go:616-618` (end of file).

**Problem:** The file ends with a comment header `// validatePasswordComplexity ...` and `// Requirements (from security doc):` but **no function follows** — leftover after the validator was moved to `internal/validators`. Harmless (compiles) but confusing.

**Fix:** Delete the dangling comment block. Same class of issue: the `RefreshToken` handler doc header is missing in `internal/handlers/auth.go` (cosmetic).

---

## P3 — Dead / unwired code that will rot

**Where / what:**
- `internal/repositories/wallet_repository.go:121-131,173-180` — `FindByUserIDForUpdate` (row-lock) and `TransactionExists` are implemented but **not on the `WalletRepository` interface**, and the constructor returns the interface — so they're **unreachable**. They're exactly what Wallet M3 needs; **add them to the interface** when you build the wallet service rather than leaving them stranded.
- `internal/utils/token/token.go` — entire package unused (email-verify/reset not built). Fine to keep; wire it when you build those flows.
- `internal/dto` — `TokenPair`, `WalletResponse`, `WalletSummaryResponse`, `ErrorResponse`, `ValidationErrorResponse`, and `UserToResponse`/wallet mappers are unused; services build DTOs inline. This is why the response shapes drift. Prefer using the mappers (`dto/mappers.go`) consistently and delete the truly dead ones.
- `internal/database/database.go` `AutoMigrate()` — deprecated and unused; keep it clearly marked dev-only or remove.
- `internal/auth/` — empty directory from an older layout; remove (the folder authority `1.3` does not define an `auth` package).

**Fix:** As each feature is built, wire or delete. Don't let parallel-but-unused abstractions accumulate — they are the root cause of the response-shape and validation drift already present.

---

## P3 — `.env.example` hygiene and config coverage gaps

**Where:** `.env.example` and `internal/config/config.go`.

**Problems:**
1. `.env.example` embeds **real-looking base64 JWT secrets** (lines ~45/48) rather than obvious placeholders — bad habit, and someone will ship them.
2. `.env.example` documents many vars (`PAYSTACK_*`, `SMTP_*`, `BCRYPT_COST`, DB pool tunables, `RATE_LIMIT_*`, feature flags, `BASE_URL`) that the `Config` struct **does not map**, so they're silently ignored. Cloudinary vars are duplicated in two sections.
3. DB pool sizes are **hardcoded** in `database.go` (MaxOpen 25 etc.) instead of read from the documented env vars.

**Fix:** Use placeholders like `changeme-generate-with-openssl-rand`. Add the missing fields to `Config` as you implement the features that need them (Paystack for M3, SMTP for M6). Read DB pool + bcrypt cost from env. De-dupe the Cloudinary block.

---

## P3 — `otp_verifications` uses `TIMESTAMP` not `TIMESTAMPTZ`

**Where:** `internal/database/migrations/000003_add_otp_verifications.up.sql`.

**Problem:** Every other table uses `TIMESTAMP WITH TIME ZONE`; this one uses naive `TIMESTAMP`. OTP expiry math around DST/UTC could be subtly wrong depending on server timezone.

**Fix:** New migration to `ALTER ... TYPE TIMESTAMPTZ`. Low urgency (OTP windows are minutes) but fix for consistency.

---

## Things the previous dev did *well* (keep these)

- Clean, genuinely layered architecture with constructor DI in `cmd/api/main.go`.
- Repository pattern with a `BaseRepository.Transaction` helper and context-aware methods.
- **Refresh-token rotation with hashed storage** — correctly implemented (`auth_service.go:413`), including owner check and old-token revocation.
- User+wallet created atomically in one transaction (`auth_service.go:173`).
- Centralized `AppError` + `HTTPStatusFromError` mapping, and a response envelope abstraction (just needs shape convergence).
- Money as integer kobo + append-only ledger intent — the right financial modeling.
- Idempotent, well-commented SQL migrations with CHECK constraints (`main_balance >= 0`, etc.), FKs, indexes, and `updated_at` triggers.
- OTP security done sensibly: hashed codes, attempt caps, rate limiting, short expiry.

The foundation is good. The work now is (1) fix the P1s, (2) add the missing cross-cutting foundations (logging, tests, CI), then (3) build features M3→M7 and harden in M8.

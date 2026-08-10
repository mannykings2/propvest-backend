# 10 — BUILD GUIDE (execute in order)

> Goal: make `go build ./...` pass, then implement M3–M7 endpoints. Every signature below is copied from the real code (see `11-CONTRACTS.md` for the full reference). Follow the existing patterns exactly; do not invent new packages.
>
> Module path: `github.com/mannykings2/propvest-backend`. Go 1.25, Gin, GORM.
>
> Fastest path: **A → B → C → D**, running `go build ./...` after each. Then tests (`12`).

Reference implementation to copy for style: `internal/services/wallet_service.go` (service w/ tx + locking + provider + notifier + queue) and `internal/handlers/user.go` (handler pattern).

---

## Handler pattern (memorize — every handler file uses this)

```go
func (h *XHandler) Method(c *gin.Context) {
	userID, err := getUserIDFromContext(c)            // defined in handlers/user.go; reuse it
	if err != nil { response.Error(c, http.StatusUnauthorized, "Unauthorized"); return }

	var req dto.SomeRequest
	if err := c.ShouldBindJSON(&req); err != nil { response.ValidationError(c, err); return }

	out, err := h.svc.Do(c.Request.Context(), userID, req)
	if err != nil { response.Error(c, err); return }   // maps sentinel→HTTP automatically

	response.Success(c, out)                            // or response.Created / Paginated / SuccessWithMessage
}
```

Key facts:
- `response.Error(c, err)` auto-maps any sentinel from `internal/errors` to the right HTTP status/code/message (`errors/handler.go`). **Just return the sentinel from the service.** All property/investment/wallet/permission errors already exist — see catalog in `11-CONTRACTS.md §Errors`.
- Path params: `c.Param("id")` then `uuid.Parse(...)` → on failure return `apperrors.ErrInvalidPropertyID` / `ErrInvalidUUID`.
- Query: `c.Query("status")`, `c.DefaultQuery("page","1")`, `strconv.Atoi`. Clamp page≥1, limit 1..100.
- Role in ctx: `c.GetString("role")`. User id: `getUserIDFromContext(c)` (returns `uuid.UUID`).
- Paginated list responses: `response.Paginated(c, data, page, limit, total)`.
- Import alias for errors: `apperrors "github.com/mannykings2/propvest-backend/internal/errors"`.

---

## BLOCKER A — finish auth (`internal/services/auth_service.go` + `internal/handlers/auth.go` + routes)

**Symptom:** build fails first here; `authService` doesn't implement `AuthService` (missing `VerifyEmail`, `ResendVerification`, `ForgotPassword`, `ResetPassword`), `issueVerificationEmail` undefined (called at line ~313), and imports `fmt`, `.../utils/token` are unused.

Everything you need already exists:
- Struct fields: `s.tokenRepo` (`VerificationTokenRepository`), `s.mailer` (`mailer.Sender`), `s.audit`, `s.userRepo`, `s.refreshTokenRepo`, `s.config`, `s.db`.
- DTOs already defined in `internal/dto/auth_dto.go`: `VerifyEmailRequest`, `ResendVerificationRequest`, `ForgotPasswordRequest`, `ResetPasswordRequest`, `MessageResponse`.
- Token util: `token.GenerateRandomToken(32)` (`internal/utils/token`). Hash it with the file's existing `hashToken(...)` helper (used at lines 298/412) before storing — store only the hash (`VerificationToken.TokenHash`).
- Model: `models.VerificationToken{UserID, TokenHash, Purpose, ExpiresAt}` with `models.PurposeEmailVerification` / `models.PurposePasswordReset`, and `IsUsable()`.
- Repo: `tokenRepo.Create/FindByHash/MarkUsed/InvalidateActive/DeleteExpired` (see contracts).
- Password hashing: use the same `password` util already imported in this file; validators in `internal/validators` (password policy = min 12, upper/lower/digit/special → `ErrWeakPassword`).

Implement these 5 functions (methods on `*authService`):

1. `issueVerificationEmail(ctx, user *models.User)` (unexported helper):
   - `InvalidateActive(ctx, user.ID, PurposeEmailVerification)`; generate raw token; `Create` a `VerificationToken` (hash, purpose, `ExpiresAt: time.Now().Add(24h)`).
   - Build verify link `fmt.Sprintf("%s/verify-email?token=%s", s.config.BaseURL, raw)`; `s.mailer.Send(mailer.Email{To:user.Email, Subject:..., HTMLBody:..., TextBody:...})`. Best-effort: log on error, never return it. (This uses `fmt` → fixes unused import.)

2. `VerifyEmail(ctx, tokenRaw string) error`: `FindByHash(hash(tokenRaw))`; if not found/`!IsUsable()` → `ErrInvalidToken`; set user `email_verified=true` via `userRepo.UpdatePartial(ctx, uid, map[string]any{"email_verified":true})`; `MarkUsed(id)`; audit `user.email_verified`.

3. `ResendVerification(ctx, email string) error`: find user by email; if found & not verified → `issueVerificationEmail`. **Always return nil** (don't reveal existence).

4. `ForgotPassword(ctx, email string) error`: find user; if found → invalidate active `PurposePasswordReset`, create reset token (`ExpiresAt` 1h), email reset link `%s/reset-password?token=%s`. **Always return nil.**

5. `ResetPassword(ctx, tokenRaw, newPassword string) error`: validate password policy (`ErrWeakPassword`); `FindByHash`; `!IsUsable()`→`ErrInvalidToken`; hash new password; `userRepo.UpdatePartial(uid,{"password_hash":...})`; `MarkUsed`; **`refreshTokenRepo.RevokeAllByUserID(uid)`** (security-critical — propagate this error); audit `user.password_reset`.

Then remove the now-unused `token` import only if you didn't use it (you should use it → keep). Ensure `fmt` is used.

**Handlers** (`internal/handlers/auth.go`) — add 4 methods mirroring the pattern (all public, no auth). Use `response.SuccessWithMessage(c, http.StatusOK, "...")` returning `dto.MessageResponse`-style text:
- `VerifyEmail(c)` — bind `VerifyEmailRequest`, call svc, "Email verified".
- `ResendVerification(c)` — bind `ResendVerificationRequest`.
- `ForgotPassword(c)` — bind `ForgotPasswordRequest`.
- `ResetPassword(c)` — bind `ResetPasswordRequest`.

**Routes:** enable in the new `routes.go` (Blocker D) under the `auth` group:
```go
auth.POST("/verify-email", h.Auth.VerifyEmail)
auth.POST("/resend-verification", h.Auth.ResendVerification)
auth.POST("/forgot-password", h.Auth.ForgotPassword)
auth.POST("/reset-password", h.Auth.ResetPassword)
```

`go build ./internal/services/ ./internal/handlers/` must pass before moving on.

---

## BLOCKER B — missing service files

`cmd/api/main.go` calls three constructors that don't exist. Create each file. Exact constructor signatures (match `main.go` arg order):

### B1 `internal/services/property_service.go`
```go
func NewPropertyService(
    propertyRepo repositories.PropertyRepository,
    cloudinary  *cloudinary.CloudinaryService,   // type from internal/utils/cloudinary (confirm exact name)
    audit       audit.Recorder,
) PropertyService
```
Define interface `PropertyService`:
```go
List(ctx, f repositories.PropertyFilter, page, limit int) ([]dto.PropertyResponse, int64, error)
Get(ctx, id uuid.UUID) (*dto.PropertyResponse, error)
Create(ctx, developerID uuid.UUID, req dto.CreatePropertyRequest) (*dto.PropertyResponse, error)
Update(ctx, actorID uuid.UUID, id uuid.UUID, req dto.UpdatePropertyRequest) (*dto.PropertyResponse, error)
Delete(ctx, actorID uuid.UUID, id uuid.UUID) error
Submit(ctx, actorID, id uuid.UUID) (*dto.PropertyResponse, error)          // draft→submitted
Approve(ctx, adminID, id uuid.UUID) (*dto.PropertyResponse, error)         // submitted→approved
Reject(ctx, adminID, id uuid.UUID, reason string) (*dto.PropertyResponse, error)
AddImage(ctx, actorID, id uuid.UUID, file *multipart.FileHeader) (*dto.PropertyResponse, error) // optional now
```
Rules: `Create` → status `models.PropertyStatusDraft`, set `DeveloperID`, generate `PropCode` (e.g. `"PROP-"+upper(uuid[:8])`), compute nothing the client sent for funded/slots_sold (default 0). `Update`/`Delete`/`Submit` require owner or admin (`ErrUnauthorizedProperty`) and only when status==draft (else `ErrForbidden`). `Approve`/`Reject` are admin-only (enforce in route AND assert here). Map with a `propertyToResponse` helper (compute `SlotsLeft = TotalSlots-SlotsSold`, `Images` from `Documents` where Type==image). Record audit on create/approve/reject. NotFound → `ErrPropertyNotFound`.

### B2 `internal/services/investment_service.go`  ← highest risk, copy wallet_service tx pattern
```go
func NewInvestmentService(
    investmentRepo repositories.InvestmentRepository,
    walletRepo     repositories.WalletRepository,
    propertyRepo   repositories.PropertyRepository,
    notifier       NotificationService,
    cfg            *config.Config,
    db             *gorm.DB,
) InvestmentService
```
Interface:
```go
Create(ctx, userID uuid.UUID, req dto.CreateInvestmentRequest) (*dto.InvestmentResponse, error)
List(ctx, userID uuid.UUID, page, limit int) ([]dto.InvestmentResponse, int64, error)
Get(ctx, userID, id uuid.UUID) (*dto.InvestmentResponse, error)
PortfolioSummary(ctx, userID uuid.UUID) (*dto.PortfolioSummaryResponse, error)
```
`Create` — the atomic core (see wallet_service.go:172-210 for the exact tx idiom):
1. Load property (`propertyRepo.FindByID`). NotFound→`ErrPropertyNotFound`. `if !property.AcceptsInvestment() { return ErrPropertyClosed }`.
2. Validate `req.Slots>0`; `amount := int64(req.Slots) * property.SlotPrice`; enforce `cfg.MinInvestmentAmount/MaxInvestmentAmount` (`ErrInvestmentTooSmall/TooLarge`); if `cfg.RequireKYCForInvestment` check user KYC (`ErrKYCRequired`).
3. `s.db.Transaction(func(tx *gorm.DB) error {`
   - `wallet := walletRepo.FindByUserIDForUpdate(ctx, userID, tx)`; `if wallet.MainBalance < amount { return ErrInsufficientFunds }`.
   - Debit: `tx.Model(&models.Wallet{}).Where("id=?",wallet.ID).Update("main_balance", wallet.MainBalance-amount)`.
   - Ledger row: `tx.Create(&models.WalletTransaction{Type:"investment", Amount:amount, BalanceBefore, BalanceAfter, Reference:"INV-"+..., Status:"completed", UserID, WalletID})`.
   - **Oversell guard (atomic):** `res := tx.Model(&models.Property{}).Where("id = ? AND slots_sold + ? <= total_slots", property.ID, req.Slots).UpdateColumn("slots_sold", gorm.Expr("slots_sold + ?", req.Slots)); if res.RowsAffected==0 { return ErrPropertyFullyFunded }`. Then recompute `funded_pct`.
   - Create investment: `investmentRepo.Create(ctx, &models.Investment{UserID,PropertyID,Slots,AmountKobo:amount,Status:active,Reference:invRef}, tx)`.
   - `return nil` (commit).
4. Post-commit: `notifier.Notify(ctx, userID, models.NotificationInvestmentCreated, ...)`.
5. Map to `dto.InvestmentResponse` (include Property via propertyToResponse). Use unique `Reference` (idempotency anchor).

`PortfolioSummary`: `investmentRepo.PortfolioSummary` + `walletRepo.FindByUserID` → fill `dto.PortfolioSummaryResponse`.

### B3 `internal/services/admin_service.go`
```go
func NewAdminService(
    userRepo       repositories.UserRepository,
    propertyRepo   repositories.PropertyRepository,
    investmentRepo repositories.InvestmentRepository,
    walletRepo     repositories.WalletRepository,
    refreshTokenRepo repositories.RefreshTokenRepository,
    audit          audit.Recorder,
) AdminService
```
Interface:
```go
Dashboard(ctx) (*dto.AdminDashboardResponse, error)
ListUsers(ctx, q string, page, limit int) ([]dto.AdminUserResponse, int64, error)   // add repo method if needed
UpdateUser(ctx, adminID, targetID uuid.UUID, req dto.UpdateUserRequest) (*dto.AdminUserResponse, error)
```
`Dashboard`: aggregate via `investmentRepo.CountAll/SumAll`, `propertyRepo.CountByStatus(submitted)` for PendingApprovals, counts for users/properties (add small repo counts if missing). `UpdateUser` actions: `suspend`→set `is_active=false` + `refreshTokenRepo.RevokeAllByUserID` (force logout); `activate`→`is_active=true`; `promote/demote`→set `role`; `verify_kyc`→set kyc. Audit every action. Property approve/reject live in PropertyService (admin routes call those).

> Note: `ListUsers` needs a repo method (`List(ctx, q, limit, offset) ([]User,int64,error)`). If absent on `UserRepository`, add it (impl + interface) following the existing repo style. Same for any `CountAll` on users.

`go build ./internal/services/` must pass.

---

## BLOCKER C — missing handler files

Create one file per handler. Constructors/fields exactly as `main.go` expects:

| File | Constructor | Service field |
|---|---|---|
| `handlers/wallet.go` | `NewWalletHandler(s services.WalletService) *WalletHandler` | wallet |
| `handlers/property.go` | `NewPropertyHandler(s services.PropertyService) *PropertyHandler` | property |
| `handlers/investment.go` | `NewInvestmentHandler(s services.InvestmentService) *InvestmentHandler` | investment |
| `handlers/notification.go` | `NewNotificationHandler(s services.NotificationService) *NotificationHandler` | notification |
| `handlers/admin.go` | `NewAdminHandler(s services.AdminService) *AdminHandler` | admin |
| `handlers/webhook.go` | `NewWebhookHandler(s services.WalletService, cfg *config.Config) *WebhookHandler` | wallet+cfg |
| `handlers/websocket.go` | `NewWebSocketHandler(hub *realtime.Hub, cfg *config.Config) *WebSocketHandler` | hub+cfg |

Methods to implement (thin; delegate to services; use the handler pattern):

- **wallet.go**: `GetWallet` (`svc.GetWallet(ctx,userID)`→Success); `InitiateDeposit` (bind `dto.DepositRequest`; read `c.GetHeader("Idempotency-Key")`; `svc.InitiateDeposit(ctx,userID,req.Amount,idemKey)`→Created); `InitiateWithdrawal` (bind `dto.WithdrawRequest`); `GetTransactionHistory` (query `type`,`status`,`page`,`limit`; `svc.GetTransactionHistory(...)`→`response.Paginated`).
- **property.go**: `List` (public; parse `repositories.PropertyFilter` from query + page/limit → Paginated); `Get` (public; `:id`); `Create` (bind `dto.CreatePropertyRequest`, developerID from ctx → Created); `Update` (`:id`+`dto.UpdatePropertyRequest`); `Delete` (`:id`→`response.NoContent`); `Submit`; `Approve`; `Reject` (bind `dto.RejectPropertyRequest`). `AddImage` optional (uses `c.FormFile("image")`).
- **investment.go**: `Create` (bind `dto.CreateInvestmentRequest`→Created); `List` (Paginated); `Get` (`:id`); `PortfolioSummary`.
- **notification.go**: `List` (query `unread` bool,`page`,`limit`; `svc.List`→ return the `*dto.NotificationListResponse` with `response.Success` OR Paginated — List returns `(*dto.NotificationListResponse,int64,error)`, use Success with the struct); `MarkRead` (`:id`); `MarkAllRead`; `Delete` (`:id`→NoContent).
- **admin.go**: `Dashboard`; `ListUsers` (query `q`,page,limit→Paginated); `UpdateUser` (`:id`+`dto.UpdateUserRequest`). Property approve/reject: either add `ApproveProperty`/`RejectProperty` here delegating to `propertyService` (then admin handler also needs propertyService — simpler: point admin property routes at `propertyHandler.Approve/Reject`).
- **webhook.go** `Paystack(c)` — **NOT behind Auth**:
  ```go
  body, _ := io.ReadAll(c.Request.Body)
  sig := c.GetHeader("X-Paystack-Signature")
  if !h.wallet.VerifyWebhookSignature(sig, body) { c.Status(401); return }
  // parse {"data":{"reference":"..."}} → reference
  if err := h.wallet.CreditFromPaymentReference(c.Request.Context(), reference, body); err != nil { c.Status(500); return }
  c.Status(200)
  ```
  (Return 200 fast; credit is idempotent. Don't use the JSON envelope for provider webhooks.)
- **websocket.go** `Connect(c)` — authenticate via `?token=` query using `jwt.ValidateToken(token, cfg.JWTSecret)` → userID; upgrade with gorilla/websocket (already a dep, see `internal/realtime`), then `hub.NewClient(userID.String(), conn)`.

`go build ./internal/handlers/` must pass.

---

## BLOCKER D — rewrite `internal/routes/v1/routes.go`

Current file has the OLD signature and no `Handlers` struct; `main.go` calls `v1.RegisterRoutes(apiV1, &v1.Handlers{...}, cfg)`. The commented blocks already in `routes.go` are the blueprint — uncomment/adapt them. Replace the file with:

```go
type Handlers struct {
	Auth         *handlers.AuthHandler
	User         *handlers.UserHandler
	Wallet       *handlers.WalletHandler
	Property     *handlers.PropertyHandler
	Investment   *handlers.InvestmentHandler
	Notification *handlers.NotificationHandler
	Admin        *handlers.AdminHandler
	Webhook      *handlers.WebhookHandler
	WebSocket    *handlers.WebSocketHandler
}

func RegisterRoutes(router *gin.RouterGroup, h *Handlers, cfg *config.Config) {
	auth := router.Group("/auth")                    // register/login/refresh/logout/logout-all + 4 new (Blocker A)
	users := router.Group("/users"); users.Use(middleware.Auth(cfg))   // existing 6 routes
	wallet := router.Group("/wallet"); wallet.Use(middleware.Auth(cfg))
	// GET "" ; POST /deposit ; POST /withdraw ; GET /transactions
	props := router.Group("/properties")
	props.GET("", h.Property.List); props.GET("/:id", h.Property.Get)   // public
	pa := props.Group(""); pa.Use(middleware.Auth(cfg))
	pa.POST("", middleware.RequireRole("developer","admin"), h.Property.Create)
	pa.PATCH("/:id", middleware.RequireRole("developer","admin"), h.Property.Update)
	pa.DELETE("/:id", middleware.RequireRole("developer","admin"), h.Property.Delete)
	pa.POST("/:id/submit", middleware.RequireRole("developer","admin"), h.Property.Submit)
	inv := router.Group("/investments"); inv.Use(middleware.Auth(cfg), middleware.RequireInvestor())
	// POST "" ; GET "" ; GET /:id ; GET /portfolio/summary
	notif := router.Group("/notifications"); notif.Use(middleware.Auth(cfg))
	// GET "" ; PATCH /:id/read ; PATCH /read-all ; DELETE /:id
	admin := router.Group("/admin"); admin.Use(middleware.Auth(cfg), middleware.RequireAdmin())
	// GET /dashboard ; GET /users ; PATCH /users/:id ; GET /properties ;
	// PATCH /properties/:id/approve -> h.Property.Approve ; .../reject -> h.Property.Reject ; GET /audit-logs
	webhooks := router.Group("/webhooks")
	webhooks.POST("/paystack", h.Webhook.Paystack)   // NO auth
	router.GET("/ws", h.WebSocket.Connect)           // auth via ?token=
}
```
Keep the existing auth+users route bodies verbatim (they work). Middleware helpers available: `middleware.Auth(cfg)`, `RequireRole(...)`, `RequireInvestor()`, `RequireDeveloper()`, `RequireAdmin()`, `RequireAdminOrStaff()`.

---

## Final: make it compile & smoke-test

```powershell
go build ./...            # must be clean
go vet ./...
gofmt -l .               # should list nothing
```
Run: `make up` (Postgres :5435), `make migrate-up`, `make run` → `GET http://localhost:8080/health`. Env additions in `12`. Then write tests (`12 §Testing`). **Do not commit until `go build ./...` is clean.**

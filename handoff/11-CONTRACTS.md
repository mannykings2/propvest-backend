# 11 — CONTRACTS (exact signatures to build against)

> Copied verbatim from the codebase on handoff. Use these so new services/handlers compile against real types. Module: `github.com/mannykings2/propvest-backend`.

## Repositories (interfaces + constructors)

```go
// user_repository.go
type UserRepository interface {
	Create(ctx, *models.User) error
	FindByID(ctx, uuid.UUID) (*models.User, error)
	FindByEmail(ctx, string) (*models.User, error)
	FindByPhone(ctx, string) (*models.User, error)
	Update(ctx, *models.User) error
	UpdatePartial(ctx, id uuid.UUID, updates map[string]interface{}) error
	Delete(ctx, uuid.UUID) error
	ExistsByEmail(ctx, string) (bool, error)
	ExistsByPhone(ctx, string) (bool, error)
	FindByIDWithWallet(ctx, uuid.UUID) (*models.User, error)
}
func NewUserRepository(db *gorm.DB) UserRepository
func IsErrRecordNotFound(err error) bool   // package helper

// wallet_repository.go
type WalletRepository interface {
	Create(ctx, *models.Wallet) error
	FindByID(ctx, uuid.UUID) (*models.Wallet, error)
	FindByUserID(ctx, uuid.UUID) (*models.Wallet, error)
	Update(ctx, *models.Wallet) error
	UpdateBalance(ctx, walletID uuid.UUID, main, earnings int64) error
	CreateTransaction(ctx, *models.WalletTransaction) error
	GetTransactions(ctx, walletID uuid.UUID, limit, offset int) ([]models.WalletTransaction, error)
	GetTransactionByReference(ctx, reference string) (*models.WalletTransaction, error)
	FindByUserIDForUpdate(ctx, userID uuid.UUID, tx *gorm.DB) (*models.Wallet, error)  // row lock
	TransactionExists(ctx, reference string) (bool, error)
	ListTransactions(ctx, userID uuid.UUID, txType, status string, limit, offset int) ([]models.WalletTransaction, int64, error)
}

// property_repository.go
type PropertyFilter struct { Status, State, Location, Type string; DeveloperID *uuid.UUID; Sort, Order string }
type PropertyRepository interface {
	Create(ctx, *models.Property) error
	FindByID(ctx, uuid.UUID) (*models.Property, error)
	FindByIDForUpdate(ctx, id uuid.UUID, tx *gorm.DB) (*models.Property, error)
	Update(ctx, *models.Property) error
	UpdatePartial(ctx, id uuid.UUID, updates map[string]any) error
	Delete(ctx, uuid.UUID) error
	List(ctx, f PropertyFilter, limit, offset int) ([]models.Property, int64, error)
	AddDocument(ctx, *models.PropertyDocument) error
	CountByStatus(ctx, status string) (int64, error)
}

// investment_repository.go
type InvestmentRepository interface {
	Create(ctx, *models.Investment, tx *gorm.DB) error          // pass tx from the DB transaction
	FindByID(ctx, uuid.UUID) (*models.Investment, error)
	ListByUser(ctx, userID uuid.UUID, limit, offset int) ([]models.Investment, int64, error)
	PortfolioSummary(ctx, userID uuid.UUID) (totalInvested int64, count int64, err error)
	CountAll(ctx) (int64, error)
	SumAll(ctx) (int64, error)
}

// notification_repository.go
type NotificationRepository interface {
	Create(ctx, *models.Notification) error
	ListByUser(ctx, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]models.Notification, int64, error)
	CountUnread(ctx, userID uuid.UUID) (int64, error)
	MarkRead(ctx, id, userID uuid.UUID) error
	MarkAllRead(ctx, userID uuid.UUID) error
	Delete(ctx, id, userID uuid.UUID) error
}

// payment_repository.go
type PaymentRepository interface {
	Create(ctx, *models.Payment) error
	FindByReference(ctx, reference string) (*models.Payment, error)
	MarkSuccess(ctx, reference, providerRef string, raw datatypes.JSON) error
	MarkFailed(ctx, reference string, raw datatypes.JSON) error
	ListByUser(ctx, userID uuid.UUID, limit, offset int) ([]models.Payment, int64, error)
}

// verification_token_repository.go
type VerificationTokenRepository interface {
	Create(ctx, *models.VerificationToken) error
	FindByHash(ctx, tokenHash string) (*models.VerificationToken, error)
	MarkUsed(ctx, id uuid.UUID) error
	InvalidateActive(ctx, userID uuid.UUID, purpose string) error
	DeleteExpired(ctx) error
}

// refresh_token_repository.go (key methods)
RevokeAllByUserID(ctx, userID uuid.UUID) error   // use for password reset / suspend
```

## Infra packages

```go
// internal/queue  (mq := queue.New(url); mq.Close())
func New(url string) *Client                       // no error; disabled no-op if broker down
func (c *Client) Enabled() bool
func (c *Client) Publish(ctx, queueName string, v any) error
func (c *Client) Consume(queueName string, handler func(ctx, body []byte) error)
func (c *Client) Close()
const QueueEmailDispatch, QueueSMSDispatch, QueueWithdrawalProcess, QueueRealtimePush, QueueDepositReceipt string
// message structs: EmailMessage{To,Subject,HTMLBody,TextBody}; SMSMessage{To,Message};
//   WithdrawalMessage{TransactionID,UserID,AmountKobo,Reference}; DepositReceiptMessage{UserID,Email,AmountKobo,Reference};
//   RealtimeMessage{UserID,Event,Payload}

// internal/mailer
type Email struct { To, Subject, HTMLBody, TextBody string }
type Sender interface { Send(email Email) error }
func New(cfg *config.Config) Sender                // "smtp" -> real, else mock (logs)

// internal/payments
type InitResult   struct { AuthorizationURL, AccessCode, Reference string }
type VerifyResult struct { Reference string; AmountKobo int64; Status, Email string } // Status: success|failed|pending
type Provider interface {
	InitializeDeposit(ctx, email string, amountKobo int64, reference string) (*InitResult, error)
	VerifyTransaction(ctx, reference string) (*VerifyResult, error)
	VerifyWebhookSignature(signature string, body []byte) bool
	Name() string
}
func NewProvider(cfg *config.Config) Provider      // alias of New(cfg); "mock" default

// internal/audit
type Event struct { Action, ActorID, TargetType, TargetID, IP, UserAgent, RequestID string; Metadata map[string]any }
type Recorder interface { Record(ctx, e Event) }
func NewLogRecorder() *LogRecorder

// internal/realtime
func NewHub() *Hub
func (h *Hub) Run()                                // goroutine
func (h *Hub) PushToUser(userID string, data []byte)
func (h *Hub) NewClient(userID string, conn *websocket.Conn)   // after upgrade
func (h *Hub) OnlineUsers() int

// internal/logger
func Init(appEnv string); func L() *slog.Logger
func FromContext(ctx) *slog.Logger                 // preferred in services: logger.FromContext(ctx).Error(...)
func Info/Warn/Error/Debug(msg string, args ...any)

// internal/utils/token
func GenerateRandomToken(length int) (string, error)   // use 32 for verification/reset
func GenerateNumericCode(length int) (string, error)
// internal/utils/jwt
func GenerateAccessToken(userID uuid.UUID, role, secret string, ttl time.Duration) (string, error)
func GenerateRefreshToken(...) (string, error)
func ValidateToken(tokenString, secret string) (*Claims, error)  // Claims{UserID uuid.UUID, Role string}
func ExtractUserID(tokenString, secret string) (uuid.UUID, error)
```

## Existing services (already implemented — construct/consume, don't rewrite)

```go
type WalletService interface {
	GetWallet(ctx, userID uuid.UUID) (*dto.WalletResponse, error)
	InitiateDeposit(ctx, userID uuid.UUID, amountKobo int64, idempotencyKey string) (*dto.DepositResponse, error)
	CreditFromPaymentReference(ctx, reference string, rawPayload []byte) error   // webhook uses this
	InitiateWithdrawal(ctx, userID uuid.UUID, req dto.WithdrawRequest) (*dto.TransactionResponse, error)
	GetTransactionHistory(ctx, userID uuid.UUID, txType, status string, page, limit int) ([]dto.TransactionResponse, int64, error)
	VerifyWebhookSignature(signature string, body []byte) bool
}
func NewWalletService(walletRepo, userRepo, paymentRepo, provider, notifier, mq, cfg, db) WalletService

type NotificationService interface {
	Notify(ctx, userID uuid.UUID, nType, title, body string, metadata map[string]any)
	NotifyWithEmail(ctx, userID uuid.UUID, email, nType, title, body string, metadata map[string]any)
	List(ctx, userID uuid.UUID, unreadOnly bool, page, limit int) (*dto.NotificationListResponse, int64, error)
	MarkRead(ctx, userID, id uuid.UUID) error
	MarkAllRead(ctx, userID uuid.UUID) error
	Delete(ctx, userID, id uuid.UUID) error
	StartRealtimeConsumer(ctx)
}

type AuthService interface {   // impl missing the last 4 — see Build Guide Blocker A
	Register/Login/RefreshAccessToken/Logout/LogoutAllDevices ...
	VerifyEmail(ctx, token string) error
	ResendVerification(ctx, email string) error
	ForgotPassword(ctx, email string) error
	ResetPassword(ctx, token, newPassword string) error
}
```

## DTOs (request/response shapes you must produce/consume)

```go
// wallet: WalletResponse{ID,UserID,MainBalance,EarningsBalance,Currency,VirtualAcctNo*,VirtualBank*,CreatedAt}
//         DepositRequest{Amount int64 kobo}  DepositResponse{AuthorizationURL,Reference}
//         WithdrawRequest{Amount,BankCode,AccountNumber(10),AccountName}
//         TransactionResponse{ID,Type,Amount,BalanceBefore,BalanceAfter,Reference,Description,Status,CreatedAt}
// property: PropertyResponse{...,SlotsLeft,Images[]string,...}
//   CreatePropertyRequest{Name,Location,State,Type(oneof Residential Apartment Land Commercial),IncomeType(oneof rental resale),
//                         SPVName,SPVCACNo,TotalSlots>0,SlotPrice>0,TotalValue>0,PurchasePrice>0,YieldPct,HoldYears,Description,Tag}
//   UpdatePropertyRequest{*Name,*Location,*Description,*SlotPrice,*YieldPct,*Tag}   // pointers = partial
//   RejectPropertyRequest{Reason required min=3}
// investment: CreateInvestmentRequest{PropertyID,Slots>0}   // amount derived server-side = slots*SlotPrice
//   InvestmentResponse{ID,PropertyID,Property*,Slots,AmountKobo,Status,Reference,CreatedAt}
//   PortfolioSummaryResponse{TotalInvested,ActiveCount,WalletBalance,EarningsBalance}
// notification: NotificationResponse{ID,Type,Title,Body,Read,Metadata,CreatedAt}
//   NotificationListResponse{Notifications[],UnreadCount}
// admin: AdminDashboardResponse{TotalUsers,TotalProperties,PendingApprovals,TotalInvestments,TotalInvestedKobo,TotalWalletVolume}
//   AdminUserResponse{ID,UserCode,FullName,Email,Phone,Role,KYCStatus,IsActive,EmailVerified,CreatedAt}
//   UpdateUserRequest{Action(oneof suspend activate promote demote verify_kyc),Role}
// auth (already present): VerifyEmailRequest, ResendVerificationRequest, ForgotPasswordRequest, ResetPasswordRequest, MessageResponse
```

## Models — enums (use the consts, don't hardcode strings)

```go
// Property.Status: models.PropertyStatus{Draft,Submitted,Approved,Rejected,Funding,Funded,Completed}
//   func (p *Property) AcceptsInvestment() bool   // approved || funding
// Investment.Status: models.InvestmentStatus{Active,Completed,Cancelled,Refunded}
// Payment.Status: {Pending,Success,Failed}; Payment.Channel: {Deposit,Withdrawal}
// VerificationToken.Purpose: models.Purpose{EmailVerification,PasswordReset}; (t *VerificationToken).IsUsable() bool
// PropertyDocument.Type: models.PropertyDoc{Image,Legal,Agreement,Other}
// Notification.Type: models.Notification{DepositSuccess,WithdrawalUpdate,InvestmentCreated,PropertyApproved,Welcome}
// WalletTransaction.Type strings (no consts): deposit|withdrawal|investment|refund|reversal|fee|rental_income|transfer
//   .Status: pending|processing|completed|failed|reversed  (Amount always positive kobo; set BalanceBefore/After)
```

## Response helpers (`internal/response`)

```go
Success(c, data)                    // 200 {success,data,request_id}
SuccessWithMessage(c, status, msg[, data])
Created(c, data)                    // 201
NoContent(c)                        // 204
Error(c, err)                       // maps sentinel -> status/code/message automatically
Error(c, status, msg[, code])       // explicit
ValidationError(c, err)             // 422 field map from binding errors
Paginated(c, data, page, limit, total)
```

## Error → HTTP status map (`internal/errors/handler.go`) — return the sentinel, get the status

| Status | Sentinels (use `apperrors.X`) |
|---|---|
| 400 | ErrValidation, ErrInvalidEmail/Phone/UUID, ErrMissingField, ErrInvalidJSON, ErrInvalidAmount, ErrInvalidUserID, ErrInvalidPropertyID |
| 401 | ErrInvalidCredentials, ErrInvalidToken, ErrTokenExpired, ErrUnauthorized, ErrPasswordWrong |
| 403 | ErrForbidden, ErrInsufficientRole, ErrAdminOnly, ErrDeveloperOnly, ErrEmailNotVerified, ErrAccountSuspended, ErrUnauthorizedProperty, ErrKYCRequired |
| 404 | ErrUserNotFound, ErrWalletNotFound, ErrPropertyNotFound, ErrInvestmentNotFound, gorm.ErrRecordNotFound |
| 409 | ErrEmailTaken/ErrEmailAlreadyExists, ErrPhoneTaken, ErrUserExists, ErrDuplicateReference, ErrAlreadyInvested, ErrSamePassword |
| 422 | ErrInsufficientFunds, ErrNegativeBalance, ErrMin/MaxDeposit, ErrMin/MaxWithdrawal, ErrPropertyClosed, ErrPropertyNotApproved, ErrPropertyFullyFunded, ErrInvestmentTooSmall/Large/Exceeds, ErrPasswordWeak/TooLong, ErrInvalidPhoneFormat |
| 429 | ErrRateLimitExceeded, ErrTooManyRequests |
| 500 | ErrDatabase, ErrCache, ErrStorage, ErrExternal, ErrInternal/ErrInternalServer, `WrapError`/`NewInternalError` |

Note: **all property/investment/permission sentinels already exist** — no need to add error vars for M4/M5/M7. Wrap unexpected infra failures with `apperrors.ErrInternalServer` (log the real cause via `logger.FromContext(ctx)`).

## Middleware (`internal/middleware`)

```go
Auth(cfg) gin.HandlerFunc            // requires Bearer; sets c.Set("user_id",<uuid str>), c.Set("role",<role>)
OptionalAuth(cfg) gin.HandlerFunc
RequireRole(roles ...string) gin.HandlerFunc
RequireInvestor()  // = RequireRole("investor","admin")
RequireDeveloper() // = RequireRole("developer","admin")
RequireAdmin()     // = RequireRole("admin")
RequireAdminOrStaff()
// In handlers: userID,_ := getUserIDFromContext(c)  (helper already in handlers/user.go); role := c.GetString("role")
```

## Composition root arg order (from `cmd/api/main.go`) — constructors MUST match

```go
NewNotificationService(notificationRepo, hub, mq)
NewAuthService(userRepo, walletRepo, refreshTokenRepo, tokenRepo, emailSender, auditRecorder, cfg, database.DB)
NewWalletService(walletRepo, userRepo, paymentRepo, paymentProvider, notificationService, mq, cfg, database.DB)
NewPropertyService(propertyRepo, cloudinaryService, auditRecorder)                    // TO BUILD
NewInvestmentService(investmentRepo, walletRepo, propertyRepo, notificationService, cfg, database.DB) // TO BUILD
NewAdminService(userRepo, propertyRepo, investmentRepo, walletRepo, refreshTokenRepo, auditRecorder)  // TO BUILD
// handlers: NewWalletHandler(walletService), NewPropertyHandler(propertyService),
//   NewInvestmentHandler(investmentService), NewNotificationHandler(notificationService),
//   NewAdminHandler(adminService), NewWebhookHandler(walletService, cfg), NewWebSocketHandler(hub, cfg)
```

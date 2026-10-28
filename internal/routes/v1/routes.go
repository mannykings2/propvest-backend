package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/handlers"
	"github.com/mannykings2/propvest-backend/internal/middleware"
	"github.com/mannykings2/propvest-backend/internal/repositories"
)

// RegisterRoutes registers all v1 API routes.
// This is called from main.go after all dependencies are initialized.
//
// Parameters:
//   - router: Gin router group for /api/v1
//   - authHandler: Handler for authentication endpoints
//   - userHandler: Handler for user management endpoints
//   - walletHandler: Handler for wallet operations (Milestone 3)
//   - propertyHandler: Handler for property management (Milestone 4)
//   - investmentHandler: Handler for investment operations (Milestone 5)
//   - notificationHandler: Handler for notifications (Milestone 6)
//   - adminMessagingHandler: Handler for admin bulk messaging (Milestone 6)
//   - adminDashboardHandler: Handler for admin dashboard (Milestone 7)
//   - adminUserHandler: Handler for admin user management (Milestone 7)
//   - adminFinancialHandler: Handler for admin financial monitoring (Milestone 7)
//   - adminReportsHandler: Handler for admin reports and exports (Milestone 7)
//   - adminHealthHandler: Handler for admin system health (Milestone 7)
//   - offplanHandler: Handler for off-plan subscriptions
//   - supportHandler: Handler for user support tickets (Phase 4.2)
//   - adminSupportHandler: Handler for the staff/admin support queue (Phase 4.2)
//   - userRepo: User repository — the auth middleware re-checks the account
//     row (suspended/deleted/role) on every authenticated request
//   - cfg: Application configuration (needed for middleware)
//
// Route organization:
//
//	/api/v1/health          - Health check (public)
//	/api/v1/auth/*          - Authentication (Milestone 1)
//	/api/v1/users/*         - User management (Milestone 2)
//	/api/v1/wallet/*        - Wallet operations (Milestone 3)
//	/api/v1/properties/*    - Property management (Milestone 4)
//	/api/v1/investments/*   - Investment operations (Milestone 5)
//	/api/v1/notifications/* - Notifications (Milestone 6)
//	/api/v1/admin/*         - Admin operations (Milestone 7)
//
// Each route group will be extracted to its own file as it grows.
func RegisterRoutes(
	router *gin.RouterGroup,
	authHandler *handlers.AuthHandler,
	userHandler *handlers.UserHandler,
	walletHandler *handlers.WalletHandler,
	propertyHandler *handlers.PropertyHandler,
	investmentHandler *handlers.InvestmentHandler,
	notificationHandler *handlers.NotificationHandler,
	adminMessagingHandler *handlers.AdminMessagingHandler,
	adminDashboardHandler *handlers.AdminDashboardHandler,
	adminUserHandler *handlers.AdminUserHandler,
	adminFinancialHandler *handlers.AdminFinancialHandler,
	adminReportsHandler *handlers.AdminReportsHandler,
	adminHealthHandler *handlers.AdminHealthHandler,
	adminFinanceHandler *handlers.AdminFinanceHandler,
	adminRiskHandler *handlers.AdminRiskHandler,
	offplanHandler *handlers.OffplanHandler,
	supportHandler *handlers.SupportHandler,
	adminSupportHandler *handlers.AdminSupportHandler,
	privacyHandler *handlers.PrivacyHandler,
	adminPrivacyHandler *handlers.AdminPrivacyHandler,
	userRepo repositories.UserRepository,
	cfg *config.Config,
	readiness gin.HandlerFunc,
) {
	// ───────────────────────────────────────────────────────────────────
	// PUBLIC ROUTES (no authentication required)
	// ───────────────────────────────────────────────────────────────────
	router.GET("/health", handlers.HealthCheck)
	router.GET("/health/ready", readiness)

	// Public property browsing (no authentication required)
	router.GET("/properties", propertyHandler.ListProperties)
	router.GET("/properties/:id", propertyHandler.GetProperty)

	// ───────────────────────────────────────────────────────────────────
	// AUTHENTICATION ROUTES (Milestone 1)
	// ───────────────────────────────────────────────────────────────────
	// Public routes (no authentication required)
	auth := router.Group("/auth")
	{
		// POST /api/v1/auth/register - Create new account
		auth.POST("/register", authHandler.Register)

		// POST /api/v1/auth/login - Login with email/password
		auth.POST("/login", authHandler.Login)

		// POST /api/v1/auth/refresh - Get new access token from refresh token
		auth.POST("/refresh", authHandler.RefreshToken)

		// POST /api/v1/auth/logout - Logout from current device (requires auth)
		auth.POST("/logout", authHandler.Logout)

		// POST /api/v1/auth/logout-all - Logout from all devices (requires auth)
		// Must be authenticated to logout everywhere
		auth.POST("/logout-all", middleware.Auth(cfg, userRepo), authHandler.LogoutAll)

		// Password reset + email verification flows (Phase 2, Workstream D3).
		// Public by design: token possession authorizes the operation, and the
		// reset/verify endpoints are enumeration-safe (always respond success).
		auth.POST("/forgot-password", authHandler.ForgotPassword)
		auth.POST("/reset-password", authHandler.ResetPassword)
		auth.POST("/verify-email", authHandler.VerifyEmail)
		auth.POST("/resend-verification", authHandler.ResendVerification)
	}

	// ─────────────────────────────────────────────────────────────────
	// PRIVACY / DATA-RIGHTS ROUTES (Phase 9 — NDPA 2023)
	// ─────────────────────────────────────────────────────────────────
	// Public by design: both endpoints are exercised by bearer LINKS (the
	// unsubscribe URL in every bulk message; the 24h data-export download
	// link). The token IS the credential — neither route touches a session.
	router.GET("/unsubscribe", privacyHandler.Unsubscribe)
	router.GET("/me/exports/download/:token", privacyHandler.DownloadExport)

	// ───────────────────────────────────────────────────────────────────
	// USER ROUTES (Milestone 2, requires authentication)
	// ───────────────────────────────────────────────────────────────────
	users := router.Group("/users")
	users.Use(middleware.Auth(cfg, userRepo)) // All routes require authentication
	{
		// GET /api/v1/users/me - Get current user's profile
		users.GET("/me", userHandler.GetProfile)

		// PATCH /api/v1/users/me - Update profile (name)
		users.PATCH("/me", userHandler.UpdateProfile)

		// PATCH /api/v1/users/avatar - Upload avatar image
		users.PATCH("/avatar", userHandler.UploadAvatar)

		// PATCH /api/v1/users/password - Change password
		users.PATCH("/password", userHandler.ChangePassword)

		// POST /api/v1/users/phone/request - Request phone number change (sends OTP)
		users.POST("/phone/request", userHandler.RequestPhoneChange)

		// POST /api/v1/users/phone/verify - Verify phone change with OTP
		users.POST("/phone/verify", userHandler.VerifyPhoneChange)
	}

	// ───────────────────────────────────────────────────────────────────
	// KYC SUBMISSION (Phase 2, Workstream A4)
	// ───────────────────────────────────────────────────────────────────
	// Registered at the router ROOT, not under /users, because the frontend
	// posts to /api/v1/me/kyc/submit exactly. Adjusting either side was a
	// decision point; we matched the frontend's existing contract.
	me := router.Group("/me")
	me.Use(middleware.Auth(cfg, userRepo))
	{
		me.POST("/kyc/submit", userHandler.SubmitKYC)
		me.GET("/kyc", userHandler.GetKYCStatus)

		// Phase 9 (NDPA): consent read/withdraw, marketing state + the user's
		// own unsubscribe link, async data export, deferred account erasure.
		me.GET("/consents", privacyHandler.GetConsents)
		me.POST("/consents", privacyHandler.GiveConsents)
		me.GET("/marketing", privacyHandler.GetMarketing)
		me.POST("/exports", privacyHandler.RequestExport)
		me.GET("/exports", privacyHandler.ListExports)
		me.POST("/account/delete", privacyHandler.RequestDeletion)
		me.GET("/account/delete", privacyHandler.DeletionStatus)
		me.DELETE("/account/delete", privacyHandler.CancelDeletion)
	}

	// ───────────────────────────────────────────────────────────────────
	// WALLET ROUTES (Milestone 3, requires authentication)
	// ───────────────────────────────────────────────────────────────────
	// Wallet endpoints for managing user's wallet, deposits, withdrawals, and transaction history.
	//
	// AUTHENTICATION:
	// All wallet routes require authentication (auth middleware applied to group).
	// User can only access their own wallet (enforced by extracting user_id from JWT).
	//
	// ENDPOINTS:
	//   GET  /wallet             - Get wallet balance and info
	//   POST /wallet/deposit     - Initiate deposit via payment provider
	//   POST /wallet/withdraw    - Request withdrawal to bank account
	//   GET  /wallet/transactions - Get transaction history (paginated, filterable)
	//   GET  /wallet/deposit/callback - Payment success callback (PUBLIC)
	//
	// WEBHOOK (PUBLIC):
	//   POST /webhooks/payment   - Payment provider callback (signature-verified, not in this group)
	wallet := router.Group("/wallet")
	{
		// Authenticated routes (require JWT)
		authenticated := wallet.Group("")
		authenticated.Use(middleware.Auth(cfg, userRepo))
		{
			// GET /api/v1/wallet
			// Returns user's wallet with main_balance, earnings_balance, currency, etc.
			authenticated.GET("", walletHandler.GetWallet)

			// POST /api/v1/wallet/deposit
			// Initiates deposit flow with payment provider.
			authenticated.POST("/deposit", walletHandler.InitiateDeposit)

			// POST /api/v1/wallet/withdraw
			// Debits wallet and queues payout to bank account.
			// Rate limited: Maximum 3 withdrawals per hour per user
			// Launch policy gate applies too (money out).
			authenticated.POST("/withdraw",
				middleware.WithdrawalRateLimit(3),      // Apply rate limiting
				middleware.RequireVerifiedAccount(cfg, userRepo), // Launch policy flags
				walletHandler.RequestWithdrawal)

			// GET /api/v1/wallet/transactions?type=deposit&status=completed&page=1&limit=20
			// Returns paginated transaction history with optional filters.
			authenticated.GET("/transactions", walletHandler.GetTransactions)

			// GET /api/v1/wallet/banks — selectable bank list with CBN codes.
			// Needed so withdrawals can carry a real bank_code (previously the
			// frontend hardcoded a default because no codes existed).
			authenticated.GET("/banks", walletHandler.ListBanks)

			// POST /api/v1/wallet/verify-account — resolve account number → name.
			// Replaces the withdrawal page's fake setTimeout verification.
			authenticated.POST("/verify-account", walletHandler.VerifyAccount)

			// POST /api/v1/wallet/transfer-earnings — earnings → main wallet.
			// Locked transaction + single ledger row; rejects when earnings are
			// insufficient.
			authenticated.POST("/transfer-earnings", walletHandler.TransferEarnings)
		}

		// Public routes (no authentication required)
		// GET /api/v1/wallet/deposit/callback?trxref=DEP-123&reference=DEP-123
		// Called by user's browser after completing payment on Paystack page.
		// Shows "Payment Successful!" message. Actual wallet crediting happens via webhook.
		// No auth required because user's browser (not our app) calls this.
		wallet.GET("/deposit/callback", walletHandler.HandleDepositCallback)
	}

	// ───────────────────────────────────────────────────────────────────
	// PROPERTY ROUTES (Milestone 4)
	// ───────────────────────────────────────────────────────────────────
	// Property management endpoints for creating, updating, and managing properties.
	//
	// AUTHENTICATION & AUTHORIZATION:
	//   - Public routes: Browse published properties (no auth)
	//   - Admin routes: Full CRUD, publish, manage images/documents (admin role required)
	//
	// ENDPOINTS:
	//   PUBLIC:
	//     GET  /properties           - List published properties (public browse)
	//     GET  /properties/:id       - Get single published property
	//
	//   ADMIN:
	//     POST   /admin/properties                              - Create new property
	//     GET    /admin/properties                              - List all properties (including drafts)
	//     GET    /admin/properties/:id                          - Get property (any status)
	//     PATCH  /admin/properties/:id                          - Update property details
	//     DELETE /admin/properties/:id                          - Delete property
	//     POST   /admin/properties/:id/publish                  - Publish property (draft→published)
	//
	//   IMAGES (admin only):
	//     POST   /admin/properties/:id/images                   - Upload property image
	//     DELETE /admin/properties/:id/images/:imageId          - Delete image
	//     PATCH  /admin/properties/:id/images/:imageId/cover    - Set as cover image
	//
	//   DOCUMENTS (admin only):
	//     POST   /admin/properties/:id/documents                      - Upload document
	//     DELETE /admin/properties/:id/documents/:documentId          - Delete document
	//     PATCH  /admin/properties/:id/documents/:documentId/visibility - Toggle public/private
	admin := router.Group("/admin/properties")
	admin.Use(middleware.Auth(cfg, userRepo), middleware.RequireRole("admin"))
	{
		// Property CRUD
		admin.POST("", propertyHandler.CreateProperty)
		admin.GET("", propertyHandler.ListAdminProperties)
		admin.GET("/:id", propertyHandler.GetAdminProperty)
		admin.PATCH("/:id", propertyHandler.UpdateProperty)
		admin.DELETE("/:id", propertyHandler.DeleteProperty)

		// Property lifecycle
		admin.POST("/:id/publish", propertyHandler.PublishProperty)

		// Image management
		admin.POST("/:id/images", propertyHandler.UploadImage)
		admin.DELETE("/:id/images/:imageId", propertyHandler.DeleteImage)
		admin.PATCH("/:id/images/:imageId/cover", propertyHandler.SetCoverImage)

		// Document management
		admin.POST("/:id/documents", propertyHandler.UploadDocument)
		admin.DELETE("/:id/documents/:documentId", propertyHandler.DeleteDocument)
		admin.PATCH("/:id/documents/:documentId/visibility", propertyHandler.ToggleDocumentVisibility)
	}

	// ───────────────────────────────────────────────────────────────────
	// INVESTMENT ROUTES (Milestone 5, requires authentication)
	// ───────────────────────────────────────────────────────────────────
	// Investment endpoints for purchasing property slots and managing portfolio.
	//
	// AUTHENTICATION:
	// All investment routes require authentication (auth middleware applied to group).
	// User can only access their own investments (enforced by user_id from JWT).
	//
	// ENDPOINTS:
	//   POST /investments               - Create new investment (purchase slots)
	//   GET  /investments               - List user's investments (paginated)
	//   GET  /investments/:id           - Get single investment details
	//   GET  /portfolio/summary         - Get portfolio summary (total invested, counts, balances)
	//
	// ADMIN ENDPOINTS (separate admin group below)
	investments := router.Group("/investments")
	investments.Use(middleware.Auth(cfg, userRepo))
	{
		// POST /api/v1/investments
		// Creates a new investment (user purchases property slots).
		// Request body: { "property_id": "uuid", "slots": 10, "idempotency_key": "optional" }
		// Launch policy gate: REQUIRE_EMAIL_VERIFICATION / REQUIRE_KYC_FOR_INVESTMENT
		// are enforced here (money in) — see middleware/launch_policy.go.
		investments.POST("", middleware.RequireVerifiedAccount(cfg, userRepo), investmentHandler.CreateInvestment)

		// GET /api/v1/investments?page=1&page_size=20
		// Returns paginated list of user's investments.
		investments.GET("", investmentHandler.ListMyInvestments)

		// GET /api/v1/investments/:id
		// Returns single investment details (only if user owns it).
		investments.GET("/:id", investmentHandler.GetInvestment)

		// ── Sale-vote governance (Phase 2, Workstream A3) ──
		// POST /api/v1/investments/:id/trigger-sale — open a sale campaign
		// (requires hold period elapsed; 409 if one is already open).
		investments.POST("/:id/trigger-sale", investmentHandler.TriggerSaleVote)

		// POST /api/v1/investments/:id/vote — cast a vote ({vote: for|against}).
		// 409 on duplicate vote; closes the campaign on majority/all-voted.
		investments.POST("/:id/vote", investmentHandler.CastVote)

		// GET /api/v1/investments/:id/vote — live tally + trigger eligibility.
		investments.GET("/:id/vote", investmentHandler.GetVoteState)
	}

	// Portfolio summary (separate endpoint for clarity)
	portfolio := router.Group("/portfolio")
	portfolio.Use(middleware.Auth(cfg, userRepo))
	{
		// GET /api/v1/portfolio/summary
		// Returns aggregate portfolio metrics: total invested, active count, wallet balances.
		portfolio.GET("/summary", investmentHandler.GetPortfolioSummary)
	}

	// Admin investment endpoints
	adminInvestments := router.Group("/admin/investments")
	adminInvestments.Use(middleware.Auth(cfg, userRepo), middleware.RequireRole("admin"))
	{
		// GET /api/v1/admin/investments?status=active&page=1&page_size=20
		// Lists all investments with optional status filter.
		adminInvestments.GET("", investmentHandler.ListAllInvestments)

		// GET /api/v1/admin/investments/metrics
		// Returns platform-wide investment statistics for dashboard.
		adminInvestments.GET("/metrics", investmentHandler.GetInvestmentMetrics)
	}

	// Admin property investments (list all investors in a property)
	adminPropertyInvestments := router.Group("/admin/properties/:id/investments")
	adminPropertyInvestments.Use(middleware.Auth(cfg, userRepo), middleware.RequireRole("admin"))
	{
		// GET /api/v1/admin/properties/:id/investments?page=1&page_size=20
		// Returns all investors in a specific property.
		adminPropertyInvestments.GET("", investmentHandler.ListPropertyInvestments)
	}

	// ───────────────────────────────────────────────────────────────────
	// NOTIFICATION ROUTES (Milestone 6, requires authentication)
	// ───────────────────────────────────────────────────────────────────
	// User notification management endpoints for in-app notifications.
	//
	// AUTHENTICATION:
	//   - All routes require authentication (auth middleware applied to group)
	//   - User can only access their own notifications (enforced by service layer)
	//
	// ENDPOINTS:
	//   GET    /notifications          - List user's notifications (paginated)
	//   GET    /notifications/:id      - Get single notification
	//   PATCH  /notifications/:id/read - Mark notification as read
	//   PATCH  /notifications/read-all - Mark all notifications as read
	//   DELETE /notifications/:id      - Delete notification
	//   GET    /preferences            - Get notification preferences
	//   PUT    /preferences/:type      - Update type-specific preference
	//   PUT    /preferences            - Batch update preferences
	notifications := router.Group("/notifications")
	notifications.Use(middleware.Auth(cfg, userRepo))
	{
		// Notification CRUD
		notifications.GET("", notificationHandler.ListNotifications)
		notifications.PATCH("/:id/read", notificationHandler.MarkAsRead)
		notifications.PATCH("/read-all", notificationHandler.MarkAllAsRead)
		notifications.DELETE("/:id", notificationHandler.DeleteNotification)

		// Notification preferences
		notifications.GET("/preferences", notificationHandler.GetPreferences)
		notifications.PUT("/preferences/:type", notificationHandler.UpdatePreference)
		notifications.PUT("/preferences", notificationHandler.BatchUpdatePreferences)
	}

	// ───────────────────────────────────────────────────────────────────
	// OFF-PLAN MODULE (Phase 2, Workstream B)
	// ───────────────────────────────────────────────────────────────────
	// Public read of active projects; authenticated subscription (wallet debit).
	// NOTE: GET returns a BARE ARRAY as `data` — OffPlan.jsx does
	// `setProjectsList(data || [])` and would crash on {projects: [...]}.
	offplan := router.Group("/offplan")
	{
		offplan.GET("/projects", offplanHandler.ListProjects)
		offplan.GET("/projects/:id", offplanHandler.GetProject)

		authed := offplan.Group("")
		authed.Use(middleware.Auth(cfg, userRepo))
		{
			// POST /api/v1/offplan/projects/:id/subscribe
			// Body: {plan_id, finish_type_id} (snake_case). Debits down payment
			// + 2% fee from the main wallet in one locked transaction.
			// Launch policy gate applies (money out of the wallet).
			authed.POST("/projects/:id/subscribe", middleware.RequireVerifiedAccount(cfg, userRepo), offplanHandler.Subscribe)
			authed.GET("/subscriptions", offplanHandler.ListSubscriptions)
		}
	}

	// ───────────────────────────────────────────────────────────────────
	// SUPPORT TICKETS (Phase 4.2, requires authentication)
	// ───────────────────────────────────────────────────────────────────
	// The Support page previously kept tickets in React component state —
	// nothing persisted. These routes back it with real storage; every
	// user-side query is scoped by the JWT's user_id (ownership enforced
	// inside the service, so a foreign ticket reads as "not found").
	support := router.Group("/support/tickets")
	support.Use(middleware.Auth(cfg, userRepo))
	{
		support.POST("", supportHandler.CreateTicket)
		support.GET("", supportHandler.ListTickets)
		support.GET("/:id", supportHandler.GetTicket)
		support.POST("/:id/replies", supportHandler.ReplyTicket)
	}

	// Agent queue: staff work the tickets; RBAC gates the whole group.
	adminSupport := router.Group("/admin/support/tickets")
	adminSupport.Use(middleware.Auth(cfg, userRepo),
		middleware.RequireRole("staff", "admin", "super_admin"))
	{
		adminSupport.GET("", adminSupportHandler.AdminListTickets)
		adminSupport.GET("/:id", adminSupportHandler.AdminGetTicket)
		adminSupport.POST("/:id/replies", adminSupportHandler.AdminReplyTicket)
		adminSupport.PATCH("/:id/status", adminSupportHandler.AdminUpdateStatus)
	}

	// ───────────────────────────────────────────────────────────────────
	// ADMIN MESSAGING ROUTES (Milestone 6, requires admin role)
	// ───────────────────────────────────────────────────────────────────
	// Admin bulk messaging endpoints for sending emails/SMS to users.
	//
	// AUTHENTICATION & AUTHORIZATION:
	//   - All endpoints require authentication (auth middleware)
	//   - All endpoints require admin role (RequireRole middleware)
	//
	// ENDPOINTS:
	//   POST /admin/messaging/send - Send bulk email/SMS to users
	//
	// TARGETING MODES:
	//   - single: Target specific user IDs
	//   - segment: Target users matching filters (KYC status, investment status)
	//   - all: Target all users (requires explicit confirmation)
	//
	// SAFETY LIMITS:
	//   - Max 10,000 users per broadcast (enforced by service)
	//   - "all" mode requires confirm_all_users=true
	adminMessaging := router.Group("/admin/messaging")
	adminMessaging.Use(middleware.Auth(cfg, userRepo), middleware.RequireRole("admin"))
	{
		// POST /api/v1/admin/messaging/send
		// Send bulk message (email, SMS, or both) to targeted users.
		adminMessaging.POST("/send", adminMessagingHandler.SendBulkMessage)
	}

	// ───────────────────────────────────────────────────────────────────
	// ADMIN ROUTES (Milestone 7, requires admin role)
	// ───────────────────────────────────────────────────────────────────
	// admin := router.Group("/admin")
	// admin.Use(middleware.Auth(), middleware.RequireRole("admin"))
	// {
	//     admin.GET("/dashboard", adminHandler.GetDashboard)
	//     admin.GET("/users", adminHandler.ListUsers)
	//     admin.PATCH("/users/:id", adminHandler.UpdateUser)
	//     admin.PATCH("/users/:id/suspend", adminHandler.SuspendUser)
	//     admin.PATCH("/users/:id/activate", adminHandler.ActivateUser)
	//
	//     admin.GET("/properties", adminHandler.ListProperties)
	//     admin.PATCH("/properties/:id/approve", adminHandler.ApproveProperty)
	//     admin.PATCH("/properties/:id/reject", adminHandler.RejectProperty)
	//
	//     admin.GET("/audit-logs", adminHandler.ListAuditLogs)
	// }

	// ───────────────────────────────────────────────────────────────────
	// WEBHOOK ROUTES (public but signature-verified)
	// ───────────────────────────────────────────────────────────────────
	// Webhooks are called by external systems (payment providers, etc.) to notify
	// us of events. They do NOT use JWT authentication - instead they use
	// cryptographic signature verification.
	//
	// SECURITY:
	//   - NO auth middleware (external systems can't get JWT tokens)
	//   - Signature verification in handler (HMAC-SHA512 with secret key)
	//   - Always verify with provider API (never trust webhook alone)
	//   - Rate limiting recommended (prevent webhook spam attacks)
	//
	// IDEMPOTENCY:
	//   - Webhooks may be delivered multiple times (provider retries)
	//   - Handlers must be idempotent (safe to process same webhook twice)
	//   - Check for duplicate processing before taking action
	webhooks := router.Group("/webhooks")
	{
		// POST /api/v1/webhooks/payment
		// Called by payment provider (Paystack, Flutterwave) when payment completes.
		// Headers: X-Paystack-Signature (or provider-specific signature header)
		// Body: Provider-specific payload with payment reference and status
		//
		// Handler responsibilities:
		//   1. Verify signature (proves webhook is from provider)
		//   2. Extract payment reference
		//   3. Verify payment with provider API (server-side verification)
		//   4. Credit wallet if payment successful (idempotent)
		//   5. Return 200 OK (so provider stops retrying)
		//
		// This is a PUBLIC endpoint - anyone can POST to it.
		// Security relies on signature verification, not authentication.
		webhooks.POST("/payment", walletHandler.HandleWebhook)

		// POST /api/v1/webhooks/paystack/transfer
		// Called by Paystack when a transfer (withdrawal) succeeds or fails.
		// Headers: X-Paystack-Signature
		// Body: Paystack payload with transfer reference and status
		//
		// Events handled:
		//   - transfer.success: Transfer completed successfully
		//   - transfer.failed: Transfer failed (invalid account, etc.)
		//   - transfer.reversed: Transfer was reversed (rare)
		//
		// Handler responsibilities:
		//   1. Verify signature (prevents forged webhooks)
		//   2. Extract transfer reference (WD-xxx)
		//   3. Call FinalizeWithdrawal() to complete or reverse withdrawal
		//   4. Return 200 OK (idempotent)
		//
		// PUBLIC endpoint - security via signature verification.
		webhooks.POST("/paystack/transfer", walletHandler.HandleTransferWebhook)
	}

	// ───────────────────────────────────────────────────────────────────────────
	// ADMINISTRATION ROUTES (Milestone 7, requires admin role)
	// ───────────────────────────────────────────────────────────────────────────
	// Admin dashboard and operational management endpoints.
	//
	// AUTHENTICATION & AUTHORIZATION:
	//   - All endpoints require authentication (Auth middleware)
	//   - Most endpoints require admin or super_admin role (RequireAdmin middleware)
	//   - Privileged endpoints require super_admin role (RequireSuperAdmin middleware)
	//
	// ROUTE GROUPS:
	//   /api/v1/admin/dashboard     - Dashboard metrics and analytics (Phase 3)
	//   /api/v1/admin/users          - User management (Phase 4)
	//   /api/v1/admin/audit          - Audit log search (Phase 8)
	//   /api/v1/admin/reports        - Financial reports and exports (Phase 8)
	//
	// NOTE: Property and investment admin endpoints already registered above

	// Admin Dashboard (Phase 3)
	adminDashboard := router.Group("/admin/dashboard")
	adminDashboard.Use(middleware.Auth(cfg, userRepo), middleware.RequireAdmin())
	{
		adminDashboard.GET("", adminDashboardHandler.GetDashboard)
	}

	// Admin User Management (Phase 4)
	adminUsers := router.Group("/admin/users")
	adminUsers.Use(middleware.Auth(cfg, userRepo), middleware.RequireAdmin())
	{
		// User list and details
		adminUsers.GET("", adminUserHandler.ListUsers)
		adminUsers.GET("/:id", adminUserHandler.GetUser)
		adminUsers.PATCH("/:id", adminUserHandler.UpdateUser)

		// User account actions (admin or super_admin)
		adminUsers.PATCH("/:id/suspend", adminUserHandler.SuspendUser)
		adminUsers.PATCH("/:id/activate", adminUserHandler.ActivateUser)
		adminUsers.POST("/:id/reset-password", adminUserHandler.ForcePasswordReset)
		adminUsers.POST("/:id/logout-all", adminUserHandler.LogoutAllSessions)
		adminUsers.PATCH("/:id/kyc", adminUserHandler.DecideKYC)
	}

	// Admin KYC review queue (Phase 2, Workstream A4 / Phase 6).
	// List + decision are PII-safe (last-4 only); GET /submissions/:id is the
	// single endpoint returning full BVN/NIN and audits every read (6.5).
	adminKYC := router.Group("/admin/kyc")
	adminKYC.Use(middleware.Auth(cfg, userRepo), middleware.RequireAdmin())
	{
		adminKYC.GET("/submissions", adminUserHandler.ListKYCSubmissions)
		adminKYC.GET("/submissions/:id", adminUserHandler.GetKYCDetail)
		adminKYC.POST("/submissions/:id/screening", adminUserHandler.RecordScreening)
	}

	// Privileged admin routes (super_admin only)
	adminPrivileged := router.Group("/admin/users")
	adminPrivileged.Use(middleware.Auth(cfg, userRepo), middleware.RequireSuperAdmin())
	{
		// Role changes (super_admin only)
		adminPrivileged.PATCH("/:id/role", adminUserHandler.ChangeUserRole)

		// User deletion (super_admin only)
		adminPrivileged.DELETE("/:id", adminUserHandler.DeleteUser)
	}

	// Admin Financial Monitoring (Phase 7)
	adminFinancial := router.Group("/admin")
	adminFinancial.Use(middleware.Auth(cfg, userRepo), middleware.RequireAdmin())
	{
		// Transaction monitoring
		adminFinancial.GET("/transactions", adminFinancialHandler.ListTransactions)
		adminFinancial.GET("/transactions/:id", adminFinancialHandler.GetTransactionDetail)

		// Payment monitoring
		adminFinancial.GET("/payments", adminFinancialHandler.ListPayments)
		adminFinancial.GET("/payments/:id", adminFinancialHandler.GetPaymentDetail)
	}

	// Admin Money layer (Phase 7.8): fee revenue, settlement batches,
	// trust-reconciliation history — plus on-demand run/sweep controls.
	adminFinance := router.Group("/admin/finance")
	adminFinance.Use(middleware.Auth(cfg, userRepo), middleware.RequireAdmin())
	{
		adminFinance.GET("/fees", adminFinanceHandler.GetFees)
		adminFinance.POST("/fees/sweep", adminFinanceHandler.SweepFees)

		adminFinance.GET("/settlements", adminFinanceHandler.ListSettlements)
		adminFinance.POST("/settlements", adminFinanceHandler.IngestSettlement)
		// Automatic-import escape hatch: the worker pulls Paystack settlement
		// reports on schedule; this forces a sync now (idempotent).
		adminFinance.POST("/settlements/import", adminFinanceHandler.ImportSettlements)

		adminFinance.GET("/reconciliation", adminFinanceHandler.ListReconciliation)
		adminFinance.POST("/reconciliation/run", adminFinanceHandler.RunReconciliation)
	}

	// Admin Privacy surface (Phase 9.5/9.6): consent evidence, suppression
	// list, data-export jobs (re-issue sends the link TO THE USER), erasure
	// requests with grace-period approve/cancel, policy version bumps.
	adminPrivacy := router.Group("/admin/privacy")
	adminPrivacy.Use(middleware.Auth(cfg, userRepo), middleware.RequireAdmin())
	{
		adminPrivacy.GET("/consents/:userId", adminPrivacyHandler.GetConsents)

		adminPrivacy.GET("/suppression", adminPrivacyHandler.ListSuppressions)
		adminPrivacy.POST("/suppression", adminPrivacyHandler.AddSuppression)
		adminPrivacy.DELETE("/suppression/:id", adminPrivacyHandler.RemoveSuppression)

		adminPrivacy.GET("/exports", adminPrivacyHandler.ListExports)
		adminPrivacy.POST("/exports/:id/reissue", adminPrivacyHandler.ReissueExport)

		adminPrivacy.GET("/deletions", adminPrivacyHandler.ListDeletions)
		adminPrivacy.PATCH("/deletions/:id", adminPrivacyHandler.DecideDeletion)

		adminPrivacy.GET("/policies", adminPrivacyHandler.ListPolicies)
		adminPrivacy.POST("/policies/version", adminPrivacyHandler.BumpPolicy)
	}

	// Admin Risk layer (Phase 8): AML alert triage, rule on/off toggles with
	// editable thresholds, manual screening history + re-screen reminder, and
	// the STR/CTR filing screens. Every write is audited server-side.
	adminRisk := router.Group("/admin/risk")
	adminRisk.Use(middleware.Auth(cfg, userRepo), middleware.RequireAdmin())
	{
		// Alerts — queue, detail, triage (assign / status / notes)
		adminRisk.GET("/alerts", adminRiskHandler.ListAlerts)
		adminRisk.GET("/alerts/:id", adminRiskHandler.GetAlert)
		adminRisk.PATCH("/alerts/:id", adminRiskHandler.TriageAlert)

		// Manual flag-user (blocks withdrawals while set)
		adminRisk.POST("/users/:id/flag", adminRiskHandler.FlagUser)

		// Rules — view thresholds, toggle on/off, edit limits
		adminRisk.GET("/rules", adminRiskHandler.ListRules)
		adminRisk.PATCH("/rules/:code", adminRiskHandler.UpdateRule)

		// Screening history + re-screen reminder (8.3/8.6)
		adminRisk.GET("/screening", adminRiskHandler.ListScreening)
		adminRisk.GET("/rescreen", adminRiskHandler.RescreenDue)

		// On-demand monitoring pass (proves a rule toggle end-to-end)
		adminRisk.POST("/scan/run", adminRiskHandler.RunScan)

		// STR workflow (8.7) — 24h NFIU clock, goAML reference on filing
		adminRisk.GET("/str", adminRiskHandler.ListSTR)
		adminRisk.PATCH("/str/:id", adminRiskHandler.UpdateSTR)

		// CTR monitor (8.8) — daily threshold rows, 7-day clock
		adminRisk.GET("/ctr", adminRiskHandler.ListCTR)
		adminRisk.PATCH("/ctr/:id/file", adminRiskHandler.FileCTR)
	}

	// Admin Reports & Audit Logs (Phase 8)
	adminReports := router.Group("/admin")
	adminReports.Use(middleware.Auth(cfg, userRepo), middleware.RequireAdmin())
	{
		// Audit log search
		adminReports.GET("/audit-logs", adminReportsHandler.ListAuditLogs)
		adminReports.GET("/audit-logs/:id", adminReportsHandler.GetAuditLogDetail)

		// Audit log export (CSV)
		adminReports.GET("/audit-logs/export", adminReportsHandler.ExportAuditLogsCSV)

		// Financial reports
		adminReports.POST("/reports/generate", adminReportsHandler.GenerateFinancialReport)
	}

	// Admin System Health (Phase 9)
	adminHealth := router.Group("/admin/health")
	adminHealth.Use(middleware.Auth(cfg, userRepo), middleware.RequireAdmin())
	{
		// System health check
		adminHealth.GET("/system", adminHealthHandler.CheckSystemHealth)

		// Queue health check
		adminHealth.GET("/queues", adminHealthHandler.CheckQueueHealth)
	}
}

package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/handlers"
	"github.com/mannykings2/propvest-backend/internal/middleware"
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
//   - cfg: Application configuration (needed for middleware)
//
// Route organization:
//   /api/v1/health          - Health check (public)
//   /api/v1/auth/*          - Authentication (Milestone 1)
//   /api/v1/users/*         - User management (Milestone 2)
//   /api/v1/wallet/*        - Wallet operations (Milestone 3)
//   /api/v1/properties/*    - Property management (Milestone 4)
//   /api/v1/investments/*   - Investment operations (Milestone 5)
//   /api/v1/notifications/* - Notifications (Milestone 6)
//   /api/v1/admin/*         - Admin operations (Milestone 7)
//
// Each route group will be extracted to its own file as it grows.
func RegisterRoutes(
	router *gin.RouterGroup,
	authHandler *handlers.AuthHandler,
	userHandler *handlers.UserHandler,
	walletHandler *handlers.WalletHandler,
	propertyHandler *handlers.PropertyHandler,
	cfg *config.Config,
) {
	// ───────────────────────────────────────────────────────────────────
	// PUBLIC ROUTES (no authentication required)
	// ───────────────────────────────────────────────────────────────────
	router.GET("/health", handlers.HealthCheck)

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
		auth.POST("/logout-all", middleware.Auth(cfg), authHandler.LogoutAll)

		// Future endpoints (later milestones):
		// auth.POST("/forgot-password", authHandler.ForgotPassword)
		// auth.POST("/reset-password", authHandler.ResetPassword)
		// auth.GET("/verify-email", authHandler.VerifyEmail)
		// auth.POST("/resend-verification", authHandler.ResendVerification)
	}

	// ───────────────────────────────────────────────────────────────────
	// USER ROUTES (Milestone 2, requires authentication)
	// ───────────────────────────────────────────────────────────────────
	users := router.Group("/users")
	users.Use(middleware.Auth(cfg))  // All routes require authentication
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

		// Future endpoints:
		// users.DELETE("/me", userHandler.DeleteAccount)
		// users.GET("/preferences", userHandler.GetPreferences)
		// users.PATCH("/preferences", userHandler.UpdatePreferences)
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
		authenticated.Use(middleware.Auth(cfg))
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
			authenticated.POST("/withdraw",
				middleware.WithdrawalRateLimit(3),  // Apply rate limiting
				walletHandler.RequestWithdrawal)

			// GET /api/v1/wallet/transactions?type=deposit&status=completed&page=1&limit=20
			// Returns paginated transaction history with optional filters.
			authenticated.GET("/transactions", walletHandler.GetTransactions)
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
	admin.Use(middleware.Auth(cfg), middleware.RequireRole("admin"))
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
	// investments := router.Group("/investments")
	// investments.Use(middleware.Auth())
	// {
	//     investments.POST("", investmentHandler.Create)
	//     investments.GET("", investmentHandler.GetUserInvestments)
	//     investments.GET("/:id", investmentHandler.Get)
	//     investments.GET("/portfolio/summary", investmentHandler.GetPortfolioSummary)
	// }

	// ───────────────────────────────────────────────────────────────────
	// NOTIFICATION ROUTES (Milestone 6, requires authentication)
	// ───────────────────────────────────────────────────────────────────
	// notifications := router.Group("/notifications")
	// notifications.Use(middleware.Auth())
	// {
	//     notifications.GET("", notificationHandler.List)
	//     notifications.PATCH("/:id/read", notificationHandler.MarkAsRead)
	//     notifications.PATCH("/read-all", notificationHandler.MarkAllAsRead)
	//     notifications.DELETE("/:id", notificationHandler.Delete)
	// }

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
}


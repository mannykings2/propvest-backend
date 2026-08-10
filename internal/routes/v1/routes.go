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
	cfg *config.Config,
) {
	// ───────────────────────────────────────────────────────────────────
	// PUBLIC ROUTES (no authentication required)
	// ───────────────────────────────────────────────────────────────────
	router.GET("/health", handlers.HealthCheck)

	// TODO: Add public property listing endpoint (unauthenticated browse)
	// router.GET("/properties", propertyHandler.List)
	// router.GET("/properties/:id", propertyHandler.GetPublic)

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
	//
	// WEBHOOK (PUBLIC):
	//   POST /webhooks/payment   - Payment provider callback (signature-verified, not in this group)
	wallet := router.Group("/wallet")
	wallet.Use(middleware.Auth(cfg)) // All wallet routes require authentication
	{
		// GET /api/v1/wallet
		// Returns user's wallet with main_balance, earnings_balance, currency, etc.
		// Example response:
		//   {
		//     "main_balance": 150000,
		//     "main_balance_formatted": "₦1,500.00",
		//     "earnings_balance": 50000,
		//     "earnings_balance_formatted": "₦500.00",
		//     "currency": "NGN"
		//   }
		wallet.GET("", walletHandler.GetWallet)

		// POST /api/v1/wallet/deposit
		// Initiates deposit flow with payment provider.
		// Request body: {"amount_kobo": 150000, "idempotency_key": "optional"}
		// Returns: {"authorization_url": "https://checkout.paystack.com/...", "reference": "DEP-xxx"}
		// User is redirected to authorization_url to complete payment.
		wallet.POST("/deposit", walletHandler.InitiateDeposit)

		// POST /api/v1/wallet/withdraw
		// Debits wallet and queues payout to bank account.
		// Request body: {"amount_kobo": 50000, "account_number": "0123456789", "account_name": "John Doe", "bank_code": "058", "bank_name": "GTBank"}
		// Returns: Transaction record with status "pending"
		// Actual payout processed asynchronously by worker.
		wallet.POST("/withdraw", walletHandler.RequestWithdrawal)

		// GET /api/v1/wallet/transactions?type=deposit&status=completed&page=1&limit=20
		// Returns paginated transaction history with optional filters.
		// Query parameters:
		//   - type: Filter by transaction type (deposit, withdrawal, credit, debit)
		//   - status: Filter by status (pending, completed, failed)
		//   - page: Page number (default: 1)
		//   - limit: Items per page (default: 20, max: 100)
		// Returns: {"transactions": [...], "total": 42, "page": 1, "pages": 3}
		wallet.GET("/transactions", walletHandler.GetTransactions)
	}

	// ───────────────────────────────────────────────────────────────────
	// PROPERTY ROUTES (Milestone 4)
	// ───────────────────────────────────────────────────────────────────
	// properties := router.Group("/properties")
	// {
	//     // Public routes
	//     properties.GET("", propertyHandler.List)
	//     properties.GET("/:id", propertyHandler.Get)
	//
	//     // Authenticated routes
	//     authenticated := properties.Group("")
	//     authenticated.Use(middleware.Auth())
	//     {
	//         authenticated.POST("", middleware.RequireRole("developer", "admin"), propertyHandler.Create)
	//         authenticated.PATCH("/:id", middleware.RequireRole("developer", "admin"), propertyHandler.Update)
	//         authenticated.DELETE("/:id", middleware.RequireRole("developer", "admin"), propertyHandler.Delete)
	//     }
	// }

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
	}
}

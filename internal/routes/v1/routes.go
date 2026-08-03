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
	// wallet := router.Group("/wallet")
	// wallet.Use(middleware.Auth())
	// {
	//     wallet.GET("", walletHandler.GetWallet)
	//     wallet.POST("/deposit", walletHandler.InitiateDeposit)
	//     wallet.POST("/withdraw", walletHandler.InitiateWithdrawal)
	//     wallet.GET("/transactions", walletHandler.GetTransactionHistory)
	// }

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
	// webhooks := router.Group("/webhooks")
	// {
	//     webhooks.POST("/paystack", webhookHandler.Paystack)
	//     // webhooks.POST("/flutterwave", webhookHandler.Flutterwave)
	// }
}

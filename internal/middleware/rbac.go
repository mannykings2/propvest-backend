package middleware

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/response"
)

// RequireRole is middleware that enforces role-based access control
//
// Purpose:
//   - Check if authenticated user has required role
//   - Reject requests from users with insufficient permissions
//   - Works with Auth() middleware (must be applied after Auth)
//
// Usage in routes:
//   // Only admins can access
//   adminRoutes := router.Group("/api/v1/admin")
//   adminRoutes.Use(middleware.Auth(cfg))
//   adminRoutes.Use(middleware.RequireRole("admin"))
//
//   // Developers and admins can access
//   devRoutes := router.Group("/api/v1/properties")
//   devRoutes.Use(middleware.Auth(cfg))
//   devRoutes.POST("/", middleware.RequireRole("developer", "admin"), handler.CreateProperty)
//
// Roles hierarchy (from lowest to highest privilege):
//   1. investor   - Can invest, manage portfolio, fund wallet
//   2. developer  - Can create properties, upload documents
//   3. staff      - Can view reports, assist users (future)
//   4. admin      - Can do everything
//
// Parameters:
//   - allowedRoles: One or more role strings
//   - User must have ONE of the specified roles
//
// Error responses:
//   - 401 Unauthorized: User not authenticated (Auth middleware not applied)
//   - 403 Forbidden: User authenticated but role insufficient
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Step 1: Get user role from context
		// This was set by Auth() middleware
		// If this doesn't exist, it means Auth() middleware wasn't applied
		role, exists := c.Get("role")
		if !exists {
			// User is not authenticated
			response.Error(c, http.StatusUnauthorized, "Authentication required", "unauthorized")
			c.Abort()
			return
		}

		// Step 2: Type assertion (context stores interface{})
		userRole, ok := role.(string)
		if !ok {
			// Role in context is not a string (corrupted somehow)
			response.Error(c, http.StatusInternalServerError, "Internal server error", "internal_error")
			c.Abort()
			return
		}

		// Step 3: Check if user's role matches any allowed role
		for _, allowedRole := range allowedRoles {
			if userRole == allowedRole {
				// User has required role - allow access
				c.Next()
				return
			}
		}

		// Step 4: User doesn't have any of the required roles
		// Return 403 Forbidden (not 401 - user IS authenticated, just not authorized)
		response.Error(c, http.StatusForbidden, "Insufficient permissions", "forbidden")
		c.Abort()
	}
}

// RequireInvestor is a convenience wrapper for RequireRole("investor", "admin")
// Allows investors and admins (admins can do everything)
func RequireInvestor() gin.HandlerFunc {
	return RequireRole("investor", "admin")
}

// RequireDeveloper is a convenience wrapper for RequireRole("developer", "admin")
// Allows developers and admins
func RequireDeveloper() gin.HandlerFunc {
	return RequireRole("developer", "admin")
}

// RequireAdmin is a convenience wrapper for RequireRole("admin")
// Only admins can access
func RequireAdmin() gin.HandlerFunc {
	return RequireRole("admin")
}

// RequireAdminOrStaff allows both admin and staff roles
// Used for operational endpoints (user support, reports, etc.)
func RequireAdminOrStaff() gin.HandlerFunc {
	return RequireRole("admin", "staff")
}

// ═══════════════════════════════════════════════════════════════════════════
// USAGE EXAMPLES
// ═══════════════════════════════════════════════════════════════════════════
//
// Example 1: Admin-only endpoint
//   adminRoutes := router.Group("/api/v1/admin")
//   adminRoutes.Use(middleware.Auth(cfg))        // Authenticate
//   adminRoutes.Use(middleware.RequireAdmin())   // Authorize (admin only)
//   adminRoutes.GET("/users", handler.ListUsers)
//
// Example 2: Developer can create, admin can create/delete
//   propRoutes := router.Group("/api/v1/properties")
//   propRoutes.Use(middleware.Auth(cfg))
//   propRoutes.POST("/", middleware.RequireDeveloper(), handler.CreateProperty)
//   propRoutes.DELETE("/:id", middleware.RequireAdmin(), handler.DeleteProperty)
//
// Example 3: Investor can invest, but admin can also (for testing)
//   investRoutes := router.Group("/api/v1/investments")
//   investRoutes.Use(middleware.Auth(cfg))
//   investRoutes.POST("/", middleware.RequireInvestor(), handler.CreateInvestment)
//
// Example 4: Custom role combination
//   reportRoutes := router.Group("/api/v1/reports")
//   reportRoutes.Use(middleware.Auth(cfg))
//   reportRoutes.GET("/", middleware.RequireRole("admin", "staff", "finance"), handler.GetReports)
//
// ═══════════════════════════════════════════════════════════════════════════
// SECURITY NOTES
// ═══════════════════════════════════════════════════════════════════════════
//
// 1. Always apply Auth() before RequireRole()
//    RequireRole assumes user_id and role exist in context
//
// 2. Don't use RequireRole() alone
//    It only checks role, not authentication
//    Attacker could potentially set context values (unlikely but defensive)
//
// 3. Admin role has full access
//    When using RequireInvestor(), RequireDeveloper(), etc.,
//    admin is always included for operational flexibility
//
// 4. Role changes require re-login
//    Roles are stored in JWT claims
//    If admin changes user role, user must login again to get new token
//
// 5. Principle of least privilege
//    Give users minimum role needed
//    investor: default role for new users
//    developer: only for verified property creators
//    admin: only for trusted employees
//
// ═══════════════════════════════════════════════════════════════════════════

package middleware

import (
	"strings"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/utils/jwt"
)

// Auth is middleware that validates JWT tokens and extracts user information
//
// Purpose:
//   - Extract JWT from Authorization header
//   - Validate token signature and expiration
//   - Extract user_id and role from token claims
//   - Store in Gin context for use by handlers
//   - Reject requests with invalid/missing/expired tokens
//
// Usage in routes:
//   protected := router.Group("/api/v1/protected")
//   protected.Use(middleware.Auth(cfg))
//   protected.GET("/profile", handler.GetProfile)  // Requires authentication
//
// Authorization header format:
//   Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
//
// What handlers get from context after this middleware:
//   user_id := c.GetString("user_id")  // UUID as string
//   role := c.GetString("role")         // "investor", "developer", "admin"
//
// Error responses:
//   - 401 Unauthorized: Missing token, invalid format, expired, bad signature
func Auth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Step 1: Extract Authorization header
		// Standard format: "Authorization: Bearer <token>"
		// Example: "Authorization: Bearer eyJhbGciOiJIUzI1..."
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// No Authorization header present
			// This is a public endpoint being accessed, or client forgot the header
			response.Error(c, http.StatusUnauthorized, "Authorization header required", "unauthorized")
			c.Abort() // Stop processing, don't call handler
			return
		}

		// Step 2: Parse the header format
		// Expected: "Bearer <token>"
		// We split on space and expect exactly 2 parts
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 {
			// Malformed header (e.g., just "eyJhbGc..." without "Bearer")
			response.Error(c, http.StatusUnauthorized, "Invalid authorization header format", "invalid_token")
			c.Abort()
			return
		}

		// Step 3: Verify the scheme is "Bearer"
		// OAuth 2.0 standard token type
		// Other types exist (Basic, Digest) but we only support Bearer
		scheme := parts[0]
		if scheme != "Bearer" {
			response.Error(c, http.StatusUnauthorized, "Invalid authorization scheme", "invalid_token")
			c.Abort()
			return
		}

		// Step 4: Extract the actual token
		tokenString := parts[1]
		if tokenString == "" {
			response.Error(c, http.StatusUnauthorized, "Token is empty", "invalid_token")
			c.Abort()
			return
		}

		// Step 5: Validate the JWT token
		// This checks:
		//   - Token format is correct (three base64 parts)
		//   - Signature matches (token hasn't been tampered with)
		//   - Expiration hasn't passed (exp claim)
		//   - Claims can be parsed
		claims, err := jwt.ValidateToken(tokenString, cfg.JWTSecret)
		if err != nil {
			// Token is invalid, expired, or has wrong signature
			response.Error(c, http.StatusUnauthorized, "Invalid or expired token", "invalid_token")
			c.Abort()
			return
		}

		// Step 6: Store user info in context
		// Handlers can now access these values with:
		//   userID := c.GetString("user_id")
		//   role := c.GetString("role")
		//
		// We store UUID as string because Gin context uses interface{}
		// and UUID → interface{} → UUID requires type assertion
		// String is simpler: string → interface{} → string works naturally
		c.Set("user_id", claims.UserID.String())
		c.Set("role", claims.Role)

		// Step 7: Continue to next middleware or handler
		// Request is authenticated, proceed with business logic
		c.Next()
	}
}

// OptionalAuth is middleware that extracts JWT if present but doesn't require it
//
// Use cases:
//   - Endpoints that behave differently for authenticated vs anonymous users
//   - Example: Property listings show more details to logged-in users
//   - Example: Search results personalized if user is authenticated
//
// If token is present and valid:
//   - Sets "user_id" and "role" in context
//   - Handler can check: if userID, exists := c.Get("user_id"); exists { ... }
//
// If token is missing or invalid:
//   - Does nothing, request proceeds as anonymous
//   - Handler checks: if userID, exists := c.Get("user_id"); !exists { ... }
//
// Usage:
//   router.GET("/api/v1/properties", middleware.OptionalAuth(cfg), handler.ListProperties)
func OptionalAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to extract and validate token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// No token provided - continue as anonymous user
			c.Next()
			return
		}

		// Parse header
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			// Malformed header - continue as anonymous (don't fail request)
			c.Next()
			return
		}

		tokenString := parts[1]
		if tokenString == "" {
			c.Next()
			return
		}

		// Validate token
		claims, err := jwt.ValidateToken(tokenString, cfg.JWTSecret)
		if err != nil {
			// Invalid token - continue as anonymous (don't fail request)
			c.Next()
			return
		}

		// Token is valid - store user info in context
		c.Set("user_id", claims.UserID.String())
		c.Set("role", claims.Role)

		c.Next()
	}
}

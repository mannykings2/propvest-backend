package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS is a middleware that handles Cross-Origin Resource Sharing.
//
// What is CORS?
//   When a frontend (e.g., https://propvest.com) makes an API request to a
//   backend on a different origin (e.g., https://api.propvest.com), browsers
//   block the request by default for security. CORS headers tell the browser
//   "yes, this cross-origin request is allowed."
//
// Why we need this:
//   - Frontend runs on localhost:5173 (Vite dev server)
//   - Backend runs on localhost:8080
//   - These are different origins → browser blocks requests
//   - This middleware allows the frontend to call our API
//
// Production considerations:
//   - Only allow specific origins (not "*")
//   - Origins should come from environment config
//   - Credentials should only be allowed for trusted origins
//
// Headers explained:
//   - Access-Control-Allow-Origin: Which domains can make requests
//   - Access-Control-Allow-Credentials: Allow cookies/auth headers
//   - Access-Control-Allow-Methods: Which HTTP methods are allowed
//   - Access-Control-Allow-Headers: Which request headers are allowed
//   - Access-Control-Expose-Headers: Which response headers JS can read
//   - Access-Control-Max-Age: How long browsers can cache preflight results
func CORS(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if the origin is in our allowed list
		// In production, this would come from environment config
		if isOriginAllowed(origin, allowedOrigins) {
			// Allow this specific origin
			c.Header("Access-Control-Allow-Origin", origin)

			// Allow credentials (cookies, authorization headers)
			c.Header("Access-Control-Allow-Credentials", "true")

			// Allow these HTTP methods
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

			// Allow these request headers
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-Idempotency-Key")

			// Expose these response headers to JavaScript
			c.Header("Access-Control-Expose-Headers", "X-Request-ID, X-RateLimit-Limit, X-RateLimit-Remaining")

			// Cache preflight requests for 12 hours
			c.Header("Access-Control-Max-Age", "43200")
		}

		// Handle preflight requests
		// Browsers send an OPTIONS request before the actual request
		// to check if CORS is allowed
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204) // No Content
			return
		}

		c.Next()
	}
}

// isOriginAllowed checks if an origin is in the allowed list.
// Supports:
//   - Exact matches: http://localhost:5173
//   - Wildcard: * (allow all origins — NEVER use in production)
func isOriginAllowed(origin string, allowedOrigins []string) bool {
	// If no origin header, reject
	if origin == "" {
		return false
	}

	// Check against allowed origins
	for _, allowed := range allowedOrigins {
		// Wildcard: allow all (development only!)
		if allowed == "*" {
			return true
		}

		// Exact match
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}

	return false
}

// ParseAllowedOrigins converts a comma-separated string into a slice.
// This is used to parse the ALLOWED_ORIGINS environment variable.
//
// Example:
//   "http://localhost:5173,http://localhost:3000" → ["http://localhost:5173", "http://localhost:3000"]
func ParseAllowedOrigins(originsString string) []string {
	if originsString == "" {
		return []string{}
	}

	origins := strings.Split(originsString, ",")
	result := make([]string, 0, len(origins))

	for _, origin := range origins {
		trimmed := strings.TrimSpace(origin)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

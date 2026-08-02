package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID is a middleware that generates a unique ID for every incoming request.
//
// Why Request IDs Matter:
//   1. Debugging: When a user reports "my payment failed", you can search logs for
//      their specific request ID instead of wading through millions of log lines.
//   2. Distributed Tracing: If this request triggers background jobs, webhooks, or
//      external API calls, they all carry the same request ID.
//   3. Error Tracking: When something goes wrong, the request ID appears in the
//      error response so users can include it in support tickets.
//
// The ID is stored in:
//   - Gin context (accessible via c.Get("request_id"))
//   - Response header (X-Request-ID)
//   - All structured logs for this request
//
// Usage in handlers:
//   requestID := c.GetString("request_id")
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if the client sent a request ID (some API clients do this)
		requestID := c.GetHeader("X-Request-ID")

		// If not, generate a new UUID
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Store in Gin context so handlers and other middleware can access it
		c.Set("request_id", requestID)

		// Return it in the response header so the client knows which request this was
		c.Header("X-Request-ID", requestID)

		// Continue to the next middleware/handler
		c.Next()
	}
}

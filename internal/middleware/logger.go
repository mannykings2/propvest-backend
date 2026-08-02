package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"log"
)

// Logger is a middleware that logs every HTTP request in a structured format.
//
// What it logs:
//   - Request ID (for tracing)
//   - HTTP method (GET, POST, etc.)
//   - Request path (/api/v1/users/me)
//   - Status code (200, 404, 500, etc.)
//   - Duration (how long the request took)
//   - Client IP address
//   - User-Agent (browser/client information)
//
// Why structured logging matters:
//   - Easy to search: "Show me all 500 errors"
//   - Easy to aggregate: "What's the average request duration?"
//   - Easy to alert: "Notify me if error rate exceeds 1%"
//
// In production, this would use a structured logger like Zap or Zerolog
// that outputs JSON instead of plain text.
//
// TODO: Replace with structured logger (Zap) in later implementation
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Record start time
		startTime := time.Now()

		// Get request ID from context (set by RequestID middleware)
		requestID := c.GetString("request_id")

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(startTime)

		// Log the request
		// In production, this would be structured JSON logging
		log.Printf(
			"[%s] %s %s %d %v %s",
			requestID,
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			duration,
			c.ClientIP(),
		)

		// If there were any errors during request processing, log them
		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				log.Printf("[%s] Error: %v", requestID, err.Error())
			}
		}
	}
}

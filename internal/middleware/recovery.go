package middleware

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// Recovery is a middleware that recovers from panics and returns a 500 error.
//
// Why this matters:
//   A panic in one request should NOT crash the entire server.
//   Without this middleware, a single nil pointer dereference would bring
//   down the API for all users.
//
// What it does:
//   1. Catches any panic that occurs during request handling
//   2. Logs the panic message and stack trace
//   3. Returns HTTP 500 to the client
//   4. Allows the server to continue serving other requests
//
// In production:
//   - Send panic details to error tracking (Sentry, Rollbar, etc.)
//   - Alert the on-call engineer immediately
//   - Include request ID for debugging
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get request ID for debugging
				requestID := c.GetString("request_id")

				// Get the stack trace
				stack := debug.Stack()

				// Log the panic with full details
				// In production, this would go to error tracking service
				log.Printf(
					"[PANIC RECOVERED] RequestID: %s\nError: %v\nStack Trace:\n%s",
					requestID,
					err,
					string(stack),
				)

				// Return a generic error to the client
				// We don't expose the panic details for security reasons
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":      "Internal server error",
					"request_id": requestID,
				})

				// Abort prevents any remaining handlers from running
				c.Abort()
			}
		}()

		// Continue processing the request
		c.Next()
	}
}

// PanicMessage extracts a clean error message from a panic.
// Panics can be any type (string, error, struct, etc.) so we handle all cases.
func PanicMessage(err interface{}) string {
	switch e := err.(type) {
	case string:
		return e
	case error:
		return e.Error()
	default:
		return fmt.Sprintf("%v", e)
	}
}

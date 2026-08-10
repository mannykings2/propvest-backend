package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/logger"
)

// Logger is middleware that logs every HTTP request as a single STRUCTURED line.
//
// The engineering docs (6.2 §15) require these fields on every request log:
//   request_id, method, path, status, duration_ms, ip, user_agent, and user_id
//   when the request is authenticated.
//
// We build a request-scoped slog logger that already carries request_id (and
// user_id once the auth middleware has run) and stash it on the context so
// handlers/services can log with the same correlation id via
// logger.FromContext(c.Request.Context()).
//
// Log level is chosen from the response status: 5xx -> ERROR, 4xx -> WARN,
// everything else -> INFO. That makes "show me all failures" a level filter
// instead of a text search.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		requestID := c.GetString("request_id")

		// Bind request_id to a logger and place it on the context BEFORE the
		// handler runs, so anything the handler logs is correlated.
		reqLogger := logger.L().With("request_id", requestID)
		ctx := logger.WithContext(c.Request.Context(), reqLogger)
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		// user_id is set by the Auth middleware (if the route is protected).
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		}
		if uid := c.GetString("user_id"); uid != "" {
			attrs = append(attrs, "user_id", uid)
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "gin_errors", c.Errors.String())
		}

		msg := "http_request"
		switch {
		case status >= 500:
			reqLogger.Error(msg, attrs...)
		case status >= 400:
			reqLogger.Warn(msg, attrs...)
		default:
			reqLogger.Info(msg, attrs...)
		}
	}
}

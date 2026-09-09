package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/response"
)

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	// Requests per minute for anonymous users
	AnonymousLimit int

	// Requests per minute for authenticated users
	AuthenticatedLimit int

	// Requests per minute for sensitive auth endpoints (login, register)
	AuthEndpointLimit int
}

// rateLimitEntry tracks request count and reset time for a single key
type rateLimitEntry struct {
	count     int
	resetTime time.Time
	mu        sync.Mutex
}

// rateLimiter is an in-memory rate limiter using token bucket algorithm
//
// PRODUCTION NOTE: This implementation stores rate limit data in memory.
// For production with multiple servers, consider using Redis:
//   - github.com/go-redis/redis_rate (Redis-based rate limiting)
//   - Allows rate limiting across multiple API instances
//   - Persistent across server restarts
//
// This in-memory implementation is suitable for:
//   - Single server deployments
//   - Development/testing
//   - MVP/early stage
type rateLimiter struct {
	entries map[string]*rateLimitEntry
	mu      sync.RWMutex
	config  RateLimitConfig
}

// newRateLimiter creates a new in-memory rate limiter
func newRateLimiter(config RateLimitConfig) *rateLimiter {
	limiter := &rateLimiter{
		entries: make(map[string]*rateLimitEntry),
		config:  config,
	}

	// Start cleanup goroutine to prevent memory leaks
	go limiter.cleanup()

	return limiter
}

// allow checks if a request is allowed and increments the counter
func (rl *rateLimiter) allow(key string, limit int) (bool, int, time.Time) {
	rl.mu.Lock()
	entry, exists := rl.entries[key]
	if !exists {
		entry = &rateLimitEntry{
			count:     0,
			resetTime: time.Now().Add(time.Minute),
		}
		rl.entries[key] = entry
	}
	rl.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now()

	// Reset if window has passed
	if now.After(entry.resetTime) {
		entry.count = 0
		entry.resetTime = now.Add(time.Minute)
	}

	// Check if limit exceeded
	if entry.count >= limit {
		return false, limit - entry.count, entry.resetTime
	}

	// Increment and allow
	entry.count++
	return true, limit - entry.count, entry.resetTime
}

// cleanup periodically removes expired entries to prevent memory leaks
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		rl.mu.Lock()
		for key, entry := range rl.entries {
			entry.mu.Lock()
			if now.After(entry.resetTime.Add(time.Minute)) {
				delete(rl.entries, key)
			}
			entry.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

// Global rate limiter instance
var globalRateLimiter *rateLimiter

// RateLimit returns a middleware that limits requests per client
//
// Rate limiting prevents abuse by limiting how many requests a client can make.
// Different limits apply based on authentication status:
//
//   - Anonymous users: Lower limit (default: 60/min)
//   - Authenticated users: Higher limit (default: 300/min)
//   - Auth endpoints (login/register): Very low limit (default: 10/min)
//
// Key generation:
//   - Anonymous: Uses IP address
//   - Authenticated: Uses user ID (more accurate, can't be bypassed)
//
// Headers set in response:
//   - X-RateLimit-Limit: Total requests allowed per window
//   - X-RateLimit-Remaining: Requests remaining in current window
//   - X-RateLimit-Reset: Unix timestamp when the limit resets
//   - Retry-After: Seconds until limit resets (only when blocked)
//
// PRODUCTION UPGRADE:
//   For production with multiple servers, replace this with Redis-based
//   rate limiting:
//
//     import "github.com/go-redis/redis_rate"
//
//     limiter := redis_rate.NewLimiter(redisClient)
//     res, err := limiter.Allow(ctx, key, redis_rate.PerMinute(60))
//
// Example usage:
//
//	// Apply to specific routes
//	authRoutes := r.Group("/api/v1/auth")
//	authRoutes.Use(middleware.RateLimit(middleware.RateLimitConfig{
//		AnonymousLimit: 10,
//		AuthenticatedLimit: 20,
//		AuthEndpointLimit: 5,
//	}))
func RateLimit(config RateLimitConfig) gin.HandlerFunc {
	// Set defaults if not provided
	if config.AnonymousLimit == 0 {
		config.AnonymousLimit = 60 // 60 requests per minute
	}
	if config.AuthenticatedLimit == 0 {
		config.AuthenticatedLimit = 300 // 300 requests per minute
	}
	if config.AuthEndpointLimit == 0 {
		config.AuthEndpointLimit = 10 // 10 requests per minute for login/register
	}

	// Initialize global rate limiter if not already done
	if globalRateLimiter == nil {
		globalRateLimiter = newRateLimiter(config)
	}

	return func(c *gin.Context) {
		// Determine rate limit based on endpoint and authentication
		limit := config.AnonymousLimit
		key := c.ClientIP() // Default: use IP address

		// Check if user is authenticated
		if userID, exists := c.Get("user_id"); exists {
			// Authenticated user - higher limit, use user ID as key
			limit = config.AuthenticatedLimit
			key = fmt.Sprintf("user:%v", userID)
		}

		// Special handling for auth endpoints (login, register, password reset)
		path := c.Request.URL.Path
		if isAuthEndpoint(path) {
			limit = config.AuthEndpointLimit
			// Keep IP-based key for auth endpoints to prevent account enumeration
			key = fmt.Sprintf("auth:%s", c.ClientIP())
		}

		// Check rate limit
		allowed, remaining, resetTime := globalRateLimiter.allow(key, limit)

		// Set rate limit headers
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

		if !allowed {
			// Rate limit exceeded
			retryAfter := int(time.Until(resetTime).Seconds())
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))

			response.Error(c, apperrors.ErrRateLimitExceeded)
			c.Abort()
			return
		}

		c.Next()
	}
}

// isAuthEndpoint checks if the path is an authentication endpoint
func isAuthEndpoint(path string) bool {
	authPaths := []string{
		"/api/v1/auth/login",
		"/api/v1/auth/register",
		"/api/v1/auth/forgot-password",
		"/api/v1/auth/reset-password",
		"/api/v1/auth/verify-email",
		"/api/v1/auth/resend-verification",
	}

	for _, authPath := range authPaths {
		if path == authPath {
			return true
		}
	}
	return false
}

// RateLimitStrict returns a stricter rate limiter for sensitive endpoints
//
// Use this for endpoints that are expensive or security-sensitive:
//   - File uploads
//   - Password changes
//   - Email verification requests
//   - OTP generation
//
// Example:
//
//	r.POST("/api/v1/users/me/avatar", middleware.RateLimitStrict(5), handler)
func RateLimitStrict(requestsPerMinute int) gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		AnonymousLimit:     requestsPerMinute,
		AuthenticatedLimit: requestsPerMinute,
		AuthEndpointLimit:  requestsPerMinute,
	})
}

// RateLimitByIP returns a rate limiter that always uses IP address as key
//
// Use this when you want to limit requests regardless of authentication:
//   - Webhook endpoints
//   - Public API endpoints
//   - Health checks (if needed)
//
// Example:
//
//	r.POST("/webhooks/paystack", middleware.RateLimitByIP(100), handler)
func RateLimitByIP(requestsPerMinute int) gin.HandlerFunc {
	if globalRateLimiter == nil {
		globalRateLimiter = newRateLimiter(RateLimitConfig{})
	}

	return func(c *gin.Context) {
		key := c.ClientIP()
		allowed, remaining, resetTime := globalRateLimiter.allow(key, requestsPerMinute)

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", requestsPerMinute))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

		if !allowed {
			retryAfter := int(time.Until(resetTime).Seconds())
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "Too many requests. Please try again later.",
				"code":    "rate_limit_exceeded",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}


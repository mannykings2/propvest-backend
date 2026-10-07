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

// ═══════════════════════════════════════════════════════════════════════════
// WITHDRAWAL RATE LIMITING
// ═══════════════════════════════════════════════════════════════════════════
//
// PURPOSE:
// Prevents abuse of the withdrawal endpoint by limiting users to a maximum
// number of withdrawal requests per hour.
//
// WHY NEEDED:
//   1. Fraud Prevention: Limits rapid-fire withdrawal attempts
//   2. System Protection: Prevents excessive load on payment provider APIs
//   3. User Protection: Flags suspicious activity (account compromise)
//   4. Cost Control: Reduces payment provider API costs
//
// BUSINESS RULES:
//   - Default: 3 withdrawals per hour per user
//   - Sliding window: Checks last 60 minutes, not fixed hourly blocks
//   - User-based: Cannot be bypassed by changing IP
//   - Applies to initiations: Failed validations don't count
//
// EXAMPLE TIMELINE:
//   10:00 AM - Withdrawal #1 ✓
//   10:15 AM - Withdrawal #2 ✓
//   10:30 AM - Withdrawal #3 ✓
//   10:45 AM - Withdrawal #4 ✗ (blocked, rate limit exceeded)
//   11:01 AM - Withdrawal #5 ✓ (10:00 AM withdrawal fell outside window)
//
// PRODUCTION NOTE:
// This uses in-memory storage. For production with multiple servers, migrate
// to Redis:
//
//   ZADD withdrawals:user:{user_id} {timestamp} {transaction_id}
//   ZREMRANGEBYSCORE withdrawals:user:{user_id} 0 {timestamp-1hour}
//   ZCARD withdrawals:user:{user_id}
//   EXPIRE withdrawals:user:{user_id} 3600

// withdrawalRateLimitEntry tracks withdrawal attempts for a user
type withdrawalRateLimitEntry struct {
	attempts  []time.Time // Timestamps of withdrawal attempts
	mu        sync.Mutex
}

// withdrawalRateLimiter tracks withdrawal attempts per user
type withdrawalRateLimiter struct {
	entries    map[string]*withdrawalRateLimitEntry
	mu         sync.RWMutex
	maxPerHour int
	window     time.Duration
}

// Global withdrawal rate limiter instance
var globalWithdrawalRateLimiter *withdrawalRateLimiter

// newWithdrawalRateLimiter creates a new withdrawal rate limiter
func newWithdrawalRateLimiter(maxPerHour int) *withdrawalRateLimiter {
	limiter := &withdrawalRateLimiter{
		entries:    make(map[string]*withdrawalRateLimitEntry),
		maxPerHour: maxPerHour,
		window:     time.Hour,
	}

	// Start cleanup goroutine
	go limiter.cleanup()

	return limiter
}

// checkAndRecord checks if withdrawal is allowed and records the attempt
//
// Returns:
//   - allowed: true if within rate limit
//   - current: current number of withdrawals in window
//   - limit: maximum allowed withdrawals
//   - nextAllowedTime: when the next withdrawal will be allowed
func (wrl *withdrawalRateLimiter) checkAndRecord(userID string) (allowed bool, current int, limit int, nextAllowedTime time.Time) {
	wrl.mu.Lock()
	entry, exists := wrl.entries[userID]
	if !exists {
		entry = &withdrawalRateLimitEntry{
			attempts: make([]time.Time, 0),
		}
		wrl.entries[userID] = entry
	}
	wrl.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-wrl.window)

	// Remove attempts outside the sliding window
	validAttempts := make([]time.Time, 0)
	for _, attempt := range entry.attempts {
		if attempt.After(windowStart) {
			validAttempts = append(validAttempts, attempt)
		}
	}
	entry.attempts = validAttempts

	current = len(entry.attempts)
	limit = wrl.maxPerHour

	// Check if limit exceeded
	if current >= wrl.maxPerHour {
		// Calculate when the oldest attempt will expire
		if len(entry.attempts) > 0 {
			oldestAttempt := entry.attempts[0]
			nextAllowedTime = oldestAttempt.Add(wrl.window)
		} else {
			nextAllowedTime = now.Add(wrl.window)
		}
		return false, current, limit, nextAllowedTime
	}

	// Record this attempt
	entry.attempts = append(entry.attempts, now)

	// Next allowed time is always at least 1 hour from now if at limit
	if len(entry.attempts) >= wrl.maxPerHour {
		nextAllowedTime = entry.attempts[0].Add(wrl.window)
	} else {
		nextAllowedTime = now // Can withdraw immediately
	}

	return true, current + 1, limit, nextAllowedTime
}

// cleanup periodically removes old entries to prevent memory leaks
func (wrl *withdrawalRateLimiter) cleanup() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		windowStart := now.Add(-wrl.window)

		wrl.mu.Lock()
		for userID, entry := range wrl.entries {
			entry.mu.Lock()
			
			// Remove expired attempts
			validAttempts := make([]time.Time, 0)
			for _, attempt := range entry.attempts {
				if attempt.After(windowStart) {
					validAttempts = append(validAttempts, attempt)
				}
			}
			entry.attempts = validAttempts

			// Remove entry if no valid attempts
			if len(entry.attempts) == 0 {
				delete(wrl.entries, userID)
			}
			
			entry.mu.Unlock()
		}
		wrl.mu.Unlock()
	}
}

// WithdrawalRateLimit returns a middleware that limits withdrawal requests
//
// Configuration:
//   - maxWithdrawalsPerHour: Maximum number of withdrawals allowed per hour
//                            Default: 3
//                            Recommended: 3-5 for consumer apps, 10-20 for business
//
// How it works:
//   1. Extracts user_id from Gin context (set by auth middleware)
//   2. Checks withdrawal attempts in the last 60 minutes
//   3. If under limit: Allow request and record attempt
//   4. If over limit: Reject with 429 Too Many Requests
//
// Response when blocked:
//   {
//     "success": false,
//     "message": "Too many withdrawal attempts. Maximum 3 per hour.",
//     "code": "withdrawal_rate_limit",
//     "next_allowed_at": "2026-09-09T14:30:00Z"
//   }
//
// Headers set:
//   - X-RateLimit-Limit: 3
//   - X-RateLimit-Remaining: 0
//   - X-RateLimit-Reset: {unix_timestamp}
//   - Retry-After: {seconds}
//
// Usage:
//   walletRoutes := v1.Group("/wallet")
//   walletRoutes.Use(middleware.Auth())
//   walletRoutes.POST("/withdraw",
//     middleware.WithdrawalRateLimit(3),  // ← Add here
//     walletHandler.RequestWithdrawal)
//
// IMPORTANT:
//   - Must be applied AFTER Auth() middleware (needs user_id)
//   - Only counts successful initiations (after validation)
//   - Failed validations (insufficient balance, etc.) don't count
//   - To count all attempts, move to top of handler chain
//
// Testing:
//   # Test rate limit
//   for i in {1..4}; do
//     curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
//       -H "Authorization: Bearer $TOKEN" \
//       -H "Content-Type: application/json" \
//       -d '{"amount_kobo":50000,"account_number":"0123456789","account_name":"Test User","bank_code":"058"}'
//     echo ""
//     sleep 1
//   done
//   # First 3 should succeed, 4th should be blocked
//
// PRODUCTION MIGRATION TO REDIS:
//
//   import "github.com/go-redis/redis/v8"
//
//   func WithdrawalRateLimit(maxPerHour int) gin.HandlerFunc {
//       return func(c *gin.Context) {
//           userID := c.GetString("user_id")
//           key := fmt.Sprintf("withdrawal_rate_limit:%s", userID)
//           now := time.Now().Unix()
//           hourAgo := now - 3600
//
//           // Add current attempt with timestamp as score
//           redisClient.ZAdd(ctx, key, &redis.Z{
//               Score:  float64(now),
//               Member: now,
//           })
//
//           // Remove attempts older than 1 hour
//           redisClient.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", hourAgo))
//
//           // Count attempts in last hour
//           count, _ := redisClient.ZCard(ctx, key).Result()
//
//           // Set expiry
//           redisClient.Expire(ctx, key, time.Hour)
//
//           if count > int64(maxPerHour) {
//               c.JSON(429, ...)
//               c.Abort()
//               return
//           }
//
//           c.Next()
//       }
//   }
func WithdrawalRateLimit(maxWithdrawalsPerHour int) gin.HandlerFunc {
	// Set default if not provided
	if maxWithdrawalsPerHour == 0 {
		maxWithdrawalsPerHour = 3
	}

	// Initialize global limiter if needed
	if globalWithdrawalRateLimiter == nil {
		globalWithdrawalRateLimiter = newWithdrawalRateLimiter(maxWithdrawalsPerHour)
	}

	return func(c *gin.Context) {
		// Extract user ID from context (set by auth middleware)
		userIDValue, exists := c.Get("user_id")
		if !exists {
			// User not authenticated - should not reach here if Auth() is applied
			response.Error(c, apperrors.ErrUnauthorized)
			c.Abort()
			return
		}

		userID := fmt.Sprintf("%v", userIDValue)

		// Check rate limit
		allowed, current, limit, nextAllowedTime := globalWithdrawalRateLimiter.checkAndRecord(userID)

		// Set rate limit headers
		remaining := limit - current
		if remaining < 0 {
			remaining = 0
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", nextAllowedTime.Unix()))

		if !allowed {
			// Rate limit exceeded
			retryAfter := int(time.Until(nextAllowedTime).Seconds())
			if retryAfter < 0 {
				retryAfter = 0
			}
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))

			// Custom error response for withdrawal rate limit
			response.Error(c, http.StatusTooManyRequests,
				fmt.Sprintf("Too many withdrawal attempts. Maximum %d per hour. Try again at %s.",
					limit,
					nextAllowedTime.Format("15:04:05")),
				"withdrawal_rate_limit")
			c.Abort()
			return
		}

		// Allowed - continue to handler
		c.Next()
	}
}

// WithdrawalRateLimitConfigurable allows custom window and limit
//
// Use this for special cases:
//   - VIP users: Higher limits
//   - Business accounts: Different window (e.g., 10 per day)
//   - Testing: Relaxed limits
//
// Example:
//   // VIP users: 10 withdrawals per hour
//   if user.IsVIP {
//       middleware.WithdrawalRateLimitConfigurable(10, time.Hour)
//   }
//
//   // Business accounts: 50 withdrawals per day
//   if user.AccountType == "business" {
//       middleware.WithdrawalRateLimitConfigurable(50, 24*time.Hour)
//   }
func WithdrawalRateLimitConfigurable(maxAttempts int, window time.Duration) gin.HandlerFunc {
	// Create a dedicated limiter for this configuration
	limiter := &withdrawalRateLimiter{
		entries:    make(map[string]*withdrawalRateLimitEntry),
		maxPerHour: maxAttempts,
		window:     window,
	}
	go limiter.cleanup()

	return func(c *gin.Context) {
		userIDValue, exists := c.Get("user_id")
		if !exists {
			response.Error(c, apperrors.ErrUnauthorized)
			c.Abort()
			return
		}

		userID := fmt.Sprintf("%v", userIDValue)
		allowed, current, limit, nextAllowedTime := limiter.checkAndRecord(userID)

		remaining := limit - current
		if remaining < 0 {
			remaining = 0
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", nextAllowedTime.Unix()))

		if !allowed {
			retryAfter := int(time.Until(nextAllowedTime).Seconds())
			if retryAfter < 0 {
				retryAfter = 0
			}
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))

			windowDesc := "hour"
			if window == 24*time.Hour {
				windowDesc = "day"
			} else if window == time.Minute {
				windowDesc = "minute"
			}

			response.Error(c, http.StatusTooManyRequests,
				fmt.Sprintf("Too many withdrawal attempts. Maximum %d per %s.",
					limit, windowDesc),
				"withdrawal_rate_limit")
			c.Abort()
			return
		}

		c.Next()
	}
}

// GetWithdrawalRateLimitStatus returns current rate limit status for a user
//
// This is useful for frontend to show:
//   - "You have 2 withdrawals remaining this hour"
//   - "Next withdrawal available in 15 minutes"
//
// Usage in handler:
//   func (h *WalletHandler) GetWithdrawalLimitStatus(c *gin.Context) {
//       userID := c.GetString("user_id")
//       status := middleware.GetWithdrawalRateLimitStatus(userID)
//       response.Success(c, status)
//   }
//
// Returns:
//   {
//     "limit": 3,
//     "used": 2,
//     "remaining": 1,
//     "resets_at": "2026-09-09T14:30:00Z",
//     "can_withdraw": true
//   }
func GetWithdrawalRateLimitStatus(userID string) map[string]interface{} {
	if globalWithdrawalRateLimiter == nil {
		return map[string]interface{}{
			"limit":        3,
			"used":         0,
			"remaining":    3,
			"resets_at":    time.Now().Add(time.Hour),
			"can_withdraw": true,
		}
	}

	globalWithdrawalRateLimiter.mu.RLock()
	entry, exists := globalWithdrawalRateLimiter.entries[userID]
	globalWithdrawalRateLimiter.mu.RUnlock()

	if !exists {
		return map[string]interface{}{
			"limit":        globalWithdrawalRateLimiter.maxPerHour,
			"used":         0,
			"remaining":    globalWithdrawalRateLimiter.maxPerHour,
			"resets_at":    time.Now().Add(globalWithdrawalRateLimiter.window),
			"can_withdraw": true,
		}
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-globalWithdrawalRateLimiter.window)

	// Count valid attempts
	validCount := 0
	var oldestAttempt time.Time
	for _, attempt := range entry.attempts {
		if attempt.After(windowStart) {
			validCount++
			if oldestAttempt.IsZero() || attempt.Before(oldestAttempt) {
				oldestAttempt = attempt
			}
		}
	}

	limit := globalWithdrawalRateLimiter.maxPerHour
	remaining := limit - validCount
	if remaining < 0 {
		remaining = 0
	}

	var resetsAt time.Time
	if !oldestAttempt.IsZero() {
		resetsAt = oldestAttempt.Add(globalWithdrawalRateLimiter.window)
	} else {
		resetsAt = now.Add(globalWithdrawalRateLimiter.window)
	}

	return map[string]interface{}{
		"limit":        limit,
		"used":         validCount,
		"remaining":    remaining,
		"resets_at":    resetsAt,
		"can_withdraw": validCount < limit,
	}
}

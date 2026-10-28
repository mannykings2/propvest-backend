package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ═══════════════════════════════════════════════════════════════════════════
// SETUP HELPERS
// ═══════════════════════════════════════════════════════════════════════════

// setupRateLimitTest creates a fresh rate limiter for testing
func setupRateLimitTest() {
	globalRateLimiter = nil
}

// ═══════════════════════════════════════════════════════════════════════════
// RATE LIMIT TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestRateLimit_AnonymousUser tests rate limiting for unauthenticated users
func TestRateLimit_AnonymousUser(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit:     3, // Low limit for testing
		AuthenticatedLimit: 10,
		AuthEndpointLimit:  2,
	}

	router.GET("/test", RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// First 3 requests should succeed
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345" // Same IP
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Request %d should succeed", i)
		assert.Equal(t, "3", w.Header().Get("X-RateLimit-Limit"))
	}

	// 4th request should be blocked
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Header().Get("Retry-After"), "")
}

// TestRateLimit_AuthenticatedUser tests higher limits for authenticated users
func TestRateLimit_AuthenticatedUser(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit:     3,
		AuthenticatedLimit: 10, // Higher limit
		AuthEndpointLimit:  2,
	}

	router.GET("/test", func(c *gin.Context) {
		c.Set("user_id", "user-123") // Simulate authenticated user
		c.Next()
	}, RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// First 10 requests should succeed
	for i := 1; i <= 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Request %d should succeed", i)
		assert.Equal(t, "10", w.Header().Get("X-RateLimit-Limit"))
	}

	// 11th request should be blocked
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestRateLimit_DifferentIPs tests that different IPs have separate limits
func TestRateLimit_DifferentIPs(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit:     2,
		AuthenticatedLimit: 10,
		AuthEndpointLimit:  1,
	}

	router.GET("/test", RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// IP 1: Use up limit
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// IP 1: Should be blocked
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusTooManyRequests, w1.Code)

	// IP 2: Should still work (different IP)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "192.168.1.2:12345"
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestRateLimit_DifferentUsers tests that different users have separate limits
func TestRateLimit_DifferentUsers(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit:     2,
		AuthenticatedLimit: 2,
		AuthEndpointLimit:  1,
	}

	// User 1 exhausts limit
	router.GET("/test1", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	}, RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// User 1: Should be blocked
	req1 := httptest.NewRequest(http.MethodGet, "/test1", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusTooManyRequests, w1.Code)

	// User 2: Should still work (different user)
	router.GET("/test2", func(c *gin.Context) {
		c.Set("user_id", "user-2")
		c.Next()
	}, RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req2 := httptest.NewRequest(http.MethodGet, "/test2", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestRateLimit_AuthEndpoints tests stricter limits for auth endpoints
func TestRateLimit_AuthEndpoints(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit:     10,
		AuthenticatedLimit: 20,
		AuthEndpointLimit:  2, // Very low for auth endpoints
	}

	router.POST("/api/v1/auth/login", RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// First 2 requests should succeed
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Request %d should succeed", i)
		assert.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
	}

	// 3rd request should be blocked
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestRateLimit_RemainingHeader tests X-RateLimit-Remaining header
func TestRateLimit_RemainingHeader(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit:     5,
		AuthenticatedLimit: 10,
		AuthEndpointLimit:  3,
	}

	router.GET("/test", RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Track remaining count
	expectedRemaining := []string{"4", "3", "2", "1", "0"}

	for i, expected := range expectedRemaining {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, expected, w.Header().Get("X-RateLimit-Remaining"),
			"Request %d should have remaining=%s", i+1, expected)
	}
}

// TestRateLimit_ResetHeader tests X-RateLimit-Reset header
func TestRateLimit_ResetHeader(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit:     5,
		AuthenticatedLimit: 10,
		AuthEndpointLimit:  3,
	}

	router.GET("/test", RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	before := time.Now().Unix()
	router.ServeHTTP(w, req)
	after := time.Now().Add(time.Minute).Unix()

	resetHeader := w.Header().Get("X-RateLimit-Reset")
	assert.NotEmpty(t, resetHeader)

	// Reset should be roughly 1 minute from now
	var resetTime int64
	fmt.Sscanf(resetHeader, "%d", &resetTime)
	assert.True(t, resetTime >= before && resetTime <= after,
		"Reset time should be within 1 minute from now")
}

// TestRateLimit_DefaultValues tests that defaults are applied
func TestRateLimit_DefaultValues(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	// Empty config should use defaults
	router.GET("/test", RateLimit(RateLimitConfig{}), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Default anonymous limit is 60
	assert.Equal(t, "60", w.Header().Get("X-RateLimit-Limit"))
}

// TestIsAuthEndpoint tests the isAuthEndpoint helper function
func TestIsAuthEndpoint(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"/api/v1/auth/login", true},
		{"/api/v1/auth/register", true},
		{"/api/v1/auth/forgot-password", true},
		{"/api/v1/auth/reset-password", true},
		{"/api/v1/auth/verify-email", true},
		{"/api/v1/auth/resend-verification", true},
		{"/api/v1/auth/logout", false},
		{"/api/v1/users/profile", false},
		{"/api/v1/wallet/balance", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := isAuthEndpoint(tc.path)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// RATE LIMIT STRICT TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestRateLimitStrict tests the strict rate limiter
func TestRateLimitStrict(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	router.POST("/upload", RateLimitStrict(3), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// First 3 should succeed
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/upload", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 4th should be blocked
	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestRateLimitStrict_SameForAllUsers tests that strict limits apply equally
func TestRateLimitStrict_SameForAllUsers(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	router.POST("/upload", func(c *gin.Context) {
		// Simulate authenticated user
		c.Set("user_id", "user-123")
		c.Next()
	}, RateLimitStrict(2), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Even authenticated users should have the strict limit
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/upload", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 3rd should be blocked (strict limit of 2)
	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// ═══════════════════════════════════════════════════════════════════════════
// RATE LIMIT BY IP TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestRateLimitByIP tests IP-based rate limiting
func TestRateLimitByIP(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	router.POST("/webhook", RateLimitByIP(5), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// First 5 should succeed
	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "5", w.Header().Get("X-RateLimit-Limit"))
	}

	// 6th should be blocked
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestRateLimitByIP_IgnoresAuth tests that RateLimitByIP ignores authentication
func TestRateLimitByIP_IgnoresAuth(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	router.POST("/webhook", func(c *gin.Context) {
		// Even with user_id set, should use IP-based limiting
		c.Set("user_id", "user-123")
		c.Next()
	}, RateLimitByIP(3), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Should be limited by IP (3), not user (which might have different limit)
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 4th blocked
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// ═══════════════════════════════════════════════════════════════════════════
// INTEGRATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestRateLimit_MixedEndpoints tests different endpoints with different limits
func TestRateLimit_MixedEndpoints(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit:     5,
		AuthenticatedLimit: 10,
		AuthEndpointLimit:  2,
	}

	// Normal endpoint
	router.GET("/api/v1/data", RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "data"})
	})

	// Auth endpoint
	router.POST("/api/v1/auth/login", RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "login"})
	})

	// Use up normal endpoint limit (5)
	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/data", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Normal endpoint should be blocked
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/data", nil)
	req1.RemoteAddr = "192.168.1.100:12345"
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusTooManyRequests, w1.Code)

	// Auth endpoint should still work (separate limit)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req2.RemoteAddr = "192.168.1.100:12345"
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestRateLimit_Headers tests all rate limit headers are set correctly
func TestRateLimit_Headers(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit: 10,
	}

	router.GET("/test", RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
}

// TestRateLimit_HeadersWhenBlocked tests headers when rate limit exceeded
func TestRateLimit_HeadersWhenBlocked(t *testing.T) {
	setupRateLimitTest()
	router := setupTestRouter()

	config := RateLimitConfig{
		AnonymousLimit: 2,
	}

	router.GET("/test", RateLimit(config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Exhaust limit
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Blocked request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

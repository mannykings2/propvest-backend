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

// setupWithdrawalRateLimitTest creates a fresh withdrawal rate limiter
func setupWithdrawalRateLimitTest() {
	globalWithdrawalRateLimiter = nil
}

// ═══════════════════════════════════════════════════════════════════════════
// WITHDRAWAL RATE LIMIT TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestWithdrawalRateLimit_Success tests successful withdrawals within limit
func TestWithdrawalRateLimit_Success(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(3), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "withdrawal initiated"})
	})

	// First 3 withdrawals should succeed
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Withdrawal %d should succeed", i)
		assert.Equal(t, "3", w.Header().Get("X-RateLimit-Limit"))
	}
}

// TestWithdrawalRateLimit_ExceedsLimit tests blocking when limit exceeded
func TestWithdrawalRateLimit_ExceedsLimit(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(3), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "withdrawal initiated"})
	})

	// Use up limit
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 4th withdrawal should be blocked
	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "Too many withdrawal attempts")
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

// TestWithdrawalRateLimit_DifferentUsers tests separate limits per user
func TestWithdrawalRateLimit_DifferentUsers(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	// User 1 exhausts limit
	router.POST("/withdraw1", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	}, WithdrawalRateLimit(2), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// User 1: Should be blocked
	req1 := httptest.NewRequest(http.MethodPost, "/withdraw1", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusTooManyRequests, w1.Code)

	// User 2: Should still work
	router.POST("/withdraw2", func(c *gin.Context) {
		c.Set("user_id", "user-2")
		c.Next()
	}, WithdrawalRateLimit(2), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req2 := httptest.NewRequest(http.MethodPost, "/withdraw2", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestWithdrawalRateLimit_Unauthenticated tests rejection of unauthenticated requests
func TestWithdrawalRateLimit_Unauthenticated(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", WithdrawalRateLimit(3), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// No user_id in context
	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestWithdrawalRateLimit_DefaultLimit tests default limit of 3
func TestWithdrawalRateLimit_DefaultLimit(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(0), func(c *gin.Context) { // 0 = use default
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// First 3 should succeed (default)
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 4th should be blocked
	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestWithdrawalRateLimit_RemainingHeader tests X-RateLimit-Remaining header
func TestWithdrawalRateLimit_RemainingHeader(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(5), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	expectedRemaining := []string{"4", "3", "2", "1", "0"}

	for i, expected := range expectedRemaining {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, expected, w.Header().Get("X-RateLimit-Remaining"),
			"Withdrawal %d should have remaining=%s", i+1, expected)
	}
}

// TestWithdrawalRateLimit_Headers tests all headers are set
func TestWithdrawalRateLimit_Headers(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(3), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "3", w.Header().Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
}

// TestWithdrawalRateLimit_RetryAfterHeader tests Retry-After header when blocked
func TestWithdrawalRateLimit_RetryAfterHeader(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(2), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Exhaust limit
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Blocked request
	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	
	retryAfter := w.Header().Get("Retry-After")
	assert.NotEmpty(t, retryAfter)
	
	// Should be a positive number (seconds)
	var seconds int
	fmt.Sscanf(retryAfter, "%d", &seconds)
	assert.True(t, seconds > 0, "Retry-After should be positive seconds")
}

// ═══════════════════════════════════════════════════════════════════════════
// WITHDRAWAL RATE LIMIT CONFIGURABLE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestWithdrawalRateLimitConfigurable_CustomLimit tests custom limits
func TestWithdrawalRateLimitConfigurable_CustomLimit(t *testing.T) {
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "vip-user")
		c.Next()
	}, WithdrawalRateLimitConfigurable(10, time.Hour), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// First 10 should succeed (custom limit)
	for i := 1; i <= 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "Withdrawal %d should succeed", i)
	}

	// 11th should be blocked
	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestWithdrawalRateLimitConfigurable_CustomWindow tests custom time windows
func TestWithdrawalRateLimitConfigurable_CustomWindow(t *testing.T) {
	router := setupTestRouter()

	// 2 withdrawals per minute (instead of per hour)
	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimitConfigurable(2, time.Minute), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// First 2 should succeed
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 3rd should be blocked
	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestWithdrawalRateLimitConfigurable_Unauthenticated tests authentication requirement
func TestWithdrawalRateLimitConfigurable_Unauthenticated(t *testing.T) {
	router := setupTestRouter()

	router.POST("/withdraw", WithdrawalRateLimitConfigurable(5, time.Hour), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// No user_id in context
	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ═══════════════════════════════════════════════════════════════════════════
// GET WITHDRAWAL RATE LIMIT STATUS TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestGetWithdrawalRateLimitStatus_NoLimiter tests when limiter not initialized
func TestGetWithdrawalRateLimitStatus_NoLimiter(t *testing.T) {
	setupWithdrawalRateLimitTest()

	status := GetWithdrawalRateLimitStatus("user-123")

	assert.Equal(t, 3, status["limit"])
	assert.Equal(t, 0, status["used"])
	assert.Equal(t, 3, status["remaining"])
	assert.Equal(t, true, status["can_withdraw"])
	assert.NotNil(t, status["resets_at"])
}

// TestGetWithdrawalRateLimitStatus_NoAttempts tests status with no attempts
func TestGetWithdrawalRateLimitStatus_NoAttempts(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	// Initialize limiter
	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(5), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	status := GetWithdrawalRateLimitStatus("user-new")

	assert.Equal(t, 5, status["limit"])
	assert.Equal(t, 0, status["used"])
	assert.Equal(t, 5, status["remaining"])
	assert.Equal(t, true, status["can_withdraw"])
}

// TestGetWithdrawalRateLimitStatus_SomeAttempts tests status with partial attempts
func TestGetWithdrawalRateLimitStatus_SomeAttempts(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(5), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Make 2 withdrawals
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	status := GetWithdrawalRateLimitStatus("user-123")

	assert.Equal(t, 5, status["limit"])
	assert.Equal(t, 2, status["used"])
	assert.Equal(t, 3, status["remaining"])
	assert.Equal(t, true, status["can_withdraw"])
}

// TestGetWithdrawalRateLimitStatus_AtLimit tests status when limit reached
func TestGetWithdrawalRateLimitStatus_AtLimit(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(3), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Exhaust limit
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	status := GetWithdrawalRateLimitStatus("user-123")

	assert.Equal(t, 3, status["limit"])
	assert.Equal(t, 3, status["used"])
	assert.Equal(t, 0, status["remaining"])
	assert.Equal(t, false, status["can_withdraw"])
	assert.NotNil(t, status["resets_at"])
}

// ═══════════════════════════════════════════════════════════════════════════
// INTEGRATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestWithdrawalRateLimit_MultipleUsers tests concurrent users
func TestWithdrawalRateLimit_MultipleUsers(t *testing.T) {
	setupWithdrawalRateLimitTest()

	users := []string{"user-1", "user-2", "user-3"}

	for _, userID := range users {
		t.Run(userID, func(t *testing.T) {
			router := setupTestRouter()
			
			router.POST("/withdraw", func(c *gin.Context) {
				c.Set("user_id", userID)
				c.Next()
			}, WithdrawalRateLimit(2), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			// Each user should have independent limit
			for i := 1; i <= 2; i++ {
				req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				assert.Equal(t, http.StatusOK, w.Code)
			}

			// Each user's 3rd request should be blocked
			req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusTooManyRequests, w.Code)
		})
	}
}

// TestWithdrawalRateLimit_ErrorMessage tests error message format
func TestWithdrawalRateLimit_ErrorMessage(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(2), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Exhaust limit
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Blocked request
	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "Too many withdrawal attempts")
	assert.Contains(t, w.Body.String(), "Maximum 2 per hour")
}

// TestWithdrawalRateLimit_HeadersWhenBlocked tests all headers when blocked
func TestWithdrawalRateLimit_HeadersWhenBlocked(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "user-123")
		c.Next()
	}, WithdrawalRateLimit(2), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Exhaust limit
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Blocked request
	req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

// TestWithdrawalRateLimit_HighVolumeUsers tests users making many attempts
func TestWithdrawalRateLimit_HighVolumeUsers(t *testing.T) {
	setupWithdrawalRateLimitTest()
	router := setupTestRouter()

	router.POST("/withdraw", func(c *gin.Context) {
		c.Set("user_id", "high-volume-user")
		c.Next()
	}, WithdrawalRateLimit(5), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	successCount := 0
	blockedCount := 0

	// Try 10 withdrawals
	for i := 1; i <= 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/withdraw", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			successCount++
		} else if w.Code == http.StatusTooManyRequests {
			blockedCount++
		}
	}

	assert.Equal(t, 5, successCount, "Should allow exactly 5 withdrawals")
	assert.Equal(t, 5, blockedCount, "Should block 5 excess withdrawals")
}

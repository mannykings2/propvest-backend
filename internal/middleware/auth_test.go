package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/utils/jwt"
	"gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accountStoreFunc adapts a plain function to the AccountStore interface so
// tests can describe the account row without building a repository fake.
type accountStoreFunc func(uuid.UUID) (*repositories.AccountState, error)

func (f accountStoreFunc) FindSessionState(_ context.Context, id uuid.UUID) (*repositories.AccountState, error) {
	return f(id)
}

// accountsWithRole reports an ACTIVE account for any token subject, with the
// given role. The middleware reads role/suspension from the account row rather
// than the JWT claims, so tests supply the row here.
func accountsWithRole(role string) AccountStore {
	return accountStoreFunc(func(id uuid.UUID) (*repositories.AccountState, error) {
		return &repositories.AccountState{
			UserID:        id,
			Role:          role,
			IsActive:      true,
			EmailVerified: true,
			KYCStatus:     "verified",
		}, nil
	})
}

// accountsSuspended reports a suspended account for any token subject.
func accountsSuspended() AccountStore {
	return accountStoreFunc(func(id uuid.UUID) (*repositories.AccountState, error) {
		return &repositories.AccountState{
			UserID:    id,
			Role:      "investor",
			IsActive:  false,
			Suspended: true,
		}, nil
	})
}

// accountsMissing reports the account as gone (soft-deleted) for any subject.
func accountsMissing() AccountStore {
	return accountStoreFunc(func(uuid.UUID) (*repositories.AccountState, error) {
		return nil, gorm.ErrRecordNotFound
	})
}

// setupTestRouter creates a test Gin router with test mode enabled
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// setupTestConfig creates a test configuration for JWT
func setupTestConfig() *config.Config {
	return &config.Config{
		JWTSecret:        "test-jwt-secret-for-middleware-testing",
		JWTRefreshSecret: "test-refresh-secret",
		AccessTokenTTL:   "15m",
		RefreshTokenTTL:  "168h", // 7 days
	}
}

// generateTestToken creates a valid JWT token for testing
func generateTestToken(userID uuid.UUID, role string, cfg *config.Config) (string, error) {
	ttl := 15 * time.Minute // Use fixed duration for testing
	return jwt.GenerateAccessToken(userID, role, cfg.JWTSecret, ttl)
}

// ═══════════════════════════════════════════════════════════════════════════
// AUTH MIDDLEWARE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestAuth_ValidToken tests successful authentication with valid token
func TestAuth_ValidToken(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	userID := uuid.New()
	role := "investor"

	// Generate valid token
	token, err := generateTestToken(userID, role, cfg)
	require.NoError(t, err)

	// Setup test endpoint
	router.GET("/protected", Auth(cfg, accountsWithRole("investor")), func(c *gin.Context) {
		// Verify context was set correctly
		contextUserID := c.GetString("user_id")
		contextRole := c.GetString("role")
		
		c.JSON(http.StatusOK, gin.H{
			"user_id": contextUserID,
			"role":    contextRole,
		})
	})

	// Make request
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), userID.String())
	assert.Contains(t, w.Body.String(), role)
}

// TestAuth_MissingAuthorizationHeader tests request without Authorization header
func TestAuth_MissingAuthorizationHeader(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()

	router.GET("/protected", Auth(cfg, accountsWithRole("investor")), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	// No Authorization header set
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Authorization header required")
}

// TestAuth_MalformedHeader tests various malformed Authorization headers
func TestAuth_MalformedHeader(t *testing.T) {
	cfg := setupTestConfig()

	testCases := []struct {
		name   string
		header string
	}{
		{"no bearer prefix", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."},
		{"lowercase bearer", "bearer eyJhbGci..."},
		{"extra spaces", "Bearer  eyJhbGci..."},
		{"no token after bearer", "Bearer"},
		{"empty bearer", "Bearer "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := setupTestRouter()
			router.GET("/protected", Auth(cfg, accountsWithRole("investor")), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", tc.header)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code, "Failed for: %s", tc.name)
		})
	}
}

// TestAuth_InvalidToken tests authentication with invalid token
func TestAuth_InvalidToken(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()

	router.GET("/protected", Auth(cfg, accountsWithRole("investor")), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid or expired token")
}

// TestAuth_ExpiredToken tests authentication with expired token
func TestAuth_ExpiredToken(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	userID := uuid.New()
	role := "investor"

	// Generate token with negative TTL (already expired)
	expiredToken, err := jwt.GenerateAccessToken(userID, role, cfg.JWTSecret, -1*time.Hour)
	require.NoError(t, err)

	router.GET("/protected", Auth(cfg, accountsWithRole("investor")), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Give token time to be definitely expired
	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid or expired token")
}

// TestAuth_WrongSecret tests token signed with different secret
func TestAuth_WrongSecret(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	userID := uuid.New()
	role := "investor"

	// Generate token with wrong secret
	wrongSecret := "wrong-secret-key"
	wrongToken, err := jwt.GenerateAccessToken(userID, role, wrongSecret, 15*time.Minute)
	require.NoError(t, err)

	router.GET("/protected", Auth(cfg, accountsWithRole("investor")), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+wrongToken)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestAuth_ContextValues tests that user_id and role are correctly set in context
func TestAuth_ContextValues(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	userID := uuid.New()
	expectedRole := "admin"

	token, err := generateTestToken(userID, expectedRole, cfg)
	require.NoError(t, err)

	var capturedUserID string
	var capturedRole string

	router.GET("/protected", Auth(cfg, accountsWithRole(expectedRole)), func(c *gin.Context) {
		capturedUserID = c.GetString("user_id")
		capturedRole = c.GetString("role")
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, userID.String(), capturedUserID)
	assert.Equal(t, expectedRole, capturedRole)
}

// TestAuth_DifferentRoles tests authentication with various user roles
func TestAuth_DifferentRoles(t *testing.T) {
	cfg := setupTestConfig()
	userID := uuid.New()
	roles := []string{"investor", "developer", "admin", "staff", "super_admin"}

	for _, role := range roles {
		t.Run("role_"+role, func(t *testing.T) {
			router := setupTestRouter()
			token, err := generateTestToken(userID, role, cfg)
			require.NoError(t, err)

			var contextRole string
			router.GET("/protected", Auth(cfg, accountsWithRole(role)), func(c *gin.Context) {
				contextRole = c.GetString("role")
				c.JSON(http.StatusOK, gin.H{"role": contextRole})
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, role, contextRole)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// OPTIONAL AUTH MIDDLEWARE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestOptionalAuth_ValidToken tests OptionalAuth with valid token
func TestOptionalAuth_ValidToken(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	userID := uuid.New()
	role := "investor"

	token, err := generateTestToken(userID, role, cfg)
	require.NoError(t, err)

	router.GET("/optional", OptionalAuth(cfg), func(c *gin.Context) {
		contextUserID, exists := c.Get("user_id")
		c.JSON(http.StatusOK, gin.H{
			"authenticated": exists,
			"user_id":       contextUserID,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), userID.String())
	assert.Contains(t, w.Body.String(), "true")
}

// TestOptionalAuth_NoToken tests OptionalAuth without token (anonymous access)
func TestOptionalAuth_NoToken(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()

	router.GET("/optional", OptionalAuth(cfg), func(c *gin.Context) {
		_, exists := c.Get("user_id")
		c.JSON(http.StatusOK, gin.H{
			"authenticated": exists,
			"message":       "anonymous access allowed",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	// No Authorization header
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed (200 OK) as anonymous user
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "anonymous access allowed")
}

// TestOptionalAuth_InvalidToken tests OptionalAuth with invalid token (doesn't fail)
func TestOptionalAuth_InvalidToken(t *testing.T) {
	cfg := setupTestConfig()

	router := setupTestRouter()
	router.GET("/optional", OptionalAuth(cfg), func(c *gin.Context) {
		_, exists := c.Get("user_id")
		c.JSON(http.StatusOK, gin.H{
			"authenticated": exists,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	req.Header.Set("Authorization", "Bearer invalid.token")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should still succeed as anonymous (invalid token is ignored)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "false") // Not authenticated
}

// TestOptionalAuth_ExpiredToken tests OptionalAuth with expired token (doesn't fail)
func TestOptionalAuth_ExpiredToken(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	userID := uuid.New()

	// Generate expired token
	expiredToken, err := jwt.GenerateAccessToken(userID, "investor", cfg.JWTSecret, -1*time.Hour)
	require.NoError(t, err)

	router.GET("/optional", OptionalAuth(cfg), func(c *gin.Context) {
		_, exists := c.Get("user_id")
		c.JSON(http.StatusOK, gin.H{
			"authenticated": exists,
		})
	})

	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed as anonymous (expired token ignored)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestOptionalAuth_MalformedHeader tests OptionalAuth with malformed header (doesn't fail)
func TestOptionalAuth_MalformedHeader(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()

	router.GET("/optional", OptionalAuth(cfg), func(c *gin.Context) {
		_, exists := c.Get("user_id")
		c.JSON(http.StatusOK, gin.H{
			"authenticated": exists,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	req.Header.Set("Authorization", "NoBearer malformed")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed as anonymous
	assert.Equal(t, http.StatusOK, w.Code)
}

// ═══════════════════════════════════════════════════════════════════════════
// INTEGRATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestAuth_MultipleRequests tests that middleware works across multiple requests
func TestAuth_MultipleRequests(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()

	router.GET("/protected", Auth(cfg, accountsWithRole("investor")), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Generate tokens for different users
	user1 := uuid.New()
	user2 := uuid.New()
	token1, err := generateTestToken(user1, "investor", cfg)
	require.NoError(t, err)
	token2, err := generateTestToken(user2, "developer", cfg)
	require.NoError(t, err)

	// Request 1
	req1 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req1.Header.Set("Authorization", "Bearer "+token1)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Request 2
	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("Authorization", "Bearer "+token2)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Request 3 - no token
	req3 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusUnauthorized, w3.Code)
}

// TestAuth_CaseSensitiveBearer tests that "Bearer" is case-sensitive
func TestAuth_CaseSensitiveBearer(t *testing.T) {
	cfg := setupTestConfig()
	userID := uuid.New()

	token, err := generateTestToken(userID, "investor", cfg)
	require.NoError(t, err)

	variations := []string{
		"bearer " + token,  // lowercase
		"BEARER " + token,  // uppercase
		"BeArEr " + token,  // mixed case
	}

	for _, authHeader := range variations {
		t.Run(authHeader[:6], func(t *testing.T) {
			router := setupTestRouter()
			router.GET("/protected", Auth(cfg, accountsWithRole("investor")), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", authHeader)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Only "Bearer" (capital B) should work
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

// ═════════════════════════════════════════════════════════════════════
// ACCOUNT STATE (P1-1.2 session revocation)
// ═════════════════════════════════════════════════════════════════════

// TestAuth_SuspendedAccountRejectsValidToken proves a token that is still
// cryptographically valid stops working the moment the account is suspended —
// the whole point of re-reading the account row instead of trusting `exp`.
func TestAuth_SuspendedAccountRejectsValidToken(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	userID := uuid.New()

	token, err := generateTestToken(userID, "investor", cfg)
	require.NoError(t, err)

	router.GET("/protected", Auth(cfg, accountsSuspended()), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "account_suspended")
}

// TestAuth_DeletedAccountRejectsValidToken proves a soft-deleted account's
// token is treated as dead rather than honoured until expiry.
func TestAuth_DeletedAccountRejectsValidToken(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	userID := uuid.New()

	token, err := generateTestToken(userID, "admin", cfg)
	require.NoError(t, err)

	router.GET("/protected", Auth(cfg, accountsMissing()), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid_token")
}

// TestAuth_RoleComesFromAccountRow pins the behaviour change: the role in the
// context is the row's role, not the token's claim, so a privilege change
// lands on the next request instead of the next login.
func TestAuth_RoleComesFromAccountRow(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	userID := uuid.New()

	// Token claims admin, but the row now says investor.
	token, err := generateTestToken(userID, "admin", cfg)
	require.NoError(t, err)

	var gotRole string
	router.GET("/protected", Auth(cfg, accountsWithRole("investor")), func(c *gin.Context) {
		gotRole = c.GetString("role")
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "investor", gotRole)
}

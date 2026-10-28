package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAuthenticatedContext simulates Auth middleware by setting user_id and role
func setupAuthenticatedContext(c *gin.Context, userID string, role string) {
	c.Set("user_id", userID)
	c.Set("role", role)
}

// ═══════════════════════════════════════════════════════════════════════════
// REQUIRE ROLE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestRequireRole_Success tests successful authorization with matching role
func TestRequireRole_Success(t *testing.T) {
	router := setupTestRouter()
	userID := uuid.New().String()

	router.GET("/protected", func(c *gin.Context) {
		setupAuthenticatedContext(c, userID, "admin")
		c.Next()
	}, RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "success")
}

// TestRequireRole_MultipleAllowedRoles tests authorization with multiple allowed roles
func TestRequireRole_MultipleAllowedRoles(t *testing.T) {
	userID := uuid.New().String()

	testCases := []struct {
		userRole string
		expected int
	}{
		{"investor", http.StatusOK},
		{"admin", http.StatusOK},
		{"developer", http.StatusForbidden}, // Not in allowed list
	}

	for _, tc := range testCases {
		t.Run("role_"+tc.userRole, func(t *testing.T) {
			router := setupTestRouter()
			
			router.GET("/protected", func(c *gin.Context) {
				setupAuthenticatedContext(c, userID, tc.userRole)
				c.Next()
			}, RequireRole("investor", "admin"), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expected, w.Code, "Failed for role: %s", tc.userRole)
		})
	}
}

// TestRequireRole_Forbidden tests authorization failure with insufficient role
func TestRequireRole_Forbidden(t *testing.T) {
	router := setupTestRouter()
	userID := uuid.New().String()

	router.GET("/admin-only", func(c *gin.Context) {
		setupAuthenticatedContext(c, userID, "investor")
		c.Next()
	}, RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Insufficient permissions")
}

// TestRequireRole_Unauthenticated tests that RequireRole rejects unauthenticated requests
func TestRequireRole_Unauthenticated(t *testing.T) {
	router := setupTestRouter()

	// Don't set user_id or role in context (simulate missing Auth middleware)
	router.GET("/protected", RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Authentication required")
}

// TestRequireRole_InvalidRoleType tests handling of corrupted role in context
func TestRequireRole_InvalidRoleType(t *testing.T) {
	router := setupTestRouter()

	router.GET("/protected", func(c *gin.Context) {
		c.Set("user_id", uuid.New().String())
		c.Set("role", 12345) // Wrong type (int instead of string)
		c.Next()
	}, RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ═══════════════════════════════════════════════════════════════════════════
// CONVENIENCE WRAPPER TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestRequireInvestor tests RequireInvestor convenience function
func TestRequireInvestor(t *testing.T) {
	testCases := []struct {
		role     string
		expected int
	}{
		{"investor", http.StatusOK},
		{"admin", http.StatusOK},       // Admins can access investor endpoints
		{"developer", http.StatusForbidden},
		{"staff", http.StatusForbidden},
	}

	for _, tc := range testCases {
		t.Run(tc.role, func(t *testing.T) {
			router := setupTestRouter()
			userID := uuid.New().String()

			router.GET("/investor-endpoint", func(c *gin.Context) {
				setupAuthenticatedContext(c, userID, tc.role)
				c.Next()
			}, RequireInvestor(), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest(http.MethodGet, "/investor-endpoint", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expected, w.Code, "Failed for role: %s", tc.role)
		})
	}
}

// TestRequireDeveloper tests RequireDeveloper convenience function
func TestRequireDeveloper(t *testing.T) {
	testCases := []struct {
		role     string
		expected int
	}{
		{"developer", http.StatusOK},
		{"admin", http.StatusOK},       // Admins can access developer endpoints
		{"investor", http.StatusForbidden},
		{"staff", http.StatusForbidden},
	}

	for _, tc := range testCases {
		t.Run(tc.role, func(t *testing.T) {
			router := setupTestRouter()
			userID := uuid.New().String()

			router.GET("/developer-endpoint", func(c *gin.Context) {
				setupAuthenticatedContext(c, userID, tc.role)
				c.Next()
			}, RequireDeveloper(), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest(http.MethodGet, "/developer-endpoint", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expected, w.Code, "Failed for role: %s", tc.role)
		})
	}
}

// TestRequireAdmin tests RequireAdmin convenience function
func TestRequireAdmin(t *testing.T) {
	testCases := []struct {
		role     string
		expected int
	}{
		{"admin", http.StatusOK},
		{"super_admin", http.StatusOK},  // Super admins can access admin endpoints
		{"investor", http.StatusForbidden},
		{"developer", http.StatusForbidden},
		{"staff", http.StatusForbidden},
	}

	for _, tc := range testCases {
		t.Run(tc.role, func(t *testing.T) {
			router := setupTestRouter()
			userID := uuid.New().String()

			router.GET("/admin-endpoint", func(c *gin.Context) {
				setupAuthenticatedContext(c, userID, tc.role)
				c.Next()
			}, RequireAdmin(), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest(http.MethodGet, "/admin-endpoint", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expected, w.Code, "Failed for role: %s", tc.role)
		})
	}
}

// TestRequireSuperAdmin tests RequireSuperAdmin convenience function
func TestRequireSuperAdmin(t *testing.T) {
	testCases := []struct {
		role     string
		expected int
	}{
		{"super_admin", http.StatusOK},
		{"admin", http.StatusForbidden},      // Regular admins can't access super admin endpoints
		{"investor", http.StatusForbidden},
		{"developer", http.StatusForbidden},
	}

	for _, tc := range testCases {
		t.Run(tc.role, func(t *testing.T) {
			router := setupTestRouter()
			userID := uuid.New().String()

			router.GET("/superadmin-endpoint", func(c *gin.Context) {
				setupAuthenticatedContext(c, userID, tc.role)
				c.Next()
			}, RequireSuperAdmin(), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest(http.MethodGet, "/superadmin-endpoint", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expected, w.Code, "Failed for role: %s", tc.role)
		})
	}
}

// TestRequireAdminOrStaff tests RequireAdminOrStaff convenience function
func TestRequireAdminOrStaff(t *testing.T) {
	testCases := []struct {
		role     string
		expected int
	}{
		{"admin", http.StatusOK},
		{"staff", http.StatusOK},
		{"investor", http.StatusForbidden},
		{"developer", http.StatusForbidden},
	}

	for _, tc := range testCases {
		t.Run(tc.role, func(t *testing.T) {
			router := setupTestRouter()
			userID := uuid.New().String()

			router.GET("/staff-endpoint", func(c *gin.Context) {
				setupAuthenticatedContext(c, userID, tc.role)
				c.Next()
			}, RequireAdminOrStaff(), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest(http.MethodGet, "/staff-endpoint", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expected, w.Code, "Failed for role: %s", tc.role)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// INTEGRATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestRBAC_MultipleMiddlewareChain tests combining Auth and RBAC middlewares
func TestRBAC_MultipleMiddlewareChain(t *testing.T) {
	router := setupTestRouter()
	cfg := setupTestConfig()
	investorID := uuid.New()
	adminID := uuid.New()

	// Create tokens for different roles. Each token names its own account:
	// Auth now reads the role from the account row, so the two roles must live
	// on two different rows just as they would in the database.
	investorToken, err := generateTestToken(investorID, "investor", cfg)
	require.NoError(t, err)
	adminToken, err := generateTestToken(adminID, "admin", cfg)
	require.NoError(t, err)

	accounts := accountStoreFunc(func(id uuid.UUID) (*repositories.AccountState, error) {
		role := "investor"
		if id == adminID {
			role = "admin"
		}
		return &repositories.AccountState{UserID: id, Role: role, IsActive: true}, nil
	})

	// Admin-only endpoint with Auth + RequireAdmin
	router.GET("/admin-only", Auth(cfg, accounts), RequireAdmin(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access"})
	})

	// Test with investor token (should fail)
	req1 := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	req1.Header.Set("Authorization", "Bearer "+investorToken)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusForbidden, w1.Code)

	// Test with admin token (should succeed)
	req2 := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestRBAC_RoleHierarchy tests role hierarchy expectations
func TestRBAC_RoleHierarchy(t *testing.T) {
	router := setupTestRouter()
	userID := uuid.New().String()

	// Create different endpoints with different role requirements
	router.GET("/investor", func(c *gin.Context) {
		setupAuthenticatedContext(c, userID, "investor")
		c.Next()
	}, RequireInvestor(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"endpoint": "investor"})
	})

	router.GET("/developer", func(c *gin.Context) {
		setupAuthenticatedContext(c, userID, "developer")
		c.Next()
	}, RequireDeveloper(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"endpoint": "developer"})
	})

	router.GET("/admin", func(c *gin.Context) {
		setupAuthenticatedContext(c, userID, "admin")
		c.Next()
	}, RequireAdmin(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"endpoint": "admin"})
	})

	// Test investor endpoint
	req1 := httptest.NewRequest(http.MethodGet, "/investor", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Test developer endpoint
	req2 := httptest.NewRequest(http.MethodGet, "/developer", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Test admin endpoint
	req3 := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)
}

// TestRBAC_EmptyRole tests handling of empty role string
func TestRBAC_EmptyRole(t *testing.T) {
	router := setupTestRouter()
	userID := uuid.New().String()

	router.GET("/protected", func(c *gin.Context) {
		setupAuthenticatedContext(c, userID, "")
		c.Next()
	}, RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestRBAC_CaseSensitiveRole tests that role matching is case-sensitive
func TestRBAC_CaseSensitiveRole(t *testing.T) {
	userID := uuid.New().String()

	testCases := []struct {
		role     string
		expected int
	}{
		{"admin", http.StatusOK},
		{"Admin", http.StatusForbidden},  // Wrong case
		{"ADMIN", http.StatusForbidden},  // Wrong case
		{"aDmIn", http.StatusForbidden},  // Wrong case
	}

	for _, tc := range testCases {
		t.Run("role_"+tc.role, func(t *testing.T) {
			router := setupTestRouter()
			
			router.GET("/protected", func(c *gin.Context) {
				setupAuthenticatedContext(c, userID, tc.role)
				c.Next()
			}, RequireRole("admin"), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expected, w.Code, "Failed for role: %s", tc.role)
		})
	}
}

// TestRBAC_MultipleRolesInAllowedList tests complex role combinations
func TestRBAC_MultipleRolesInAllowedList(t *testing.T) {
	userID := uuid.New().String()

	allowedRoles := []string{"admin", "staff", "developer"}

	testCases := []struct {
		role     string
		expected int
	}{
		{"admin", http.StatusOK},
		{"staff", http.StatusOK},
		{"developer", http.StatusOK},
		{"investor", http.StatusForbidden},
		{"super_admin", http.StatusForbidden}, // Not in list
	}

	for _, tc := range testCases {
		t.Run(tc.role, func(t *testing.T) {
			router := setupTestRouter()
			
			router.GET("/protected", func(c *gin.Context) {
				setupAuthenticatedContext(c, userID, tc.role)
				c.Next()
			}, RequireRole(allowedRoles...), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expected, w.Code, "Failed for role: %s", tc.role)
		})
	}
}

// TestRBAC_NoAllowedRoles tests RequireRole with no roles specified (edge case)
func TestRBAC_NoAllowedRoles(t *testing.T) {
	router := setupTestRouter()
	userID := uuid.New().String()

	router.GET("/protected", func(c *gin.Context) {
		setupAuthenticatedContext(c, userID, "admin")
		c.Next()
	}, RequireRole(), func(c *gin.Context) { // No roles specified
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should always be forbidden (no role can match empty list)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// The auth middleware stores the user ID as a string (claims.UserID.String()).
// Admin handlers used to assert uuid.UUID directly, which either panicked or
// answered 500 on every /admin/* route. These tests pin the extraction contract.
func TestGetUserIDFromContext(t *testing.T) {
	id := uuid.New()

	cases := []struct {
		name    string
		setup   func(*gin.Context)
		wantErr bool
		wantID  uuid.UUID
	}{
		{
			name:   "middleware string form",
			setup:  func(c *gin.Context) { c.Set("user_id", id.String()) },
			wantID: id,
		},
		{
			name:   "typed uuid form is also accepted",
			setup:  func(c *gin.Context) { c.Set("user_id", id) },
			wantID: id,
		},
		{
			name:    "missing key",
			setup:   func(c *gin.Context) {},
			wantErr: true,
		},
		{
			name:    "wrong type",
			setup:   func(c *gin.Context) { c.Set("user_id", 42) },
			wantErr: true,
		},
		{
			name:    "malformed uuid string",
			setup:   func(c *gin.Context) { c.Set("user_id", "not-a-uuid") },
			wantErr: true,
		},
		{
			name:    "nil uuid",
			setup:   func(c *gin.Context) { c.Set("user_id", uuid.Nil) },
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			tc.setup(c)

			got, err := getUserIDFromContext(c)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantID {
				t.Fatalf("got %v, want %v", got, tc.wantID)
			}
		})
	}
}

// TestAdminHandlersRejectMissingActor proves every admin handler refuses the
// request cleanly (401) instead of panicking when the actor is absent. The
// services are deliberately nil: these handlers must return before touching
// them, which is only true if context extraction no longer panics.
func TestAdminHandlersRejectMissingActor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		run  func(*gin.Context)
	}{
		{"ListUsers", (&AdminUserHandler{}).ListUsers},
		{"GetUser", (&AdminUserHandler{}).GetUser},
		{"SuspendUser", (&AdminUserHandler{}).SuspendUser},
		{"GetDashboard", (&AdminDashboardHandler{}).GetDashboard},
		{"CheckSystemHealth", (&AdminHealthHandler{}).CheckSystemHealth},
		{"CheckQueueHealth", (&AdminHealthHandler{}).CheckQueueHealth},
		{"ListAuditLogs", (&AdminReportsHandler{}).ListAuditLogs},
		{"ListTransactions", (&AdminFinancialHandler{}).ListTransactions},
		{"ListPayments", (&AdminFinancialHandler{}).ListPayments},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)

			tc.run(c)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

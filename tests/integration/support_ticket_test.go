package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/handlers"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// supportRouter wires the REAL handlers (service + repository + Postgres).
// Auth middleware is replaced by a stub that injects the acting user — the
// JWT layer has its own tests; this exercises ownership, lifecycle and status
// rules end-to-end through HTTP.
func supportRouter(userID uuid.UUID) (*gin.Engine, *handlers.SupportHandler, *handlers.AdminSupportHandler) {
	gin.SetMode(gin.TestMode)

	supportRepo := repositories.NewSupportRepository(testDBGlobal)
	supportSvc := services.NewSupportService(supportRepo, nil) // no notifications in tests
	userH := handlers.NewSupportHandler(supportSvc)
	adminH := handlers.NewAdminSupportHandler(supportSvc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", userID.String())
		c.Set("role", "investor")
		c.Next()
	})

	r.POST("/api/v1/support/tickets", userH.CreateTicket)
	r.GET("/api/v1/support/tickets", userH.ListTickets)
	r.GET("/api/v1/support/tickets/:id", userH.GetTicket)
	r.POST("/api/v1/support/tickets/:id/replies", userH.ReplyTicket)

	r.GET("/api/v1/admin/support/tickets", adminH.AdminListTickets)
	r.GET("/api/v1/admin/support/tickets/:id", adminH.AdminGetTicket)
	r.POST("/api/v1/admin/support/tickets/:id/replies", adminH.AdminReplyTicket)
	r.PATCH("/api/v1/admin/support/tickets/:id/status", adminH.AdminUpdateStatus)

	return r, userH, adminH
}

// testDBGlobal is set by TestSupportTicketLifecycle so the router helper can
// reach the same connection the test migrated.
var testDBGlobal *gorm.DB

// doPost/decode helpers -------------------------------------------------------

func supportJSON(t *testing.T, r *gin.Engine, method, path string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()

	var buf *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, path, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var parsed map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response was not JSON (%d): %s", w.Code, w.Body.String())
	}
	return w, parsed
}

func dataField(t *testing.T, resp map[string]interface{}) map[string]interface{} {
	t.Helper()
	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok, "response data should be an object: %v", resp)
	return data
}

// createSupportUser inserts a minimal user row (FK target for tickets).
func createSupportUser(t *testing.T, email, phone, name string) uuid.UUID {
	t.Helper()
	user := &models.User{
		UserCode:     fmt.Sprintf("USR-%s", uuid.New().String()[:8]),
		Email:        email,
		Phone:        phone,
		PasswordHash: "bcrypt-not-needed-in-tests",
		FullName:     name,
		Role:         "investor",
	}
	require.NoError(t, testDBGlobal.Create(user).Error)
	return user.ID
}

// The lifecycle test ------------------------------------------------------------

func TestSupportTicketLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup()
	testDBGlobal = testDB.DB
	defer func() { testDBGlobal = nil }()

	userA := createSupportUser(t, "qa_support_a@example.com", "+2348011111111", "QA Support A")
	userB := createSupportUser(t, "qa_support_b@example.com", "+2348011111112", "QA Support B")

	routerA, _, _ := supportRouter(userA)
	routerB, _, _ := supportRouter(userB)
	// The agent acts through the admin endpoints as the same stub identity.
	agentRouter, _, _ := supportRouter(userA)

	t.Run("create allocates a ticket number and stores the opening message", func(t *testing.T) {
		w, resp := supportJSON(t, routerA, http.MethodPost, "/api/v1/support/tickets", map[string]string{
			"subject":  "Withdrawal stuck for 3 days",
			"category": "Withdrawal",
			"message":  "My withdrawal of ₦50,000 has been pending since Monday.",
		})
		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
		assert.True(t, resp["success"].(bool))

		data := dataField(t, resp)
		num := data["ticket_number"].(string)
		assert.Regexp(t, `^TKT-\d{6}$`, num, "human-friendly number format")
		assert.Equal(t, "open", data["status"])
		assert.Equal(t, "medium", data["priority"])
		assert.Equal(t, 1, int(data["reply_count"].(float64)), "opening message is the first reply")

		replies := data["replies"].([]interface{})
		require.Len(t, replies, 1)
		first := replies[0].(map[string]interface{})
		assert.Equal(t, "user", first["author_role"])
	})

	t.Run("invalid category is rejected with 422", func(t *testing.T) {
		w, _ := supportJSON(t, routerA, http.MethodPost, "/api/v1/support/tickets", map[string]string{
			"subject":  "Something about rockets",
			"category": "Space Travel",
			"message":  "This category does not exist in the allowlist at all.",
		})
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "body: %s", w.Body.String())
	})

	t.Run("subject below the minimum is rejected with a field-level 422", func(t *testing.T) {
		w, resp := supportJSON(t, routerA, http.MethodPost, "/api/v1/support/tickets", map[string]string{
			"subject":  "hi",
			"category": "Other",
			"message":  "perfectly fine message body",
		})
		// response.ValidationError answers 422 with the documented
		// {field: [messages]} map (not a flat 400 string).
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, "validation_error", fmt.Sprint(resp["code"]))
		errs, ok := resp["errors"].(map[string]interface{})
		require.True(t, ok, "422 must carry an errors map: %v", resp)
		assert.Contains(t, errs, "Subject")
	})

	t.Run("another user cannot see or read the ticket", func(t *testing.T) {
		// List is scoped: B sees nothing.
		_, resp := supportJSON(t, routerB, http.MethodGet, "/api/v1/support/tickets", nil)
		data := dataField(t, resp)
		tickets := data["tickets"].([]interface{})
		assert.Len(t, tickets, 0, "user B must not see user A's tickets")

		// Direct read of A's ticket by B is a 404 — not 403, so probing IDs
		// reveals nothing about existence.
		_, created := supportJSON(t, routerA, http.MethodPost, "/api/v1/support/tickets", map[string]string{
			"subject":  "Private matter about my deed",
			"category": "Documents",
			"message":  "I need a copy of my co-ownership deed please.",
		})
		ticketID := dataField(t, created)["id"].(string)

		w, resp := supportJSON(t, routerB, http.MethodGet, "/api/v1/support/tickets/"+ticketID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "ticket_not_found", fmt.Sprint(resp["code"]))
	})

	t.Run("user reply, agent pickup, resolve, reopen, close", func(t *testing.T) {
		// Create (as A).
		_, created := supportJSON(t, routerA, http.MethodPost, "/api/v1/support/tickets", map[string]string{
			"subject":  "KYC document rejected twice",
			"category": "KYC / Verification",
			"message":  "My utility bill keeps getting rejected, please advise.",
		})
		data := dataField(t, created)
		ticketID := data["id"].(string)
		ticketNumber := data["ticket_number"].(string)

		// User follow-up reply → 201, thread now 2.
		w, _ := supportJSON(t, routerA, http.MethodPost, "/api/v1/support/tickets/"+ticketID+"/replies",
			map[string]string{"message": "Adding: the bill is less than 3 months old."})
		require.Equal(t, http.StatusCreated, w.Code)

		// Agent replies → ticket moves open → in_progress automatically.
		w, resp := supportJSON(t, agentRouter, http.MethodPost, "/api/v1/admin/support/tickets/"+ticketID+"/replies",
			map[string]string{"message": "Thanks — re-uploading a clearer scan now."})
		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, "agent", dataField(t, resp)["author_role"])

		_, got := supportJSON(t, agentRouter, http.MethodGet, "/api/v1/admin/support/tickets/"+ticketID, nil)
		assert.Equal(t, "in_progress", dataField(t, got)["status"], "agent reply picks up an open ticket")

		// Agent resolves.
		w, _ = supportJSON(t, agentRouter, http.MethodPatch, "/api/v1/admin/support/tickets/"+ticketID+"/status",
			map[string]string{"status": "resolved"})
		require.Equal(t, http.StatusOK, w.Code)

		// User answers a resolved ticket → reply accepted AND ticket reopens.
		w, _ = supportJSON(t, routerA, http.MethodPost, "/api/v1/support/tickets/"+ticketID+"/replies",
			map[string]string{"message": "Still blurry, could you resend? Thanks anyway."})
		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		_, got = supportJSON(t, routerA, http.MethodGet, "/api/v1/support/tickets/"+ticketID, nil)
		assert.Equal(t, "open", dataField(t, got)["status"], "user reply reopens a resolved ticket")

		// Close it for good.
		w, _ = supportJSON(t, agentRouter, http.MethodPatch, "/api/v1/admin/support/tickets/"+ticketID+"/status",
			map[string]string{"status": "closed"})
		require.Equal(t, http.StatusOK, w.Code)

		_, got = supportJSON(t, routerA, http.MethodGet, "/api/v1/support/tickets/"+ticketID, nil)
		closed := dataField(t, got)
		assert.Equal(t, "closed", closed["status"])
		assert.NotNil(t, closed["closed_at"], "closed_at stamped on close")

		// Closed tickets accept no further replies → 409 with a stable code.
		w, resp = supportJSON(t, routerA, http.MethodPost, "/api/v1/support/tickets/"+ticketID+"/replies",
			map[string]string{"message": "One more question about this ticket please."})
		assert.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, "ticket_closed", fmt.Sprint(resp["code"]))

		// Thread length is preserved across the whole lifecycle.
		_, got = supportJSON(t, routerA, http.MethodGet, "/api/v1/support/tickets/"+ticketID, nil)
		replies := dataField(t, got)["replies"].([]interface{})
		assert.Len(t, replies, 4, "opening + user + agent + reopening user reply")
		assert.Equal(t, 4, int(dataField(t, got)["reply_count"].(float64)))
		_ = ticketNumber
	})

	t.Run("agent queue lists tickets with owner summary and filters", func(t *testing.T) {
		_, resp := supportJSON(t, agentRouter, http.MethodGet, "/api/v1/admin/support/tickets?status=open", nil)
		data := dataField(t, resp)
		tickets := data["tickets"].([]interface{})
		require.NotEmpty(t, tickets, "at least one open ticket exists from earlier steps")

		first := tickets[0].(map[string]interface{})
		user := first["user"].(map[string]interface{})
		assert.Equal(t, "qa_support_a@example.com", user["email"], "agent sees who wrote in")
		assert.NotEmpty(t, first["ticket_number"])
		// List rows carry the thread length via a read-only subquery — a plain
		// 0 here means the reply_count select silently stopped working.
		assert.Greater(t, int(first["reply_count"].(float64)), 0,
			"list rows must report the real reply count")
	})

	t.Run("status filter rejects unknown values", func(t *testing.T) {
		w, _ := supportJSON(t, agentRouter, http.MethodGet, "/api/v1/admin/support/tickets?status=bogus", nil)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("ticket numbers are unique and ordered", func(t *testing.T) {
		_, first := supportJSON(t, routerB, http.MethodPost, "/api/v1/support/tickets", map[string]string{
			"subject":  "First ticket from user B",
			"category": "Other",
			"message":  "Number allocation check, message number one.",
		})
		_, second := supportJSON(t, routerB, http.MethodPost, "/api/v1/support/tickets", map[string]string{
			"subject":  "Second ticket from user B",
			"category": "Other",
			"message":  "Number allocation check, message number two.",
		})

		n1 := dataField(t, first)["ticket_number"].(string)
		n2 := dataField(t, second)["ticket_number"].(string)
		assert.NotEqual(t, n1, n2, "sequence must never hand out duplicates")
		assert.Greater(t, n2, n1, "later tickets get larger numbers")
	})
}

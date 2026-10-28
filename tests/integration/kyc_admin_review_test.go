package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/handlers"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/services"
	"github.com/mannykings2/propvest-backend/internal/utils/piienc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// kycRouter wires the REAL handlers (service + repository + Postgres) for the
// manual admin KYC workflow. Auth is a stub that reads the acting user from a
// header — the JWT layer has its own tests; this exercises the review rules,
// the screening gate and the PII contract end-to-end through HTTP.
func kycRouter(t *testing.T, db *gorm.DB, cfg *config.Config, actingUserID uuid.UUID, role string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cipher, err := piienc.New("integration-test-kyc-key")
	require.NoError(t, err)

	userRepo := repositories.NewUserRepository(db)
	tokenRepo := repositories.NewRefreshTokenRepository(db)
	investRepo := repositories.NewInvestmentRepository(db)
	kycRepo := repositories.NewKYCSubmissionRepository(db, cipher)
	auditSvc := services.NewAuditService(repositories.NewAuditRepository(db))

	adminSvc := services.NewAdminUserService(userRepo, tokenRepo, investRepo, kycRepo, auditSvc, db)
	adminH := handlers.NewAdminUserHandler(adminSvc)

	userSvc := services.NewUserService(userRepo, repositories.NewOTPVerificationRepository(db),
		tokenRepo, kycRepo, nil, nil, cfg)
	userH := handlers.NewUserHandler(userSvc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", actingUserID.String())
		c.Set("role", role)
		c.Next()
	})

	// User-facing (Phase 6.1/6.2)
	r.POST("/api/v1/me/kyc/submit", userH.SubmitKYC)
	r.GET("/api/v1/me/kyc", userH.GetKYCStatus)

	// Admin review (Phase 6.2/6.3/6.5)
	r.GET("/api/v1/admin/kyc/submissions", adminH.ListKYCSubmissions)
	r.GET("/api/v1/admin/kyc/submissions/:id", adminH.GetKYCDetail)
	r.POST("/api/v1/admin/kyc/submissions/:id/screening", adminH.RecordScreening)
	r.PATCH("/api/v1/admin/users/:id/kyc", adminH.DecideKYC)

	return r
}

// kycJSON posts/parses a JSON exchange on the router under test.
func kycJSON(t *testing.T, r *gin.Engine, method, path string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
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

// createKYCUser inserts a user row with the given role.
func createKYCUser(t *testing.T, db *gorm.DB, email, phone, name, role string) uuid.UUID {
	t.Helper()
	user := &models.User{
		UserCode:     fmt.Sprintf("USR-%s", uuid.New().String()[:8]),
		Email:        email,
		Phone:        phone,
		PasswordHash: "bcrypt-not-needed-in-tests",
		FullName:     name,
		Role:         role,
	}
	require.NoError(t, db.Create(user).Error)
	return user.ID
}

func countAudit(t *testing.T, db *gorm.DB, action string, targetID uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.AuditLog{}).
		Where("action = ? AND target_id = ?", action, targetID).Count(&n).Error)
	return n
}

// TestKYCAdminReviewWorkflow covers Phase 6 end to end against Postgres:
// encrypted-at-rest submission (6.1), reason + atomic audit (6.2), the
// screening gate and its 422s (6.3), and the PII contract (6.5).
func TestKYCAdminReviewWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup()

	const (
		fullBVN = "12345678901"
		fullNIN = "98765432109"
	)

	applicant := createKYCUser(t, testDB.DB, "qa_kyc_applicant@example.com", "+2348022222222", "QA KYC Applicant", "investor")
	admin := createKYCUser(t, testDB.DB, "qa_kyc_admin@example.com", "+2348033333333", "QA KYC Admin", "admin")

	asUser := kycRouter(t, testDB.DB, testDB.Config, applicant, "investor")
	asAdmin := kycRouter(t, testDB.DB, testDB.Config, admin, "admin")

	// ── 6.1: submission lands encrypted at rest ────────────────────────────────
	w, resp := kycJSON(t, asUser, http.MethodPost, "/api/v1/me/kyc/submit", map[string]string{
		"bvn": fullBVN, "nin": fullNIN, "doc_type": "national_id",
	})
	require.Equal(t, http.StatusOK, w.Code, "submit should succeed: %v", resp)

	var storedBVN string
	require.NoError(t, testDB.DB.Raw("SELECT bvn FROM kyc_submissions WHERE user_id = ?", applicant).Scan(&storedBVN).Error)
	assert.True(t, strings.HasPrefix(storedBVN, "v1:"), "BVN must be AES-GCM ciphertext at rest, got %q", storedBVN)
	assert.NotContains(t, storedBVN, fullBVN, "plaintext BVN must never reach the database")

	// ── 6.2/6.5: the queue is last-4 only ─────────────────────────────────────
	w, resp = kycJSON(t, asAdmin, http.MethodGet, "/api/v1/admin/kyc/submissions", nil)
	require.Equal(t, http.StatusOK, w.Code, "queue should list: %v", resp)
	require.NotContains(t, w.Body.String(), fullBVN, "admin queue must be last-4 only")
	require.NotContains(t, w.Body.String(), fullNIN, "admin queue must be last-4 only")
	require.Contains(t, w.Body.String(), "8901", "admin queue shows last-4")

	// ── 6.3: approval refused until screening says clear ──────────────────────
	w, resp = kycJSON(t, asAdmin, http.MethodPatch, fmt.Sprintf("/api/v1/admin/users/%s/kyc", applicant),
		map[string]string{"decision": "verified", "reason": "identity documents match the applicant"})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "approve without screening must be 422: %v", resp)

	// A recorded hit blocks approval outright, even with a reason.
	w, resp = kycJSON(t, asAdmin, http.MethodPost, fmt.Sprintf("/api/v1/admin/kyc/submissions/%s/screening", applicant),
		map[string]string{"source": "OFAC SDN (manual check)", "result": "hit", "notes": "name matches an entry"})
	require.Equal(t, http.StatusOK, w.Code, "screening should record: %v", resp)

	w, resp = kycJSON(t, asAdmin, http.MethodPatch, fmt.Sprintf("/api/v1/admin/users/%s/kyc", applicant),
		map[string]string{"decision": "verified", "reason": "identity documents match the applicant"})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "a screening hit must block approval: %v", resp)

	var auditAfterHit int64 = countAudit(t, testDB.DB, models.AuditActionKYCScreeningSaved, applicant)
	assert.GreaterOrEqual(t, auditAfterHit, int64(1), "each screening record writes an audit row")

	// ── 6.2: reason is required for approval too ──────────────────────────────
	w, resp = kycJSON(t, asAdmin, http.MethodPost, fmt.Sprintf("/api/v1/admin/kyc/submissions/%s/screening", applicant),
		map[string]string{"source": "Nigeria sanctions list (manual check)", "result": "clear"})
	require.Equal(t, http.StatusOK, w.Code, "clear screening should record: %v", resp)
	assert.GreaterOrEqual(t, countAudit(t, testDB.DB, models.AuditActionKYCScreeningSaved, applicant), int64(2),
		"re-screening writes a second audit row")

	w, resp = kycJSON(t, asAdmin, http.MethodPatch, fmt.Sprintf("/api/v1/admin/users/%s/kyc", applicant),
		map[string]string{"decision": "verified"})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "approval without a reason must be 422: %v", resp)

	// Nothing should have changed on the user yet.
	var kycStatus string
	require.NoError(t, testDB.DB.Raw("SELECT kyc_status FROM users WHERE id = ?", applicant).Scan(&kycStatus).Error)
	assert.Equal(t, "pending", kycStatus, "a refused decision must not flip kyc_status")

	// ── 6.2: a clean approval commits status + submission + audit together ────
	w, resp = kycJSON(t, asAdmin, http.MethodPatch, fmt.Sprintf("/api/v1/admin/users/%s/kyc", applicant),
		map[string]string{"decision": "verified", "reason": "BVN/NIN match, screening clear"})
	require.Equal(t, http.StatusOK, w.Code, "clean approval should succeed: %v", resp)

	require.NoError(t, testDB.DB.Raw("SELECT kyc_status FROM users WHERE id = ?", applicant).Scan(&kycStatus).Error)
	assert.Equal(t, "verified", kycStatus, "users.kyc_status flips on approval")

	var subStatus string
	require.NoError(t, testDB.DB.Raw("SELECT status FROM kyc_submissions WHERE user_id = ?", applicant).Scan(&subStatus).Error)
	assert.Equal(t, "verified", subStatus, "kyc_submissions stays in sync with users.kyc_status")

	assert.GreaterOrEqual(t, countAudit(t, testDB.DB, models.AuditActionUserKYCApproved, applicant), int64(1),
		"approval writes an audit_logs row")

	// ── 6.5: the admin detail view returns full PII and audits itself ─────────
	w, resp = kycJSON(t, asAdmin, http.MethodGet, fmt.Sprintf("/api/v1/admin/kyc/submissions/%s", applicant), nil)
	require.Equal(t, http.StatusOK, w.Code, "admin detail should load: %v", resp)
	require.Contains(t, w.Body.String(), fullBVN, "admin review screen needs the full BVN")
	require.Contains(t, w.Body.String(), fullNIN, "admin review screen needs the full NIN")
	assert.GreaterOrEqual(t, countAudit(t, testDB.DB, models.AuditActionUserKYCViewed, applicant), int64(1),
		"every admin PII read writes an audit_logs row")

	// ── 6.5: the user's own view never carries the full numbers ───────────────
	w, resp = kycJSON(t, asUser, http.MethodGet, "/api/v1/me/kyc", nil)
	require.Equal(t, http.StatusOK, w.Code, "user status should load: %v", resp)
	assert.NotContains(t, w.Body.String(), fullBVN, "non-admin responses must never include the full BVN")
	assert.NotContains(t, w.Body.String(), fullNIN, "non-admin responses must never include the full NIN")
	assert.Contains(t, w.Body.String(), "verified", "the user sees the outcome")
}

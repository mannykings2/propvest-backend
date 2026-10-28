package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/utils/piienc"
)

// ═══════════════════════════════════════════════════════════════════════════
// PRIVACY SERVICE (Phase 9 — NDPA 2023 data-subject rights)
// ═══════════════════════════════════════════════════════════════════════════
//
// Three data-subject rights plus the marketing objection, end to end:
//
//   ACCESS   — GET /me/exports assembles every category of the user's data
//              into a JSON bundle (async job, 24h link, token hashed at rest,
//              bundle wiped on expiry).
//   ERASURE  — DELETE /me/account starts a 30-day grace; execution ANONYMISES
//              identity and RETAINS financial rows (the statutory AML/tax
//              retention exception — the ledger must still reconcile).
//   OBJECTION— the unsubscribe token and the admin suppression list both
//              write marketing_suppressions, which SendBulkMessage MUST read
//              before every send.
//
// Admin sees jobs and requests but NEVER a user's bundle or download token:
// "re-issue" regenerates the link and delivers it TO THE USER.

const (
	exportLinkTTL     = 24 * time.Hour // the 24h SLA window from the plan
	unsubscribePrefix = "unsubscribe:"
)

type PrivacyService interface {
	// ── User-facing (NDPA rights) ────────────────────────────────────────
	RequestDataExport(ctx context.Context, userID uuid.UUID) (*DataExportRequest, error)
	ListDataExports(ctx context.Context, userID uuid.UUID) ([]models.DataExportJob, error)
	DownloadDataExport(ctx context.Context, token string) (json.RawMessage, error)

	RequestAccountDeletion(ctx context.Context, userID uuid.UUID, reason string) (*models.AccountDeletionRequest, error)
	CancelAccountDeletion(ctx context.Context, userID uuid.UUID) (*models.AccountDeletionRequest, error)
	AccountDeletionStatus(ctx context.Context, userID uuid.UUID) (*models.AccountDeletionRequest, error)

	// Unsubscribe resolves a token from an email link, suppresses the user's
	// marketing addresses and records the withdrawal as a consent row.
	Unsubscribe(ctx context.Context, token string) (string, error)
	// UnsubscribeToken builds the user's own bearer link token.
	UnsubscribeToken(userID uuid.UUID) string
	// SuppressedAddresses returns the set the bulk sender must skip.
	SuppressedAddresses(ctx context.Context) (map[string]bool, error)

	// ── Worker sweeps ────────────────────────────────────────────────────
	ExpireExportBundles(ctx context.Context) (int, error)
	ExecuteDueDeletions(ctx context.Context) (int, error)

	// ── Admin privacy surface (9.5) ──────────────────────────────────────
	AdminConsentView(ctx context.Context, userID uuid.UUID) (*AdminConsentView, error)
	AdminListSuppressions(ctx context.Context, page, limit int) ([]models.MarketingSuppression, int64, error)
	AdminAddSuppression(ctx context.Context, actor uuid.UUID, channel, address, reason string) (*models.MarketingSuppression, error)
	AdminRemoveSuppression(ctx context.Context, actor, id uuid.UUID) error
	AdminListExports(ctx context.Context, page, limit int) ([]AdminExportRow, int64, error)
	AdminReissueExport(ctx context.Context, jobID uuid.UUID) error
	AdminListDeletions(ctx context.Context, page, limit int) ([]AdminDeletionRow, int64, error)
	AdminDecideDeletion(ctx context.Context, actor, id uuid.UUID, approve bool, reason string) error
	ListPolicyVersions(ctx context.Context) ([]models.PolicyVersion, error)
	BumpPolicyVersion(ctx context.Context, actor uuid.UUID, policy, version string) (*models.PolicyVersion, error)

	// WithKYCCipher attaches the PII cipher (builder-style) so the OWNER's
	// export can carry decrypted BVN/NIN.
	WithKYCCipher(key string) PrivacyService
}

// DataExportRequest is the job plus the ONE download token — returned only to
// the owner at request/reissue time.
type DataExportRequest struct {
	Job           models.DataExportJob `json:"job"`
	DownloadToken string                `json:"download_token,omitempty"`
	DownloadPath  string                `json:"download_path,omitempty"`
}

// AdminConsentView is one user's consent evidence for the admin Privacy tab.
type AdminConsentView struct {
	UserID   uuid.UUID                 `json:"user_id"`
	Email    string                    `json:"email"`
	Policies map[string]string         `json:"policies"`
	Consents []dto.ConsentRecordResponse `json:"consents"`
}

// AdminExportRow is one export job with its owner's email.
type AdminExportRow struct {
	models.DataExportJob
	UserEmail string `json:"user_email"`
}

// AdminDeletionRow is one deletion request with its owner's email + countdown.
type AdminDeletionRow struct {
	models.AccountDeletionRequest
	UserEmail string `json:"user_email"`
	DaysLeft  int    `json:"days_left"`
}

type privacyService struct {
	db       *gorm.DB
	notifier NotificationService
	audit    AuditService
	// cipher decrypts BVN/NIN for the OWNER's export (nil when no key set).
	cipher *piienc.Cipher
	// unsubscribe HMAC key (derived from the JWT secret — no new config).
	unsubKey []byte
}

// NewPrivacyService constructs the service. notifier/audit are optional (tests
// and minimal wiring pass nil); every use is nil-guarded. The PII cipher is
// attached with WithKYCCipher — BVN/NIN decryption for the owner's own export
// is the one place decryption is legal without an admin role.
func NewPrivacyService(db *gorm.DB, notifier NotificationService, audit AuditService, unsubscribeKey string) PrivacyService {
	return &privacyService{
		db:       db,
		notifier: notifier,
		audit:    audit,
		unsubKey: []byte(unsubscribeKey),
	}
}

// WithKYCCipher attaches the PII cipher (KYC_ENCRYPTION_KEY). A missing or
// invalid key leaves the export's BVN/NIN withheld — noted in the bundle.
func (s *privacyService) WithKYCCipher(key string) PrivacyService {
	if key == "" {
		return s
	}
	if c, err := piienc.New(key); err == nil {
		s.cipher = c
	}
	return s
}

// ── Data export (NDPA access right) ───────────────────────────────────────

func (s *privacyService) RequestDataExport(ctx context.Context, userID uuid.UUID) (*DataExportRequest, error) {
	// One live job at a time: a queued/ready job is reused instead of minting
	// parallel bundles of the same PII.
	var existing models.DataExportJob
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND status IN (?, ?)", userID, models.ExportStatusQueued, models.ExportStatusReady).
		Order("created_at DESC").First(&existing).Error
	if err == nil && existing.Status == models.ExportStatusReady {
		// Ready job: hand back a FRESH link (re-issue semantics) so the caller
		// always leaves with a working token.
		token, terr := s.rotateExportToken(ctx, &existing)
		if terr != nil {
			return nil, terr
		}
		return exportRequestFrom(existing, token), nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("data export: existing job: %w", err)
	}

	if err == nil {
		// A queued job is already assembling — reuse it, no token yet.
		return &DataExportRequest{Job: existing}, nil
	}

	job := models.DataExportJob{ID: uuid.New(), UserID: userID, Status: models.ExportStatusQueued}
	if err := s.db.WithContext(ctx).Create(&job).Error; err != nil {
		return nil, fmt.Errorf("data export: queue job: %w", err)
	}

	// Async: assembly queries many tables and (for KYC) decrypts; the request
	// must not block on it. The worker is not required — status lives in the
	// row, so any process could finish it.
	go s.runExport(context.Background(), job.ID, userID)

	return &DataExportRequest{Job: job}, nil
}

// runExport assembles the bundle and finalises the job. Panics are contained:
// a failed export must never take the process down.
func (s *privacyService) runExport(ctx context.Context, jobID, userID uuid.UUID) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("data export: panic", "job_id", jobID, "panic", r)
			s.failExport(ctx, jobID, fmt.Sprintf("internal error: %v", r))
		}
	}()

	bundle, err := s.assembleBundle(ctx, userID)
	if err != nil {
		logger.Error("data export: assemble failed", "job_id", jobID, "error", err)
		s.failExport(ctx, jobID, err.Error())
		return
	}

	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		s.failExport(ctx, jobID, fmt.Sprintf("encode bundle: %v", err))
		return
	}

	token, err := randomToken()
	if err != nil {
		s.failExport(ctx, jobID, fmt.Sprintf("token: %v", err))
		return
	}
	now := time.Now()
	expires := now.Add(exportLinkTTL)
	hash := tokenHash(token)

	updates := map[string]interface{}{
		"status":              models.ExportStatusReady,
		"bundle":              raw,
		"download_token_hash": hash,
		"expires_at":          expires,
		"completed_at":        now,
		"error":               nil,
	}
	if err := s.db.WithContext(ctx).Model(&models.DataExportJob{}).
		Where("id = ?", jobID).Updates(updates).Error; err != nil {
		logger.Error("data export: finalise failed", "job_id", jobID, "error", err)
		return
	}

	// Deliver the link to the OWNER (never anyone else). Best-effort both.
	if s.notifier != nil {
		var user models.User
		if err := s.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err == nil {
			s.notifier.NotifyWithEmail(ctx, userID, user.Email, "data_export",
				"Your data export is ready",
				"Your personal data export is ready to download. The link expires in 24 hours.",
				map[string]any{"download_path": "/me/exports/download/" + token})
		}
	}
	logger.Info("data export ready", "job_id", jobID, "user_id", userID)
}

func (s *privacyService) failExport(ctx context.Context, jobID uuid.UUID, msg string) {
	_ = s.db.WithContext(ctx).Model(&models.DataExportJob{}).
		Where("id = ?", jobID).
		Updates(map[string]interface{}{"status": models.ExportStatusFailed, "error": msg}).Error
}

// assembleBundle collects every category the plan lists. BVN/NIN are included
// DECRYPTED — the acceptance rule is that they appear only in the owner's
// authenticated export, which is what this job is (the token proves ownership).
func (s *privacyService) assembleBundle(ctx context.Context, userID uuid.UUID) (map[string]interface{}, error) {
	db := s.db.WithContext(ctx)
	bundle := map[string]interface{}{
		"generated_at": time.Now().UTC(),
		"user_id":      userID,
	}

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}
	user.PasswordHash = "[redacted]"
	bundle["profile"] = user

	appendRows := func(category string, rows interface{}) error {
		if err := db.Where("user_id = ?", userID).Find(rows).Error; err != nil {
			return fmt.Errorf("%s: %w", category, err)
		}
		bundle[category] = rows
		return nil
	}

	var wallet models.Wallet
	if err := db.First(&wallet, "user_id = ?", userID).Error; err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("wallet: %w", err)
	}
	if wallet.ID != uuid.Nil {
		bundle["wallet"] = wallet
	}

	if err := appendRows("wallet_transactions", &[]models.WalletTransaction{}); err != nil {
		return nil, err
	}
	if err := appendRows("payments", &[]models.Payment{}); err != nil {
		return nil, err
	}
	if err := appendRows("investments", &[]models.Investment{}); err != nil {
		return nil, err
	}
	if err := appendRows("offplan_subscriptions", &[]models.OffplanSubscription{}); err != nil {
		return nil, err
	}
	if err := appendRows("notifications", &[]models.Notification{}); err != nil {
		return nil, err
	}
	if err := appendRows("consents", &[]models.UserConsent{}); err != nil {
		return nil, err
	}

	// KYC metadata — BVN/NIN decrypted for the owner; the ciphertext itself is
	// never placed in the bundle. (The model tags BVN/NIN json:"-" so raw rows
	// would silently drop them — the export row is explicit.)
	var kycs []models.KYCSubmission
	if err := appendRows("kyc_submissions", &kycs); err != nil {
		return nil, err
	}
	kycRows := make([]kycExportRow, 0, len(kycs))
	for _, k := range kycs {
		row := kycExportRow{
			ID: k.ID, DocType: k.DocType, Status: k.Status,
			SubmittedAt: k.SubmittedAt, DecidedAt: k.DecidedAt,
		}
		if s.cipher != nil {
			if bvn, err := s.cipher.Decrypt(k.BVN); err == nil {
				row.BVN = bvn
			}
			if nin, err := s.cipher.Decrypt(k.NIN); err == nil {
				row.NIN = nin
			}
		} else {
			row.WithheldNote = "BVN/NIN withheld: no encryption key configured on this instance"
		}
		kycRows = append(kycRows, row)
	}
	bundle["kyc_submissions"] = kycRows

	// Support history (tickets + their replies).
	var tickets []models.SupportTicket
	if err := appendRows("support_tickets", &tickets); err != nil {
		return nil, err
	}
	if len(tickets) > 0 {
		ids := make([]uuid.UUID, 0, len(tickets))
		for _, t := range tickets {
			ids = append(ids, t.ID)
		}
		var replies []models.SupportReply
		if err := db.Where("ticket_id IN ?", ids).Find(&replies).Error; err != nil {
			return nil, fmt.Errorf("support_replies: %w", err)
		}
		bundle["support_replies"] = replies
	}

	return bundle, nil
}

// kycExportRow is the owner-visible slice of a KYC submission: decrypted
// BVN/NIN plus the review trail. Doc evidence files are never bundled.
type kycExportRow struct {
	ID            uuid.UUID  `json:"id"`
	DocType      string     `json:"doc_type,omitempty"`
	Status       string     `json:"status"`
	BVN          string     `json:"bvn,omitempty"`
	NIN          string     `json:"nin,omitempty"`
	WithheldNote string     `json:"withheld,omitempty"`
	SubmittedAt  time.Time  `json:"submitted_at"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
}

func (s *privacyService) ListDataExports(ctx context.Context, userID uuid.UUID) ([]models.DataExportJob, error) {
	var jobs []models.DataExportJob
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Limit(20).Find(&jobs).Error
	return jobs, err
}

// DownloadDataExport resolves a bearer token to its bundle. Only status=ready
// within the expiry window answers; the token hash is the lookup key, so the
// raw token exists nowhere at rest.
func (s *privacyService) DownloadDataExport(ctx context.Context, token string) (json.RawMessage, error) {
	var job models.DataExportJob
	err := s.db.WithContext(ctx).
		Where("download_token_hash = ? AND status = ?", tokenHash(token), models.ExportStatusReady).
		First(&job).Error
	if err != nil {
		return nil, fmt.Errorf("export link is invalid or expired")
	}
	if job.ExpiresAt == nil || time.Now().After(*job.ExpiresAt) {
		return nil, fmt.Errorf("export link is invalid or expired")
	}
	return json.RawMessage(job.Bundle), nil
}

// rotateExportToken issues a fresh token for an existing ready job.
func (s *privacyService) rotateExportToken(ctx context.Context, job *models.DataExportJob) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(exportLinkTTL)
	if err := s.db.WithContext(ctx).Model(&models.DataExportJob{}).
		Where("id = ?", job.ID).
		Updates(map[string]interface{}{
			"download_token_hash": tokenHash(token),
			"expires_at":          expires,
			"status":              models.ExportStatusReady,
		}).Error; err != nil {
		return "", fmt.Errorf("data export: rotate token: %w", err)
	}
	job.DownloadTokenHash = nil
	job.ExpiresAt = &expires
	job.Status = models.ExportStatusReady
	return token, nil
}

func exportRequestFrom(job models.DataExportJob, token string) *DataExportRequest {
	out := &DataExportRequest{Job: job}
	if token != "" {
		out.DownloadToken = token
		out.DownloadPath = "/me/exports/download/" + token
	}
	return out
}

// ExpireExportBundles enforces the 24h window: an expired job stops serving
// AND its PII bundle is wiped — retention minimalism, not just a 404.
func (s *privacyService) ExpireExportBundles(ctx context.Context) (int, error) {
	res := s.db.WithContext(ctx).Model(&models.DataExportJob{}).
		Where("status = ? AND expires_at <= now()", models.ExportStatusReady).
		Updates(map[string]interface{}{
			"status":              models.ExportStatusExpired,
			"bundle":              nil,
			"download_token_hash": nil,
		})
	return int(res.RowsAffected), res.Error
}

// ── Account deletion (NDPA erasure + statutory retention exception) ───────

func (s *privacyService) RequestAccountDeletion(ctx context.Context, userID uuid.UUID, reason string) (*models.AccountDeletionRequest, error) {
	var row models.AccountDeletionRequest
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, models.DeletionStatusRequested).
		First(&row).Error
	if err == nil {
		return &row, nil // idempotent: asking twice returns the live request
	}
	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("account deletion: existing: %w", err)
	}

	row = models.AccountDeletionRequest{
		ID:           uuid.New(),
		UserID:       userID,
		Status:       models.DeletionStatusRequested,
		RequestedAt:  time.Now(),
		ExecuteAfter: time.Now().AddDate(0, 0, models.DeletionGraceDays),
	}
	if reason != "" {
		row.Reason = &reason
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		// The partial unique index on (user_id) WHERE requested is the race
		// backstop — a concurrent second request must return the live row.
		var live models.AccountDeletionRequest
		if e2 := s.db.WithContext(ctx).
			Where("user_id = ? AND status = ?", userID, models.DeletionStatusRequested).
			First(&live).Error; e2 == nil {
			return &live, nil
		}
		return nil, fmt.Errorf("account deletion: create: %w", err)
	}
	logger.Info("account deletion requested", "user_id", userID, "execute_after", row.ExecuteAfter)
	return &row, nil
}

func (s *privacyService) CancelAccountDeletion(ctx context.Context, userID uuid.UUID) (*models.AccountDeletionRequest, error) {
	var row models.AccountDeletionRequest
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, models.DeletionStatusRequested).
		First(&row).Error
	if err != nil {
		return nil, fmt.Errorf("no pending deletion request")
	}
	now := time.Now()
	row.Status = models.DeletionStatusCancelled
	row.CancelledAt = &now
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, fmt.Errorf("account deletion: cancel: %w", err)
	}
	return &row, nil
}

func (s *privacyService) AccountDeletionStatus(ctx context.Context, userID uuid.UUID) (*models.AccountDeletionRequest, error) {
	var row models.AccountDeletionRequest
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("requested_at DESC").First(&row).Error
	if err != nil {
		return nil, fmt.Errorf("no deletion request")
	}
	return &row, nil
}

// ExecuteDueDeletions anonymises identities whose grace period has passed.
// THE LEDGER IS NEVER TOUCHED: wallet_transactions, ledger_entries, payments
// and investments stay — the statutory retention exception that lets AML and
// tax records reconcile after the person is gone.
func (s *privacyService) ExecuteDueDeletions(ctx context.Context) (int, error) {
	var due []models.AccountDeletionRequest
	if err := s.db.WithContext(ctx).
		Where("status = ? AND execute_after <= now()", models.DeletionStatusRequested).
		Find(&due).Error; err != nil {
		return 0, fmt.Errorf("deletion sweep: load: %w", err)
	}

	executed := 0
	for _, req := range due {
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Identity → anonymised placeholders. email/phone are unique, so
			// derive values that cannot collide with a real account.
			if err := tx.Exec(`
				UPDATE users SET
					full_name     = 'Deleted User',
					email         = 'deleted+' || replace(id::text, '-', '') || '@deleted.invalid',
					phone         = '+234' || substr(md5(gen_random_uuid()::text), 1, 10),
					password_hash = 'deleted-' || md5(gen_random_uuid()::text),
					address       = NULL,
					occupation    = NULL,
					employer      = NULL,
					avatar_url    = NULL,
					updated_at    = now()
				WHERE id = ?`, req.UserID).Error; err != nil {
				return fmt.Errorf("anonymise user: %w", err)
			}
			// KYC evidence: BVN/NIN ciphertext and documents go; the submission
			// ROW (status history) stays for audit.
			if err := tx.Exec(`
				UPDATE kyc_submissions SET bvn = '', nin = '', evidence = NULL, updated_at = now()
				WHERE user_id = ?`, req.UserID).Error; err != nil {
				return fmt.Errorf("scrub kyc: %w", err)
			}
			// Sessions die with the identity.
			if err := tx.Exec(`DELETE FROM refresh_tokens WHERE user_id = ?`, req.UserID).Error; err != nil {
				return fmt.Errorf("revoke sessions: %w", err)
			}

			now := time.Now()
			if err := tx.Model(&models.AccountDeletionRequest{}).
				Where("id = ?", req.ID).
				Updates(map[string]interface{}{"status": models.DeletionStatusExecuted, "executed_at": now}).Error; err != nil {
				return fmt.Errorf("mark executed: %w", err)
			}

			if s.audit != nil {
				target := req.UserID
				return s.audit.Record(ctx, tx, AuditEvent{
					ActorType:  "system",
					Action:     "privacy.account_deletion_executed",
					Description: "Erasure executed after grace period; identity anonymised, financial records retained",
					TargetType: "user",
					TargetID:   &target,
					Reason:     "NDPA erasure request served after 30-day grace",
				})
			}
			return nil
		})
		if err != nil {
			logger.Error("deletion sweep: execute failed", "user_id", req.UserID, "error", err)
			continue
		}
		executed++
		logger.Info("account erased", "user_id", req.UserID)
	}
	return executed, nil
}

// ── Marketing suppression / unsubscribe ───────────────────────────────────

// UnsubscribeToken builds the bearer link token for a user's unsubscribe URL:
// the user id in the clear, HMAC-signed — so the server can resolve the owner
// by verification alone, and links stay valid (suppression dedupes repeats).
func (s *privacyService) UnsubscribeToken(userID uuid.UUID) string {
	return unsubToken(s.unsubKey, userID)
}

// Unsubscribe resolves a token, suppresses every marketing address the user
// has on file, and records the withdrawal as consent rows (the "withdrawable"
// half of NDPA consent).
func (s *privacyService) Unsubscribe(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("unsubscribe token is required")
	}
	userID, ok := verifyUnsubscribeToken(s.unsubKey, token)
	if !ok {
		return "", fmt.Errorf("unsubscribe link is invalid")
	}

	var user models.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		return "", fmt.Errorf("unsubscribe link is invalid")
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, pair := range [][2]string{{"email", user.Email}, {"sms", user.Phone}} {
			if pair[1] == "" {
				continue
			}
			row := models.MarketingSuppression{
				Channel: pair[0], Address: pair[1], UserID: &userID,
				Reason: models.SuppressionReasonUnsubscribe,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "channel"}, {Name: "address"}},
				DoNothing: true,
			}).Create(&row).Error; err != nil {
				return fmt.Errorf("suppress %s: %w", pair[0], err)
			}
		}
		// The withdrawal is consent evidence too.
		md := RequestMetadataFromContext(ctx)
		if err := RecordConsent(ctx, tx, userID, models.ConsentPurposeMarketing, false, nil, md.IPAddress, md.UserAgent); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return user.Email, nil
}

// SuppressedAddresses is the list SendBulkMessage must respect: lowercased
// email AND phone addresses.
func (s *privacyService) SuppressedAddresses(ctx context.Context) (map[string]bool, error) {
	var rows []models.MarketingSuppression
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("suppression list: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[r.Channel+"|"+r.Address] = true
	}
	return out, nil
}

// ── Admin privacy surface ─────────────────────────────────────────────────

func (s *privacyService) AdminConsentView(ctx context.Context, userID uuid.UUID) (*AdminConsentView, error) {
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		return nil, fmt.Errorf("user not found")
	}
	policies, err := CurrentPolicyVersions(ctx, s.db)
	if err != nil {
		return nil, err
	}
	var rows []models.UserConsent
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("consents: %w", err)
	}
	view := &AdminConsentView{UserID: userID, Email: user.Email, Policies: policies}
	for _, r := range rows {
		view.Consents = append(view.Consents, consentResponseFrom(r))
	}
	return view, nil
}

func (s *privacyService) AdminListSuppressions(ctx context.Context, page, limit int) ([]models.MarketingSuppression, int64, error) {
	page, limit = pageLimit(page, limit)
	var total int64
	if err := s.db.WithContext(ctx).Model(&models.MarketingSuppression{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.MarketingSuppression
	err := s.db.WithContext(ctx).Order("created_at DESC").
		Limit(limit).Offset((page - 1) * limit).Find(&rows).Error
	return rows, total, err
}

func (s *privacyService) AdminAddSuppression(ctx context.Context, actor uuid.UUID, channel, address, reason string) (*models.MarketingSuppression, error) {
	if channel == "" {
		channel = "email"
	}
	row := models.MarketingSuppression{Channel: channel, Address: address, Reason: models.SuppressionReasonAdmin}
	if reason != "" {
		row.Reason = "admin: " + reason
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel"}, {Name: "address"}},
			DoNothing: true,
		}).Create(&row).Error; err != nil {
			return fmt.Errorf("add suppression: %w", err)
		}
		return s.recordPrivacyAudit(ctx, tx, actor, "privacy.suppression_added",
			"Added address to marketing suppression list", map[string]interface{}{"channel": channel, "address": address})
	})
	if err != nil {
		return nil, err
	}
	// OnConflict DoNothing leaves row zero on duplicates — reload.
	if err := s.db.WithContext(ctx).
		Where("channel = ? AND address = ?", channel, address).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *privacyService) AdminRemoveSuppression(ctx context.Context, actor, id uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row models.MarketingSuppression
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			return fmt.Errorf("suppression not found")
		}
		if err := tx.Delete(&models.MarketingSuppression{}, "id = ?", id).Error; err != nil {
			return fmt.Errorf("remove suppression: %w", err)
		}
		return s.recordPrivacyAudit(ctx, tx, actor, "privacy.suppression_removed",
			"Removed address from marketing suppression list",
			map[string]interface{}{"channel": row.Channel, "address": row.Address})
	})
}

func (s *privacyService) AdminListExports(ctx context.Context, page, limit int) ([]AdminExportRow, int64, error) {
	page, limit = pageLimit(page, limit)
	var total int64
	if err := s.db.WithContext(ctx).Model(&models.DataExportJob{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []struct {
		models.DataExportJob
		Email string
	}
	err := s.db.WithContext(ctx).
		Table("data_export_jobs").
		Select("data_export_jobs.*, users.email AS email").
		Joins("JOIN users ON users.id = data_export_jobs.user_id").
		Order("data_export_jobs.created_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]AdminExportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdminExportRow{DataExportJob: r.DataExportJob, UserEmail: r.Email})
	}
	return out, total, nil
}

// AdminReissueExport rotates the link AND DELIVERS IT TO THE USER. The admin
// learns only that a fresh link was sent — a support role must not be able to
// read a customer's data bundle.
func (s *privacyService) AdminReissueExport(ctx context.Context, jobID uuid.UUID) error {
	var job models.DataExportJob
	if err := s.db.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
		return fmt.Errorf("export job not found")
	}
	if job.Status != models.ExportStatusReady {
		return fmt.Errorf("only a ready export can be re-issued (job is %s)", job.Status)
	}
	token, err := s.rotateExportToken(ctx, &job)
	if err != nil {
		return err
	}
	if s.notifier != nil {
		var user models.User
		if err := s.db.WithContext(ctx).First(&user, "id = ?", job.UserID).Error; err == nil {
			s.notifier.NotifyWithEmail(ctx, job.UserID, user.Email, "data_export",
				"A fresh data-export link was requested",
				"A new download link was issued for your data export. The old link no longer works. It expires in 24 hours.",
				map[string]any{"download_path": "/me/exports/download/" + token})
		}
	}
	return nil
}

func (s *privacyService) AdminListDeletions(ctx context.Context, page, limit int) ([]AdminDeletionRow, int64, error) {
	page, limit = pageLimit(page, limit)
	var total int64
	if err := s.db.WithContext(ctx).Model(&models.AccountDeletionRequest{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []struct {
		models.AccountDeletionRequest
		Email string
	}
	err := s.db.WithContext(ctx).
		Table("account_deletions").
		Select("account_deletions.*, users.email AS email").
		Joins("JOIN users ON users.id = account_deletions.user_id").
		Order("account_deletions.requested_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]AdminDeletionRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdminDeletionRow{
			AccountDeletionRequest: r.AccountDeletionRequest,
			UserEmail:              r.Email,
			DaysLeft:               daysLeft(r.ExecuteAfter),
		})
	}
	return out, total, nil
}

// AdminDecideDeletion approves (accelerates to immediate execution — the
// worker sweep picks it up) or cancels a request inside its grace period.
func (s *privacyService) AdminDecideDeletion(ctx context.Context, actor, id uuid.UUID, approve bool, reason string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row models.AccountDeletionRequest
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			return fmt.Errorf("deletion request not found")
		}
		if row.Status != models.DeletionStatusRequested {
			return fmt.Errorf("request is already %s", row.Status)
		}

		updates := map[string]interface{}{}
		action := ""
		if approve {
			updates["execute_after"] = time.Now() // sweep executes it
			action = "privacy.deletion_approved"
		} else {
			now := time.Now()
			updates["status"] = models.DeletionStatusCancelled
			updates["cancelled_at"] = now
			action = "privacy.deletion_cancelled"
		}
		if err := tx.Model(&models.AccountDeletionRequest{}).
			Where("id = ?", id).Updates(updates).Error; err != nil {
			return fmt.Errorf("decide deletion: %w", err)
		}
		return s.recordPrivacyAudit(ctx, tx, actor, action,
			"Admin "+map[bool]string{true: "approved", false: "cancelled"}[approve]+" an erasure request",
			map[string]interface{}{"user_id": row.UserID, "reason": reason})
	})
}

// ListPolicyVersions returns the version history both policies were published
// under (newest first) — the Privacy tab's "policy versions" view.
func (s *privacyService) ListPolicyVersions(ctx context.Context) ([]models.PolicyVersion, error) {
	var rows []models.PolicyVersion
	err := s.db.WithContext(ctx).Order("effective_at DESC").Find(&rows).Error
	return rows, err
}

// BumpPolicyVersion publishes a new policy version; every user whose consent
// pins an older version then reads needs_reconsent=true.
func (s *privacyService) BumpPolicyVersion(ctx context.Context, actor uuid.UUID, policy, version string) (*models.PolicyVersion, error) {
	var row models.PolicyVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if version == "" {
			var current models.PolicyVersion
			if err := tx.Where("policy = ?", policy).Order("effective_at DESC").First(&current).Error; err == nil {
				version = NextPolicyVersion(current.Version)
			} else {
				version = DefaultPolicyVersion
			}
		}
		row = models.PolicyVersion{
			ID: uuid.New(), Policy: policy, Version: version,
			EffectiveAt: time.Now(), CreatedBy: &actor,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("bump policy version: %w", err)
		}
		return s.recordPrivacyAudit(ctx, tx, actor, "privacy.policy_version_bumped",
			"Published a new "+policy+" version (re-consent required)",
			map[string]interface{}{"policy": policy, "version": version})
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ── helpers ───────────────────────────────────────────────────────────────

func (s *privacyService) recordPrivacyAudit(ctx context.Context, tx *gorm.DB, actor uuid.UUID, action, description string, details map[string]interface{}) error {
	if s.audit == nil {
		return nil
	}
	target := actor
	return s.audit.Record(ctx, tx, AuditEvent{
		ActorID:     &target,
		ActorType:   "admin",
		Action:      action,
		Description: description,
		TargetType:  "privacy",
		NewValues:   details,
		Reason:      description,
	})
}

// unsubToken signs a user id into an unsubscribe bearer token.
func unsubToken(key []byte, userID uuid.UUID) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(unsubscribePrefix + userID.String()))
	return userID.String() + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifyUnsubscribeToken checks the signature and recovers the owner.
func verifyUnsubscribeToken(key []byte, token string) (uuid.UUID, bool) {
	var idPart, sig string
	if n := strings.IndexByte(token, '.'); n > 0 {
		idPart, sig = token[:n], token[n+1:]
	} else {
		return uuid.Nil, false
	}
	userID, err := uuid.Parse(idPart)
	if err != nil {
		return uuid.Nil, false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(unsubscribePrefix + userID.String()))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return uuid.Nil, false
	}
	return userID, true
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func pageLimit(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}

func daysLeft(executeAfter time.Time) int {
	d := int(time.Until(executeAfter).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}
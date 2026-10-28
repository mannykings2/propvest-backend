package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrAlertExists is returned when a live alert already occupies the dedupe
// slot — the caller treats it as success (the pattern is already queued).
var ErrAlertExists = errors.New("risk alert already exists")

// TxnFact is one wallet transaction in the scan feed. The monitoring rules are
// pure functions over []TxnFact so they can be unit-tested without a database.
//
// Only the two fields rules actually read are projected out of the JSONB
// metadata (customer email for duplicate-funding, bank account name for the
// KYC-name check) — pulling whole payloads would make a 7-day feed enormous.
type TxnFact struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Type      string    `json:"type"`
	Amount    int64     `json:"amount"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	// FundingEmail is metadata->customer->>'email' (deposit webhook payload).
	FundingEmail string `json:"funding_email"`
	// BankAccountName is metadata->>'account_name' — the bank-verified name
	// frozen at withdrawal initiation.
	BankAccountName string `json:"bank_account_name"`
}

// RiskRepository is the data-access contract for the Risk layer (Phase 8).
type RiskRepository interface {
	// ── Rules ────────────────────────────────────────────────────────────
	ListRules(ctx context.Context) ([]models.RiskRule, error)
	// GetRule accepts the caller's transaction so a read-after-write inside a
	// tx sees its own uncommitted update (a second connection would not).
	GetRule(ctx context.Context, code string, tx ...*gorm.DB) (*models.RiskRule, error)
	// UpdateRule flips enabled and/or replaces params; updatedBy stamps who.
	// A nil enabled leaves the flag untouched; nil params keeps existing ones.
	UpdateRule(ctx context.Context, code string, enabled *bool, params []byte, updatedBy uuid.UUID, tx ...*gorm.DB) (*models.RiskRule, error)

	// ── Alerts ───────────────────────────────────────────────────────────
	// CreateAlert inserts unless a LIVE alert holds the same dedupe_key, in
	// which case ErrAlertExists is returned (no duplicate rows — 8.1).
	CreateAlert(ctx context.Context, alert *models.RiskAlert, tx ...*gorm.DB) error
	// GetAlertByDedupe is used by blocking rules to append evidence to the
	// alert their own denial already opened.
	GetAlertByDedupe(ctx context.Context, dedupeKey string) (*models.RiskAlert, error)
	ListAlerts(ctx context.Context, filter AlertFilter, limit, offset int) ([]models.RiskAlert, int64, error)
	GetAlert(ctx context.Context, id uuid.UUID) (*models.RiskAlert, error)
	// UpdateAlert applies a partial patch inside the caller's optional tx so
	// the audit row and the status change commit together.
	UpdateAlert(ctx context.Context, id uuid.UUID, patch map[string]interface{}, tx ...*gorm.DB) error
	// CountOpenAlerts powers the dashboard badge.
	CountOpenAlerts(ctx context.Context) (int64, error)

	// ── Screening (8.3) ──────────────────────────────────────────────────
	CreateScreening(ctx context.Context, row *models.ScreeningResultRow, tx ...*gorm.DB) error
	ListScreening(ctx context.Context, result string, limit, offset int) ([]models.ScreeningResultRow, int64, error)
	// StaleScreenings lists ACTIVE users whose newest screening is older than
	// olderThan (NULL = never screened). Powers the re-screen reminder.
	StaleScreenings(ctx context.Context, olderThan time.Time, limit int) ([]StaleScreening, error)

	// ── STR (8.7) ────────────────────────────────────────────────────────
	CreateSTR(ctx context.Context, report *models.STRReport) error
	ListSTR(ctx context.Context, status string, limit, offset int) ([]models.STRReport, int64, error)
	GetSTRByAlert(ctx context.Context, alertID uuid.UUID) (*models.STRReport, error)
	UpdateSTR(ctx context.Context, id uuid.UUID, patch map[string]interface{}, tx ...*gorm.DB) error

	// ── CTR (8.8) ────────────────────────────────────────────────────────
	// CreateCTR inserts unless (user, day, threshold) already exists; the
	// bool reports whether a row was actually created (hourly rescan safety).
	CreateCTR(ctx context.Context, report *models.CTRReport) (bool, error)
	ListCTR(ctx context.Context, status string, limit, offset int) ([]models.CTRReport, int64, error)
	GetCTR(ctx context.Context, id uuid.UUID) (*models.CTRReport, error)
	UpdateCTR(ctx context.Context, id uuid.UUID, patch map[string]interface{}) error

	// ── Scan feed (8.2 monitoring rules) ─────────────────────────────────
	// FeedSince returns every deposit/withdrawal in [since, now) across all
	// users — one pass feeds every monitoring rule.
	FeedSince(ctx context.Context, since time.Time) ([]TxnFact, error)
	// CountWithdrawalsSince is the blocking velocity check on the hot path.
	CountWithdrawalsSince(ctx context.Context, userID uuid.UUID, since time.Time) (int64, error)
}

// AlertFilter narrows the triage queue. Empty fields mean "any".
type AlertFilter struct {
	UserID   *uuid.UUID
	Status   string
	Severity string
	RuleCode string
}

// StaleScreening is a re-screen reminder row (8.3).
type StaleScreening struct {
	UserID       uuid.UUID  `json:"user_id"`
	FullName     string     `json:"full_name"`
	Email        string     `json:"email"`
	LastScreened *time.Time `json:"last_screened_at"` // NULL = never
}

type riskRepository struct {
	*BaseRepository
}

// NewRiskRepository constructs the repository.
func NewRiskRepository(db *gorm.DB) RiskRepository {
	return &riskRepository{BaseRepository: NewBaseRepository(db)}
}

// ── Rules ──────────────────────────────────────────────────────────────────

func (r *riskRepository) ListRules(ctx context.Context) ([]models.RiskRule, error) {
	var rules []models.RiskRule
	err := r.WithContext(ctx).Order("mode, code").Find(&rules).Error
	return rules, err
}

func (r *riskRepository) GetRule(ctx context.Context, code string, tx ...*gorm.DB) (*models.RiskRule, error) {
	db := r.WithContext(ctx)
	if len(tx) > 0 && tx[0] != nil {
		db = tx[0]
	}
	var rule models.RiskRule
	if err := db.Where("code = ?", code).First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *riskRepository) UpdateRule(ctx context.Context, code string, enabled *bool, params []byte, updatedBy uuid.UUID, tx ...*gorm.DB) (*models.RiskRule, error) {
	db := r.WithContext(ctx)
	if len(tx) > 0 && tx[0] != nil {
		db = tx[0]
	}
	patch := map[string]interface{}{"updated_by": updatedBy}
	if enabled != nil {
		patch["enabled"] = *enabled
	}
	if params != nil {
		patch["params"] = params
	}
	if err := db.Model(&models.RiskRule{}).
		Where("code = ?", code).
		Updates(patch).Error; err != nil {
		return nil, err
	}
	return r.GetRule(ctx, code, tx...)
}

// ── Alerts ─────────────────────────────────────────────────────────────────

func (r *riskRepository) CreateAlert(ctx context.Context, alert *models.RiskAlert, tx ...*gorm.DB) error {
	db := r.WithContext(ctx)
	if len(tx) > 0 && tx[0] != nil {
		db = tx[0]
	}
	// Bare ON CONFLICT DO NOTHING (no target): gorm cannot emit the partial
	// index's WHERE predicate in the right position (it lands after DO
	// NOTHING, which is a syntax error), and a target-less clause collides
	// against every unique index — including uq_risk_alerts_live, whose
	// predicate (status open/in_review/escalated) decides which rows are
	// "live". Skipped inserts report RowsAffected == 0.
	res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(alert)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAlertExists
	}
	return nil
}

func (r *riskRepository) GetAlertByDedupe(ctx context.Context, dedupeKey string) (*models.RiskAlert, error) {
	var alert models.RiskAlert
	err := r.WithContext(ctx).
		Where("dedupe_key = ? AND status IN ('open','in_review','escalated')", dedupeKey).
		First(&alert).Error
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

// alertSelect joins the acting user + assignee so lists render without N+1.
const alertSelect = `risk_alerts.*, ` +
	`u.full_name AS user_name, u.email AS user_email, ` +
	`a.full_name AS assignee_name`

func (r *riskRepository) ListAlerts(ctx context.Context, filter AlertFilter, limit, offset int) ([]models.RiskAlert, int64, error) {
	q := r.WithContext(ctx).Model(&models.RiskAlert{})
	if filter.UserID != nil {
		q = q.Where("risk_alerts.user_id = ?", *filter.UserID)
	}
	if filter.Status != "" {
		q = q.Where("risk_alerts.status = ?", filter.Status)
	}
	if filter.Severity != "" {
		q = q.Where("risk_alerts.severity = ?", filter.Severity)
	}
	if filter.RuleCode != "" {
		q = q.Where("risk_alerts.rule_code = ?", filter.RuleCode)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Scan into a row struct carrying the extra join columns, then map into
	// the model (the model's User/Assignee preloads would cost two extra
	// queries per page; this is one).
	type alertRow struct {
		models.RiskAlert
		UserName     string `gorm:"column:user_name"`
		UserEmail    string `gorm:"column:user_email"`
		AssigneeName string `gorm:"column:assignee_name"`
	}
	var rows []alertRow
	err := q.Session(&gorm.Session{}).
		Select(alertSelect).
		Joins("LEFT JOIN users u ON u.id = risk_alerts.user_id").
		Joins("LEFT JOIN users a ON a.id = risk_alerts.assignee_id").
		Order("risk_alerts.created_at DESC").
		Limit(limit).Offset(offset).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	out := make([]models.RiskAlert, len(rows))
	for i, row := range rows {
		row.RiskAlert.User = models.User{FullName: row.UserName, Email: row.UserEmail}
		row.RiskAlert.User.ID = row.RiskAlert.UserID
		if row.RiskAlert.AssigneeID != nil {
			au := models.User{FullName: row.AssigneeName}
			au.ID = *row.RiskAlert.AssigneeID
			row.RiskAlert.Assignee = &au
		}
		out[i] = row.RiskAlert
	}
	return out, total, nil
}

func (r *riskRepository) GetAlert(ctx context.Context, id uuid.UUID) (*models.RiskAlert, error) {
	var alert models.RiskAlert
	if err := r.WithContext(ctx).Preload("User").Preload("Assignee").
		Where("id = ?", id).First(&alert).Error; err != nil {
		return nil, err
	}
	return &alert, nil
}

func (r *riskRepository) UpdateAlert(ctx context.Context, id uuid.UUID, patch map[string]interface{}, tx ...*gorm.DB) error {
	db := r.WithContext(ctx)
	if len(tx) > 0 && tx[0] != nil {
		db = tx[0]
	}
	return db.Model(&models.RiskAlert{}).Where("id = ?", id).Updates(patch).Error
}

func (r *riskRepository) CountOpenAlerts(ctx context.Context) (int64, error) {
	var n int64
	err := r.WithContext(ctx).Model(&models.RiskAlert{}).
		Where("status IN ('open','in_review','escalated')").Count(&n).Error
	return n, err
}

// ── Screening ──────────────────────────────────────────────────────────────

func (r *riskRepository) CreateScreening(ctx context.Context, row *models.ScreeningResultRow, tx ...*gorm.DB) error {
	db := r.WithContext(ctx)
	if len(tx) > 0 && tx[0] != nil {
		db = tx[0]
	}
	return db.Create(row).Error
}

func (r *riskRepository) ListScreening(ctx context.Context, result string, limit, offset int) ([]models.ScreeningResultRow, int64, error) {
	q := r.WithContext(ctx).Model(&models.ScreeningResultRow{})
	if result != "" {
		q = q.Where("screening_results.result = ?", result)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.ScreeningResultRow
	err := q.Session(&gorm.Session{}).
		Preload("User").
		Order("screening_results.screened_at DESC").
		Limit(limit).Offset(offset).
		Find(&rows).Error
	return rows, total, err
}

func (r *riskRepository) StaleScreenings(ctx context.Context, olderThan time.Time, limit int) ([]StaleScreening, error) {
	var rows []StaleScreening
	err := r.WithContext(ctx).Raw(`
		SELECT u.id AS user_id,
		       u.full_name,
		       u.email,
		       MAX(s.screened_at) AS last_screened
		FROM users u
		LEFT JOIN screening_results s ON s.user_id = u.id
		WHERE u.deleted_at IS NULL
		  AND u.is_active = TRUE
		  AND u.kyc_status = 'approved'
		GROUP BY u.id, u.full_name, u.email
		HAVING MAX(s.screened_at) IS NULL OR MAX(s.screened_at) < ?
		ORDER BY last_screened ASC NULLS FIRST
		LIMIT ?`, olderThan, limit).Scan(&rows).Error
	return rows, err
}

// ── STR ────────────────────────────────────────────────────────────────────

func (r *riskRepository) CreateSTR(ctx context.Context, report *models.STRReport) error {
	return r.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "alert_id"}},
		DoNothing: true,
	}).Create(report).Error
}

func (r *riskRepository) ListSTR(ctx context.Context, status string, limit, offset int) ([]models.STRReport, int64, error) {
	q := r.WithContext(ctx).Model(&models.STRReport{})
	if status != "" {
		q = q.Where("str_reports.status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.STRReport
	err := q.Session(&gorm.Session{}).
		Preload("Alert").Preload("Alert.User").
		Order("str_reports.deadline_at ASC").
		Limit(limit).Offset(offset).
		Find(&rows).Error
	return rows, total, err
}

func (r *riskRepository) GetSTRByAlert(ctx context.Context, alertID uuid.UUID) (*models.STRReport, error) {
	var report models.STRReport
	if err := r.WithContext(ctx).Where("alert_id = ?", alertID).First(&report).Error; err != nil {
		return nil, err
	}
	return &report, nil
}

func (r *riskRepository) UpdateSTR(ctx context.Context, id uuid.UUID, patch map[string]interface{}, tx ...*gorm.DB) error {
	db := r.WithContext(ctx)
	if len(tx) > 0 && tx[0] != nil {
		db = tx[0]
	}
	return db.Model(&models.STRReport{}).Where("id = ?", id).Updates(patch).Error
}

// ── CTR ────────────────────────────────────────────────────────────────────

func (r *riskRepository) CreateCTR(ctx context.Context, report *models.CTRReport) (bool, error) {
	res := r.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "window_date"}, {Name: "threshold_kobo"}},
		DoNothing: true,
	}).Create(report)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *riskRepository) ListCTR(ctx context.Context, status string, limit, offset int) ([]models.CTRReport, int64, error) {
	q := r.WithContext(ctx).Model(&models.CTRReport{})
	if status != "" {
		q = q.Where("ctr_reports.status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.CTRReport
	err := q.Session(&gorm.Session{}).
		Preload("User").
		Order("ctr_reports.window_date DESC, ctr_reports.amount_kobo DESC").
		Limit(limit).Offset(offset).
		Find(&rows).Error
	return rows, total, err
}

func (r *riskRepository) GetCTR(ctx context.Context, id uuid.UUID) (*models.CTRReport, error) {
	var report models.CTRReport
	if err := r.WithContext(ctx).Preload("User").Where("id = ?", id).First(&report).Error; err != nil {
		return nil, err
	}
	return &report, nil
}

func (r *riskRepository) UpdateCTR(ctx context.Context, id uuid.UUID, patch map[string]interface{}) error {
	return r.WithContext(ctx).Model(&models.CTRReport{}).Where("id = ?", id).Updates(patch).Error
}

// ── Scan feed ──────────────────────────────────────────────────────────────

func (r *riskRepository) FeedSince(ctx context.Context, since time.Time) ([]TxnFact, error) {
	var facts []TxnFact
	err := r.WithContext(ctx).Raw(`
		SELECT id, user_id, type, amount, status, created_at,
		       COALESCE(metadata->'customer'->>'email', '') AS funding_email,
		       COALESCE(metadata->>'account_name', '')      AS bank_account_name
		FROM wallet_transactions
		WHERE created_at >= ?
		  AND type IN ('deposit','withdrawal')`, since).
		Scan(&facts).Error
	return facts, err
}

func (r *riskRepository) CountWithdrawalsSince(ctx context.Context, userID uuid.UUID, since time.Time) (int64, error) {
	var n int64
	err := r.WithContext(ctx).Model(&models.WalletTransaction{}).
		Where("user_id = ? AND type = 'withdrawal' AND created_at >= ?", userID, since).
		Count(&n).Error
	return n, err
}

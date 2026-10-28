package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ── Rule modes (risk_rules.mode) ─────────────────────────────────────────────
//
// blocking   — evaluated synchronously on the transaction path: the request is
//
//	DENIED when the rule fires (InitiateWithdrawal etc.).
//
// monitoring — evaluated by the background scan: the transaction proceeds and
//
//	an alert lands in the triage queue.
const (
	RiskModeBlocking   = "blocking"
	RiskModeMonitoring = "monitoring"
)

// Rule codes (risk_rules.code). Seeded by migration 000032; the engine looks
// rules up by these constants — never by hardcoded thresholds, which live in
// the row's params JSONB and are editable from the admin UI.
const (
	RuleWithdrawVelocity24h = "withdraw_velocity_24h"
	RuleRiskFlaggedUser     = "risk_flagged_user"
	RuleNewAccountHighValue = "new_account_high_value_withdrawal"
	RuleDepositVelocity10m  = "deposit_velocity_10m"
	RuleDepositCap24h       = "deposit_cap_24h"
	RuleStructuring7d       = "structuring_7d"
	RuleRoundAmountCycling  = "round_amount_cycling"
	RuleRapidInOut          = "rapid_in_out"
	RuleDuplicateFunding    = "duplicate_account_funding"
	RuleKYCNameMismatch     = "kyc_name_mismatch"
	// RuleScreeningHit is not scan-evaluated: the manual screening flow raises
	// it directly when an admin records a 'hit' (8.3).
	RuleScreeningHit = "screening_hit"
)

// Alert status lifecycle (risk_alerts.status).
const (
	RiskAlertOpen      = "open"
	RiskAlertInReview  = "in_review"
	RiskAlertEscalated = "escalated"
	RiskAlertCleared   = "cleared"
)

// Alert severities.
const (
	RiskSeverityLow      = "low"
	RiskSeverityMedium   = "medium"
	RiskSeverityHigh     = "high"
	RiskSeverityCritical = "critical"
)

// RiskRule is one detection rule: its mode, an enabled toggle the admin can
// flip, and JSONB params holding the thresholds (so editing a limit is a
// PATCH, not a deploy).
type RiskRule struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string         `gorm:"size:60;uniqueIndex;not null" json:"code"`
	Name        string         `gorm:"size:120;not null" json:"name"`
	Description string         `gorm:"type:text;not null;default:''" json:"description"`
	Mode        string         `gorm:"size:20;not null" json:"mode"` // blocking | monitoring
	Enabled     bool           `gorm:"not null;default:true" json:"enabled"`
	Params      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"params"`
	UpdatedBy   *uuid.UUID     `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (RiskRule) TableName() string { return "risk_rules" }

// RiskAlert is one detection hit awaiting triage. dedupe_key keeps exactly one
// LIVE alert per pattern window (partial unique index in the migration), so a
// rule firing every scan tick does not flood the queue.
type RiskAlert struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID   uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	RuleCode string    `gorm:"size:60;not null;index" json:"rule_code"`
	Severity string    `gorm:"size:20;not null;default:'medium'" json:"severity"`
	// Subject marks what the alert is about: "user" (behavioural rules) or a
	// transaction type; subject_id links the specific row when there is one.
	SubjectType string         `gorm:"size:30;not null;default:'user'" json:"subject_type"`
	SubjectID   *uuid.UUID     `gorm:"type:uuid" json:"subject_id,omitempty"`
	DedupeKey   string         `gorm:"size:200;not null" json:"-"`
	Details     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"details"`
	Status      string         `gorm:"size:20;not null;default:'open';index" json:"status"`

	AssigneeID      *uuid.UUID `gorm:"type:uuid" json:"assignee_id,omitempty"`
	ResolutionNotes *string    `json:"resolution_notes,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`

	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Assignee  *User     `gorm:"foreignKey:AssigneeID" json:"assignee,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (RiskAlert) TableName() string { return "risk_alerts" }

// IsLive reports whether the alert still occupies its dedupe slot — the same
// predicate as the uq_risk_alerts_live partial index (open, in_review AND
// escalated: an escalated alert has an STR in flight and must not be
// duplicated).
func (a *RiskAlert) IsLive() bool {
	return a.Status == RiskAlertOpen || a.Status == RiskAlertInReview || a.Status == RiskAlertEscalated
}

// ScreeningKind values (screening_results.kind).
const (
	ScreeningKindInitial  = "initial"
	ScreeningKindRescreen = "rescreen"
	ScreeningKindAdHoc    = "ad_hoc"
)

// ScreeningResultRow is one manual sanctions/PEP check (Phase 6.3 wrote these
// onto kyc_submissions only; Phase 8.3 gives them their own table so re-screen
// history and hit→alert links survive a KYC row replacement).
type ScreeningResultRow struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Kind       string     `gorm:"size:20;not null;default:'initial'" json:"kind"`
	Source     string     `gorm:"size:120;not null" json:"source"`
	Result     string     `gorm:"size:20;not null;index" json:"result"` // clear | hit
	Notes      *string    `json:"notes,omitempty"`
	ScreenedBy *uuid.UUID `gorm:"type:uuid" json:"screened_by,omitempty"`
	ScreenedAt time.Time  `gorm:"not null;default:now()" json:"screened_at"`
	// AlertID links a 'hit' to the escalated alert it raised (NULL for clear).
	AlertID *uuid.UUID `gorm:"type:uuid" json:"alert_id,omitempty"`

	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (ScreeningResultRow) TableName() string { return "screening_results" }

// STR report statuses (str_reports.status).
const (
	STRStatusDraft = "draft"
	STRStatusFiled = "filed"
)

// STRReport is the suspicious-transaction report package for one escalated
// alert. deadline_at is escalation + 24h — the NFIU filing window under the
// MLPPA 2022. Filed rows carry the goAML portal reference.
type STRReport struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AlertID    uuid.UUID  `gorm:"type:uuid;uniqueIndex;not null" json:"alert_id"`
	Narrative  string     `gorm:"type:text;not null;default:''" json:"narrative"`
	PreparedBy *uuid.UUID `gorm:"type:uuid" json:"prepared_by,omitempty"`
	Status     string     `gorm:"size:20;not null;default:'draft'" json:"status"`
	DeadlineAt time.Time  `gorm:"not null" json:"deadline_at"`
	FiledAt    *time.Time `json:"filed_at,omitempty"`
	// column:goaml_ref is required — GORM's naming strategy would otherwise
	// map GoAMLRef to `go_aml_ref`, which is not the column.
	GoAMLRef *string `gorm:"size:120;column:goaml_ref" json:"goaml_ref,omitempty"`

	Alert     RiskAlert `gorm:"foreignKey:AlertID" json:"alert,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (STRReport) TableName() string { return "str_reports" }

// IsOverdue reports whether the 24-hour window has passed without filing.
func (s *STRReport) IsOverdue() bool {
	return s.Status != STRStatusFiled && time.Now().After(s.DeadlineAt)
}

// CTR report statuses (ctr_reports.status).
const (
	CTRStatusOpen  = "open"
	CTRStatusFiled = "filed"
)

// CTRReport is one day's cash-threshold activity for one user: ₦5,000,000
// individual / ₦10,000,000 body corporate → CTR within 7 days. PropVest's
// flows are electronic, so rows are produced from qualifying deposit volume
// and annotated — the compliance officer decides whether a cash CTR applies.
type CTRReport struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	WindowDate    time.Time  `gorm:"type:date;not null;uniqueIndex:idx_ctr_user_day_threshold,priority:2" json:"window_date"`
	AmountKobo    int64      `gorm:"not null" json:"amount_kobo"`
	TxnCount      int        `gorm:"not null;default:1" json:"txn_count"`
	ThresholdKobo int64      `gorm:"not null;uniqueIndex:idx_ctr_user_day_threshold,priority:3" json:"threshold_kobo"`
	Status        string     `gorm:"size:20;not null;default:'open'" json:"status"`
	DeadlineAt    time.Time  `gorm:"not null" json:"deadline_at"`
	FiledAt       *time.Time `json:"filed_at,omitempty"`
	Notes         *string    `json:"notes,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (CTRReport) TableName() string { return "ctr_reports" }

// CTR/STR thresholds (kobo). Cash CTR: ₦5M individual, ₦10M corporate.
const (
	CTRThresholdIndividualKobo = 500000000  // ₦5,000,000
	CTRThresholdCorporateKobo  = 1000000000 // ₦10,000,000
)

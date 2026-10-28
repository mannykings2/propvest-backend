package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ═══════════════════════════════════════════════════════════════════════════
// PRIVACY / DATA-RIGHTS MODELS (Phase 9 — NDPA 2023 + GAID 2025)
// ═══════════════════════════════════════════════════════════════════════════

// Consent purposes. privacy_policy and terms are versioned (a consent pins the
// version it was given for); marketing is a pure opt-in/opt-out.
const (
	ConsentPurposePrivacyPolicy = "privacy_policy"
	ConsentPurposeTerms         = "terms"
	ConsentPurposeMarketing     = "marketing"
)

// PolicyVersion is one published version of a legal document. Bumping it makes
// every user whose latest consent pins an older version need to re-consent.
type PolicyVersion struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Policy      string    `gorm:"size:40;uniqueIndex;not null" json:"policy"`
	Version     string    `gorm:"size:20;not null" json:"version"`
	EffectiveAt time.Time `gorm:"not null;default:now()" json:"effective_at"`
	CreatedBy   *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (PolicyVersion) TableName() string { return "policy_versions" }

// UserConsent is one consent DECISION. The table is append-only evidence: a
// withdrawal is a new row with Granted=false, never an edit of history.
type UserConsent struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Purpose       string     `gorm:"size:60;not null" json:"purpose"`
	Granted       bool       `gorm:"not null" json:"granted"`
	PolicyVersion *string    `json:"policy_version,omitempty"` // NULL for marketing
	IPAddress     *string    `gorm:"size:64" json:"ip_address,omitempty"`
	UserAgent     *string    `json:"user_agent,omitempty"`
	CreatedAt     time.Time  `gorm:"index" json:"created_at"`
}

func (UserConsent) TableName() string { return "user_consents" }

// Suppression reasons.
const (
	SuppressionReasonUnsubscribe = "unsubscribe"
	SuppressionReasonAdmin       = "admin"
)

// MarketingSuppression is one address the platform must NOT send marketing to.
// The bulk-messaging service checks this list before every send (NDPA
// objection-to-direct-marketing; FCCPC unsubscribe baseline).
type MarketingSuppression struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Channel   string    `gorm:"size:20;not null;default:'email';uniqueIndex:idx_suppressions_addr" json:"channel"`
	Address   string    `gorm:"size:255;not null;uniqueIndex:idx_suppressions_addr" json:"address"`
	UserID    *uuid.UUID `gorm:"type:uuid" json:"user_id,omitempty"`
	Reason    string    `gorm:"size:40;not null;default:'unsubscribe'" json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

func (MarketingSuppression) TableName() string { return "marketing_suppressions" }

// Data export job lifecycle.
const (
	ExportStatusQueued  = "queued"
	ExportStatusReady   = "ready"
	ExportStatusFailed  = "failed"
	ExportStatusExpired = "expired"
)

// DataExportJob is one NDPA access-right request: an async job that assembles
// the user's data into a JSON bundle reachable only through a bearer token
// (the SHA-256 is stored; the raw token is handed to the user once). The
// bundle is wiped when the job expires.
type DataExportJob struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Status    string    `gorm:"size:20;not null;default:'queued'" json:"status"`

	DownloadTokenHash *string         `gorm:"size:64" json:"-"`
	Bundle            datatypes.JSON `gorm:"type:jsonb" json:"-"`
	Error             *string         `json:"error,omitempty"`
	ExpiresAt         *time.Time      `json:"expires_at,omitempty"`

	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (DataExportJob) TableName() string { return "data_export_jobs" }

// Account deletion lifecycle. requested → (cancelled | executed after the
// 30-day grace); execution anonymises identity but RETAINS financial rows.
const (
	DeletionStatusRequested  = "requested"
	DeletionStatusCancelled  = "cancelled"
	DeletionStatusExecuted   = "executed"
	DeletionGraceDays        = 30
)

// AccountDeletionRequest is one erasure request with its grace-period clock.
type AccountDeletionRequest struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID   uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	Status   string    `gorm:"size:20;not null;default:'requested'" json:"status"`
	Reason   *string   `json:"reason,omitempty"`

	RequestedAt  time.Time  `gorm:"not null;default:now()" json:"requested_at"`
	ExecuteAfter time.Time  `gorm:"not null" json:"execute_after"`
	CancelledAt  *time.Time `json:"cancelled_at,omitempty"`
	ExecutedAt   *time.Time `json:"executed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (AccountDeletionRequest) TableName() string { return "account_deletions" }
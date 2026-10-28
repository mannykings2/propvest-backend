package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// KYCSubmission is a user's identity-verification submission (BVN + NIN +
// document type), queued for admin review.
//
// One row per user (UNIQUE user_id): a re-submit updates the existing row,
// which keeps the admin queue deduplicated.
//
// SECURITY: BVN and NIN are PII. They are never logged, and list endpoints only
// expose the last 4 digits. At rest they are AES-GCM ciphertext — the
// repository encrypts on write and decrypts on read (internal/utils/piienc),
// so this struct holds plaintext only in process memory. json:"-" keeps it
// out of every API response; the admin detail endpoint is the one deliberate
// exception and builds its own DTO.
type KYCSubmission struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`

	BVN string `gorm:"type:text;not null" json:"-"`
	NIN string `gorm:"type:text;not null" json:"-"`

	// DocType identifies the supporting document the user claims to have
	// uploaded (e.g. "national_id", "drivers_license", "international_passport").
	DocType string `gorm:"size:60;not null" json:"doc_type"`

	// Status: pending | verified | rejected
	Status string `gorm:"size:20;not null;default:'pending'" json:"status"`

	Reason      *string    `json:"reason,omitempty"`
	SubmittedAt time.Time  `json:"submitted_at"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
	DecidedBy   *uuid.UUID `json:"decided_by,omitempty"`

	// Manual sanctions/PEP screening (Phase 6.3): the admin records a check
	// before approval. ScreeningResult is "" (not screened) | "clear" | "hit";
	// approval is refused unless it is "clear", and a "hit" blocks approval
	// outright (reject or escalate instead).
	ScreeningSource *string    `gorm:"size:120" json:"screening_source,omitempty"`
	ScreeningResult *string    `gorm:"size:20" json:"screening_result,omitempty"`
	ScreeningNotes  *string    `json:"screening_notes,omitempty"`
	ScreeningAt     *time.Time `json:"screening_at,omitempty"`
	ScreenedBy      *uuid.UUID `gorm:"type:uuid" json:"screened_by,omitempty"`

	// Evidence carries review artifacts (document references, admin notes)
	// as JSONB. Never contains the full BVN/NIN — only references.
	Evidence datatypes.JSON `gorm:"type:jsonb" json:"evidence,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Screening result values (kyc_submissions.screening_result).
const (
	ScreeningClear = "clear"
	ScreeningHit   = "hit"
)

// KYC submission status values.
const (
	KYCStatusPending  = "pending"
	KYCStatusVerified = "verified"
	KYCStatusRejected = "rejected"
)

// TableName maps to kyc_submissions.
func (KYCSubmission) TableName() string {
	return "kyc_submissions"
}

// BVNLast4 returns the last 4 digits of the BVN for safe display in admin
// lists. The full BVN must never leave the server.
func (k *KYCSubmission) BVNLast4() string {
	if len(k.BVN) <= 4 {
		return k.BVN
	}
	return k.BVN[len(k.BVN)-4:]
}

// NINLast4 returns the last 4 digits of the NIN for safe display.
func (k *KYCSubmission) NINLast4() string {
	if len(k.NIN) <= 4 {
		return k.NIN
	}
	return k.NIN[len(k.NIN)-4:]
}

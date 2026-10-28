package dto

import (
	"time"

	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════════════════════
// PRIVACY / DATA-RIGHTS DTOs (Phase 9)
// ═══════════════════════════════════════════════════════════════════════════

// ConsentRecordResponse is one consent decision as the API shows it.
type ConsentRecordResponse struct {
	Purpose       string     `json:"purpose"`
	Granted       bool       `json:"granted"`
	PolicyVersion *string    `json:"policy_version,omitempty"`
	IPAddress     *string    `json:"ip_address,omitempty"`
	CreatedAt     time.Time  `json:"granted_at"`
}

// ConsentStatusResponse is GET /me/consents: the live policy versions, the
// user's latest decision per purpose, and whether a policy bump obliges them
// to re-consent.
type ConsentStatusResponse struct {
	Policies       map[string]string     `json:"policies"`
	Consents       []ConsentRecordResponse `json:"consents"`
	NeedsReconsent bool                  `json:"needs_reconsent"`
}

// GiveConsentRequest is POST /me/consents. Pointers so "absent" differs from
// "explicitly false" — only provided purposes record a row.
type GiveConsentRequest struct {
	PrivacyPolicy *bool `json:"privacy_policy"`
	Terms         *bool `json:"terms"`
	Marketing     *bool `json:"marketing"`
}

// DataExportJobResponse is the public view of an export job (the bundle and
// token hash never leave the server).
type DataExportJobResponse struct {
	ID        uuid.UUID  `json:"id"`
	Status    string     `json:"status"`
	Error     *string    `json:"error,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// DataExportRequestResponse wraps the job plus the one-time download token,
// returned only from the request/reissue endpoints so the owner can fetch the
// bundle immediately (the same link also arrives by notification/email).
type DataExportRequestResponse struct {
	DataExportJobResponse
	DownloadToken string `json:"download_token,omitempty"`
	DownloadPath  string `json:"download_path,omitempty"` // /me/exports/download/<token>
}

// AccountDeletionResponse is GET /me/account/delete — the grace-period clock.
type AccountDeletionResponse struct {
	ID           uuid.UUID  `json:"id"`
	Status       string     `json:"status"`
	Reason       *string    `json:"reason,omitempty"`
	RequestedAt  time.Time  `json:"requested_at"`
	ExecuteAfter time.Time  `json:"execute_after"`
	DaysLeft     int        `json:"days_left"`
	CancelledAt  *time.Time `json:"cancelled_at,omitempty"`
	ExecutedAt   *time.Time `json:"executed_at,omitempty"`
}

// AccountDeletionRequest is POST /me/account/delete.
type AccountDeletionRequest struct {
	Reason string `json:"reason" binding:"max=500"`
}

// ── Admin privacy DTOs ─────────────────────────────────────────────────────

// AddSuppressionRequest puts one address on the marketing suppression list by
// hand (the unsubscribe token writes rows too, reason=unsubscribe).
type AddSuppressionRequest struct {
	Channel string `json:"channel" binding:"omitempty,oneof=email sms"`
	// NOT validated as `email`: channel=sms suppresses phone numbers too. The
	// list is an opt-out ledger — a malformed address is worse skipped than
	// rejected — so only length is enforced here.
	Address string `json:"address" binding:"required,max=255"`
	Reason  string `json:"reason" binding:"omitempty,max=200"`
}

// BumpPolicyVersionRequest publishes a new privacy-policy/terms version. When
// Version is empty the service increments the minor of the current one.
type BumpPolicyVersionRequest struct {
	Policy  string `json:"policy" binding:"required,oneof=privacy_policy terms"`
	Version string `json:"version" binding:"omitempty,max=20"`
}

// DecideDeletionRequest approves (accelerate to now) or cancels a deletion
// during its grace period.
type DecideDeletionRequest struct {
	Action string `json:"action" binding:"required,oneof=approve cancel"`
	Reason string `json:"reason" binding:"max=500"`
}
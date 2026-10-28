package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════════════════════
// WALLET: BANKS, ACCOUNT VERIFICATION, EARNINGS TRANSFER (Workstream A1/A2)
// ═══════════════════════════════════════════════════════════════════════════

// BankResponse is a selectable Nigerian bank. Codes are stable CBN BINs, so a
// static provider-agnostic list is correct — no provider call needed.
type BankResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// VerifyAccountRequest resolves an account number at a bank to its true name.
type VerifyAccountRequest struct {
	AccountNumber string `json:"account_number" binding:"required,len=10"`
	BankCode      string `json:"bank_code" binding:"required"`
}

// VerifyAccountResponse carries the bank-verified account name.
type VerifyAccountResponse struct {
	AccountName   string `json:"account_name"`
	AccountNumber string `json:"account_number"`
	BankName      string `json:"bank_name"`
	BankCode      string `json:"bank_code"`
}

// TransferEarningsRequest moves funds from the earnings balance to the main
// balance so they become withdrawable/investable. Amount is in kobo.
type TransferEarningsRequest struct {
	Amount int64 `json:"amount" binding:"required,gt=0"`
}

// ═══════════════════════════════════════════════════════════════════════════
// KYC SUBMISSIONS (Workstream A4)
// ═══════════════════════════════════════════════════════════════════════════

// SubmitKYCRequest is the Profile modal's payload. Frontend sends docType
// (camelCase); we accept both spellings so neither side breaks.
type SubmitKYCRequest struct {
	BVN     string `json:"bvn" binding:"required,len=11"`
	NIN     string `json:"nin" binding:"required,len=11"`
	DocType string `json:"doc_type" binding:"required,max=60"`
}

// KYCStatusResponse reports the user's submission state (never the raw BVN/NIN).
type KYCStatusResponse struct {
	KYCStatus  string     `json:"kyc_status"`
	Status     string     `json:"status"`
	DocType    string     `json:"doc_type"`
	BVNLast4   string     `json:"bvn_last4"`
	NINLast4   string     `json:"nin_last4"`
	Reason     *string    `json:"reason,omitempty"`
	SubmittedAt time.Time `json:"submitted_at"`
	DecidedAt  *time.Time `json:"decided_at,omitempty"`
}

// KYCQueueItem is one row in the admin KYC review queue. PII-safe: only the
// last 4 digits of BVN/NIN leave the server.
type KYCQueueItem struct {
	SubmissionID uuid.UUID  `json:"submission_id"`
	UserID       uuid.UUID  `json:"user_id"`
	FullName     string     `json:"full_name"`
	Email        string     `json:"email"`
	Phone        string     `json:"phone"`
	KYCStatus    string     `json:"kyc_status"`
	DocType      string     `json:"doc_type"`
	BVNLast4     string     `json:"bvn_last4"`
	NINLast4     string     `json:"nin_last4"`
	Status       string     `json:"status"`
	Reason       *string    `json:"reason,omitempty"`
	SubmittedAt  time.Time  `json:"submitted_at"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`

	// Screening lets the queue show at a glance whether an approval is
	// currently possible (clear), blocked (hit), or not yet done (empty).
	ScreeningResult *string    `json:"screening_result,omitempty"`
	ScreeningSource *string    `json:"screening_source,omitempty"`
	ScreeningAt     *time.Time `json:"screening_at,omitempty"`
}

// KYCDetailResponse is the admin-only submission detail (Phase 6.2).
//
// THE ONE PLACE FULL PII IS RETURNED: BVN/NIN appear decrypted here, on an
// admin route only, and reading it writes a `user.kyc_viewed` audit row (6.5).
// Every other representation (queue, user-facing status) is last-4 only.
type KYCDetailResponse struct {
	SubmissionID uuid.UUID `json:"submission_id"`
	UserID       uuid.UUID `json:"user_id"`
	FullName     string    `json:"full_name"`
	Email        string    `json:"email"`
	Phone        string    `json:"phone"`
	KYCStatus    string    `json:"kyc_status"`

	BVN   string `json:"bvn"` // full, decrypted — admin route only
	NIN   string `json:"nin"` // full, decrypted — admin route only
	DocType string `json:"doc_type"`

	Status      string     `json:"status"`
	Reason      *string    `json:"reason,omitempty"`
	SubmittedAt time.Time  `json:"submitted_at"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
	DecidedBy   *uuid.UUID `json:"decided_by,omitempty"`

	ScreeningSource *string    `json:"screening_source,omitempty"`
	ScreeningResult *string    `json:"screening_result,omitempty"`
	ScreeningNotes  *string    `json:"screening_notes,omitempty"`
	ScreeningAt     *time.Time `json:"screening_at,omitempty"`
	ScreenedBy      *uuid.UUID `json:"screened_by,omitempty"`

	Evidence json.RawMessage `json:"evidence,omitempty"`
}

// RecordScreeningRequest is the admin's manual sanctions/PEP attestation
// (Phase 6.3). `result` must be "clear" or "hit" — the service enforces it,
// so the allowlist lives in one place.
type RecordScreeningRequest struct {
	Source string `json:"source" binding:"required,max=120"` // e.g. "OFAC SDN (manual check)"
	Result string `json:"result" binding:"required"`          // clear | hit
	Notes  string `json:"notes,omitempty" binding:"omitempty,max=1000"`
}

// KYCQueueResponse wraps the queue with a total count.
type KYCQueueResponse struct {
	Submissions []KYCQueueItem `json:"submissions"`
	Total       int64          `json:"total"`
	Page        int            `json:"page"`
	Limit       int            `json:"limit"`
}

// ═══════════════════════════════════════════════════════════════════════════
// OFF-PLAN PROJECTS & SUBSCRIPTIONS (Workstream B)
// ═══════════════════════════════════════════════════════════════════════════

// OffplanProjectResponse is the public shape OffPlan.jsx renders. JSONB blobs
// are passed through verbatim (raw JSON) so the frontend sees exactly the
// structure its static constants had — no shape translation layer.
type OffplanProjectResponse struct {
	ID           uuid.UUID       `json:"id"`
	Name         string          `json:"name"`
	Loc          string          `json:"loc"`
	Type         string          `json:"type"`
	Price        int64           `json:"price"` // NAIRA (frontend divides nothing)
	Stage        string          `json:"stage"`
	Completion   string          `json:"completion"`
	Developer    string          `json:"developer"`
	DownPct      int             `json:"downPct"`
	Img          string          `json:"img"`
	Features     json.RawMessage `json:"features"`
	Renders      json.RawMessage `json:"renders"`
	FinishTypes  json.RawMessage `json:"finishTypes"`
	Plans        json.RawMessage `json:"plans"`
}

// SubscribeOffplanRequest starts an instalment purchase. snake_case to match
// the backend's convention (the frontend service sends these keys).
type SubscribeOffplanRequest struct {
	PlanID       string `json:"plan_id" binding:"required"`
	FinishTypeID string `json:"finish_type_id" binding:"required"`
}

// OffplanSubscriptionResponse confirms a subscription and reports the amounts
// debited, so the UI can show a real receipt.
type OffplanSubscriptionResponse struct {
	SubscriptionID  uuid.UUID `json:"subscription_id"`
	ProjectID       uuid.UUID `json:"project_id"`
	ProjectName     string    `json:"project_name"`
	Reference       string    `json:"reference"`
	PlanID          string    `json:"plan_id"`
	FinishTypeID    string    `json:"finish_type_id"`
	PriceKobo       int64     `json:"price_kobo"`
	DownPaymentKobo int64     `json:"down_payment_kobo"`
	FeeKobo         int64     `json:"fee_kobo"`
	TotalDebitedKobo int64    `json:"total_debited_kobo"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// ═══════════════════════════════════════════════════════════════════════════
// SALE VOTES (Workstream A3)
// ═══════════════════════════════════════════════════════════════════════════

// CastVoteRequest records a co-owner's vote on an open sale campaign.
type CastVoteRequest struct {
	Vote string `json:"vote" binding:"required,oneof=for against"`
}

// SaleVoteStateResponse reports a campaign's live tally for one investment.
type SaleVoteStateResponse struct {
	CampaignID  uuid.UUID `json:"campaign_id"`
	PropertyID  uuid.UUID `json:"property_id"`
	Status      string    `json:"status"`
	Electors    int       `json:"electors"`
	ForCount    int       `json:"for_count"`
	AgainstCount int      `json:"against_count"`
	MyVote      *string   `json:"my_vote,omitempty"`
	CanTrigger  bool      `json:"can_trigger"`
	HoldYears   int       `json:"hold_years"`
	OpenedAt    time.Time `json:"opened_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}

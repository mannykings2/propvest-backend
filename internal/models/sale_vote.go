package models

import (
	"time"

	"github.com/google/uuid"
)

// SaleVoteCampaign is a governance campaign opened by a co-owner once their
// hold period has elapsed. Every distinct investor in the property becomes an
// elector. A simple majority (>50% of electors) in favour passes the campaign
// and triggers an independent NIESV valuation; otherwise it fails.
//
// The partial unique index on (property_id) WHERE status='open' guarantees at
// most one open campaign per property — the database enforces this even under
// concurrent triggers.
type SaleVoteCampaign struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PropertyID uuid.UUID `gorm:"type:uuid;not null;index" json:"property_id"`

	// Status: open | passed | failed
	Status string `gorm:"size:20;not null;default:'open'" json:"status"`

	TriggeredBy uuid.UUID `gorm:"type:uuid;not null" json:"triggered_by"`

	// Electors is the number of distinct investors in the property at trigger
	// time. It is the voting denominator and is frozen when the campaign opens.
	Electors int `gorm:"not null" json:"electors"`

	ForCount     int `gorm:"not null;default:0" json:"for_count"`
	AgainstCount int `gorm:"not null;default:0" json:"against_count"`

	// Result mirrors Status once closed ("passed"/"failed"), kept for clarity
	// in API responses.
	Result *string `json:"result,omitempty"`

	OpenedAt time.Time  `json:"opened_at"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Campaign status values.
const (
	SaleVoteOpen   = "open"
	SaleVotePassed = "passed"
	SaleVoteFailed = "failed"
)

// TableName maps to sale_vote_campaigns.
func (SaleVoteCampaign) TableName() string {
	return "sale_vote_campaigns"
}

// Vote choices.
const (
	SaleVoteChoiceFor     = "for"
	SaleVoteChoiceAgainst = "against"
)

// SaleVote is a single investor's vote on a campaign. One vote per investor is
// enforced by UNIQUE (campaign_id, user_id) at the database level.
type SaleVote struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CampaignID   uuid.UUID `gorm:"type:uuid;not null;index" json:"campaign_id"`
	InvestmentID uuid.UUID `gorm:"type:uuid;not null" json:"investment_id"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	PropertyID   uuid.UUID `gorm:"type:uuid;not null;index" json:"property_id"`

	// Vote: for | against
	Vote string `gorm:"size:10;not null" json:"vote"`

	CreatedAt time.Time `json:"created_at"`
}

// TableName maps to sale_votes.
func (SaleVote) TableName() string {
	return "sale_votes"
}

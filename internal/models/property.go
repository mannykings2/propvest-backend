package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Property is a real-estate asset offered for fractional investment (docs 1.2
// §8, 5.2, 2.2 §13). Investors buy "slots"; SlotsSold/FundedPct track funding.
//
// Money fields are int64 kobo (DECISION D4). The Status field drives business
// behaviour and follows the lifecycle in the domain model (see status constants
// below). Migration 000012 widened the DB CHECK to allow the full set.
type Property struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PropCode      string     `gorm:"uniqueIndex;not null" json:"prop_code"` // e.g. PROP-001
	Name          string     `gorm:"not null" json:"name"`
	Location      string     `gorm:"not null" json:"location"`
	State         string     `gorm:"not null" json:"state"`
	Type          string     `gorm:"not null" json:"type"`        // Residential|Apartment|Land|Commercial
	IncomeType    string     `gorm:"not null" json:"income_type"` // rental|resale
	SPVName       string     `gorm:"not null" json:"spv_name"`
	SPVCACNo      string     `json:"spv_cac_no"`
	TotalSlots    int        `gorm:"not null" json:"total_slots"`
	SlotPrice     int64      `gorm:"not null" json:"slot_price"` // kobo
	TotalValue    int64      `gorm:"not null" json:"total_value"`
	PurchasePrice int64      `gorm:"not null" json:"purchase_price"`
	YieldPct      float64    `json:"yield_pct"`
	AnnualRent    *int64     `json:"annual_rent,omitempty"`
	MonthlyRent   *int64     `json:"monthly_rent,omitempty"`
	FundedPct     int        `gorm:"default:0" json:"funded_pct"`
	SlotsSold     int        `gorm:"default:0" json:"slots_sold"`
	HoldYears     int        `gorm:"default:3" json:"hold_years"`
	Status        string     `gorm:"default:'draft'" json:"status"`
	DeveloperID   *uuid.UUID `gorm:"type:uuid" json:"developer_id,omitempty"`
	Description   string     `json:"description"`
	Tag           string     `json:"tag"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Documents are eager-loadable images/legal files for the detail view.
	Documents []PropertyDocument `gorm:"foreignKey:PropertyID" json:"documents,omitempty"`
}

// Property status lifecycle (docs 1.2 §8). draft/submitted/approved/rejected are
// pre-funding admin states; funding/funded/completed track the money side.
const (
	PropertyStatusDraft     = "draft"
	PropertyStatusSubmitted = "submitted"
	PropertyStatusApproved  = "approved"
	PropertyStatusRejected  = "rejected"
	PropertyStatusFunding   = "funding"
	PropertyStatusFunded    = "funded"
	PropertyStatusCompleted = "completed"
)

// AcceptsInvestment reports whether the property is currently open to new
// investment. Only approved-and-funding properties accept money.
func (p *Property) AcceptsInvestment() bool {
	return p.Status == PropertyStatusApproved || p.Status == PropertyStatusFunding
}

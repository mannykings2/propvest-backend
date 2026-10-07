package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Property represents a real estate investment opportunity on the PropVest platform.
//
// LIFECYCLE:
//   draft    → Only visible to admins, not yet published
//   active   → Published and accepting investments
//   funded   → Target amount reached, no longer accepting investments
//   completed → Investment period ended, returns distributed
//
// FINANCIAL MODEL:
//   - All monetary values stored as int64 in minor currency units (kobo for NGN)
//   - target_amount = unit_price × total_units (fractional investment model)
//   - raised_amount updated by Investment Module when investments are made
//   - raised_amount must never exceed target_amount (enforced by DB constraint)
//
// OWNERSHIP:
//   - Properties are created and managed by admins only
//   - created_by and updated_by track which admin performed actions
//
// DELETION:
//   - Soft deletion only (deleted_at timestamp)
//   - Properties with investments should NEVER be physically deleted
//   - This preserves financial audit trail and investment history
type Property struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// ═══════════════════════════════════════════════════════════════════════════
	// BASIC INFORMATION
	// ═══════════════════════════════════════════════════════════════════════════

	// Title is the property name shown to users
	// Example: "Luxury Apartment Complex - Lekki Phase 1"
	Title string `gorm:"size:255;not null" json:"title"`

	// Slug is a URL-friendly version of the title
	// Must be unique across all properties
	// Example: "luxury-apartment-complex-lekki-phase-1"
	// Generated automatically from title with uniqueness guarantee
	Slug string `gorm:"size:280;uniqueIndex;not null" json:"slug"`

	// Description contains detailed information about the property
	// Supports markdown formatting for rich text display
	Description string `gorm:"type:text;not null" json:"description"`

	// ═══════════════════════════════════════════════════════════════════════════
	// PROPERTY CLASSIFICATION
	// ═══════════════════════════════════════════════════════════════════════════

	// PropertyType categorizes the property
	// Allowed values: residential, commercial, land
	// Enforced by database CHECK constraint
	PropertyType string `gorm:"size:30;not null" json:"property_type"`

	// ═══════════════════════════════════════════════════════════════════════════
	// LOCATION DETAILS
	// ═══════════════════════════════════════════════════════════════════════════

	Address string `gorm:"type:text;not null" json:"address"`
	City    string `gorm:"size:100;not null" json:"city"`
	State   string `gorm:"size:100;not null" json:"state"`
	Country string `gorm:"size:100;not null;default:'Nigeria'" json:"country"`

	// Geolocation coordinates (optional)
	// Both must be present or both must be null (enforced by DB constraint)
	Latitude  *float64 `gorm:"type:numeric(10,7)" json:"latitude,omitempty"`
	Longitude *float64 `gorm:"type:numeric(10,7)" json:"longitude,omitempty"`

	// ═══════════════════════════════════════════════════════════════════════════
	// FINANCIAL DETAILS (all amounts in kobo - minor currency units)
	// ═══════════════════════════════════════════════════════════════════════════

	// TargetAmount is the total funding goal in kobo
	// Example: ₦100,000,000 = 10000000000 kobo
	// Must be greater than zero (enforced by DB constraint)
	TargetAmount int64 `gorm:"not null" json:"target_amount"`

	// RaisedAmount tracks how much has been invested so far
	// Updated by Investment Module when investments are made
	// Must be between 0 and TargetAmount (enforced by DB constraint)
	// IMPORTANT: This is system-managed, NOT manually editable by admins
	RaisedAmount int64 `gorm:"default:0;not null" json:"raised_amount"`

	// Currency (NGN for Nigerian Naira)
	// Allows future multi-currency support
	Currency string `gorm:"size:3;not null;default:'NGN'" json:"currency"`

	// ═══════════════════════════════════════════════════════════════════════════
	// INVESTMENT RETURNS
	// ═══════════════════════════════════════════════════════════════════════════

	// ROIPercent is the expected total return on investment
	// Stored as decimal: 15.50 means 15.5% total return
	// NOT annualized unless specified in business logic
	// Must be greater than zero (enforced by DB constraint)
	ROIPercent float64 `gorm:"type:numeric(10,2);not null" json:"roi_percent"`

	// DurationMonths is the investment period in months
	// Example: 12 months, 24 months, etc.
	// Must be greater than zero (enforced by DB constraint)
	DurationMonths int `gorm:"not null" json:"duration_months"`

	// ═══════════════════════════════════════════════════════════════════════════
	// UNIT-BASED INVESTMENT
	// ═══════════════════════════════════════════════════════════════════════════

	// TotalUnits is the number of fractional units available
	// Example: 1000 units at ₦100,000 each = ₦100,000,000 target
	// Must be greater than zero (enforced by DB constraint)
	TotalUnits int64 `gorm:"not null" json:"total_units"`

	// UnitsSold tracks how many units have been purchased
	// Updated by Investment Module when investments are made
	// Must be between 0 and TotalUnits (enforced by DB constraint)
	// IMPORTANT: This is system-managed, NOT manually editable by admins
	UnitsSold int64 `gorm:"default:0;not null" json:"units_sold"`

	// UnitPrice is the cost of one investment unit in kobo
	// Example: ₦100,000 = 10000000 kobo
	// Ideally: TargetAmount = UnitPrice × TotalUnits
	// Must be greater than zero (enforced by DB constraint)
	UnitPrice int64 `gorm:"not null" json:"unit_price"`

	// MinimumInvestment is the smallest investment allowed in kobo
	// Example: ₦10,000 = 1000000 kobo
	// Must be positive and <= TargetAmount (enforced by DB constraint)
	MinimumInvestment int64 `gorm:"not null" json:"minimum_investment"`

	// ═══════════════════════════════════════════════════════════════════════════
	// STATISTICS
	// ═══════════════════════════════════════════════════════════════════════════

	// InvestorCount tracks unique investors (not number of transactions)
	// Updated by Investment Module
	// IMPORTANT: This is system-managed, NOT manually editable by admins
	InvestorCount int `gorm:"default:0;not null" json:"investor_count"`

	// ═══════════════════════════════════════════════════════════════════════════
	// STATUS & FLAGS
	// ═══════════════════════════════════════════════════════════════════════════

	// Status represents the property lifecycle state
	// Allowed values: draft, active, funded, completed
	// Enforced by database CHECK constraint
	// Only active/funded/completed properties are visible to public
	Status string `gorm:"size:20;not null;default:'draft'" json:"status"`

	// Featured properties appear prominently in listings
	Featured bool `gorm:"default:false;not null" json:"featured"`

	// Verified indicates admin has verified property documentation
	Verified bool `gorm:"default:false;not null" json:"verified"`

	// Trending properties show "trending" badge
	Trending bool `gorm:"default:false;not null" json:"trending"`

	// ═══════════════════════════════════════════════════════════════════════════
	// MEDIA
	// ═══════════════════════════════════════════════════════════════════════════

	// CoverImageURL is the main thumbnail image
	// Should match one of the images in the Images relationship
	CoverImageURL *string `gorm:"type:text" json:"cover_image_url,omitempty"`

	// ═══════════════════════════════════════════════════════════════════════════
	// IMPORTANT DATES
	// ═══════════════════════════════════════════════════════════════════════════

	// LaunchDate is when the property becomes available for investment
	// NULL for draft properties
	LaunchDate *time.Time `json:"launch_date,omitempty"`

	// ExpectedCompletionDate is when the investment period ends
	// Used to estimate when returns will be distributed
	ExpectedCompletionDate *time.Time `json:"expected_completion_date,omitempty"`

	// ═══════════════════════════════════════════════════════════════════════════
	// AUDIT FIELDS
	// ═══════════════════════════════════════════════════════════════════════════

	// CreatedBy is the admin who created this property
	CreatedBy uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`

	// UpdatedBy is the admin who last modified this property
	UpdatedBy *uuid.UUID `gorm:"type:uuid" json:"updated_by,omitempty"`

	// DeletedAt enables soft deletion
	// Properties with investments should NEVER be physically deleted
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// ═══════════════════════════════════════════════════════════════════════════
	// RELATIONSHIPS
	// ═══════════════════════════════════════════════════════════════════════════

	// Images contains all property gallery images
	// Loaded via Preload("Images") when needed
	Images []PropertyImage `gorm:"foreignKey:PropertyID;constraint:OnDelete:CASCADE" json:"images,omitempty"`

	// Documents contains legal/certification documents
	// Loaded via Preload("Documents") when needed
	Documents []PropertyDocument `gorm:"foreignKey:PropertyID;constraint:OnDelete:CASCADE" json:"documents,omitempty"`

	// StatusHistory tracks all status changes for audit trail
	// Loaded via Preload("StatusHistory") when needed
	StatusHistory []PropertyStatusHistory `gorm:"foreignKey:PropertyID;constraint:OnDelete:CASCADE" json:"status_history,omitempty"`
}

// TableName specifies the table name for GORM
func (Property) TableName() string {
	return "properties"
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════

// FundingPercentage calculates how much of the target has been raised
// Returns a value between 0 and 100
func (p *Property) FundingPercentage() float64 {
	if p.TargetAmount == 0 {
		return 0
	}
	percentage := (float64(p.RaisedAmount) / float64(p.TargetAmount)) * 100
	if percentage > 100 {
		return 100 // Should never happen due to DB constraint
	}
	return percentage
}

// RemainingAmount returns how much more funding is needed
// Returns 0 if fully funded
func (p *Property) RemainingAmount() int64 {
	remaining := p.TargetAmount - p.RaisedAmount
	if remaining < 0 {
		return 0 // Should never happen due to DB constraint
	}
	return remaining
}

// RemainingUnits returns how many units are still available
// Returns 0 if all units sold
func (p *Property) RemainingUnits() int64 {
	remaining := p.TotalUnits - p.UnitsSold
	if remaining < 0 {
		return 0 // Should never happen due to DB constraint
	}
	return remaining
}

// IsFullyFunded checks if the property has reached its funding goal
func (p *Property) IsFullyFunded() bool {
	return p.RaisedAmount >= p.TargetAmount || p.UnitsSold >= p.TotalUnits
}

// IsPublic checks if the property is visible to public users
// Only active, funded, and completed properties are public
func (p *Property) IsPublic() bool {
	return p.Status == "active" || p.Status == "funded" || p.Status == "completed"
}

// IsDraft checks if the property is in draft state
func (p *Property) IsDraft() bool {
	return p.Status == "draft"
}

// CanAcceptInvestments checks if the property is accepting new investments
func (p *Property) CanAcceptInvestments() bool {
	return p.Status == "active" && !p.IsFullyFunded()
}

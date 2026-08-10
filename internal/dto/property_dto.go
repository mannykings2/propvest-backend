package dto

import (
	"time"

	"github.com/google/uuid"
)

// PropertyResponse is the public representation of a property (money in kobo).
type PropertyResponse struct {
	ID          uuid.UUID  `json:"id"`
	PropCode    string     `json:"prop_code"`
	Name        string     `json:"name"`
	Location    string     `json:"location"`
	State       string     `json:"state"`
	Type        string     `json:"type"`
	IncomeType  string     `json:"income_type"`
	SPVName     string     `json:"spv_name"`
	TotalSlots  int        `json:"total_slots"`
	SlotPrice   int64      `json:"slot_price"`
	TotalValue  int64      `json:"total_value"`
	YieldPct    float64    `json:"yield_pct"`
	FundedPct   int        `json:"funded_pct"`
	SlotsSold   int        `json:"slots_sold"`
	SlotsLeft   int        `json:"slots_left"`
	HoldYears   int        `json:"hold_years"`
	Status      string     `json:"status"`
	DeveloperID *uuid.UUID `json:"developer_id,omitempty"`
	Description string     `json:"description"`
	Tag         string     `json:"tag"`
	Images      []string   `json:"images"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CreatePropertyRequest is the developer/admin payload to list a property.
// Money fields are in kobo.
type CreatePropertyRequest struct {
	Name          string  `json:"name" binding:"required,min=3,max=255"`
	Location      string  `json:"location" binding:"required"`
	State         string  `json:"state" binding:"required"`
	Type          string  `json:"type" binding:"required,oneof=Residential Apartment Land Commercial"`
	IncomeType    string  `json:"income_type" binding:"required,oneof=rental resale"`
	SPVName       string  `json:"spv_name" binding:"required"`
	SPVCACNo      string  `json:"spv_cac_no"`
	TotalSlots    int     `json:"total_slots" binding:"required,gt=0"`
	SlotPrice     int64   `json:"slot_price" binding:"required,gt=0"`
	TotalValue    int64   `json:"total_value" binding:"required,gt=0"`
	PurchasePrice int64   `json:"purchase_price" binding:"required,gt=0"`
	YieldPct      float64 `json:"yield_pct"`
	HoldYears     int     `json:"hold_years"`
	Description   string  `json:"description"`
	Tag           string  `json:"tag"`
}

// UpdatePropertyRequest updates editable fields (only while in draft).
type UpdatePropertyRequest struct {
	Name        *string  `json:"name"`
	Location    *string  `json:"location"`
	Description *string  `json:"description"`
	SlotPrice   *int64   `json:"slot_price"`
	YieldPct    *float64 `json:"yield_pct"`
	Tag         *string  `json:"tag"`
}

// RejectPropertyRequest requires a reason (docs 3.2 §"reject").
type RejectPropertyRequest struct {
	Reason string `json:"reason" binding:"required,min=3"`
}

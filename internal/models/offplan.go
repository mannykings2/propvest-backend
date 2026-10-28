package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// OffplanProject is a off-plan development (property still under construction)
// that investors can subscribe to with an instalment plan.
//
// The JSONB columns (plans, finish_types, renders, features) store the exact
// shapes the frontend already renders from its static constants — no lossy
// flattening, no frontend shape changes.
//
// Money is stored in kobo like everything else. The frontend's static data
// carried Naira integers; the seed migration multiplies by 100.
type OffplanProject struct {
	ID   uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name string    `gorm:"size:255;not null" json:"name"`

	// Location is the human-readable location string the UI shows as "loc".
	Location string `gorm:"size:255;not null" json:"location"`

	// PropertyType is the unit type shown as "type" (e.g. "4-Bed Detached").
	PropertyType string `gorm:"size:100;not null" json:"property_type"`

	// PriceKobo is the BASE price (carcass finish) in kobo. The actual price
	// depends on the chosen finish type, which carries its own price.
	PriceKobo int64 `gorm:"not null" json:"price_kobo"`

	// Stage: Foundation | Superstructure | Roofing | Finishing
	Stage string `gorm:"size:50;not null" json:"stage"`

	// Completion is a human-readable quarter string, e.g. "Q4 2027".
	Completion string `gorm:"size:50;not null" json:"completion"`

	Developer string `gorm:"size:255;not null" json:"developer"`

	// DownPct is the required down-payment percentage (e.g. 20 for 20%).
	DownPct int `gorm:"not null" json:"down_pct"`

	// Img is the emoji thumbnail the UI renders.
	Img string `gorm:"size:10;not null;default:'🏡'" json:"img"`

	// JSONB blobs copied verbatim from the frontend constants.
	Features   datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"features"`
	Renders    datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"renders"`
	FinishTypes datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"finish_types"`
	Plans      datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"plans"`

	Status    string         `gorm:"size:20;not null;default:'active'" json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName maps to offplan_projects.
func (OffplanProject) TableName() string {
	return "offplan_projects"
}

// OffplanSubscription is a user's instalment-purchase commitment on an
// off-plan project. Created by POST /offplan/projects/:id/subscribe.
//
// The wallet debit (down payment + 2% fee) and this row are written in one
// database transaction, mirroring the investment module's financial safety
// rules.
type OffplanSubscription struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index" json:"project_id"`

	// PlanID and FinishTypeID reference entries inside the project's JSONB
	// plans/finish_types arrays (e.g. "12m", "full").
	PlanID        string `gorm:"size:20;not null" json:"plan_id"`
	FinishTypeID  string `gorm:"size:20;not null" json:"finish_type_id"`
	PriceKobo     int64  `gorm:"not null" json:"price_kobo"`
	DownPaymentKobo int64 `gorm:"not null" json:"down_payment_kobo"`
	FeeKobo       int64  `gorm:"not null;default:0" json:"fee_kobo"`

	// Reference ties this subscription to its wallet_transactions ledger row.
	Reference string `gorm:"size:60;uniqueIndex;not null" json:"reference"`

	// Status: pending | active | completed | cancelled
	Status string `gorm:"size:20;not null;default:'active'" json:"status"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Subscription status values.
const (
	OffplanSubPending   = "pending"
	OffplanSubActive    = "active"
	OffplanSubCompleted = "completed"
	OffplanSubCancelled = "cancelled"
)

// TableName maps to offplan_subscriptions.
func (OffplanSubscription) TableName() string {
	return "offplan_subscriptions"
}

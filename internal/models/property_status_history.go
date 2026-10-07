package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PropertyStatusHistory provides an audit trail for property lifecycle status changes.
//
// WHY THIS TABLE EXISTS:
//   - Compliance: Investment platforms need audit trails
//   - Debugging: Track when and why properties changed status
//   - Analytics: Understand property lifecycle patterns
//   - Accountability: Know which admin made changes
//
// STATUS TRANSITIONS:
//   NULL → draft       (initial creation)
//   draft → active     (publication)
//   active → funded    (target reached)
//   funded → completed (investment period ended)
//
// AUDIT INFORMATION:
//   - Every status change creates a new history record
//   - ChangedBy can be NULL for system-initiated changes
//   - Reason field documents why the change was made
//   - Records are IMMUTABLE - never update or delete
//
// CASCADE DELETION:
//   - When a property is deleted, its status history is deleted
//   - This is acceptable since the property itself is soft-deleted
//   - The audit trail remains as long as the property exists
type PropertyStatusHistory struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// ═══════════════════════════════════════════════════════════════════════════
	// FOREIGN KEY
	// ═══════════════════════════════════════════════════════════════════════════

	// PropertyID links this history record to its property
	// CASCADE deletion: when property is deleted, history is deleted
	PropertyID uuid.UUID `gorm:"type:uuid;not null;index:idx_property_status_history_property" json:"property_id"`

	// ═══════════════════════════════════════════════════════════════════════════
	// STATUS TRANSITION
	// ═══════════════════════════════════════════════════════════════════════════

	// FromStatus is the previous status
	// NULL for initial status (property creation: NULL → draft)
	// Otherwise must be one of: draft, active, funded, completed
	// Enforced by database CHECK constraint
	FromStatus *string `gorm:"size:20" json:"from_status,omitempty"`

	// ToStatus is the new status after the change
	// Must be one of: draft, active, funded, completed
	// Enforced by database CHECK constraint
	ToStatus string `gorm:"size:20;not null" json:"to_status"`

	// ═══════════════════════════════════════════════════════════════════════════
	// AUDIT INFORMATION
	// ═══════════════════════════════════════════════════════════════════════════

	// ChangedBy is the admin user who made the status change
	// NULL for system-initiated changes (e.g., automatic funding status update)
	// Examples:
	//   - Admin publishes property: ChangedBy = admin UUID
	//   - System marks property as funded: ChangedBy = NULL
	ChangedBy *uuid.UUID `gorm:"type:uuid" json:"changed_by,omitempty"`

	// Reason documents why the status changed
	// Optional but recommended for clarity
	// Examples:
	//   - "Property fully funded"
	//   - "Admin published property for public viewing"
	//   - "Investment period completed"
	//   - "Property information verified and approved"
	Reason *string `gorm:"type:text" json:"reason,omitempty"`

	// ═══════════════════════════════════════════════════════════════════════════
	// TIMESTAMP
	// ═══════════════════════════════════════════════════════════════════════════

	// CreatedAt is when the status change occurred
	// This table only needs CreatedAt (no UpdatedAt) because records are immutable
	CreatedAt time.Time `gorm:"index:idx_property_status_history_property" json:"created_at"`
}

// TableName specifies the table name for GORM
func (PropertyStatusHistory) TableName() string {
	return "property_status_history"
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════

// IsInitialStatus checks if this is the first status (property creation)
// Returns true if FromStatus is NULL
func (psh *PropertyStatusHistory) IsInitialStatus() bool {
	return psh.FromStatus == nil
}

// IsSystemChange checks if the change was initiated by the system
// Returns true if ChangedBy is NULL
func (psh *PropertyStatusHistory) IsSystemChange() bool {
	return psh.ChangedBy == nil
}

// TransitionDescription returns a human-readable description of the status change
// Example: "draft → active", "active → funded", "NULL → draft"
func (psh *PropertyStatusHistory) TransitionDescription() string {
	from := "NULL"
	if psh.FromStatus != nil {
		from = *psh.FromStatus
	}
	return fmt.Sprintf("%s → %s", from, psh.ToStatus)
}

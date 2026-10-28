package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// AuditLog is the append-only security/compliance trail (docs 1.2 §15, 4.2 §17,
// 6.2 §19). It answers "who did what, when, from where". Rows are written by the
// audit recorder and are NEVER updated or deleted.
//
// This is deliberately separate from application logs: audit records are
// business/security evidence, not debugging output.
//
// ATOMICITY GUARANTEE (Milestone 7):
// Audit logs are written in the SAME database transaction as the business change.
// If the business operation fails, the audit log is rolled back.
// If the audit log insert fails, the business operation is rolled back.
// This ensures audit integrity - no lost audit records, no false claims.
type AuditLog struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// ActorID is the user who performed the action (nil for anonymous/system).
	ActorID *uuid.UUID `gorm:"type:uuid;index" json:"actor_id,omitempty"`

	// ActorType distinguishes admin, user, or system actions (Milestone 7)
	// Values: "admin", "user", "system"
	ActorType string `gorm:"size:32;not null;default:'user';index" json:"actor_type"`

	// ActorRole is a snapshot of the actor's role at the time of action (Milestone 7)
	// Critical for investigating privilege escalation or role abuse
	// Example: "admin", "super_admin"
	ActorRole string `gorm:"size:32" json:"actor_role,omitempty"`

	Action string `gorm:"not null;index" json:"action"`

	// Description is human-readable action summary (Milestone 7)
	// Example: "Admin suspended user account for policy violation"
	Description string `json:"description,omitempty"`

	TargetType string     `json:"target_type"`
	TargetID   *uuid.UUID `gorm:"type:uuid" json:"target_id,omitempty"`

	// OldValues stores before-state as JSONB (Milestone 7)
	// Redacted/allowlisted - never contains passwords, tokens, secrets
	// Example: {"status": "active", "role": "user"}
	OldValues datatypes.JSON `gorm:"type:jsonb" json:"old_values,omitempty"`

	// NewValues stores after-state as JSONB (Milestone 7)
	// Redacted/allowlisted - never contains passwords, tokens, secrets
	// Example: {"status": "suspended", "role": "admin"}
	NewValues datatypes.JSON `gorm:"type:jsonb" json:"new_values,omitempty"`

	// Reason is required for consequential admin actions (Milestone 7)
	// Examples: "Policy violation", "Requested by user", "Fraud detected"
	Reason string `json:"reason,omitempty"`

	// IdempotencyKey ensures financial/high-risk operations not duplicated (Milestone 7)
	// Unique per actor to prevent concurrent duplicate requests
	IdempotencyKey *string `gorm:"size:128" json:"idempotency_key,omitempty"`

	IPAddress string         `json:"ip_address"`
	UserAgent string         `json:"user_agent"`
	RequestID string         `json:"request_id"`
	Metadata  datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// TableName specifies the table name for GORM
func (AuditLog) TableName() string {
	return "audit_logs"
}

// Actor type constants (Milestone 7)
const (
	AuditActorTypeAdmin  = "admin"
	AuditActorTypeUser   = "user"
	AuditActorTypeSystem = "system"
)

// Common audit action constants (Milestone 7)
const (
	// User management actions
	AuditActionUserSuspended   = "user.suspended"
	AuditActionUserActivated   = "user.activated"
	AuditActionUserRoleChanged = "user.role_changed"
	AuditActionUserKYCApproved = "user.kyc_approved"
	AuditActionUserKYCRejected = "user.kyc_rejected"
	// Phase 6.5: reading the full BVN/NIN through the admin detail view is a
	// privileged read and must leave a trail of its own.
	AuditActionUserKYCViewed = "user.kyc_viewed"
	// Phase 6.3: the admin's manual sanctions/PEP screening attestation.
	AuditActionKYCScreeningSaved   = "user.kyc_screening_recorded"
	AuditActionUserDeleted         = "user.deleted"
	AuditActionUserPasswordReset   = "user.password_reset"
	AuditActionUserSessionsRevoked = "user.sessions_revoked"

	// Property management actions
	AuditActionPropertyApproved      = "property.approved"
	AuditActionPropertyRejected      = "property.rejected"
	AuditActionPropertyStatusChanged = "property.status_changed"

	// Investment actions
	AuditActionInvestmentCancelled          = "investment.cancelled"
	AuditActionInvestmentMatured            = "investment.matured"
	AuditActionInvestmentReturnsDistributed = "investment.returns_distributed"

	// Financial actions
	AuditActionWithdrawalApproved = "withdrawal.approved"
	AuditActionWithdrawalRejected = "withdrawal.rejected"
	AuditActionRefundProcessed    = "refund.processed"

	// Notification actions
	AuditActionNotificationResent = "notification.resent"
	AuditActionBulkMessageSent    = "notification.bulk_message_sent"

	// Audit/report actions
	AuditActionAuditExported   = "audit.exported"
	AuditActionReportGenerated = "report.generated"

	// Phase 8 (Risk layer) actions — every triage, toggle and filing write.
	AuditActionRiskAlertUpdated  = "risk.alert.updated"
	AuditActionRiskRuleUpdated   = "risk.rule.updated"
	AuditActionScreeningRecorded = "risk.screening_recorded"
	AuditActionUserRiskFlagged   = "user.risk_flagged"
	AuditActionUserRiskUnflagged = "user.unflagged"
	AuditActionSTRFiled          = "str.filed"
	AuditActionSTRUpdated        = "str.updated"
	AuditActionCTRFiled          = "ctr.filed"
)

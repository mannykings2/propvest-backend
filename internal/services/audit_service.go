package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AuditService handles audit log creation with redaction and transaction support.
//
// CORE RESPONSIBILITIES:
// 1. Record admin actions atomically with business changes
// 2. Redact sensitive fields (passwords, tokens, secrets)
// 3. Allowlist safe fields for old_values/new_values snapshots
// 4. Extract request metadata (IP, user agent, request ID)
//
// USAGE PATTERN:
//   tx := db.Begin()
//   defer tx.Rollback()
//
//   // Business operation
//   err := userService.SuspendUser(ctx, tx, userID)
//   if err != nil {
//       return err
//   }
//
//   // Audit log (same transaction)
//   err = auditService.Record(ctx, tx, AuditEvent{...})
//   if err != nil {
//       return err
//   }
//
//   tx.Commit() // Both committed atomically
type AuditService interface {
	// Record creates an audit log entry atomically within a transaction.
	//
	// Parameters:
	//   ctx - request context (may contain request metadata)
	//   tx - database transaction handle
	//   event - audit event details (actor, action, target, old/new state)
	//
	// Returns error if audit insert fails (causes transaction rollback)
	Record(ctx context.Context, tx *gorm.DB, event AuditEvent) error
}

// AuditEvent contains all information for an audit log entry.
//
// REQUIRED FIELDS:
//   - ActorID (can be nil for system actions)
//   - ActorType (admin, user, system)
//   - Action (e.g., "user.suspended", "property.approved")
//   - TargetType (e.g., "user", "property")
//
// OPTIONAL FIELDS:
//   - TargetID (nil for bulk operations)
//   - OldValues, NewValues (will be redacted/allowlisted)
//   - Reason (required for consequential actions)
//   - IdempotencyKey (for financial/high-risk operations)
//   - Description (human-readable summary)
type AuditEvent struct {
	// Actor information
	ActorID   *uuid.UUID // User who performed the action (nil for system)
	ActorType string     // admin, user, system
	ActorRole string     // admin, super_admin, etc.

	// Action details
	Action      string // e.g., "user.suspended", "property.approved"
	Description string // Human-readable summary

	// Target information
	TargetType string     // e.g., "user", "property", "investment"
	TargetID   *uuid.UUID // Specific target (nil for bulk operations)

	// State snapshots (will be redacted)
	OldValues map[string]interface{} // Before-state
	NewValues map[string]interface{} // After-state

	// Justification
	Reason string // Required for consequential actions

	// Idempotency
	IdempotencyKey string // For financial/high-risk operations

	// Request metadata (extracted from context)
	RequestMetadata RequestMetadata
}

// RequestMetadata contains HTTP request context for audit trail.
type RequestMetadata struct {
	RequestID string // Correlation ID for request tracing
	IPAddress string // Client IP address
	UserAgent string // Client user agent
}

// ────────────────────────────────────────────────────────────────────────
// Request metadata propagation
// ────────────────────────────────────────────────────────────────────────
//
// Services must not depend on Gin, but audit rows need the request id, client
// IP and user agent. Handlers attach them once with WithRequestMetadata and
// every service that records an audit event reads them back with
// RequestMetadataFromContext. Missing metadata degrades to empty strings — an
// audit row without an IP is still an audit row.
type metadataCtxKey struct{}

// WithRequestMetadata attaches HTTP request metadata to ctx.
func WithRequestMetadata(ctx context.Context, md RequestMetadata) context.Context {
	return context.WithValue(ctx, metadataCtxKey{}, md)
}

// RequestMetadataFromContext returns the metadata attached by
// WithRequestMetadata, or the zero value when none was set.
func RequestMetadataFromContext(ctx context.Context) RequestMetadata {
	if md, ok := ctx.Value(metadataCtxKey{}).(RequestMetadata); ok {
		return md
	}
	return RequestMetadata{}
}

// ────────────────────────────────────────────────────────────────────────────
// Implementation
// ────────────────────────────────────────────────────────────────────────────

type auditService struct {
	auditRepo repositories.AuditRepository
}

// NewAuditService creates a new audit service instance.
func NewAuditService(auditRepo repositories.AuditRepository) AuditService {
	return &auditService{
		auditRepo: auditRepo,
	}
}

// Record creates an audit log entry with redaction and allowlisting.
func (s *auditService) Record(ctx context.Context, tx *gorm.DB, event AuditEvent) error {
	// Redact sensitive fields from snapshots
	oldValuesRedacted := RedactSensitiveFields(event.OldValues)
	newValuesRedacted := RedactSensitiveFields(event.NewValues)

	// Convert to JSONB (nil maps become NULL in database)
	var oldValuesJSON, newValuesJSON datatypes.JSON
	if len(oldValuesRedacted) > 0 {
		bytes, err := json.Marshal(oldValuesRedacted)
		if err != nil {
			// If serialization fails, skip snapshots rather than fail entire audit
			oldValuesJSON = nil
		} else {
			oldValuesJSON = bytes
		}
	}
	if len(newValuesRedacted) > 0 {
		bytes, err := json.Marshal(newValuesRedacted)
		if err != nil {
			newValuesJSON = nil
		} else {
			newValuesJSON = bytes
		}
	}

	// Build audit log model
	auditLog := &models.AuditLog{
		ActorID:     event.ActorID,
		ActorType:   event.ActorType,
		ActorRole:   event.ActorRole,
		Action:      event.Action,
		Description: event.Description,
		TargetType:  event.TargetType,
		TargetID:    event.TargetID,
		OldValues:   oldValuesJSON,
		NewValues:   newValuesJSON,
		Reason:      event.Reason,
		IPAddress:   event.RequestMetadata.IPAddress,
		UserAgent:   event.RequestMetadata.UserAgent,
		RequestID:   event.RequestMetadata.RequestID,
		CreatedAt:   time.Now(),
	}

	// Set idempotency key if provided
	if event.IdempotencyKey != "" {
		auditLog.IdempotencyKey = &event.IdempotencyKey
	}

	// Insert audit log in transaction
	return s.auditRepo.Create(ctx, tx, auditLog)
}

// ────────────────────────────────────────────────────────────────────────────
// Redaction and Allowlisting
// ────────────────────────────────────────────────────────────────────────────

// SENSITIVE_FIELDS are NEVER included in audit snapshots.
// These fields contain secrets that must not be logged.
var SENSITIVE_FIELDS = []string{
	// Authentication
	"password",
	"password_hash",
	"passwordhash",
	"pwd",
	
	// Tokens
	"token",
	"access_token",
	"refresh_token",
	"jwt",
	"bearer",
	"authorization",
	
	// Secrets
	"secret",
	"api_key",
	"apikey",
	"private_key",
	"privatekey",
	
	// Verification
	"otp",
	"otp_code",
	"verification_code",
	"reset_token",
	
	// Payment
	"cvv",
	"card_number",
	"cardnumber",
	"pin",
	
	// Provider credentials
	"paystack_secret",
	"cloudinary_secret",
	"smtp_password",
	"sms_api_key",
}

// RedactSensitiveFields removes sensitive fields from a map.
//
// STRATEGY:
// - Case-insensitive field name matching
// - Substring matching (e.g., "old_password", "new_password_hash")
// - Returns new map (does not modify original)
//
// EXAMPLE:
//   input:  {"email": "user@example.com", "password_hash": "bcrypt..."}
//   output: {"email": "user@example.com"}  // password_hash removed
func RedactSensitiveFields(values map[string]interface{}) map[string]interface{} {
	if values == nil || len(values) == 0 {
		return nil
	}

	redacted := make(map[string]interface{})
	for key, value := range values {
		if !isSensitiveField(key) {
			redacted[key] = value
		}
	}

	return redacted
}

// isSensitiveField checks if a field name matches sensitive patterns.
func isSensitiveField(fieldName string) bool {
	lowerField := strings.ToLower(fieldName)
	for _, sensitive := range SENSITIVE_FIELDS {
		if strings.Contains(lowerField, sensitive) {
			return true
		}
	}
	return false
}

// AllowlistedFields filters a map to only include safe, documented fields.
// This is a stricter alternative to RedactSensitiveFields.
//
// USE CASE:
// When you want to explicitly control which fields appear in audit logs.
//
// EXAMPLE:
//   input:    {"id": "...", "email": "...", "internal_flag": true}
//   allowlist: []string{"id", "email", "status", "role"}
//   output:   {"id": "...", "email": "..."}  // internal_flag not allowlisted
func AllowlistedFields(values map[string]interface{}, allowlist []string) map[string]interface{} {
	if values == nil || len(values) == 0 {
		return nil
	}

	allowed := make(map[string]interface{})
	for _, key := range allowlist {
		if value, exists := values[key]; exists {
			allowed[key] = value
		}
	}

	return allowed
}

// ────────────────────────────────────────────────────────────────────────────
// Request Metadata Extraction
// ────────────────────────────────────────────────────────────────────────────

// ExtractRequestMetadata extracts audit-relevant metadata from Gin context.
//
// USAGE (from handler):
//   metadata := audit.ExtractRequestMetadata(c)
//   err := auditService.Record(ctx, tx, AuditEvent{
//       RequestMetadata: metadata,
//       ...
//   })
//
// NOTE: This function should be moved to a handler helper or middleware
// to avoid coupling service layer to Gin. For now, it's here for convenience.
//
// TODO: Create internal/utils/audit/metadata.go for Gin-specific helpers.
/*
func ExtractRequestMetadata(c *gin.Context) RequestMetadata {
	return RequestMetadata{
		RequestID: c.GetString("request_id"), // Set by request ID middleware
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
}
*/

// ────────────────────────────────────────────────────────────────────────────
// Helper Functions
// ────────────────────────────────────────────────────────────────────────────

// NewAdminAuditEvent is a helper to create admin action audit events.
//
// USAGE:
//   event := NewAdminAuditEvent(
//       adminID,
//       "admin",
//       models.AuditActionUserSuspended,
//       "user",
//       &userID,
//       "Policy violation",
//   )
//   event.OldValues = map[string]interface{}{"is_active": true}
//   event.NewValues = map[string]interface{}{"is_active": false}
//   err := auditService.Record(ctx, tx, event)
func NewAdminAuditEvent(
	actorID uuid.UUID,
	actorRole string,
	action string,
	targetType string,
	targetID *uuid.UUID,
	reason string,
) AuditEvent {
	return AuditEvent{
		ActorID:    &actorID,
		ActorType:  models.AuditActorTypeAdmin,
		ActorRole:  actorRole,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Reason:     reason,
	}
}

// NewSystemAuditEvent is a helper to create system action audit events.
//
// USAGE:
//   event := NewSystemAuditEvent(
//       "investment.matured",
//       "investment",
//       &investmentID,
//   )
//   err := auditService.Record(ctx, tx, event)
func NewSystemAuditEvent(
	action string,
	targetType string,
	targetID *uuid.UUID,
) AuditEvent {
	return AuditEvent{
		ActorID:    nil, // System actions have no actor
		ActorType:  models.AuditActorTypeSystem,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
	}
}

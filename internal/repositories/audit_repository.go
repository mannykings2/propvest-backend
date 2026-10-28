package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// AuditRepository defines the contract for audit log data access.
//
// APPEND-ONLY ENFORCEMENT:
// This repository exposes ONLY Create() and Read operations.
// There are NO Update() or Delete() methods - audit logs are immutable.
// This design enforces append-only behavior at the application layer.
//
// TRANSACTION-AWARE:
// Create() accepts *gorm.DB which can be a transaction handle.
// This allows audit logs to be written atomically with business changes.
type AuditRepository interface {
	// Create inserts a new audit log entry.
	// Must be called within the same transaction as the business operation.
	//
	// Parameters:
	//   ctx - request context
	//   tx - database transaction handle (use db.Begin() in service)
	//   log - audit log model to insert
	//
	// Returns error if insert fails (will cause transaction rollback)
	Create(ctx context.Context, tx *gorm.DB, log *models.AuditLog) error

	// FindByID retrieves a single audit log by ID.
	// Used for audit log detail views.
	FindByID(ctx context.Context, id uuid.UUID) (*models.AuditLog, error)

	// List retrieves paginated audit logs with optional filters.
	// Used for audit log search and export.
	//
	// Returns:
	//   logs - array of audit log entries
	//   total - total count (for pagination)
	//   error - if query fails
	List(ctx context.Context, filter AuditLogFilter, page, pageSize int) ([]models.AuditLog, int64, error)
}

// AuditLogFilter defines filter criteria for audit log queries.
//
// All fields are optional - nil/zero values are ignored in query.
type AuditLogFilter struct {
	// Actor filters
	ActorID   *uuid.UUID // Filter by specific actor
	ActorType *string    // Filter by actor type (admin, user, system)
	ActorRole *string    // Filter by actor role (admin, super_admin)

	// Action filters
	Action     *string // Filter by specific action (user.suspended, property.approved)
	ActionLike *string // Filter by action pattern (user.*)

	// Target filters
	TargetType *string    // Filter by target type (user, property, investment)
	TargetID   *uuid.UUID // Filter by specific target

	// Time range filters
	CreatedFrom *time.Time // Filter by created_at >= this time
	CreatedTo   *time.Time // Filter by created_at < this time

	// Request tracking
	RequestID *string // Filter by specific request ID

	// High-risk operations
	HasIdempotencyKey bool // If true, only return audit logs with idempotency keys
}

// ────────────────────────────────────────────────────────────────────────────
// Implementation
// ────────────────────────────────────────────────────────────────────────────

type auditRepository struct {
	db *gorm.DB
}

// NewAuditRepository creates a new audit repository instance.
func NewAuditRepository(db *gorm.DB) AuditRepository {
	return &auditRepository{db: db}
}

// Create inserts a new audit log entry within a transaction.
//
// ATOMICITY PATTERN:
//   tx := repo.db.Begin()
//   defer tx.Rollback()
//
//   // Business operation
//   err := userRepo.Update(ctx, tx, user)
//   if err != nil {
//       return err // tx.Rollback() called
//   }
//
//   // Audit log
//   err = auditRepo.Create(ctx, tx, auditLog)
//   if err != nil {
//       return err // tx.Rollback() called
//   }
//
//   tx.Commit() // Both operations committed atomically
func (r *auditRepository) Create(ctx context.Context, tx *gorm.DB, log *models.AuditLog) error {
	return tx.WithContext(ctx).Create(log).Error
}

// FindByID retrieves a single audit log by ID.
func (r *auditRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.AuditLog, error) {
	var log models.AuditLog
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&log).Error

	if err != nil {
		return nil, err
	}
	return &log, nil
}

// List retrieves paginated audit logs with filters.
//
// STABLE SORTING:
// Results are ordered by (created_at DESC, id DESC) for deterministic pagination.
//
// PAGINATION:
// Uses OFFSET/LIMIT pattern. For very large result sets (millions of rows),
// consider cursor-based pagination in future.
func (r *auditRepository) List(ctx context.Context, filter AuditLogFilter, page, pageSize int) ([]models.AuditLog, int64, error) {
	// Build base query with filters
	query := r.db.WithContext(ctx).Model(&models.AuditLog{})
	query = applyAuditLogFilters(query, filter)

	// Count total matching records
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination and stable sorting
	offset := (page - 1) * pageSize
	var logs []models.AuditLog
	err := query.
		Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&logs).Error

	if err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// applyAuditLogFilters applies filter criteria to GORM query.
// Only non-nil/non-zero filter values are applied.
func applyAuditLogFilters(query *gorm.DB, filter AuditLogFilter) *gorm.DB {
	// Actor filters
	if filter.ActorID != nil {
		query = query.Where("actor_id = ?", *filter.ActorID)
	}
	if filter.ActorType != nil {
		query = query.Where("actor_type = ?", *filter.ActorType)
	}
	if filter.ActorRole != nil {
		query = query.Where("actor_role = ?", *filter.ActorRole)
	}

	// Action filters
	if filter.Action != nil {
		query = query.Where("action = ?", *filter.Action)
	}
	if filter.ActionLike != nil {
		query = query.Where("action LIKE ?", *filter.ActionLike+"%")
	}

	// Target filters
	if filter.TargetType != nil {
		query = query.Where("target_type = ?", *filter.TargetType)
	}
	if filter.TargetID != nil {
		query = query.Where("target_id = ?", *filter.TargetID)
	}

	// Time range filters
	if filter.CreatedFrom != nil {
		query = query.Where("created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("created_at < ?", *filter.CreatedTo)
	}

	// Request tracking
	if filter.RequestID != nil {
		query = query.Where("request_id = ?", *filter.RequestID)
	}

	// High-risk operations
	if filter.HasIdempotencyKey {
		query = query.Where("idempotency_key IS NOT NULL")
	}

	return query
}

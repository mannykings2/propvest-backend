package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PropertyRepository defines the interface for property data access operations.
//
// This repository handles CRUD operations for properties and provides
// specialized methods for filtering, searching, and managing property lifecycle.
//
// KEY PATTERNS:
//   - All operations accept context.Context for cancellation/timeout
//   - Transactional methods accept optional tx *gorm.DB parameter
//   - Public queries automatically exclude drafts and soft-deleted properties
//   - Admin queries can access all properties including drafts
type PropertyRepository interface {
	// ═══════════════════════════════════════════════════════════════════════════
	// CRUD OPERATIONS
	// ═══════════════════════════════════════════════════════════════════════════

	// Create inserts a new property into the database.
	// MUST be called within a transaction when creating related records.
	// The property will be created with status='draft' by default.
	Create(ctx context.Context, property *models.Property, tx *gorm.DB) error

	// FindByID retrieves a property by its UUID.
	// Returns gorm.ErrRecordNotFound if not found.
	// Includes soft-deleted properties.
	FindByID(ctx context.Context, id uuid.UUID) (*models.Property, error)

	// FindPublicByID retrieves a property only if it's publicly visible.
	// Public properties are those with status: active, funded, or completed.
	// Returns gorm.ErrRecordNotFound if:
	//   - Property doesn't exist
	//   - Property is draft
	//   - Property is soft-deleted
	FindPublicByID(ctx context.Context, id uuid.UUID) (*models.Property, error)

	// FindBySlug retrieves a property by its URL-friendly slug.
	// Returns gorm.ErrRecordNotFound if not found.
	FindBySlug(ctx context.Context, slug string) (*models.Property, error)

	// Update saves changes to an existing property.
	// MUST be called within a transaction when updating related records.
	Update(ctx context.Context, property *models.Property, tx *gorm.DB) error

	// SoftDelete marks a property as deleted by setting deleted_at timestamp.
	// The property remains in the database but is excluded from normal queries.
	// MUST be called within a transaction if checking for investments first.
	SoftDelete(ctx context.Context, id uuid.UUID, tx *gorm.DB) error

	// ═══════════════════════════════════════════════════════════════════════════
	// LISTING & FILTERING
	// ═══════════════════════════════════════════════════════════════════════════

	// List retrieves properties based on filter criteria with pagination.
	// Returns both the properties slice and total count for pagination metadata.
	// Automatically excludes soft-deleted properties.
	// For public listings, automatically filters to active/funded/completed only.
	List(ctx context.Context, filter PropertyFilter) ([]*models.Property, int64, error)

	// ═══════════════════════════════════════════════════════════════════════════
	// STATUS MANAGEMENT
	// ═══════════════════════════════════════════════════════════════════════════

	// UpdateStatus changes the property status with validation.
	// Validates that the transition is from expectedFrom to newTo status.
	// This prevents race conditions where two concurrent requests try to
	// change status from different states.
	// MUST be called within a transaction when also creating status history.
	//
	// Example: Publishing a property
	//   err := UpdateStatus(ctx, propertyID, "draft", "active", tx)
	//   // Will fail if property is no longer in draft state
	UpdateStatus(ctx context.Context, id uuid.UUID, expectedFrom, newTo string, tx *gorm.DB) error

	// ═══════════════════════════════════════════════════════════════════════════
	// FUNDING OPERATIONS (for future Investment Module)
	// ═══════════════════════════════════════════════════════════════════════════

	// IncrementFunding atomically updates property funding statistics.
	// This is called by the Investment Module when an investment is made.
	// MUST be called within a transaction with the investment operation.
	//
	// Parameters:
	//   amount: Amount in kobo to add to raised_amount
	//   units: Number of units to add to units_sold
	//   investorDelta: Change in investor count (usually 1 for new investor, 0 for existing)
	//
	// The Investment Module should use row-level locking (SELECT FOR UPDATE)
	// to prevent race conditions when multiple investments happen simultaneously.
	IncrementFunding(ctx context.Context, id uuid.UUID, amount int64, units int64, investorDelta int, tx *gorm.DB) error

	// FindByIDForUpdate retrieves a property with row-level lock.
	// This prevents concurrent modifications and is essential for investment operations.
	// MUST be called within a transaction.
	// Used by Investment Module to safely check funding capacity before investing.
	FindByIDForUpdate(ctx context.Context, id uuid.UUID, tx *gorm.DB) (*models.Property, error)
}

// PropertyFilter defines filtering and pagination options for property listings.
type PropertyFilter struct {
	// Search query (searches title, description, city, state)
	Search string

	// Exact match filters
	PropertyType string // residential, commercial, land
	City         string
	State        string
	Status       string // draft, active, funded, completed

	// Boolean filters (nil = don't filter, true/false = filter by value)
	Featured *bool
	Verified *bool
	Trending *bool

	// Range filters for investment amount
	MinInvestment *int64
	MaxInvestment *int64

	// Range filters for ROI
	MinROI *float64
	MaxROI *float64

	// Pagination
	Page     int // 1-indexed (1 = first page)
	PageSize int // Number of results per page

	// Sorting
	SortBy    string // created_at, launch_date, target_amount, roi_percent, etc.
	SortOrder string // asc or desc

	// Admin flag - if true, includes draft properties
	// If false, only returns active/funded/completed properties
	IsAdmin bool
}

// propertyRepository is the concrete implementation of PropertyRepository.
type propertyRepository struct {
	*BaseRepository
}

// NewPropertyRepository creates a new property repository instance.
func NewPropertyRepository(db *gorm.DB) PropertyRepository {
	return &propertyRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// CRUD OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

// Create inserts a new property into the database.
func (r *propertyRepository) Create(ctx context.Context, property *models.Property, tx *gorm.DB) error {
	db := r.getDB(tx)
	return db.WithContext(ctx).Create(property).Error
}

// FindByID retrieves a property by its UUID.
func (r *propertyRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Property, error) {
	var property models.Property
	err := r.WithContext(ctx).
		Where("id = ?", id).
		First(&property).Error
	if err != nil {
		return nil, err
	}
	return &property, nil
}

// FindPublicByID retrieves a property only if it's publicly visible.
func (r *propertyRepository) FindPublicByID(ctx context.Context, id uuid.UUID) (*models.Property, error) {
	var property models.Property
	err := r.WithContext(ctx).
		Where("id = ?", id).
		Where("status IN ?", []string{"active", "funded", "completed"}).
		First(&property).Error
	if err != nil {
		return nil, err
	}
	return &property, nil
}

// FindBySlug retrieves a property by its slug.
func (r *propertyRepository) FindBySlug(ctx context.Context, slug string) (*models.Property, error) {
	var property models.Property
	err := r.WithContext(ctx).
		Where("slug = ?", slug).
		First(&property).Error
	if err != nil {
		return nil, err
	}
	return &property, nil
}

// Update saves changes to an existing property.
func (r *propertyRepository) Update(ctx context.Context, property *models.Property, tx *gorm.DB) error {
	db := r.getDB(tx)
	return db.WithContext(ctx).Save(property).Error
}

// SoftDelete marks a property as deleted.
func (r *propertyRepository) SoftDelete(ctx context.Context, id uuid.UUID, tx *gorm.DB) error {
	db := r.getDB(tx)
	result := db.WithContext(ctx).Delete(&models.Property{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// LISTING & FILTERING
// ═══════════════════════════════════════════════════════════════════════════

// List retrieves properties based on filter criteria with pagination.
func (r *propertyRepository) List(ctx context.Context, filter PropertyFilter) ([]*models.Property, int64, error) {
	query := r.WithContext(ctx).Model(&models.Property{})

	// Apply public/admin filter first
	if !filter.IsAdmin {
		// Public users only see active/funded/completed properties
		query = query.Where("status IN ?", []string{"active", "funded", "completed"})
	}

	// Search across multiple fields
	if filter.Search != "" {
		searchPattern := "%" + filter.Search + "%"
		query = query.Where(
			"title ILIKE ? OR description ILIKE ? OR city ILIKE ? OR state ILIKE ?",
			searchPattern, searchPattern, searchPattern, searchPattern,
		)
	}

	// Exact match filters
	if filter.PropertyType != "" {
		query = query.Where("property_type = ?", filter.PropertyType)
	}
	if filter.City != "" {
		query = query.Where("city = ?", filter.City)
	}
	if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}
	if filter.Status != "" && filter.IsAdmin {
		// Only admins can filter by status
		query = query.Where("status = ?", filter.Status)
	}

	// Boolean filters
	if filter.Featured != nil {
		query = query.Where("featured = ?", *filter.Featured)
	}
	if filter.Verified != nil {
		query = query.Where("verified = ?", *filter.Verified)
	}
	if filter.Trending != nil {
		query = query.Where("trending = ?", *filter.Trending)
	}

	// Range filters for investment amount
	if filter.MinInvestment != nil {
		query = query.Where("minimum_investment >= ?", *filter.MinInvestment)
	}
	if filter.MaxInvestment != nil {
		query = query.Where("minimum_investment <= ?", *filter.MaxInvestment)
	}

	// Range filters for ROI
	if filter.MinROI != nil {
		query = query.Where("roi_percent >= ?", *filter.MinROI)
	}
	if filter.MaxROI != nil {
		query = query.Where("roi_percent <= ?", *filter.MaxROI)
	}

	// Get total count before pagination
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply sorting
	orderClause := r.buildOrderClause(filter.SortBy, filter.SortOrder)
	query = query.Order(orderClause)

	// Apply pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100 // Maximum page size
	}

	offset := (filter.Page - 1) * filter.PageSize
	query = query.Limit(filter.PageSize).Offset(offset)

	// Execute query
	var properties []*models.Property
	if err := query.Find(&properties).Error; err != nil {
		return nil, 0, err
	}

	return properties, total, nil
}

// buildOrderClause constructs the ORDER BY clause based on sort parameters.
func (r *propertyRepository) buildOrderClause(sortBy, sortOrder string) string {
	// Whitelist of allowed sort columns
	allowedSortColumns := map[string]bool{
		"created_at":    true,
		"updated_at":    true,
		"launch_date":   true,
		"title":         true,
		"target_amount": true,
		"raised_amount": true,
		"roi_percent":   true,
		"city":          true,
		"state":         true,
	}

	// Default sort
	if sortBy == "" || !allowedSortColumns[sortBy] {
		sortBy = "created_at"
	}

	// Validate sort order
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	return fmt.Sprintf("%s %s", sortBy, sortOrder)
}

// ═══════════════════════════════════════════════════════════════════════════
// STATUS MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// UpdateStatus changes the property status with validation.
func (r *propertyRepository) UpdateStatus(ctx context.Context, id uuid.UUID, expectedFrom, newTo string, tx *gorm.DB) error {
	db := r.getDB(tx)

	result := db.WithContext(ctx).
		Model(&models.Property{}).
		Where("id = ? AND status = ?", id, expectedFrom).
		Update("status", newTo)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("property not found or status is not '%s'", expectedFrom)
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// FUNDING OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

// IncrementFunding atomically updates property funding statistics.
func (r *propertyRepository) IncrementFunding(ctx context.Context, id uuid.UUID, amount int64, units int64, investorDelta int, tx *gorm.DB) error {
	db := r.getDB(tx)

	result := db.WithContext(ctx).
		Model(&models.Property{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"raised_amount":  gorm.Expr("raised_amount + ?", amount),
			"units_sold":     gorm.Expr("units_sold + ?", units),
			"investor_count": gorm.Expr("investor_count + ?", investorDelta),
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("property not found: %s", id)
	}

	return nil
}

// FindByIDForUpdate retrieves a property with row-level lock.
func (r *propertyRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID, tx *gorm.DB) (*models.Property, error) {
	if tx == nil {
		return nil, fmt.Errorf("FindByIDForUpdate must be called within a transaction")
	}

	var property models.Property
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		First(&property).Error

	if err != nil {
		return nil, err
	}

	return &property, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════

// getDB returns the appropriate database handle (transaction or base).
// If tx is provided, use it; otherwise use the base db.
func (r *propertyRepository) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.GetDB()
}

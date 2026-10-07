package repositories

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// PropertyDocumentRepository defines the interface for property document data access operations.
//
// This repository handles CRUD operations for property legal and informational documents
// and provides specialized methods for managing public vs private documents.
//
// KEY RESPONSIBILITIES:
//   - Managing property documents (certificates, agreements, reports, etc.)
//   - Controlling document visibility (public vs private)
//   - Maintaining display order
//   - Supporting cascading operations with property deletion
type PropertyDocumentRepository interface {
	// ═══════════════════════════════════════════════════════════════════════════
	// CRUD OPERATIONS
	// ═══════════════════════════════════════════════════════════════════════════

	// Create inserts a new property document into the database.
	// MUST be called within a transaction when part of a larger operation.
	Create(ctx context.Context, document *models.PropertyDocument, tx *gorm.DB) error

	// FindByPropertyID retrieves all documents for a property, ordered by display_order.
	// If publicOnly is true, only returns documents where is_public = true.
	// If publicOnly is false, returns all documents (admin view).
	FindByPropertyID(ctx context.Context, propertyID uuid.UUID, publicOnly bool) ([]models.PropertyDocument, error)

	// FindByID retrieves a specific document by its UUID.
	// Returns gorm.ErrRecordNotFound if not found.
	FindByID(ctx context.Context, id uuid.UUID) (*models.PropertyDocument, error)

	// Update saves changes to an existing document.
	// Used for updating metadata, visibility, or display order.
	Update(ctx context.Context, document *models.PropertyDocument, tx *gorm.DB) error

	// Delete removes a document from the database.
	// MUST be called within a transaction when also handling Cloudinary cleanup.
	Delete(ctx context.Context, id uuid.UUID, tx *gorm.DB) error

	// ═══════════════════════════════════════════════════════════════════════════
	// VISIBILITY MANAGEMENT
	// ═══════════════════════════════════════════════════════════════════════════

	// ToggleVisibility changes the is_public flag for a document.
	// Used by admins to show/hide documents from public view.
	ToggleVisibility(ctx context.Context, id uuid.UUID, isPublic bool, tx *gorm.DB) error

	// CountPublicDocuments returns the number of public documents for a property.
	// Useful for displaying "X documents available" on property listings.
	CountPublicDocuments(ctx context.Context, propertyID uuid.UUID) (int64, error)

	// ═══════════════════════════════════════════════════════════════════════════
	// DISPLAY ORDER MANAGEMENT
	// ═══════════════════════════════════════════════════════════════════════════

	// UpdateDisplayOrder updates the display_order field for a document.
	// Used for manual reordering of documents in the admin panel.
	UpdateDisplayOrder(ctx context.Context, documentID uuid.UUID, displayOrder int, tx *gorm.DB) error

	// GetNextDisplayOrder returns the next available display order for a property.
	// Used when adding a new document without specifying order.
	// Returns max(display_order) + 1, or 0 if no documents exist.
	GetNextDisplayOrder(ctx context.Context, propertyID uuid.UUID) (int, error)

	// ═══════════════════════════════════════════════════════════════════════════
	// TYPE-BASED QUERIES
	// ═══════════════════════════════════════════════════════════════════════════

	// FindByType retrieves documents of a specific type for a property.
	// Useful for fetching specific documents like "certificate_of_ownership" or "valuation_report".
	// If publicOnly is true, only returns public documents.
	FindByType(ctx context.Context, propertyID uuid.UUID, documentType string, publicOnly bool) ([]models.PropertyDocument, error)
}

// propertyDocumentRepository is the concrete implementation of PropertyDocumentRepository.
type propertyDocumentRepository struct {
	*BaseRepository
}

// NewPropertyDocumentRepository creates a new property document repository instance.
func NewPropertyDocumentRepository(db *gorm.DB) PropertyDocumentRepository {
	return &propertyDocumentRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// CRUD OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

// Create inserts a new property document into the database.
func (r *propertyDocumentRepository) Create(ctx context.Context, document *models.PropertyDocument, tx *gorm.DB) error {
	db := r.getDB(tx)
	return db.WithContext(ctx).Create(document).Error
}

// FindByPropertyID retrieves all documents for a property, ordered by display_order.
func (r *propertyDocumentRepository) FindByPropertyID(ctx context.Context, propertyID uuid.UUID, publicOnly bool) ([]models.PropertyDocument, error) {
	query := r.WithContext(ctx).
		Where("property_id = ?", propertyID)

	if publicOnly {
		query = query.Where("is_public = ?", true)
	}

	var documents []models.PropertyDocument
	err := query.
		Order("display_order ASC, created_at ASC").
		Find(&documents).Error

	if err != nil {
		return nil, err
	}

	return documents, nil
}

// FindByID retrieves a specific document by its UUID.
func (r *propertyDocumentRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.PropertyDocument, error) {
	var document models.PropertyDocument
	err := r.WithContext(ctx).
		Where("id = ?", id).
		First(&document).Error

	if err != nil {
		return nil, err
	}

	return &document, nil
}

// Update saves changes to an existing document.
func (r *propertyDocumentRepository) Update(ctx context.Context, document *models.PropertyDocument, tx *gorm.DB) error {
	db := r.getDB(tx)
	return db.WithContext(ctx).Save(document).Error
}

// Delete removes a document from the database.
func (r *propertyDocumentRepository) Delete(ctx context.Context, id uuid.UUID, tx *gorm.DB) error {
	db := r.getDB(tx)

	result := db.WithContext(ctx).Delete(&models.PropertyDocument{}, id)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// VISIBILITY MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// ToggleVisibility changes the is_public flag for a document.
func (r *propertyDocumentRepository) ToggleVisibility(ctx context.Context, id uuid.UUID, isPublic bool, tx *gorm.DB) error {
	db := r.getDB(tx)

	result := db.WithContext(ctx).
		Model(&models.PropertyDocument{}).
		Where("id = ?", id).
		Update("is_public", isPublic)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// CountPublicDocuments returns the number of public documents for a property.
func (r *propertyDocumentRepository) CountPublicDocuments(ctx context.Context, propertyID uuid.UUID) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&models.PropertyDocument{}).
		Where("property_id = ? AND is_public = ?", propertyID, true).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return count, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// DISPLAY ORDER MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// UpdateDisplayOrder updates the display_order field for a document.
func (r *propertyDocumentRepository) UpdateDisplayOrder(ctx context.Context, documentID uuid.UUID, displayOrder int, tx *gorm.DB) error {
	db := r.getDB(tx)

	result := db.WithContext(ctx).
		Model(&models.PropertyDocument{}).
		Where("id = ?", documentID).
		Update("display_order", displayOrder)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// GetNextDisplayOrder returns the next available display order for a property.
func (r *propertyDocumentRepository) GetNextDisplayOrder(ctx context.Context, propertyID uuid.UUID) (int, error) {
	var maxOrder sql.NullInt64

	err := r.WithContext(ctx).
		Model(&models.PropertyDocument{}).
		Where("property_id = ?", propertyID).
		Select("MAX(display_order)").
		Scan(&maxOrder).Error

	if err != nil {
		return 0, err
	}

	if !maxOrder.Valid {
		return 0, nil // No documents exist yet
	}

	return int(maxOrder.Int64) + 1, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TYPE-BASED QUERIES
// ═══════════════════════════════════════════════════════════════════════════

// FindByType retrieves documents of a specific type for a property.
func (r *propertyDocumentRepository) FindByType(ctx context.Context, propertyID uuid.UUID, documentType string, publicOnly bool) ([]models.PropertyDocument, error) {
	query := r.WithContext(ctx).
		Where("property_id = ? AND document_type = ?", propertyID, documentType)

	if publicOnly {
		query = query.Where("is_public = ?", true)
	}

	var documents []models.PropertyDocument
	err := query.
		Order("display_order ASC, created_at ASC").
		Find(&documents).Error

	if err != nil {
		return nil, err
	}

	return documents, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════

// getDB returns the appropriate database handle (transaction or base).
func (r *propertyDocumentRepository) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.GetDB()
}

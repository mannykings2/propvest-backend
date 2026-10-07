package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// PropertyImageRepository defines the interface for property image data access operations.
//
// This repository handles CRUD operations for property gallery images and provides
// specialized methods for managing cover images and display ordering.
//
// KEY RESPONSIBILITIES:
//   - Managing property gallery images
//   - Enforcing one cover image per property
//   - Maintaining display order
//   - Supporting cascading operations with property deletion
type PropertyImageRepository interface {
	// ═══════════════════════════════════════════════════════════════════════════
	// CRUD OPERATIONS
	// ═══════════════════════════════════════════════════════════════════════════

	// Create inserts a new property image into the database.
	// MUST be called within a transaction when also updating property.cover_image_url.
	Create(ctx context.Context, image *models.PropertyImage, tx *gorm.DB) error

	// FindByPropertyID retrieves all images for a property, ordered by display_order.
	FindByPropertyID(ctx context.Context, propertyID uuid.UUID) ([]models.PropertyImage, error)

	// FindByID retrieves a specific image by its UUID.
	// Returns gorm.ErrRecordNotFound if not found.
	FindByID(ctx context.Context, id uuid.UUID) (*models.PropertyImage, error)

	// Delete removes an image from the database.
	// MUST be called within a transaction when also handling Cloudinary cleanup
	// and updating property.cover_image_url.
	Delete(ctx context.Context, id uuid.UUID, tx *gorm.DB) error

	// ═══════════════════════════════════════════════════════════════════════════
	// COVER IMAGE MANAGEMENT
	// ═══════════════════════════════════════════════════════════════════════════

	// SetCover marks an image as the property's cover image.
	// Automatically unsets any existing cover image for the same property.
	// MUST be called within a transaction to ensure atomicity.
	//
	// Steps performed:
	//   1. Unset current cover (UPDATE ... SET is_cover = false WHERE property_id = X AND is_cover = true)
	//   2. Set new cover (UPDATE ... SET is_cover = true WHERE id = imageID)
	//
	// The unique partial index on (property_id) WHERE is_cover = true
	// provides a second line of defense against multiple covers.
	SetCover(ctx context.Context, propertyID, imageID uuid.UUID, tx *gorm.DB) error

	// UnsetCover removes the cover designation from all images of a property.
	// Used when deleting the cover image or resetting property images.
	// MUST be called within a transaction.
	UnsetCover(ctx context.Context, propertyID uuid.UUID, tx *gorm.DB) error

	// GetCover retrieves the cover image for a property.
	// Returns nil if no cover image is set.
	GetCover(ctx context.Context, propertyID uuid.UUID) (*models.PropertyImage, error)

	// ═══════════════════════════════════════════════════════════════════════════
	// DISPLAY ORDER MANAGEMENT
	// ═══════════════════════════════════════════════════════════════════════════

	// UpdateDisplayOrder updates the display_order field for an image.
	// Used for manual reordering of gallery images.
	UpdateDisplayOrder(ctx context.Context, imageID uuid.UUID, displayOrder int, tx *gorm.DB) error

	// GetNextDisplayOrder returns the next available display order for a property.
	// Used when adding a new image without specifying order.
	// Returns max(display_order) + 1, or 0 if no images exist.
	GetNextDisplayOrder(ctx context.Context, propertyID uuid.UUID) (int, error)
}

// propertyImageRepository is the concrete implementation of PropertyImageRepository.
type propertyImageRepository struct {
	*BaseRepository
}

// NewPropertyImageRepository creates a new property image repository instance.
func NewPropertyImageRepository(db *gorm.DB) PropertyImageRepository {
	return &propertyImageRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// CRUD OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

// Create inserts a new property image into the database.
func (r *propertyImageRepository) Create(ctx context.Context, image *models.PropertyImage, tx *gorm.DB) error {
	db := r.getDB(tx)
	return db.WithContext(ctx).Create(image).Error
}

// FindByPropertyID retrieves all images for a property, ordered by display_order.
func (r *propertyImageRepository) FindByPropertyID(ctx context.Context, propertyID uuid.UUID) ([]models.PropertyImage, error) {
	var images []models.PropertyImage
	err := r.WithContext(ctx).
		Where("property_id = ?", propertyID).
		Order("display_order ASC, created_at ASC").
		Find(&images).Error

	if err != nil {
		return nil, err
	}

	return images, nil
}

// FindByID retrieves a specific image by its UUID.
func (r *propertyImageRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.PropertyImage, error) {
	var image models.PropertyImage
	err := r.WithContext(ctx).
		Where("id = ?", id).
		First(&image).Error

	if err != nil {
		return nil, err
	}

	return &image, nil
}

// Delete removes an image from the database.
func (r *propertyImageRepository) Delete(ctx context.Context, id uuid.UUID, tx *gorm.DB) error {
	db := r.getDB(tx)

	result := db.WithContext(ctx).Delete(&models.PropertyImage{}, id)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// COVER IMAGE MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// SetCover marks an image as the property's cover image.
func (r *propertyImageRepository) SetCover(ctx context.Context, propertyID, imageID uuid.UUID, tx *gorm.DB) error {
	db := r.getDB(tx)

	// Step 1: Unset any existing cover for this property
	if err := db.WithContext(ctx).
		Model(&models.PropertyImage{}).
		Where("property_id = ? AND is_cover = ?", propertyID, true).
		Update("is_cover", false).Error; err != nil {
		return fmt.Errorf("failed to unset existing cover: %w", err)
	}

	// Step 2: Set the new cover
	result := db.WithContext(ctx).
		Model(&models.PropertyImage{}).
		Where("id = ? AND property_id = ?", imageID, propertyID).
		Update("is_cover", true)

	if result.Error != nil {
		return fmt.Errorf("failed to set new cover: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("image not found or doesn't belong to property")
	}

	return nil
}

// UnsetCover removes the cover designation from all images of a property.
func (r *propertyImageRepository) UnsetCover(ctx context.Context, propertyID uuid.UUID, tx *gorm.DB) error {
	db := r.getDB(tx)

	return db.WithContext(ctx).
		Model(&models.PropertyImage{}).
		Where("property_id = ? AND is_cover = ?", propertyID, true).
		Update("is_cover", false).Error
}

// GetCover retrieves the cover image for a property.
func (r *propertyImageRepository) GetCover(ctx context.Context, propertyID uuid.UUID) (*models.PropertyImage, error) {
	var image models.PropertyImage
	err := r.WithContext(ctx).
		Where("property_id = ? AND is_cover = ?", propertyID, true).
		First(&image).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // No cover image set
		}
		return nil, err
	}

	return &image, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// DISPLAY ORDER MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// UpdateDisplayOrder updates the display_order field for an image.
func (r *propertyImageRepository) UpdateDisplayOrder(ctx context.Context, imageID uuid.UUID, displayOrder int, tx *gorm.DB) error {
	db := r.getDB(tx)

	result := db.WithContext(ctx).
		Model(&models.PropertyImage{}).
		Where("id = ?", imageID).
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
func (r *propertyImageRepository) GetNextDisplayOrder(ctx context.Context, propertyID uuid.UUID) (int, error) {
	var maxOrder sql.NullInt64

	err := r.WithContext(ctx).
		Model(&models.PropertyImage{}).
		Where("property_id = ?", propertyID).
		Select("MAX(display_order)").
		Scan(&maxOrder).Error

	if err != nil {
		return 0, err
	}

	if !maxOrder.Valid {
		return 0, nil // No images exist yet
	}

	return int(maxOrder.Int64) + 1, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════

// getDB returns the appropriate database handle (transaction or base).
func (r *propertyImageRepository) getDB(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.GetDB()
}

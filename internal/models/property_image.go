package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PropertyImage represents a Cloudinary-backed image in a property's gallery.
//
// CLOUDINARY INTEGRATION:
//   - URL: Full HTTPS URL to the image (https://res.cloudinary.com/...)
//   - PublicID: Unique identifier for the image in Cloudinary
//   - PublicID is used for image deletion and transformations
//
// DISPLAY ORDER:
//   - Images are ordered by DisplayOrder field (ascending)
//   - Lower numbers appear first in the gallery
//   - Example: 0=exterior, 1=living room, 2=bedroom, 3=amenities
//
// COVER IMAGE:
//   - Each property can have exactly ONE cover image (enforced by unique partial index)
//   - Cover image is used as the property thumbnail in listings
//   - Setting a new cover image automatically unsets the previous one (handled by service layer)
//
// CASCADE DELETION:
//   - When a property is deleted, all its images are automatically deleted (ON DELETE CASCADE)
//   - Cloudinary cleanup must be handled separately by the service layer
type PropertyImage struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// ═══════════════════════════════════════════════════════════════════════════
	// FOREIGN KEY
	// ═══════════════════════════════════════════════════════════════════════════

	// PropertyID links this image to its parent property
	// CASCADE deletion: when property is deleted, image is deleted
	PropertyID uuid.UUID `gorm:"type:uuid;not null;index:idx_property_images_property" json:"property_id"`

	// ═══════════════════════════════════════════════════════════════════════════
	// CLOUDINARY DETAILS
	// ═══════════════════════════════════════════════════════════════════════════

	// URL is the full Cloudinary URL to the image
	// Example: https://res.cloudinary.com/propvest/image/upload/v1234567890/properties/abc-123/image-1.jpg
	// This is what gets stored in the database and returned to the frontend
	URL string `gorm:"type:text;not null" json:"url"`

	// PublicID is Cloudinary's unique identifier for this image
	// Example: properties/abc-123/image-1
	// Used for:
	//   - Deleting the image from Cloudinary
	//   - Generating transformed versions (thumbnails, etc.)
	// Must be unique across all property images (enforced by unique index)
	PublicID string `gorm:"size:255;uniqueIndex:idx_property_images_public_id;not null" json:"public_id"`

	// ═══════════════════════════════════════════════════════════════════════════
	// IMAGE METADATA
	// ═══════════════════════════════════════════════════════════════════════════

	// AltText provides accessibility description for screen readers
	// Example: "Exterior view of luxury apartment complex"
	// Optional but recommended for accessibility compliance
	AltText *string `gorm:"size:255" json:"alt_text,omitempty"`

	// DisplayOrder determines the sequence in the gallery
	// Lower numbers appear first
	// Default is 0, but service layer should assign sequential numbers
	DisplayOrder int `gorm:"default:0;not null;index:idx_property_images_property" json:"display_order"`

	// IsCover indicates if this is the property's main thumbnail image
	// Only ONE image per property can be marked as cover
	// Enforced by unique partial index: idx_property_images_cover
	// Setting a new cover requires:
	//   1. Unset previous cover (UPDATE old SET is_cover = false)
	//   2. Set new cover (UPDATE new SET is_cover = true)
	//   3. Update property.cover_image_url
	IsCover bool `gorm:"default:false;not null" json:"is_cover"`

	// ═══════════════════════════════════════════════════════════════════════════
	// IMAGE DIMENSIONS (optional, set after upload)
	// ═══════════════════════════════════════════════════════════════════════════

	// Width and Height store the original image dimensions
	// Both must be present or both must be null (enforced by DB constraint)
	// Used for:
	//   - Aspect ratio calculations
	//   - Responsive image sizing
	//   - Layout optimization
	Width  *int `json:"width,omitempty"`
	Height *int `json:"height,omitempty"`

	// ═══════════════════════════════════════════════════════════════════════════
	// TIMESTAMPS
	// ═══════════════════════════════════════════════════════════════════════════

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName specifies the table name for GORM
func (PropertyImage) TableName() string {
	return "property_images"
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════

// HasDimensions checks if width and height are available
func (pi *PropertyImage) HasDimensions() bool {
	return pi.Width != nil && pi.Height != nil
}

// AspectRatio calculates the width/height ratio
// Returns 0 if dimensions are not available
func (pi *PropertyImage) AspectRatio() float64 {
	if !pi.HasDimensions() {
		return 0
	}
	if *pi.Height == 0 {
		return 0
	}
	return float64(*pi.Width) / float64(*pi.Height)
}

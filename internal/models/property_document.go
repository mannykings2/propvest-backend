package models

import (
	"time"

	"github.com/google/uuid"
)

// PropertyDocument stores a file attached to a property — an image, a legal
// document, or the SPV agreement (docs 5.2 "Property media", 2.2 §17). The file
// itself lives in external storage (Cloudinary); we keep only the URL + metadata
// in the database, per the "media is stored externally" principle.
type PropertyDocument struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PropertyID uuid.UUID `gorm:"type:uuid;not null;index" json:"property_id"`

	// Type: image|legal|agreement|other
	Type string `gorm:"not null" json:"type"`
	URL  string `gorm:"not null" json:"url"`
	Name string `json:"name"`

	UploadedBy uuid.UUID `gorm:"type:uuid" json:"uploaded_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// PropertyDocument type constants.
const (
	PropertyDocImage     = "image"
	PropertyDocLegal     = "legal"
	PropertyDocAgreement = "agreement"
	PropertyDocOther     = "other"
)

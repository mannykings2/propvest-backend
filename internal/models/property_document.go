package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PropertyDocument represents legal, valuation, certification, and supporting documents
// associated with a property.
//
// DOCUMENT TYPES:
//   title_document    - Property title/deed
//   survey_plan       - Land survey documents
//   building_approval - Building permits and approvals
//   certificate       - Certificates of occupancy, etc.
//   legal_document    - Contracts, agreements, etc.
//   valuation         - Property valuation reports
//   other             - Miscellaneous documents
//
// ACCESS CONTROL:
//   - IsPublic = true: Visible to all users (public API)
//   - IsPublic = false: Visible only to admins
//   - Use with caution for sensitive legal/financial documents
//   - Default is private (false) for safety
//
// CLOUDINARY INTEGRATION:
//   - Documents are stored in Cloudinary (PDFs, images, etc.)
//   - URL: Full HTTPS URL to the document
//   - PublicID: Unique identifier for deletion
//
// SECURITY CONSIDERATIONS:
//   - Validate file types before upload (service layer)
//   - Limit file size (e.g., 20MB max)
//   - Never expose private documents in public API responses
//   - Consider signed URLs for temporary access (future enhancement)
//
// CASCADE DELETION:
//   - When a property is deleted, all its documents are automatically deleted
//   - Cloudinary cleanup must be handled separately by the service layer
type PropertyDocument struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// ═══════════════════════════════════════════════════════════════════════════
	// FOREIGN KEY
	// ═══════════════════════════════════════════════════════════════════════════

	// PropertyID links this document to its parent property
	// CASCADE deletion: when property is deleted, document is deleted
	PropertyID uuid.UUID `gorm:"type:uuid;not null;index:idx_property_documents_property" json:"property_id"`

	// ═══════════════════════════════════════════════════════════════════════════
	// DOCUMENT METADATA
	// ═══════════════════════════════════════════════════════════════════════════

	// Name is the document filename or title
	// Example: "Property Title Deed.pdf", "Survey Plan 2024.pdf"
	Name string `gorm:"size:255;not null" json:"name"`

	// DocumentType categorizes the document
	// Allowed values: title_document, survey_plan, building_approval,
	//                 certificate, legal_document, valuation, other
	// Enforced by database CHECK constraint
	DocumentType string `gorm:"size:50;not null;index:idx_property_documents_type" json:"document_type"`

	// ═══════════════════════════════════════════════════════════════════════════
	// CLOUDINARY DETAILS
	// ═══════════════════════════════════════════════════════════════════════════

	// URL is the full Cloudinary URL to the document
	// Example: https://res.cloudinary.com/propvest/raw/upload/v1234567890/properties/abc-123/title-deed.pdf
	// For PDFs and other raw files, Cloudinary uses /raw/upload/ instead of /image/upload/
	URL string `gorm:"type:text;not null" json:"url"`

	// PublicID is Cloudinary's unique identifier for this document
	// Example: properties/abc-123/documents/title-deed
	// Used for:
	//   - Deleting the document from Cloudinary
	//   - Generating download URLs
	// Must be unique across all property documents (enforced by unique index)
	PublicID string `gorm:"size:255;uniqueIndex:idx_property_documents_public_id;not null" json:"public_id"`

	// ═══════════════════════════════════════════════════════════════════════════
	// FILE INFORMATION
	// ═══════════════════════════════════════════════════════════════════════════

	// MIMEType indicates the file format
	// Examples: application/pdf, image/jpeg, image/png
	// Used for:
	//   - Validation during upload
	//   - Proper file handling/download
	//   - Browser display behavior
	MIMEType string `gorm:"size:100;not null" json:"mime_type"`

	// FileSize is the file size in bytes
	// Used for:
	//   - Storage tracking
	//   - Upload validation (reject oversized files)
	//   - Display to users ("2.5 MB")
	// Must be greater than zero (enforced by DB constraint)
	FileSize int64 `gorm:"not null" json:"file_size"`

	// ═══════════════════════════════════════════════════════════════════════════
	// ACCESS CONTROL
	// ═══════════════════════════════════════════════════════════════════════════

	// IsPublic determines document visibility
	// true:  Visible to all users (included in public property API responses)
	// false: Visible only to admins (excluded from public API)
	//
	// IMPORTANT: Default is false (private) for safety
	// Admins must explicitly mark documents as public
	//
	// Use cases for public documents:
	//   - Marketing materials
	//   - Public certifications
	//   - General property information
	//
	// Keep private:
	//   - Legal contracts
	//   - Financial statements
	//   - Sensitive agreements
	IsPublic bool `gorm:"default:false;not null;index:idx_property_documents_public" json:"is_public"`

	// ═══════════════════════════════════════════════════════════════════════════
	// AUDIT
	// ═══════════════════════════════════════════════════════════════════════════

	// UploadedBy tracks which admin uploaded this document
	// Useful for audit trails and accountability
	UploadedBy uuid.UUID `gorm:"type:uuid;not null" json:"uploaded_by"`

	// ═══════════════════════════════════════════════════════════════════════════
	// TIMESTAMPS
	// ═══════════════════════════════════════════════════════════════════════════

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName specifies the table name for GORM
func (PropertyDocument) TableName() string {
	return "property_documents"
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════

// IsPDF checks if the document is a PDF file
func (pd *PropertyDocument) IsPDF() bool {
	return pd.MIMEType == "application/pdf"
}

// IsImage checks if the document is an image
func (pd *PropertyDocument) IsImage() bool {
	return pd.MIMEType == "image/jpeg" ||
		pd.MIMEType == "image/jpg" ||
		pd.MIMEType == "image/png" ||
		pd.MIMEType == "image/gif" ||
		pd.MIMEType == "image/webp"
}

// FileSizeInMB returns the file size in megabytes
// Useful for display purposes
func (pd *PropertyDocument) FileSizeInMB() float64 {
	return float64(pd.FileSize) / (1024 * 1024)
}

// FileSizeFormatted returns a human-readable file size
// Examples: "2.5 MB", "850 KB", "125 B"
func (pd *PropertyDocument) FileSizeFormatted() string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	size := float64(pd.FileSize)

	switch {
	case pd.FileSize >= GB:
		return fmt.Sprintf("%.2f GB", size/GB)
	case pd.FileSize >= MB:
		return fmt.Sprintf("%.2f MB", size/MB)
	case pd.FileSize >= KB:
		return fmt.Sprintf("%.2f KB", size/KB)
	default:
		return fmt.Sprintf("%d B", pd.FileSize)
	}
}

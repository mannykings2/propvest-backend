package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
)

// ═══════════════════════════════════════════════════════════════════════════
// REQUEST DTOs (Input from clients)
// ═══════════════════════════════════════════════════════════════════════════

// CreatePropertyRequest is used for POST /api/v1/admin/properties
// Creates a new property in draft status. Admin must publish separately.
type CreatePropertyRequest struct {
	// Basic Information
	Title       string `json:"title" binding:"required,min=5,max=200"`
	Description string `json:"description" binding:"required,min=50,max=5000"`
	PropertyType string `json:"property_type" binding:"required,oneof=residential commercial land"`
	
	// Location
	Address    string  `json:"address" binding:"required,min=10,max=500"`
	City       string  `json:"city" binding:"required,min=2,max=100"`
	State      string  `json:"state" binding:"required,min=2,max=100"`
	Country    string  `json:"country" binding:"required,min=2,max=100"`
	Latitude   *float64 `json:"latitude,omitempty" binding:"omitempty,min=-90,max=90"`
	Longitude  *float64 `json:"longitude,omitempty" binding:"omitempty,min=-180,max=180"`
	
	// Financial Details (all amounts in kobo)
	TargetAmount      int64   `json:"target_amount" binding:"required,gt=0"`
	MinimumInvestment int64   `json:"minimum_investment" binding:"required,gt=0"`
	UnitPrice         int64   `json:"unit_price" binding:"required,gt=0"`
	TotalUnits        int64   `json:"total_units" binding:"required,gt=0"`
	ROIPercent        float64 `json:"roi_percent" binding:"required,gt=0,lte=100"`
	
	// Investment Period
	InvestmentDuration int    `json:"investment_duration" binding:"required,gt=0,lte=120"` // months
	LaunchDate         string `json:"launch_date" binding:"required"`                       // YYYY-MM-DD
	MaturityDate       string `json:"maturity_date" binding:"required"`                     // YYYY-MM-DD
	
	// Additional Details
	Features  []string `json:"features" binding:"omitempty,dive,min=2,max=200"`
	Amenities []string `json:"amenities" binding:"omitempty,dive,min=2,max=200"`
	
	// Flags
	Featured bool `json:"featured" binding:"omitempty"`
	Verified bool `json:"verified" binding:"omitempty"`
	Trending bool `json:"trending" binding:"omitempty"`
}

// UpdatePropertyRequest is used for PATCH /api/v1/admin/properties/:id
// Allows partial updates. Only provided fields are updated.
// Some fields (like financial details) may be restricted based on property status.
type UpdatePropertyRequest struct {
	// Basic Information
	Title       *string `json:"title,omitempty" binding:"omitempty,min=5,max=200"`
	Description *string `json:"description,omitempty" binding:"omitempty,min=50,max=5000"`
	PropertyType *string `json:"property_type,omitempty" binding:"omitempty,oneof=residential commercial land"`
	
	// Location
	Address   *string  `json:"address,omitempty" binding:"omitempty,min=10,max=500"`
	City      *string  `json:"city,omitempty" binding:"omitempty,min=2,max=100"`
	State     *string  `json:"state,omitempty" binding:"omitempty,min=2,max=100"`
	Country   *string  `json:"country,omitempty" binding:"omitempty,min=2,max=100"`
	Latitude  *float64 `json:"latitude,omitempty" binding:"omitempty,min=-90,max=90"`
	Longitude *float64 `json:"longitude,omitempty" binding:"omitempty,min=-180,max=180"`
	
	// Financial Details (only allowed if no investments exist)
	TargetAmount      *int64   `json:"target_amount,omitempty" binding:"omitempty,gt=0"`
	MinimumInvestment *int64   `json:"minimum_investment,omitempty" binding:"omitempty,gt=0"`
	UnitPrice         *int64   `json:"unit_price,omitempty" binding:"omitempty,gt=0"`
	TotalUnits        *int64   `json:"total_units,omitempty" binding:"omitempty,gt=0"`
	ROIPercent        *float64 `json:"roi_percent,omitempty" binding:"omitempty,gt=0,lte=100"`
	
	// Investment Period
	InvestmentDuration *int    `json:"investment_duration,omitempty" binding:"omitempty,gt=0,lte=120"`
	LaunchDate         *string `json:"launch_date,omitempty"`  // YYYY-MM-DD
	MaturityDate       *string `json:"maturity_date,omitempty"` // YYYY-MM-DD
	
	// Additional Details
	Features  []string `json:"features,omitempty" binding:"omitempty,dive,min=2,max=200"`
	Amenities []string `json:"amenities,omitempty" binding:"omitempty,dive,min=2,max=200"`
	
	// Flags (admin can toggle anytime)
	Featured *bool `json:"featured,omitempty"`
	Verified *bool `json:"verified,omitempty"`
	Trending *bool `json:"trending,omitempty"`
}

// PropertyFilterRequest is used for GET /api/v1/properties (query params)
// Allows filtering, searching, and pagination of property listings.
type PropertyFilterRequest struct {
	// Search (searches title, description, city, state)
	Search string `form:"search" binding:"omitempty,max=200"`
	
	// Exact Match Filters
	PropertyType string `form:"property_type" binding:"omitempty,oneof=residential commercial land"`
	City         string `form:"city" binding:"omitempty,max=100"`
	State        string `form:"state" binding:"omitempty,max=100"`
	Status       string `form:"status" binding:"omitempty,oneof=draft active funded completed"` // admin only
	
	// Boolean Filters
	Featured *bool `form:"featured" binding:"omitempty"`
	Verified *bool `form:"verified" binding:"omitempty"`
	Trending *bool `form:"trending" binding:"omitempty"`
	
	// Range Filters
	MinInvestment *int64   `form:"min_investment" binding:"omitempty,gt=0"`
	MaxInvestment *int64   `form:"max_investment" binding:"omitempty,gt=0"`
	MinROI        *float64 `form:"min_roi" binding:"omitempty,gte=0"`
	MaxROI        *float64 `form:"max_roi" binding:"omitempty,gte=0"`
	
	// Pagination
	Page     int `form:"page" binding:"omitempty,min=1" default:"1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100" default:"20"`
	
	// Sorting
	SortBy    string `form:"sort_by" binding:"omitempty,oneof=created_at updated_at launch_date title target_amount raised_amount roi_percent city state"`
	SortOrder string `form:"sort_order" binding:"omitempty,oneof=asc desc" default:"desc"`
}

// UploadImageRequest is used for POST /api/v1/admin/properties/:id/images
// Metadata for image uploads. The actual file comes from multipart form.
type UploadImageRequest struct {
	Caption      *string `form:"caption" binding:"omitempty,max=500"`
	DisplayOrder *int    `form:"display_order" binding:"omitempty,min=0"`
	IsCover      bool    `form:"is_cover"` // Set this image as cover
}

// SetCoverImageRequest is used for PATCH /api/v1/admin/properties/:id/images/:imageId/cover
type SetCoverImageRequest struct {
	IsCover bool `json:"is_cover" binding:"required"`
}

// UploadDocumentRequest is used for POST /api/v1/admin/properties/:id/documents
// Metadata for document uploads. The actual file comes from multipart form.
type UploadDocumentRequest struct {
	DocumentType string  `form:"document_type" binding:"required,oneof=certificate_of_ownership title_deed survey_plan valuation_report building_approval business_plan financial_projection insurance_policy other"`
	Title        string  `form:"title" binding:"required,min=5,max=200"`
	Description  *string `form:"description" binding:"omitempty,max=1000"`
	IsPublic     bool    `form:"is_public"` // If true, visible to all investors
	DisplayOrder *int    `form:"display_order" binding:"omitempty,min=0"`
}

// UpdateDocumentVisibilityRequest toggles document public/private status
type UpdateDocumentVisibilityRequest struct {
	IsPublic bool `json:"is_public" binding:"required"`
}

// ═══════════════════════════════════════════════════════════════════════════
// RESPONSE DTOs (Output to clients)
// ═══════════════════════════════════════════════════════════════════════════

// PropertyResponse is the public representation of a property.
// Used for GET /api/v1/properties/:id (public endpoint).
// Excludes internal fields and admin-only data.
type PropertyResponse struct {
	ID               uuid.UUID                  `json:"id"`
	Slug             string                     `json:"slug"`
	Title            string                     `json:"title"`
	Description      string                     `json:"description"`
	PropertyType     string                     `json:"property_type"`
	Status           string                     `json:"status"`
	CoverImageURL    *string                    `json:"cover_image_url,omitempty"`
	Location         PropertyLocationResponse   `json:"location"`
	Financial        PropertyFinancialResponse  `json:"financial"`
	InvestmentPeriod PropertyInvestmentResponse `json:"investment_period"`
	Features         []string                   `json:"features,omitempty"`
	Amenities        []string                   `json:"amenities,omitempty"`
	Images           []PropertyImageResponse    `json:"images,omitempty"`
	Documents        []PropertyDocumentResponse `json:"documents,omitempty"` // Only public documents
	Featured         bool                       `json:"featured"`
	Verified         bool                       `json:"verified"`
	Trending         bool                       `json:"trending"`
	CreatedAt        time.Time                  `json:"created_at"`
	UpdatedAt        time.Time                  `json:"updated_at"`
}

// PropertyAdminResponse is the full representation including admin-only fields.
// Used for GET /api/v1/admin/properties/:id (admin endpoint).
type PropertyAdminResponse struct {
	PropertyResponse                             // Embed public fields
	InvestorCount    int                         `json:"investor_count"`
	StatusHistory    []PropertyStatusHistoryResponse `json:"status_history,omitempty"`
	AllDocuments     []PropertyDocumentResponse  `json:"all_documents,omitempty"` // Includes private documents
}

// PropertyListItemResponse is a lightweight version for listings.
// Used for GET /api/v1/properties (list endpoint).
type PropertyListItemResponse struct {
	ID            uuid.UUID                 `json:"id"`
	Slug          string                    `json:"slug"`
	Title         string                    `json:"title"`
	PropertyType  string                    `json:"property_type"`
	Status        string                    `json:"status"`
	CoverImageURL *string                   `json:"cover_image_url,omitempty"`
	Location      PropertyLocationResponse  `json:"location"`
	Financial     PropertyFinancialResponse `json:"financial"`
	ROIPercent    float64                   `json:"roi_percent"`
	Duration      int                       `json:"duration_months"`
	Featured      bool                      `json:"featured"`
	Verified      bool                      `json:"verified"`
	Trending      bool                      `json:"trending"`
	LaunchDate    time.Time                 `json:"launch_date"`
	CreatedAt     time.Time                 `json:"created_at"`
}

// PropertyListResponse wraps a paginated list of properties.
type PropertyListResponse struct {
	Properties []PropertyListItemResponse `json:"properties"`
	Pagination PaginationMetadata         `json:"pagination"`
}

// PropertyLocationResponse contains location details.
type PropertyLocationResponse struct {
	Address   string   `json:"address"`
	City      string   `json:"city"`
	State     string   `json:"state"`
	Country   string   `json:"country"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

// PropertyFinancialResponse contains financial details and funding progress.
// All amounts in kobo.
type PropertyFinancialResponse struct {
	TargetAmount        int64   `json:"target_amount"`
	RaisedAmount        int64   `json:"raised_amount"`
	RemainingAmount     int64   `json:"remaining_amount"`
	MinimumInvestment   int64   `json:"minimum_investment"`
	UnitPrice           int64   `json:"unit_price"`
	TotalUnits          int64   `json:"total_units"`
	UnitsSold           int64   `json:"units_sold"`
	UnitsRemaining      int64   `json:"units_remaining"`
	FundingPercentage   float64 `json:"funding_percentage"`
	InvestorCount       int     `json:"investor_count"`
	IsFullyFunded       bool    `json:"is_fully_funded"`
	ROIPercent          float64 `json:"roi_percent"`
	ExpectedReturnsKobo int64   `json:"expected_returns_kobo"` // per minimum investment
}

// PropertyInvestmentResponse contains investment timeline details.
type PropertyInvestmentResponse struct {
	Duration     int       `json:"duration_months"`
	LaunchDate   time.Time `json:"launch_date"`
	MaturityDate time.Time `json:"maturity_date"`
}

// PropertyImageResponse is the public representation of a property image.
type PropertyImageResponse struct {
	ID           uuid.UUID `json:"id"`
	ImageURL     string    `json:"image_url"`
	ThumbnailURL *string   `json:"thumbnail_url,omitempty"`
	Caption      *string   `json:"caption,omitempty"`
	Width        *int      `json:"width,omitempty"`
	Height       *int      `json:"height,omitempty"`
	IsCover      bool      `json:"is_cover"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
}

// PropertyDocumentResponse is the public representation of a property document.
type PropertyDocumentResponse struct {
	ID           uuid.UUID `json:"id"`
	DocumentType string    `json:"document_type"`
	Title        string    `json:"title"`
	Description  *string   `json:"description,omitempty"`
	FileURL      string    `json:"file_url"`
	FileName     string    `json:"file_name"`
	FileSize     int64     `json:"file_size"`
	FileSizeMB   float64   `json:"file_size_mb"`
	MimeType     string    `json:"mime_type"`
	IsPublic     bool      `json:"is_public"`
	DisplayOrder int       `json:"display_order"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

// PropertyStatusHistoryResponse tracks status changes.
type PropertyStatusHistoryResponse struct {
	ID          uuid.UUID  `json:"id"`
	OldStatus   *string    `json:"old_status,omitempty"`
	NewStatus   string     `json:"new_status"`
	ChangedBy   *uuid.UUID `json:"changed_by,omitempty"`
	Reason      *string    `json:"reason,omitempty"`
	IsSystem    bool       `json:"is_system"`
	ChangedAt   time.Time  `json:"changed_at"`
}

// PaginationMetadata provides pagination information.
type PaginationMetadata struct {
	CurrentPage  int   `json:"current_page"`
	PageSize     int   `json:"page_size"`
	TotalPages   int   `json:"total_pages"`
	TotalRecords int64 `json:"total_records"`
	HasNext      bool  `json:"has_next"`
	HasPrevious  bool  `json:"has_previous"`
}

// ═══════════════════════════════════════════════════════════════════════════
// OPERATION RESPONSE DTOs
// ═══════════════════════════════════════════════════════════════════════════

// PropertyCreatedResponse is returned after creating a property.
type PropertyCreatedResponse struct {
	ID      uuid.UUID `json:"id"`
	Slug    string    `json:"slug"`
	Status  string    `json:"status"`
	Message string    `json:"message"`
}

// PropertyPublishedResponse is returned after publishing a property.
type PropertyPublishedResponse struct {
	ID        uuid.UUID `json:"id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Published time.Time `json:"published_at"`
}

// ImageUploadedResponse is returned after uploading an image.
type ImageUploadedResponse struct {
	ImageID  uuid.UUID `json:"image_id"`
	ImageURL string    `json:"image_url"`
	IsCover  bool      `json:"is_cover"`
	Message  string    `json:"message"`
}

// DocumentUploadedResponse is returned after uploading a document.
type DocumentUploadedResponse struct {
	DocumentID uuid.UUID `json:"document_id"`
	FileURL    string    `json:"file_url"`
	Message    string    `json:"message"`
}

// ═══════════════════════════════════════════════════════════════════════════
// MAPPER FUNCTIONS (Model → DTO conversions)
// ═══════════════════════════════════════════════════════════════════════════

// ToPropertyResponse converts a property model to public response DTO.
func ToPropertyResponse(p *models.Property, images []models.PropertyImage, documents []models.PropertyDocument) PropertyResponse {
	return PropertyResponse{
		ID:            p.ID,
		Slug:          p.Slug,
		Title:         p.Title,
		Description:   p.Description,
		PropertyType:  p.PropertyType,
		Status:        p.Status,
		CoverImageURL: p.CoverImageURL,
		Location: PropertyLocationResponse{
			Address:   p.Address,
			City:      p.City,
			State:     p.State,
			Country:   p.Country,
			Latitude:  p.Latitude,
			Longitude: p.Longitude,
		},
		Financial: PropertyFinancialResponse{
			TargetAmount:        p.TargetAmount,
			RaisedAmount:        p.RaisedAmount,
			RemainingAmount:     p.RemainingAmount(),
			MinimumInvestment:   p.MinimumInvestment,
			UnitPrice:           p.UnitPrice,
			TotalUnits:          p.TotalUnits,
			UnitsSold:           p.UnitsSold,
			UnitsRemaining:      p.RemainingUnits(),
			FundingPercentage:   p.FundingPercentage(),
			InvestorCount:       p.InvestorCount,
			IsFullyFunded:       p.IsFullyFunded(),
			ROIPercent:          p.ROIPercent,
			ExpectedReturnsKobo: calculateExpectedReturns(p.MinimumInvestment, p.ROIPercent),
		},
		InvestmentPeriod: PropertyInvestmentResponse{
			Duration:     p.DurationMonths,
			LaunchDate:   derefTime(p.LaunchDate),
			MaturityDate: derefTime(p.ExpectedCompletionDate),
		},
		Features:  []string{}, // TODO: Add Features field to Property model if needed
		Amenities: []string{}, // TODO: Add Amenities field to Property model if needed
		Images:    ToPropertyImageResponses(images),
		Documents: ToPropertyDocumentResponses(documents, true), // public only
		Featured:  p.Featured,
		Verified:  p.Verified,
		Trending:  p.Trending,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

// ToPropertyAdminResponse converts a property model to admin response DTO.
func ToPropertyAdminResponse(p *models.Property, images []models.PropertyImage, documents []models.PropertyDocument, history []models.PropertyStatusHistory) PropertyAdminResponse {
	publicResponse := ToPropertyResponse(p, images, documents)
	
	return PropertyAdminResponse{
		PropertyResponse: publicResponse,
		InvestorCount:    p.InvestorCount,
		StatusHistory:    ToPropertyStatusHistoryResponses(history),
		AllDocuments:     ToPropertyDocumentResponses(documents, false), // all documents
	}
}

// ToPropertyListItemResponse converts a property model to list item DTO.
func ToPropertyListItemResponse(p *models.Property) PropertyListItemResponse {
	return PropertyListItemResponse{
		ID:            p.ID,
		Slug:          p.Slug,
		Title:         p.Title,
		PropertyType:  p.PropertyType,
		Status:        p.Status,
		CoverImageURL: p.CoverImageURL,
		Location: PropertyLocationResponse{
			Address: p.Address,
			City:    p.City,
			State:   p.State,
			Country: p.Country,
		},
		Financial: PropertyFinancialResponse{
			TargetAmount:      p.TargetAmount,
			RaisedAmount:      p.RaisedAmount,
			RemainingAmount:   p.RemainingAmount(),
			MinimumInvestment: p.MinimumInvestment,
			FundingPercentage: p.FundingPercentage(),
			InvestorCount:     p.InvestorCount,
		},
		ROIPercent: p.ROIPercent,
		Duration:   p.DurationMonths,
		Featured:   p.Featured,
		Verified:   p.Verified,
		Trending:   p.Trending,
		LaunchDate: derefTime(p.LaunchDate),
		CreatedAt:  p.CreatedAt,
	}
}

// ToPropertyImageResponse converts an image model to response DTO.
func ToPropertyImageResponse(img *models.PropertyImage) PropertyImageResponse {
	return PropertyImageResponse{
		ID:           img.ID,
		ImageURL:     img.URL,
		ThumbnailURL: nil, // TODO: Generate thumbnail URL from Cloudinary if needed
		Caption:      img.AltText,
		Width:        img.Width,
		Height:       img.Height,
		IsCover:      img.IsCover,
		DisplayOrder: img.DisplayOrder,
		CreatedAt:    img.CreatedAt,
	}
}

// ToPropertyImageResponses converts multiple images to DTOs.
func ToPropertyImageResponses(images []models.PropertyImage) []PropertyImageResponse {
	responses := make([]PropertyImageResponse, len(images))
	for i, img := range images {
		responses[i] = ToPropertyImageResponse(&img)
	}
	return responses
}

// ToPropertyDocumentResponse converts a document model to response DTO.
func ToPropertyDocumentResponse(doc *models.PropertyDocument) PropertyDocumentResponse {
	return PropertyDocumentResponse{
		ID:           doc.ID,
		DocumentType: doc.DocumentType,
		Title:        doc.Name,
		Description:  nil, // TODO: Add Description field to PropertyDocument model if needed
		FileURL:      doc.URL,
		FileName:     doc.Name,
		FileSize:     doc.FileSize,
		FileSizeMB:   doc.FileSizeInMB(),
		MimeType:     doc.MIMEType,
		IsPublic:     doc.IsPublic,
		DisplayOrder: 0, // TODO: Add DisplayOrder field to PropertyDocument model if needed
		UploadedAt:   doc.CreatedAt,
	}
}

// ToPropertyDocumentResponses converts multiple documents to DTOs.
// If publicOnly is true, filters to only public documents.
func ToPropertyDocumentResponses(documents []models.PropertyDocument, publicOnly bool) []PropertyDocumentResponse {
	responses := []PropertyDocumentResponse{}
	for _, doc := range documents {
		if publicOnly && !doc.IsPublic {
			continue // Skip private documents
		}
		responses = append(responses, ToPropertyDocumentResponse(&doc))
	}
	return responses
}

// ToPropertyStatusHistoryResponse converts status history to DTO.
func ToPropertyStatusHistoryResponse(history *models.PropertyStatusHistory) PropertyStatusHistoryResponse {
	return PropertyStatusHistoryResponse{
		ID:        history.ID,
		OldStatus: history.FromStatus,
		NewStatus: history.ToStatus,
		ChangedBy: history.ChangedBy,
		Reason:    history.Reason,
		IsSystem:  history.IsSystemChange(),
		ChangedAt: history.CreatedAt,
	}
}

// ToPropertyStatusHistoryResponses converts multiple status history records.
func ToPropertyStatusHistoryResponses(history []models.PropertyStatusHistory) []PropertyStatusHistoryResponse {
	responses := make([]PropertyStatusHistoryResponse, len(history))
	for i, h := range history {
		responses[i] = ToPropertyStatusHistoryResponse(&h)
	}
	return responses
}

// BuildPaginationMetadata creates pagination metadata.
func BuildPaginationMetadata(currentPage, pageSize int, totalRecords int64) PaginationMetadata {
	totalPages := int((totalRecords + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}

	return PaginationMetadata{
		CurrentPage:  currentPage,
		PageSize:     pageSize,
		TotalPages:   totalPages,
		TotalRecords: totalRecords,
		HasNext:      currentPage < totalPages,
		HasPrevious:  currentPage > 1,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════

// calculateExpectedReturns computes expected returns for minimum investment.
// Returns amount in kobo.
func calculateExpectedReturns(minimumInvestment int64, roiPercent float64) int64 {
	returns := float64(minimumInvestment) * (roiPercent / 100.0)
	return int64(returns)
}

// derefTime safely dereferences a time pointer, returning zero time if nil.
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

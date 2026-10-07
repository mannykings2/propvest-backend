package services

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/utils/cloudinary"
	"gorm.io/gorm"
	"gorm.io/datatypes"
)

// PropertyService handles property management business logic.
//
// RESPONSIBILITIES:
//   - Property CRUD operations
//   - Property publishing with validation
//   - Image upload and management
//   - Document upload and management
//   - Status lifecycle management
//   - Outbox event creation
//   - Cloudinary integration
//
// SECURITY:
//   - All write operations require admin role (enforced by middleware)
//   - Public reads only show active/funded/completed properties
//   - Private documents filtered from public responses
//
// TRANSACTIONS:
//   - Multi-step operations use database transactions
//   - Cloudinary cleanup on transaction failure
//   - Outbox events created atomically with state changes
type PropertyService interface {
	// ═══════════════════════════════════════════════════════════════════════════
	// PROPERTY CRUD
	// ═══════════════════════════════════════════════════════════════════════════

	// CreateProperty creates a new property in draft status.
	// Only admins can create properties.
	// Property starts as draft and must be explicitly published.
	CreateProperty(ctx context.Context, adminID uuid.UUID, req dto.CreatePropertyRequest) (*dto.PropertyCreatedResponse, error)

	// UpdateProperty updates an existing property.
	// Restrictions apply based on property status:
	//   - Draft: all fields editable
	//   - Active/Funded: financial fields locked if investments exist
	//   - Completed: minimal edits allowed
	UpdateProperty(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, req dto.UpdatePropertyRequest) (*dto.PropertyResponse, error)

	// DeleteProperty soft-deletes a property.
	// Fails if property has investments (protects financial audit trail).
	DeleteProperty(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID) error

	// ═══════════════════════════════════════════════════════════════════════════
	// PROPERTY READS
	// ═══════════════════════════════════════════════════════════════════════════

	// GetPublicProperty retrieves a property for public viewing.
	// Only returns active/funded/completed properties.
	// Returns 404 for draft properties.
	// Includes public images and documents only.
	GetPublicProperty(ctx context.Context, propertyID uuid.UUID) (*dto.PropertyResponse, error)

	// GetAdminProperty retrieves full property details for admins.
	// Includes all status properties (draft, active, funded, completed).
	// Includes all documents (public and private).
	// Includes status history.
	GetAdminProperty(ctx context.Context, propertyID uuid.UUID) (*dto.PropertyAdminResponse, error)

	// ListProperties retrieves paginated property listings.
	// Filters and sorts based on provided criteria.
	// Public users see only active/funded/completed properties.
	// Admins see all properties if filter.IsAdmin = true.
	ListProperties(ctx context.Context, filter dto.PropertyFilterRequest, isAdmin bool) (*dto.PropertyListResponse, error)

	// ═══════════════════════════════════════════════════════════════════════════
	// PROPERTY LIFECYCLE
	// ═══════════════════════════════════════════════════════════════════════════

	// PublishProperty transitions property from draft to active status.
	// Validates publication requirements:
	//   - Cover image present
	//   - Required fields filled
	//   - Financial details valid
	// Creates property.published outbox event.
	// Records status history.
	PublishProperty(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID) (*dto.PropertyPublishedResponse, error)

	// ═══════════════════════════════════════════════════════════════════════════
	// IMAGE MANAGEMENT
	// ═══════════════════════════════════════════════════════════════════════════

	// UploadImage uploads a property image to Cloudinary.
	// If it's the first image, automatically sets as cover.
	// If IsCover is true, unsets previous cover and sets this as new cover.
	// Folder: properties/{propertyID}/images
	UploadImage(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, file *multipart.FileHeader, req dto.UploadImageRequest) (*dto.ImageUploadedResponse, error)

	// DeleteImage removes a property image.
	// Deletes from Cloudinary and database atomically.
	// If deleted image was cover, assigns new cover from remaining images.
	DeleteImage(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, imageID uuid.UUID) error

	// SetCoverImage designates an image as the property's cover.
	// Updates property.cover_image_url.
	// Unsets previous cover automatically.
	SetCoverImage(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, imageID uuid.UUID) error

	// ═══════════════════════════════════════════════════════════════════════════
	// DOCUMENT MANAGEMENT
	// ═══════════════════════════════════════════════════════════════════════════

	// UploadDocument uploads a property document to Cloudinary.
	// Supports PDFs, images, and other document types.
	// IsPublic controls visibility to non-admin users.
	// Folder: properties/{propertyID}/documents
	UploadDocument(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, file *multipart.FileHeader, req dto.UploadDocumentRequest) (*dto.DocumentUploadedResponse, error)

	// DeleteDocument removes a property document.
	// Deletes from Cloudinary and database atomically.
	DeleteDocument(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, documentID uuid.UUID) error

	// ToggleDocumentVisibility changes document public/private status.
	// Allows admins to show/hide documents from public view.
	ToggleDocumentVisibility(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, documentID uuid.UUID, isPublic bool) error
}

// propertyService is the concrete implementation.
type propertyService struct {
	propertyRepo   repositories.PropertyRepository
	imageRepo      repositories.PropertyImageRepository
	documentRepo   repositories.PropertyDocumentRepository
	outboxRepo     repositories.OutboxRepository
	cloudinary     *cloudinary.CloudinaryService
	db             *gorm.DB
}

// NewPropertyService creates a new property service instance.
func NewPropertyService(
	propertyRepo repositories.PropertyRepository,
	imageRepo repositories.PropertyImageRepository,
	documentRepo repositories.PropertyDocumentRepository,
	outboxRepo repositories.OutboxRepository,
	cloudinaryService *cloudinary.CloudinaryService,
	db *gorm.DB,
) PropertyService {
	return &propertyService{
		propertyRepo:   propertyRepo,
		imageRepo:      imageRepo,
		documentRepo:   documentRepo,
		outboxRepo:     outboxRepo,
		cloudinary:     cloudinaryService,
		db:             db,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// PROPERTY CRUD
// ═══════════════════════════════════════════════════════════════════════════

// CreateProperty creates a new property in draft status.
func (s *propertyService) CreateProperty(ctx context.Context, adminID uuid.UUID, req dto.CreatePropertyRequest) (*dto.PropertyCreatedResponse, error) {
	// Parse dates
	launchDate, err := time.Parse("2006-01-02", req.LaunchDate)
	if err != nil {
		return nil, errors.WrapError(errors.ErrValidationFailed, "invalid launch_date format, use YYYY-MM-DD", "validation_error")
	}

	maturityDate, err := time.Parse("2006-01-02", req.MaturityDate)
	if err != nil {
		return nil, errors.WrapError(errors.ErrValidationFailed, "invalid maturity_date format, use YYYY-MM-DD", "validation_error")
	}

	// Business validations
	if maturityDate.Before(launchDate) {
		return nil, errors.WrapError(errors.ErrValidationFailed, "maturity_date must be after launch_date", "validation_error")
	}

	if req.MinimumInvestment > req.TargetAmount {
		return nil, errors.WrapError(errors.ErrValidationFailed, "minimum_investment cannot exceed target_amount", "validation_error")
	}

	if req.UnitPrice*req.TotalUnits != req.TargetAmount {
		return nil, errors.WrapError(errors.ErrValidationFailed, "target_amount must equal unit_price × total_units", "validation_error")
	}

	// Generate unique slug
	propertySlug, err := s.generateUniqueSlug(ctx, req.Title)
	if err != nil {
		return nil, err
	}

	// Create property model
	property := &models.Property{
		Title:                  req.Title,
		Slug:                   propertySlug,
		Description:            req.Description,
		PropertyType:           req.PropertyType,
		Address:                req.Address,
		City:                   req.City,
		State:                  req.State,
		Country:                req.Country,
		Latitude:               req.Latitude,
		Longitude:              req.Longitude,
		TargetAmount:           req.TargetAmount,
		MinimumInvestment:      req.MinimumInvestment,
		UnitPrice:              req.UnitPrice,
		TotalUnits:             req.TotalUnits,
		ROIPercent:             req.ROIPercent,
		DurationMonths:         req.InvestmentDuration,
		LaunchDate:             &launchDate,
		ExpectedCompletionDate: &maturityDate,
		Status:                 "draft",
		Featured:               req.Featured,
		Verified:               req.Verified,
		Trending:               req.Trending,
		CreatedBy:              adminID,
		Currency:               "NGN",
		RaisedAmount:           0,
		UnitsSold:              0,
		InvestorCount:          0,
	}

	// Save to database
	err = s.propertyRepo.Create(ctx, property, nil)
	if err != nil {
		logger.Error("failed to create property", "error", err)
		return nil, errors.ErrInternalServer
	}

	logger.Info("property created", "property_id", property.ID, "admin_id", adminID)

	return &dto.PropertyCreatedResponse{
		ID:      property.ID,
		Slug:    property.Slug,
		Status:  property.Status,
		Message: "Property created successfully in draft status",
	}, nil
}

// UpdateProperty updates an existing property.
func (s *propertyService) UpdateProperty(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, req dto.UpdatePropertyRequest) (*dto.PropertyResponse, error) {
	// Fetch existing property
	property, err := s.propertyRepo.FindByID(ctx, propertyID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrPropertyNotFound
		}
		return nil, errors.ErrInternalServer
	}

	// Check if property has investments (for financial field restrictions)
	hasInvestments := property.InvestorCount > 0

	// Update basic fields
	if req.Title != nil {
		property.Title = *req.Title
		// Regenerate slug if title changed
		newSlug, err := s.generateUniqueSlug(ctx, *req.Title)
		if err != nil {
			return nil, err
		}
		property.Slug = newSlug
	}

	if req.Description != nil {
		property.Description = *req.Description
	}

	if req.PropertyType != nil {
		property.PropertyType = *req.PropertyType
	}

	// Location updates
	if req.Address != nil {
		property.Address = *req.Address
	}
	if req.City != nil {
		property.City = *req.City
	}
	if req.State != nil {
		property.State = *req.State
	}
	if req.Country != nil {
		property.Country = *req.Country
	}
	if req.Latitude != nil {
		property.Latitude = req.Latitude
	}
	if req.Longitude != nil {
		property.Longitude = req.Longitude
	}

	// Financial updates (only if no investments)
	if hasInvestments {
		if req.TargetAmount != nil || req.MinimumInvestment != nil || req.UnitPrice != nil || req.TotalUnits != nil {
			return nil, errors.NewAppError("cannot modify financial details after investments have been made", "business_logic_error")
		}
	} else {
		if req.TargetAmount != nil {
			property.TargetAmount = *req.TargetAmount
		}
		if req.MinimumInvestment != nil {
			property.MinimumInvestment = *req.MinimumInvestment
		}
		if req.UnitPrice != nil {
			property.UnitPrice = *req.UnitPrice
		}
		if req.TotalUnits != nil {
			property.TotalUnits = *req.TotalUnits
		}
	}

	if req.ROIPercent != nil {
		property.ROIPercent = *req.ROIPercent
	}

	if req.InvestmentDuration != nil {
		property.DurationMonths = *req.InvestmentDuration
	}

	// Date updates
	if req.LaunchDate != nil {
		launchDate, err := time.Parse("2006-01-02", *req.LaunchDate)
		if err != nil {
			return nil, errors.WrapError(errors.ErrValidationFailed, "invalid launch_date format", "validation_error")
		}
		property.LaunchDate = &launchDate
	}

	if req.MaturityDate != nil {
		maturityDate, err := time.Parse("2006-01-02", *req.MaturityDate)
		if err != nil {
			return nil, errors.WrapError(errors.ErrValidationFailed, "invalid maturity_date format", "validation_error")
		}
		property.ExpectedCompletionDate = &maturityDate
	}

	// Flags (admins can toggle anytime)
	if req.Featured != nil {
		property.Featured = *req.Featured
	}
	if req.Verified != nil {
		property.Verified = *req.Verified
	}
	if req.Trending != nil {
		property.Trending = *req.Trending
	}

	// Update audit field
	property.UpdatedBy = &adminID

	// Save changes
	err = s.propertyRepo.Update(ctx, property, nil)
	if err != nil {
		logger.Error("failed to update property", "error", err, "property_id", propertyID)
		return nil, errors.ErrInternalServer
	}

	logger.Info("property updated", "property_id", propertyID, "admin_id", adminID)

	// Return updated property
	return s.GetPublicProperty(ctx, propertyID)
}

// DeleteProperty soft-deletes a property.
func (s *propertyService) DeleteProperty(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID) error {
	// Fetch property to check for investments
	property, err := s.propertyRepo.FindByID(ctx, propertyID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return errors.ErrPropertyNotFound
		}
		return errors.ErrInternalServer
	}

	// Prevent deletion if property has investments
	if property.InvestorCount > 0 {
		return errors.NewAppError("cannot delete property with existing investments", "business_logic_error")
	}

	// Soft delete
	err = s.propertyRepo.SoftDelete(ctx, propertyID, nil)
	if err != nil {
		logger.Error("failed to delete property", "error", err, "property_id", propertyID)
		return errors.ErrInternalServer
	}

	logger.Info("property deleted", "property_id", propertyID, "admin_id", adminID)
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// PROPERTY READS
// ═══════════════════════════════════════════════════════════════════════════

// GetPublicProperty retrieves a property for public viewing.
func (s *propertyService) GetPublicProperty(ctx context.Context, propertyID uuid.UUID) (*dto.PropertyResponse, error) {
	// Use public query (excludes drafts)
	property, err := s.propertyRepo.FindPublicByID(ctx, propertyID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrPropertyNotFound
		}
		return nil, errors.ErrInternalServer
	}

	// Load public images
	images, err := s.imageRepo.FindByPropertyID(ctx, propertyID)
	if err != nil {
		logger.Error("failed to load property images", "error", err, "property_id", propertyID)
		return nil, errors.ErrInternalServer
	}

	// Load public documents only
	documents, err := s.documentRepo.FindByPropertyID(ctx, propertyID, true)
	if err != nil {
		logger.Error("failed to load property documents", "error", err, "property_id", propertyID)
		return nil, errors.ErrInternalServer
	}

	// Convert to DTO
	response := dto.ToPropertyResponse(property, images, documents)
	return &response, nil
}

// GetAdminProperty retrieves full property details for admins.
func (s *propertyService) GetAdminProperty(ctx context.Context, propertyID uuid.UUID) (*dto.PropertyAdminResponse, error) {
	// Admin can see all properties including drafts
	property, err := s.propertyRepo.FindByID(ctx, propertyID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrPropertyNotFound
		}
		return nil, errors.ErrInternalServer
	}

	// Load all images
	images, err := s.imageRepo.FindByPropertyID(ctx, propertyID)
	if err != nil {
		logger.Error("failed to load property images", "error", err, "property_id", propertyID)
		return nil, errors.ErrInternalServer
	}

	// Load all documents (public and private)
	documents, err := s.documentRepo.FindByPropertyID(ctx, propertyID, false)
	if err != nil {
		logger.Error("failed to load property documents", "error", err, "property_id", propertyID)
		return nil, errors.ErrInternalServer
	}

	// Load status history
	// TODO: Implement status history repository method
	statusHistory := []models.PropertyStatusHistory{}

	// Convert to DTO
	response := dto.ToPropertyAdminResponse(property, images, documents, statusHistory)
	return &response, nil
}

// ListProperties retrieves paginated property listings.
func (s *propertyService) ListProperties(ctx context.Context, filterReq dto.PropertyFilterRequest, isAdmin bool) (*dto.PropertyListResponse, error) {
	// Convert DTO filter to repository filter
	filter := repositories.PropertyFilter{
		Search:        filterReq.Search,
		PropertyType:  filterReq.PropertyType,
		City:          filterReq.City,
		State:         filterReq.State,
		Status:        filterReq.Status,
		Featured:      filterReq.Featured,
		Verified:      filterReq.Verified,
		Trending:      filterReq.Trending,
		MinInvestment: filterReq.MinInvestment,
		MaxInvestment: filterReq.MaxInvestment,
		MinROI:        filterReq.MinROI,
		MaxROI:        filterReq.MaxROI,
		Page:          filterReq.Page,
		PageSize:      filterReq.PageSize,
		SortBy:        filterReq.SortBy,
		SortOrder:     filterReq.SortOrder,
		IsAdmin:       isAdmin,
	}

	// Default page and page size
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}

	// Query properties
	properties, total, err := s.propertyRepo.List(ctx, filter)
	if err != nil {
		logger.Error("failed to list properties", "error", err)
		return nil, errors.ErrInternalServer
	}

	// Convert to DTOs
	propertyList := make([]dto.PropertyListItemResponse, len(properties))
	for i, property := range properties {
		propertyList[i] = dto.ToPropertyListItemResponse(property)
	}

	// Build pagination metadata
	pagination := dto.BuildPaginationMetadata(filter.Page, filter.PageSize, total)

	return &dto.PropertyListResponse{
		Properties: propertyList,
		Pagination: pagination,
	}, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// PROPERTY LIFECYCLE
// ═══════════════════════════════════════════════════════════════════════════

// PublishProperty transitions property from draft to active status.
func (s *propertyService) PublishProperty(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID) (*dto.PropertyPublishedResponse, error) {
	// Fetch property
	property, err := s.propertyRepo.FindByID(ctx, propertyID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrPropertyNotFound
		}
		return nil, errors.ErrInternalServer
	}

	// Check current status
	if property.Status != "draft" {
		return nil, errors.NewAppError("only draft properties can be published", "business_logic_error")
	}

	// Validate publication requirements
	if err := s.validatePublicationRequirements(ctx, property); err != nil {
		return nil, err
	}

	// Use transaction for atomicity
	var publishedAt time.Time
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Update status
		if err := s.propertyRepo.UpdateStatus(ctx, propertyID, "draft", "active", tx); err != nil {
			return err
		}

		publishedAt = time.Now()

		// Create status history record
		// TODO: Implement status history creation

		// Create outbox event
		eventPayload := map[string]interface{}{
			"property_id": propertyID,
			"title":       property.Title,
			"slug":        property.Slug,
			"published_at": publishedAt,
			"admin_id":    adminID,
		}

		payloadJSON, err := json.Marshal(eventPayload)
		if err != nil {
			return err
		}

		outboxEvent := &models.OutboxEvent{
			EventType:     models.EventTypePropertyPublished,
			AggregateType: "property",
			AggregateID:   propertyID,
			Payload:       datatypes.JSON(payloadJSON),
		}

		if err := s.outboxRepo.CreateEvent(ctx, outboxEvent, tx); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		logger.Error("failed to publish property", "error", err, "property_id", propertyID)
		return nil, errors.ErrInternalServer
	}

	logger.Info("property published", "property_id", propertyID, "admin_id", adminID)

	return &dto.PropertyPublishedResponse{
		ID:        propertyID,
		Status:    "active",
		Message:   "Property published successfully",
		Published: publishedAt,
	}, nil
}

// validatePublicationRequirements checks if property meets publication criteria.
func (s *propertyService) validatePublicationRequirements(ctx context.Context, property *models.Property) error {
	var validationErrors []string

	// Check cover image
	if property.CoverImageURL == nil || *property.CoverImageURL == "" {
		validationErrors = append(validationErrors, "cover image is required")
	}

	// Check required fields
	if property.Title == "" {
		validationErrors = append(validationErrors, "title is required")
	}

	if property.Description == "" {
		validationErrors = append(validationErrors, "description is required")
	}

	if property.Address == "" {
		validationErrors = append(validationErrors, "address is required")
	}

	if property.City == "" {
		validationErrors = append(validationErrors, "city is required")
	}

	if property.State == "" {
		validationErrors = append(validationErrors, "state is required")
	}

	// Check financial details
	if property.TargetAmount <= 0 {
		validationErrors = append(validationErrors, "target amount must be greater than zero")
	}

	if property.MinimumInvestment <= 0 {
		validationErrors = append(validationErrors, "minimum investment must be greater than zero")
	}

	if property.UnitPrice <= 0 {
		validationErrors = append(validationErrors, "unit price must be greater than zero")
	}

	if property.TotalUnits <= 0 {
		validationErrors = append(validationErrors, "total units must be greater than zero")
	}

	if property.ROIPercent <= 0 {
		validationErrors = append(validationErrors, "ROI percent must be greater than zero")
	}

	if property.DurationMonths <= 0 {
		validationErrors = append(validationErrors, "investment duration must be greater than zero")
	}

	// Check dates
	if property.LaunchDate == nil {
		validationErrors = append(validationErrors, "launch date is required")
	}

	if property.ExpectedCompletionDate == nil {
		validationErrors = append(validationErrors, "expected completion date is required")
	}

	if len(validationErrors) > 0 {
		return errors.NewAppError(strings.Join(validationErrors, "; "), "validation_error")
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════

// generateUniqueSlug generates a URL-friendly slug from title with uniqueness guarantee.
func (s *propertyService) generateUniqueSlug(ctx context.Context, title string) (string, error) {
	baseSlug := makeSlug(title)
	
	// Try base slug first
	_, err := s.propertyRepo.FindBySlug(ctx, baseSlug)
	if repositories.IsErrRecordNotFound(err) {
		// Slug is available
		return baseSlug, nil
	}
	if err != nil {
		// Unexpected error
		return "", errors.ErrInternalServer
	}

	// Base slug taken, append random suffix
	for i := 0; i < 10; i++ {
		// Generate short random string
		randomSlug := fmt.Sprintf("%s-%s", baseSlug, generateRandomString(6))
		_, err := s.propertyRepo.FindBySlug(ctx, randomSlug)
		if repositories.IsErrRecordNotFound(err) {
			return randomSlug, nil
		}
		if err != nil {
			return "", errors.ErrInternalServer
		}
	}

	return "", errors.NewAppError("failed to generate unique slug after 10 attempts", "business_logic_error")
}

// makeSlug converts a string to a URL-friendly slug.
func makeSlug(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)
	
	// Replace spaces and special characters with hyphens
	reg := regexp.MustCompile("[^a-z0-9]+")
	s = reg.ReplaceAllString(s, "-")
	
	// Remove leading/trailing hyphens
	s = strings.Trim(s, "-")
	
	// Limit length
	if len(s) > 200 {
		s = s[:200]
	}
	
	return s
}

// generateRandomString generates a random alphanumeric string of given length.
func generateRandomString(length int) string {
	return uuid.New().String()[:length]
}

// ═══════════════════════════════════════════════════════════════════════════
// IMAGE MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// UploadImage uploads a property image to Cloudinary.
func (s *propertyService) UploadImage(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, file *multipart.FileHeader, req dto.UploadImageRequest) (*dto.ImageUploadedResponse, error) {
	// Verify property exists
	property, err := s.propertyRepo.FindByID(ctx, propertyID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrPropertyNotFound
		}
		return nil, errors.ErrInternalServer
	}

	// Validate file
	if err := validateImageFile(file); err != nil {
		return nil, err
	}

	// Upload to Cloudinary
	// Folder: properties/{propertyID}/images
	
	uploadResult, err := s.cloudinary.UploadPropertyImage(ctx, file, propertyID)
	if err != nil {
		logger.Error("failed to upload image to cloudinary", "error", err)
		return nil, errors.NewAppError("failed to upload image", "business_logic_error")
	}

	// Determine display order
	displayOrder := 0
	if req.DisplayOrder != nil {
		displayOrder = *req.DisplayOrder
	} else {
		// Auto-assign next display order
		displayOrder, err = s.imageRepo.GetNextDisplayOrder(ctx, propertyID)
		if err != nil {
			logger.Error("failed to get next display order", "error", err)
			return nil, errors.ErrInternalServer
		}
	}

	// Check if this is the first image
	existingImages, err := s.imageRepo.FindByPropertyID(ctx, propertyID)
	if err != nil {
		logger.Error("failed to check existing images", "error", err)
		return nil, errors.ErrInternalServer
	}

	isCover := req.IsCover || len(existingImages) == 0 // First image is auto-cover

	// Create image record in transaction
	var imageID uuid.UUID
	var imageURL string
	
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create image record
		image := &models.PropertyImage{
			PropertyID:   propertyID,
			URL:          uploadResult.SecureURL,
			PublicID:     uploadResult.PublicID,
			AltText:      req.Caption,
			DisplayOrder: displayOrder,
			IsCover:      false, // Set via SetCover if needed
			Width:        &uploadResult.Width,
			Height:       &uploadResult.Height,
		}

		if err := s.imageRepo.Create(ctx, image, tx); err != nil {
			return err
		}

		imageID = image.ID
		imageURL = image.URL

		// Set as cover if needed
		if isCover {
			if err := s.imageRepo.SetCover(ctx, propertyID, image.ID, tx); err != nil {
				return err
			}

			// Update property cover_image_url
			property.CoverImageURL = &image.URL
			if err := s.propertyRepo.Update(ctx, property, tx); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		// Cleanup Cloudinary upload on failure
		_ = s.cloudinary.DeleteImage(ctx, uploadResult.PublicID)
		logger.Error("failed to save image record", "error", err)
		return nil, errors.ErrInternalServer
	}

	logger.Info("property image uploaded", "property_id", propertyID, "image_id", imageID, "is_cover", isCover)

	return &dto.ImageUploadedResponse{
		ImageID:  imageID,
		ImageURL: imageURL,
		IsCover:  isCover,
		Message:  "Image uploaded successfully",
	}, nil
}

// DeleteImage removes a property image.
func (s *propertyService) DeleteImage(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, imageID uuid.UUID) error {
	// Fetch image
	image, err := s.imageRepo.FindByID(ctx, imageID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return errors.NewAppError("image not found", "not_found")
		}
		return errors.ErrInternalServer
	}

	// Verify image belongs to property
	if image.PropertyID != propertyID {
		return errors.NewAppError("image does not belong to this property", "forbidden")
	}

	wasCover := image.IsCover

	// Delete in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Delete from database
		if err := s.imageRepo.Delete(ctx, imageID, tx); err != nil {
			return err
		}

		// If deleted image was cover, assign new cover
		if wasCover {
			// Get remaining images
			remainingImages, err := s.imageRepo.FindByPropertyID(ctx, propertyID)
			if err != nil {
				return err
			}

			if len(remainingImages) > 0 {
				// Set first remaining image as cover
				newCover := &remainingImages[0]
				if err := s.imageRepo.SetCover(ctx, propertyID, newCover.ID, tx); err != nil {
					return err
				}

				// Update property cover_image_url
				property, err := s.propertyRepo.FindByID(ctx, propertyID)
				if err != nil {
					return err
				}
				property.CoverImageURL = &newCover.URL
				if err := s.propertyRepo.Update(ctx, property, tx); err != nil {
					return err
				}
			} else {
				// No images left, clear cover
				property, err := s.propertyRepo.FindByID(ctx, propertyID)
				if err != nil {
					return err
				}
				property.CoverImageURL = nil
				if err := s.propertyRepo.Update(ctx, property, tx); err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		logger.Error("failed to delete image record", "error", err, "image_id", imageID)
		return errors.ErrInternalServer
	}

	// Move file to -deleted folder in Cloudinary (async, for recoverability)
	// This allows restoration if the soft delete needs to be undone
	go func() {
		_, moveErr := s.cloudinary.MoveToDeletedFolder(context.Background(), image.PublicID, "image")
		if moveErr != nil {
			logger.Error("failed to move image to deleted folder in cloudinary",
				"error", moveErr,
				"public_id", image.PublicID,
				"property_id", propertyID,
				"image_id", imageID)
			// Note: Database soft delete still succeeded, so operation isn't rolled back
			// The file remains in Cloudinary but won't be used
		} else {
			logger.Info("moved image to deleted folder",
				"property_id", propertyID,
				"image_id", imageID,
				"public_id", image.PublicID)
		}
	}()

	logger.Info("property image deleted", "property_id", propertyID, "image_id", imageID, "was_cover", wasCover)
	return nil
}

// SetCoverImage designates an image as the property's cover.
func (s *propertyService) SetCoverImage(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, imageID uuid.UUID) error {
	// Fetch image
	image, err := s.imageRepo.FindByID(ctx, imageID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return errors.NewAppError("image not found", "not_found")
		}
		return errors.ErrInternalServer
	}

	// Verify image belongs to property
	if image.PropertyID != propertyID {
		return errors.NewAppError("image does not belong to this property", "forbidden")
	}

	// Update in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Set as cover (automatically unsets previous cover)
		if err := s.imageRepo.SetCover(ctx, propertyID, imageID, tx); err != nil {
			return err
		}

		// Update property cover_image_url
		property, err := s.propertyRepo.FindByID(ctx, propertyID)
		if err != nil {
			return err
		}
		property.CoverImageURL = &image.URL
		if err := s.propertyRepo.Update(ctx, property, tx); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		logger.Error("failed to set cover image", "error", err, "image_id", imageID)
		return errors.ErrInternalServer
	}

	logger.Info("cover image updated", "property_id", propertyID, "image_id", imageID)
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// DOCUMENT MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// UploadDocument uploads a property document to Cloudinary.
func (s *propertyService) UploadDocument(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, file *multipart.FileHeader, req dto.UploadDocumentRequest) (*dto.DocumentUploadedResponse, error) {
	// Verify property exists
	_, err := s.propertyRepo.FindByID(ctx, propertyID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrPropertyNotFound
		}
		return nil, errors.ErrInternalServer
	}

	// Validate file
	if err := validateDocumentFile(file); err != nil {
		return nil, err
	}

	// Upload to Cloudinary
	// Folder: properties/{propertyID}/documents
	// For documents, we'll use UploadPropertyImage for now (Cloudinary handles all file types)
	
	uploadResult, err := s.cloudinary.UploadPropertyImage(ctx, file, propertyID)
	if err != nil {
		logger.Error("failed to upload document to cloudinary", "error", err)
		return nil, errors.NewAppError("failed to upload document", "business_logic_error")
	}

	// Create document record
	document := &models.PropertyDocument{
		PropertyID:   propertyID,
		Name:         req.Title,
		DocumentType: req.DocumentType,
		URL:          uploadResult.SecureURL,
		PublicID:     uploadResult.PublicID,
		MIMEType:     file.Header.Get("Content-Type"),
		FileSize:     file.Size,
		IsPublic:     req.IsPublic,
		UploadedBy:   adminID,
	}

	err = s.documentRepo.Create(ctx, document, nil)
	if err != nil {
		// Cleanup Cloudinary upload on failure
		_ = s.cloudinary.DeleteImage(ctx, uploadResult.PublicID)
		logger.Error("failed to save document record", "error", err)
		return nil, errors.ErrInternalServer
	}

	logger.Info("property document uploaded", "property_id", propertyID, "document_id", document.ID, "is_public", req.IsPublic)

	return &dto.DocumentUploadedResponse{
		DocumentID: document.ID,
		FileURL:    document.URL,
		Message:    "Document uploaded successfully",
	}, nil
}

// DeleteDocument removes a property document.
func (s *propertyService) DeleteDocument(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, documentID uuid.UUID) error {
	// Fetch document
	document, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return errors.NewAppError("document not found", "not_found")
		}
		return errors.ErrInternalServer
	}

	// Verify document belongs to property
	if document.PropertyID != propertyID {
		return errors.NewAppError("document does not belong to this property", "forbidden")
	}

	// Delete from database (soft delete)
	err = s.documentRepo.Delete(ctx, documentID, nil)
	if err != nil {
		logger.Error("failed to delete document record", "error", err, "document_id", documentID)
		return errors.ErrInternalServer
	}

	// Move document to -deleted folder in Cloudinary (async, for recoverability)
	// Documents are stored as "raw" resource type in Cloudinary
	go func() {
		_, moveErr := s.cloudinary.MoveToDeletedFolder(context.Background(), document.PublicID, "raw")
		if moveErr != nil {
			logger.Error("failed to move document to deleted folder in cloudinary",
				"error", moveErr,
				"public_id", document.PublicID,
				"property_id", propertyID,
				"document_id", documentID)
			// Note: Database soft delete still succeeded
		} else {
			logger.Info("moved document to deleted folder",
				"property_id", propertyID,
				"document_id", documentID,
				"public_id", document.PublicID)
		}
	}()

	logger.Info("property document deleted", "property_id", propertyID, "document_id", documentID)
	return nil
}

// ToggleDocumentVisibility changes document public/private status.
func (s *propertyService) ToggleDocumentVisibility(ctx context.Context, adminID uuid.UUID, propertyID uuid.UUID, documentID uuid.UUID, isPublic bool) error {
	// Fetch document
	document, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return errors.NewAppError("document not found", "not_found")
		}
		return errors.ErrInternalServer
	}

	// Verify document belongs to property
	if document.PropertyID != propertyID {
		return errors.NewAppError("document does not belong to this property", "forbidden")
	}

	// Update visibility
	err = s.documentRepo.ToggleVisibility(ctx, documentID, isPublic, nil)
	if err != nil {
		logger.Error("failed to toggle document visibility", "error", err, "document_id", documentID)
		return errors.ErrInternalServer
	}

	logger.Info("document visibility toggled", "property_id", propertyID, "document_id", documentID, "is_public", isPublic)
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// FILE VALIDATION
// ═══════════════════════════════════════════════════════════════════════════

// validateImageFile validates image file before upload.
func validateImageFile(file *multipart.FileHeader) error {
	// Check file size (10MB max)
	const maxSize = 10 * 1024 * 1024 // 10MB
	if file.Size > maxSize {
		return errors.NewAppError("image file size exceeds 10MB limit", "validation_error")
	}

	if file.Size == 0 {
		return errors.NewAppError("image file is empty", "validation_error")
	}

	// Check content type
	contentType := file.Header.Get("Content-Type")
	allowedTypes := map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/webp": true,
		"image/gif":  true,
	}

	if !allowedTypes[contentType] {
		return errors.NewAppError("invalid image file type, allowed: JPEG, PNG, WebP, GIF", "validation_error")
	}

	// Validate filename extension
	filename := file.Filename
	validExtension := regexp.MustCompile(`\.(jpg|jpeg|png|webp|gif)$`).MatchString(strings.ToLower(filename))
	if !validExtension {
		return errors.NewAppError("invalid file extension", "validation_error")
	}

	return nil
}

// validateDocumentFile validates document file before upload.
func validateDocumentFile(file *multipart.FileHeader) error {
	// Check file size (20MB max for documents)
	const maxSize = 20 * 1024 * 1024 // 20MB
	if file.Size > maxSize {
		return errors.NewAppError("document file size exceeds 20MB limit", "validation_error")
	}

	if file.Size == 0 {
		return errors.NewAppError("document file is empty", "validation_error")
	}

	// Check content type
	contentType := file.Header.Get("Content-Type")
	allowedTypes := map[string]bool{
		"application/pdf":  true,
		"image/jpeg":       true,
		"image/jpg":        true,
		"image/png":        true,
		"application/msword": true, // .doc
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true, // .docx
	}

	if !allowedTypes[contentType] {
		return errors.NewAppError("invalid document file type, allowed: PDF, JPEG, PNG, DOC, DOCX", "validation_error")
	}

	return nil
}


// ═══════════════════════════════════════════════════════════════════════════
// ERROR HELPERS
// ═══════════════════════════════════════════════════════════════════════════

// NewValidationError creates a validation error with a custom message.
func NewValidationError(message string) error {
	return errors.NewAppError(message, "validation_error")
}

// NewBusinessLogicError creates a business logic error with a custom message.
func NewBusinessLogicError(message string) error {
	return errors.NewAppError(message, "business_logic_error")
}

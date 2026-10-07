package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// PropertyHandler handles all property-related HTTP requests.
//
// RESPONSIBILITIES:
//   - Parse HTTP requests (path params, query params, JSON body, multipart forms)
//   - Extract authenticated user context
//   - Call service methods
//   - Transform responses to HTTP
//   - Handle errors with appropriate status codes
//
// SECURITY:
//   - Admin-only endpoints protected by middleware (RequireRole("admin"))
//   - User ID extracted from JWT token context
//   - No business logic here - all in service layer
type PropertyHandler struct {
	propertyService services.PropertyService
}

// NewPropertyHandler creates a new property handler instance.
func NewPropertyHandler(propertyService services.PropertyService) *PropertyHandler {
	return &PropertyHandler{
		propertyService: propertyService,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// PUBLIC ENDPOINTS (No auth required)
// ═══════════════════════════════════════════════════════════════════════════

// ListProperties handles GET /api/v1/properties
//
// Query parameters:
//   - search: string (searches title, description, city, state)
//   - property_type: residential|commercial|land
//   - city: string
//   - state: string
//   - featured: true|false
//   - verified: true|false
//   - trending: true|false
//   - min_investment: int64 (kobo)
//   - max_investment: int64 (kobo)
//   - min_roi: float64
//   - max_roi: float64
//   - page: int (default: 1)
//   - page_size: int (default: 20, max: 100)
//   - sort_by: created_at|launch_date|target_amount|roi_percent|etc
//   - sort_order: asc|desc (default: desc)
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Properties retrieved successfully",
//     "data": {
//       "properties": [...],
//       "pagination": {
//         "current_page": 1,
//         "page_size": 20,
//         "total_pages": 5,
//         "total_records": 98,
//         "has_next": true,
//         "has_previous": false
//       }
//     }
//   }
//
// Example:
//   GET /api/v1/properties?city=Lagos&min_roi=15&page=1&page_size=20
func (h *PropertyHandler) ListProperties(c *gin.Context) {
	// Parse query parameters with binding
	var filter dto.PropertyFilterRequest
	if err := c.ShouldBindQuery(&filter); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Public endpoint - isAdmin = false
	result, err := h.propertyService.ListProperties(c.Request.Context(), filter, false)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, http.StatusOK, result)
}

// GetProperty handles GET /api/v1/properties/:id
//
// Path parameters:
//   - id: Property UUID
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Property retrieved successfully",
//     "data": {
//       "id": "...",
//       "title": "Luxury Apartment Complex",
//       "status": "active",
//       "financial": {...},
//       "location": {...},
//       "images": [...],
//       "documents": [...]  // Only public documents
//     }
//   }
//
// Error responses:
//   - 400 Bad Request: Invalid UUID format
//   - 404 Not Found: Property doesn't exist or is draft
//
// Example:
//   GET /api/v1/properties/550e8400-e29b-41d4-a716-446655440000
func (h *PropertyHandler) GetProperty(c *gin.Context) {
	// Parse property ID from path
	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	// Get public property
	result, err := h.propertyService.GetPublicProperty(c.Request.Context(), propertyID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, http.StatusOK, result)
}

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN ENDPOINTS (Require admin role)
// ═══════════════════════════════════════════════════════════════════════════

// CreateProperty handles POST /api/v1/admin/properties
//
// Requires: Admin role (middleware)
//
// Request body:
//   {
//     "title": "Luxury Apartment Complex - Lekki Phase 1",
//     "description": "Premium residential property...",
//     "property_type": "residential",
//     "address": "Plot 123, Lekki Phase 1",
//     "city": "Lagos",
//     "state": "Lagos",
//     "country": "Nigeria",
//     "target_amount": 100000000000,  // kobo
//     "minimum_investment": 5000000000,
//     "unit_price": 10000000,
//     "total_units": 10000,
//     "roi_percent": 18.5,
//     "investment_duration": 12,  // months
//     "launch_date": "2024-01-15",
//     "maturity_date": "2025-01-15"
//   }
//
// Success response (201 Created):
//   {
//     "success": true,
//     "message": "Property created successfully",
//     "data": {
//       "id": "...",
//       "slug": "luxury-apartment-complex-lekki-phase-1",
//       "status": "draft",
//       "message": "Property created successfully in draft status"
//     }
//   }
//
// Error responses:
//   - 400 Bad Request: Validation failed
//   - 401 Unauthorized: Not authenticated
//   - 403 Forbidden: Not an admin
//   - 422 Unprocessable Entity: Business rule violation
func (h *PropertyHandler) CreateProperty(c *gin.Context) {
	// Extract admin ID from context (set by Auth middleware)
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Parse request body
	var req dto.CreatePropertyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Create property
	result, err := h.propertyService.CreateProperty(c.Request.Context(), adminID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusCreated, "Property created successfully", result)
}

// GetAdminProperty handles GET /api/v1/admin/properties/:id
//
// Requires: Admin role
//
// Returns full property details including:
//   - All statuses (including draft)
//   - All documents (public and private)
//   - Status history
//   - Investor count
//
// Success response (200 OK): Full property details
func (h *PropertyHandler) GetAdminProperty(c *gin.Context) {
	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	result, err := h.propertyService.GetAdminProperty(c.Request.Context(), propertyID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, http.StatusOK, result)
}

// ListAdminProperties handles GET /api/v1/admin/properties
//
// Requires: Admin role
//
// Same as public listing but includes draft properties and admin-only filters.
func (h *PropertyHandler) ListAdminProperties(c *gin.Context) {
	var filter dto.PropertyFilterRequest
	if err := c.ShouldBindQuery(&filter); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Admin endpoint - isAdmin = true
	result, err := h.propertyService.ListProperties(c.Request.Context(), filter, true)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, http.StatusOK, result)
}

// UpdateProperty handles PATCH /api/v1/admin/properties/:id
//
// Requires: Admin role
//
// Request body: Partial updates (all fields optional)
//   {
//     "title": "Updated Title",
//     "featured": true,
//     ...
//   }
//
// Success response (200 OK): Updated property details
//
// Note: Financial fields locked if property has investments
func (h *PropertyHandler) UpdateProperty(c *gin.Context) {
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	var req dto.UpdatePropertyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	result, err := h.propertyService.UpdateProperty(c.Request.Context(), adminID, propertyID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusOK, "Property updated successfully", result)
}

// DeleteProperty handles DELETE /api/v1/admin/properties/:id
//
// Requires: Admin role
//
// Soft deletes the property.
// Fails if property has investments (protects audit trail).
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Property deleted successfully"
//   }
//
// Error responses:
//   - 400 Bad Request: Invalid UUID
//   - 404 Not Found: Property doesn't exist
//   - 422 Unprocessable Entity: Property has investments
func (h *PropertyHandler) DeleteProperty(c *gin.Context) {
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	err = h.propertyService.DeleteProperty(c.Request.Context(), adminID, propertyID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusOK, "Property deleted successfully", nil)
}

// PublishProperty handles POST /api/v1/admin/properties/:id/publish
//
// Requires: Admin role
//
// Transitions property from draft to active status.
// Validates publication requirements before publishing.
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Property published successfully",
//     "data": {
//       "id": "...",
//       "status": "active",
//       "message": "Property published successfully",
//       "published_at": "2024-01-15T10:30:00Z"
//     }
//   }
//
// Error responses:
//   - 422 Unprocessable Entity: Publication requirements not met
func (h *PropertyHandler) PublishProperty(c *gin.Context) {
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	result, err := h.propertyService.PublishProperty(c.Request.Context(), adminID, propertyID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusOK, "Property published successfully", result)
}

// ═══════════════════════════════════════════════════════════════════════════
// IMAGE MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// UploadImage handles POST /api/v1/admin/properties/:id/images
//
// Requires: Admin role
//
// Content-Type: multipart/form-data
//
// Form fields:
//   - image: file (required) - Image file
//   - caption: string (optional) - Image caption
//   - display_order: int (optional) - Display order
//   - is_cover: bool (optional) - Set as cover image
//
// Success response (201 Created):
//   {
//     "success": true,
//     "message": "Image uploaded successfully",
//     "data": {
//       "image_id": "...",
//       "image_url": "https://res.cloudinary.com/...",
//       "is_cover": true,
//       "message": "Image uploaded successfully"
//     }
//   }
//
// Error responses:
//   - 400 Bad Request: No file provided
//   - 413 Payload Too Large: File exceeds 10MB
//   - 422 Unprocessable Entity: Invalid file type
func (h *PropertyHandler) UploadImage(c *gin.Context) {
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	// Get file from form
	file, err := c.FormFile("image")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "No image file provided")
		return
	}

	// Parse form data
	var req dto.UploadImageRequest
	if err := c.ShouldBind(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Upload image
	result, err := h.propertyService.UploadImage(c.Request.Context(), adminID, propertyID, file, req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusCreated, "Image uploaded successfully", result)
}

// DeleteImage handles DELETE /api/v1/admin/properties/:id/images/:imageId
//
// Requires: Admin role
//
// Deletes image from Cloudinary and database.
// If deleted image was cover, reassigns cover to first remaining image.
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Image deleted successfully"
//   }
func (h *PropertyHandler) DeleteImage(c *gin.Context) {
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	imageID, err := uuid.Parse(c.Param("imageId"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid image ID format")
		return
	}

	err = h.propertyService.DeleteImage(c.Request.Context(), adminID, propertyID, imageID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusOK, "Image deleted successfully", nil)
}

// SetCoverImage handles PATCH /api/v1/admin/properties/:id/images/:imageId/cover
//
// Requires: Admin role
//
// Sets specified image as the property's cover image.
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Cover image updated successfully"
//   }
func (h *PropertyHandler) SetCoverImage(c *gin.Context) {
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	imageID, err := uuid.Parse(c.Param("imageId"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid image ID format")
		return
	}

	err = h.propertyService.SetCoverImage(c.Request.Context(), adminID, propertyID, imageID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusOK, "Cover image updated successfully", nil)
}

// ═══════════════════════════════════════════════════════════════════════════
// DOCUMENT MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// UploadDocument handles POST /api/v1/admin/properties/:id/documents
//
// Requires: Admin role
//
// Content-Type: multipart/form-data
//
// Form fields:
//   - document: file (required) - Document file
//   - document_type: string (required) - certificate_of_ownership|title_deed|etc
//   - title: string (required) - Document title
//   - description: string (optional) - Document description
//   - is_public: bool (optional) - Make visible to public
//
// Success response (201 Created):
//   {
//     "success": true,
//     "message": "Document uploaded successfully",
//     "data": {
//       "document_id": "...",
//       "file_url": "https://res.cloudinary.com/...",
//       "message": "Document uploaded successfully"
//     }
//   }
func (h *PropertyHandler) UploadDocument(c *gin.Context) {
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	file, err := c.FormFile("document")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "No document file provided")
		return
	}

	var req dto.UploadDocumentRequest
	if err := c.ShouldBind(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	result, err := h.propertyService.UploadDocument(c.Request.Context(), adminID, propertyID, file, req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusCreated, "Document uploaded successfully", result)
}

// DeleteDocument handles DELETE /api/v1/admin/properties/:id/documents/:documentId
//
// Requires: Admin role
//
// Deletes document from Cloudinary and database.
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Document deleted successfully"
//   }
func (h *PropertyHandler) DeleteDocument(c *gin.Context) {
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	documentID, err := uuid.Parse(c.Param("documentId"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid document ID format")
		return
	}

	err = h.propertyService.DeleteDocument(c.Request.Context(), adminID, propertyID, documentID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusOK, "Document deleted successfully", nil)
}

// ToggleDocumentVisibility handles PATCH /api/v1/admin/properties/:id/documents/:documentId/visibility
//
// Requires: Admin role
//
// Request body:
//   {
//     "is_public": true
//   }
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Document visibility updated successfully"
//   }
func (h *PropertyHandler) ToggleDocumentVisibility(c *gin.Context) {
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	documentID, err := uuid.Parse(c.Param("documentId"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid document ID format")
		return
	}

	var req dto.UpdateDocumentVisibilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	err = h.propertyService.ToggleDocumentVisibility(c.Request.Context(), adminID, propertyID, documentID, req.IsPublic)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusOK, "Document visibility updated successfully", nil)
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════

// handleError maps service errors to HTTP status codes.
func (h *PropertyHandler) handleError(c *gin.Context, err error) {
	// Map errors to HTTP status codes
	switch {
	case errors.Is(err, apperrors.ErrPropertyNotFound):
		response.Error(c, http.StatusNotFound, "Property not found")
	case errors.Is(err, apperrors.ErrValidationFailed):
		response.Error(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, apperrors.ErrUnauthorized):
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
	case errors.Is(err, apperrors.ErrForbidden):
		response.Error(c, http.StatusForbidden, "Forbidden")
	default:
		// Check if it's an AppError with a code
		if appErr, ok := err.(*apperrors.AppError); ok {
			switch appErr.Code {
			case "validation_error":
				response.Error(c, http.StatusUnprocessableEntity, appErr.Message)
			case "business_logic_error":
				response.Error(c, http.StatusUnprocessableEntity, appErr.Message)
			case "not_found":
				response.Error(c, http.StatusNotFound, appErr.Message)
			case "forbidden":
				response.Error(c, http.StatusForbidden, appErr.Message)
			case "upload_error":
				response.Error(c, http.StatusBadRequest, appErr.Message)
			default:
				response.Error(c, http.StatusInternalServerError, "Internal server error")
			}
		} else {
			// Unknown error
			response.Error(c, http.StatusInternalServerError, "Internal server error")
		}
	}
}

package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// NotificationHandler handles all notification-related HTTP requests.
//
// Responsibilities:
//   - Parse and validate HTTP request bodies
//   - Call appropriate service methods
//   - Transform service responses into HTTP responses
//   - Handle errors and return appropriate status codes
//   - Enforce user ownership (users can only access their own notifications)
//
// Endpoints:
//   GET    /notifications          - List user's notifications (paginated)
//   GET    /notifications/:id      - Get single notification
//   PATCH  /notifications/:id/read - Mark notification as read
//   PATCH  /notifications/read-all - Mark all notifications as read
//   DELETE /notifications/:id      - Delete notification
//   GET    /preferences            - Get notification preferences
//   PUT    /preferences/:type      - Update type-specific preference
//   PUT    /preferences            - Batch update preferences
//
// All endpoints require authentication (enforced by middleware).
type NotificationHandler struct {
	notificationService   services.NotificationService
	preferenceService     services.NotificationPreferenceService
}

// NewNotificationHandler creates a new notification handler.
//
// Parameters:
//   - notificationService: Service for notification operations
//   - preferenceService: Service for preference management
//
// Returns:
//   - *NotificationHandler: Initialized handler
func NewNotificationHandler(
	notificationService services.NotificationService,
	preferenceService services.NotificationPreferenceService,
) *NotificationHandler {
	return &NotificationHandler{
		notificationService: notificationService,
		preferenceService:   preferenceService,
	}
}

// ListNotifications handles GET /api/v1/notifications
//
// Query parameters:
//   - page: Page number (default: 1)
//   - page_size: Items per page (default: 20, max: 100)
//   - unread_only: Filter to unread notifications (default: false)
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Notifications retrieved successfully",
//     "data": {
//       "notifications": [...],
//       "pagination": {
//         "page": 1,
//         "page_size": 20,
//         "total_items": 150,
//         "total_pages": 8
//       }
//     }
//   }
//
// Example curl:
//   curl -X GET http://localhost:8080/api/v1/notifications?page=1&page_size=20&unread_only=true \
//     -H "Authorization: Bearer <token>"
func (h *NotificationHandler) ListNotifications(c *gin.Context) {
	// Extract user ID from context (set by Auth middleware)
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	unreadOnly := c.Query("unread_only") == "true"

	// Validate and normalize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Call service layer (service returns *dto.NotificationListResponse and total count)
	notificationList, total, err := h.notificationService.List(c.Request.Context(), userID, unreadOnly, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Build pagination metadata
	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)
	if totalPages == 0 {
		totalPages = 1
	}
	pagination := map[string]interface{}{
		"page":        page,
		"page_size":   pageSize,
		"total_items": total,
		"total_pages": totalPages,
	}

	// Build response
	responseData := map[string]interface{}{
		"notifications": notificationList.Notifications,
		"unread_count":  notificationList.UnreadCount,
		"pagination":    pagination,
	}

	// Success response
	response.Success(c, http.StatusOK, responseData)
}

// MarkAsRead handles PATCH /api/v1/notifications/:id/read
//
// Path parameters:
//   - id: Notification UUID
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Notification marked as read"
//   }
//
// Error responses:
//   - 400 Bad Request: Invalid notification ID format
//   - 404 Not Found: Notification not found or not owned by user
//
// Example curl:
//   curl -X PATCH http://localhost:8080/api/v1/notifications/550e8400-e29b-41d4-a716-446655440000/read \
//     -H "Authorization: Bearer <token>"
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	// Extract user ID from context
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Parse notification ID from path
	notificationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid notification ID format")
		return
	}

	// Call service layer (service enforces ownership with userID, id order)
	err = h.notificationService.MarkRead(c.Request.Context(), userID, notificationID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Success response
	response.SuccessWithMessage(c, http.StatusOK, "Notification marked as read")
}

// MarkAllAsRead handles PATCH /api/v1/notifications/read-all
//
// Marks all of the user's notifications as read.
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "All notifications marked as read"
//   }
//
// Example curl:
//   curl -X PATCH http://localhost:8080/api/v1/notifications/read-all \
//     -H "Authorization: Bearer <token>"
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	// Extract user ID from context
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Call service layer
	err = h.notificationService.MarkAllRead(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Success response
	response.SuccessWithMessage(c, http.StatusOK, "All notifications marked as read")
}

// DeleteNotification handles DELETE /api/v1/notifications/:id
//
// Path parameters:
//   - id: Notification UUID
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Notification deleted successfully"
//   }
//
// Error responses:
//   - 400 Bad Request: Invalid notification ID format
//   - 404 Not Found: Notification not found or not owned by user
//
// Example curl:
//   curl -X DELETE http://localhost:8080/api/v1/notifications/550e8400-e29b-41d4-a716-446655440000 \
//     -H "Authorization: Bearer <token>"
func (h *NotificationHandler) DeleteNotification(c *gin.Context) {
	// Extract user ID from context
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Parse notification ID from path
	notificationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid notification ID format")
		return
	}

	// Call service layer (service enforces ownership with userID, id order)
	err = h.notificationService.Delete(c.Request.Context(), userID, notificationID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Success response
	response.SuccessWithMessage(c, http.StatusOK, "Notification deleted successfully")
}

// GetPreferences handles GET /api/v1/notifications/preferences
//
// Retrieves user's notification preferences for all notification types.
//
// Success response (200 OK):
//   {
//     "success": true,
//     "data": {
//       "preferences": [
//         {
//           "notification_type": "investment_confirmed",
//           "in_app_enabled": true,
//           "email_enabled": true,
//           "sms_enabled": false
//         },
//         ...
//       ]
//     }
//   }
//
// Example curl:
//   curl -X GET http://localhost:8080/api/v1/notifications/preferences \
//     -H "Authorization: Bearer <token>"
func (h *NotificationHandler) GetPreferences(c *gin.Context) {
	// Extract user ID from context
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Call service layer
	preferencesResponse, err := h.preferenceService.GetPreferences(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Success response (service already returns the full response)
	response.Success(c, http.StatusOK, preferencesResponse)
}

// UpdatePreference handles PUT /api/v1/notifications/preferences/:type
//
// Path parameters:
//   - type: Notification type (e.g., "investment_confirmed", "wallet_credited")
//
// Request body:
//   {
//     "in_app_enabled": true,
//     "email_enabled": false,
//     "sms_enabled": false
//   }
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Preference updated successfully",
//     "data": {
//       "notification_type": "investment_confirmed",
//       "in_app_enabled": true,
//       "email_enabled": false,
//       "sms_enabled": false
//     }
//   }
//
// Error responses:
//   - 400 Bad Request: Validation failed (invalid type or no channels enabled)
//
// Example curl:
//   curl -X PUT http://localhost:8080/api/v1/notifications/preferences/investment_confirmed \
//     -H "Authorization: Bearer <token>" \
//     -H "Content-Type: application/json" \
//     -d '{"in_app_enabled":true,"email_enabled":false,"sms_enabled":false}'
func (h *NotificationHandler) UpdatePreference(c *gin.Context) {
	// Extract user ID from context
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Get notification type from path
	notificationType := c.Param("type")
	if notificationType == "" {
		response.Error(c, http.StatusBadRequest, "Notification type is required")
		return
	}

	// Parse and validate request body
	var req dto.UpdateNotificationPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Set the notification type from the path parameter
	req.NotificationType = &notificationType

	// Call service layer
	preferenceResponse, err := h.preferenceService.UpdatePreference(c.Request.Context(), userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Success response (service already returns the response DTO)
	response.SuccessWithMessage(c, http.StatusOK, "Preference updated successfully", preferenceResponse)
}

// BatchUpdatePreferences handles PUT /api/v1/notifications/preferences
//
// Updates multiple notification preferences in a single request.
//
// Request body:
//   {
//     "preferences": [
//       {
//         "notification_type": "investment_confirmed",
//         "in_app_enabled": true,
//         "email_enabled": true,
//         "sms_enabled": false
//       },
//       {
//         "notification_type": "wallet_credited",
//         "in_app_enabled": true,
//         "email_enabled": false,
//         "sms_enabled": true
//       }
//     ]
//   }
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Preferences updated successfully",
//     "data": {
//       "preferences": [...]
//     }
//   }
//
// Error responses:
//   - 400 Bad Request: Validation failed
//
// Example curl:
//   curl -X PUT http://localhost:8080/api/v1/notifications/preferences \
//     -H "Authorization: Bearer <token>" \
//     -H "Content-Type: application/json" \
//     -d '{"preferences":[{"notification_type":"investment_confirmed","in_app_enabled":true,"email_enabled":true,"sms_enabled":false}]}'
func (h *NotificationHandler) BatchUpdatePreferences(c *gin.Context) {
	// Extract user ID from context
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Parse and validate request body
	var req struct {
		Preferences []dto.UpdateNotificationPreferenceRequest `json:"preferences" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Update each preference
	updatedPreferences := make([]dto.NotificationPreferenceResponse, 0, len(req.Preferences))
	for _, p := range req.Preferences {
		preferenceResponse, err := h.preferenceService.UpdatePreference(c.Request.Context(), userID, p)
		if err != nil {
			response.Error(c, err)
			return
		}
		updatedPreferences = append(updatedPreferences, *preferenceResponse)
	}

	// Build response
	responseData := map[string]interface{}{
		"preferences": updatedPreferences,
	}

	// Success response
	response.SuccessWithMessage(c, http.StatusOK, "Preferences updated successfully", responseData)
}

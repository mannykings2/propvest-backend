package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// AdminMessagingHandler handles admin bulk messaging endpoints.
//
// Responsibilities:
//   - Send bulk emails/SMS to users (single, segment, or all users)
//   - Apply admin authorization (middleware)
//   - Validate bulk message requests (size limits, targeting)
//   - Delegate to AdminMessagingService for execution
//
// Endpoints:
//   POST /api/v1/admin/messaging/send - Send bulk message
//
// Authorization:
//   - All endpoints require admin role (enforced by middleware)
//
// Safety limits:
//   - Max 10,000 users per broadcast
//   - Explicit confirmation required for "all users" targeting
type AdminMessagingHandler struct {
	messagingService services.AdminMessagingService
}

// NewAdminMessagingHandler creates a new admin messaging handler.
//
// Parameters:
//   - messagingService: Service for bulk messaging operations
//
// Returns:
//   - *AdminMessagingHandler: Initialized handler
func NewAdminMessagingHandler(messagingService services.AdminMessagingService) *AdminMessagingHandler {
	return &AdminMessagingHandler{
		messagingService: messagingService,
	}
}

// SendBulkMessage handles POST /api/v1/admin/messaging/send
//
// Request body:
//   {
//     "message_type": "email",              // "email", "sms", or "both"
//     "targeting": {
//       "mode": "single",                    // "single", "segment", or "all"
//       "user_ids": ["uuid1", "uuid2"],      // For "single" mode
//       "filters": {                         // For "segment" mode
//         "kyc_status": "approved",
//         "has_investments": true
//       },
//       "confirm_all_users": true            // Required for "all" mode
//     },
//     "content": {
//       "subject": "Important Update",       // For email (optional for SMS)
//       "body": "Message content...",        // Required
//       "template_id": "investment_update"   // Optional, overrides subject/body
//     }
//   }
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Bulk message sent successfully",
//     "data": {
//       "message_id": "uuid",
//       "targets_count": 150,
//       "email_queued": 150,      // If message_type includes email
//       "sms_queued": 150,        // If message_type includes sms
//       "in_app_created": 150     // Always created for tracking
//     }
//   }
//
// Error responses:
//   - 400 Bad Request: Validation failed (invalid targeting, missing content, etc.)
//   - 403 Forbidden: Not an admin
//   - 422 Unprocessable Entity: Targeting filters matched no users
//   - 500 Internal Server Error: Unexpected failure
//
// Validation rules:
//   - message_type: Must be "email", "sms", or "both"
//   - targeting.mode: Must be "single", "segment", or "all"
//   - targeting.user_ids: Required for "single" mode, max 10,000 IDs
//   - targeting.confirm_all_users: Required true for "all" mode
//   - content.body: Required (min 10 chars, max 5000 chars)
//   - content.subject: Required for email (max 200 chars)
//
// Security notes:
//   - Admin role required (middleware enforced)
//   - "all" mode requires explicit confirmation to prevent accidents
//   - Max 10,000 users per request (service-enforced)
//   - Rate limiting recommended (prevent admin abuse)
//
// Example curl:
//   curl -X POST http://localhost:8080/api/v1/admin/messaging/send \
//     -H "Content-Type: application/json" \
//     -H "Authorization: Bearer <admin_token>" \
//     -d '{
//       "message_type": "both",
//       "targeting": {
//         "mode": "segment",
//         "filters": {"kyc_status": "approved", "has_investments": true}
//       },
//       "content": {
//         "subject": "New Investment Opportunity",
//         "body": "Dear investor, we have a new property available..."
//       }
//     }'
func (h *AdminMessagingHandler) SendBulkMessage(c *gin.Context) {
	// Step 1: Extract admin ID from context (set by Auth + RequireRole middleware)
	adminID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Step 2: Parse and validate request
	var req dto.SendBulkMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Step 3: Call service to send bulk message
	// Service handles:
	//   - Resolving target users (by IDs, filters, or all)
	//   - Creating in-app notifications for tracking
	//   - Queueing email/SMS dispatch via RabbitMQ
	//   - Enforcing safety limits (max 10k users)
	result, err := h.messagingService.SendBulkMessage(c.Request.Context(), adminID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Step 4: Return success response
	response.SuccessWithMessage(c, http.StatusOK, "Bulk message sent successfully", result)
}

// handleError maps service errors to HTTP status codes.
//
// Error mappings:
//   - ErrValidation          -> 422 Unprocessable Entity
//   - ErrNotFound            -> 404 Not Found
//   - ErrNoTargets           -> 422 Unprocessable Entity (targeting matched no users)
//   - ErrTargetLimitExceeded -> 422 Unprocessable Entity (too many users)
//   - Other errors           -> 500 Internal Server Error
func (h *AdminMessagingHandler) handleError(c *gin.Context, err error) {
	// Map service errors to HTTP responses
	// The response package handles this with sensible defaults
	response.Error(c, err)
}

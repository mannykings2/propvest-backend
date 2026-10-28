package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// AdminHealthHandler handles system health monitoring endpoints.
type AdminHealthHandler struct {
	healthService services.AdminHealthService
}

// NewAdminHealthHandler creates a new admin health handler.
func NewAdminHealthHandler(healthService services.AdminHealthService) *AdminHealthHandler {
	return &AdminHealthHandler{
		healthService: healthService,
	}
}

// CheckSystemHealth handles GET /api/v1/admin/health/system
//
// RESPONSE: 200 OK with system health status
// SECURITY: No secrets, DSNs, or internal topology exposed
//
// EXAMPLE RESPONSE:
//
//	{
//	  "status": "healthy",
//	  "checked_at": "2024-10-06T12:00:00Z",
//	  "components": {
//	    "postgresql": {
//	      "status": "healthy",
//	      "message": "database connection ok",
//	      "latency_ms": 15
//	    },
//	    "rabbitmq": {
//	      "status": "healthy",
//	      "message": "rabbitmq configured"
//	    },
//	    "paystack": {
//	      "status": "healthy",
//	      "message": "paystack configured"
//	    },
//	    "cloudinary": {
//	      "status": "degraded",
//	      "message": "cloudinary not configured"
//	    },
//	    "sms_provider": {
//	      "status": "healthy",
//	      "message": "sms provider configured"
//	    }
//	  }
//	}
func (h *AdminHealthHandler) CheckSystemHealth(c *gin.Context) {
	// Get actor from context (set by auth middleware)
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Call service
	healthStatus, err := h.healthService.CheckSystemHealth(c.Request.Context(), actorUUID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "failed to check system health: "+err.Error())
		return
	}

	// Return appropriate HTTP status based on health
	var httpStatus int
	switch healthStatus.Status {
	case "healthy":
		httpStatus = http.StatusOK
	case "degraded":
		httpStatus = http.StatusOK // Still 200, but degraded
	case "unhealthy":
		httpStatus = http.StatusServiceUnavailable // 503
	default:
		httpStatus = http.StatusOK
	}

	response.Success(c, httpStatus, "system health check complete", healthStatus)
}

// CheckQueueHealth handles GET /api/v1/admin/health/queues
//
// RESPONSE: 200 OK with queue health metrics (or unsupported message)
//
// EXAMPLE RESPONSE (when supported):
//
//	{
//	  "status": "healthy",
//	  "checked_at": "2024-10-06T12:00:00Z",
//	  "queues": {
//	    "email.queue": {
//	      "depth": 5,
//	      "consumer_status": "active",
//	      "oldest_message_age_seconds": 30
//	    },
//	    "sms.queue": {
//	      "depth": 2,
//	      "consumer_status": "active",
//	      "oldest_message_age_seconds": 15
//	    }
//	  }
//	}
//
// EXAMPLE RESPONSE (when unsupported):
//
//	{
//	  "status": "unsupported",
//	  "checked_at": "2024-10-06T12:00:00Z"
//	}
func (h *AdminHealthHandler) CheckQueueHealth(c *gin.Context) {
	// Get actor from context
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Call service
	queueStatus, err := h.healthService.CheckQueueHealth(c.Request.Context(), actorUUID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "failed to check queue health: "+err.Error())
		return
	}

	response.Success(c, http.StatusOK, "queue health check complete", queueStatus)
}

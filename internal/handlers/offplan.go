package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// OffplanHandler handles the off-plan module's HTTP requests (Phase 2, B).
type OffplanHandler struct {
	offplanService services.OffplanService
}

// NewOffplanHandler constructs the handler.
func NewOffplanHandler(offplanService services.OffplanService) *OffplanHandler {
	return &OffplanHandler{offplanService: offplanService}
}

// ListProjects handles GET /api/v1/offplan/projects (public).
//
// IMPORTANT: returns a BARE ARRAY as the data payload. OffPlan.jsx does
// `setProjectsList(data || [])`, so wrapping in {projects: [...]} would crash
// the page — response.Success unwraps to `data`, which must itself be the list.
func (h *OffplanHandler) ListProjects(c *gin.Context) {
	projects, err := h.offplanService.ListProjects(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, projects)
}

// GetProject handles GET /api/v1/offplan/projects/:id (public).
func (h *OffplanHandler) GetProject(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid project ID format")
		return
	}

	project, err := h.offplanService.GetProject(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, project)
}

// Subscribe handles POST /api/v1/offplan/projects/:id/subscribe (authenticated).
//
// Body: { "plan_id": "12m", "finish_type_id": "full" }  (snake_case)
// Debits the wallet (down payment + 2% fee) and records the subscription.
func (h *OffplanHandler) Subscribe(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid project ID format")
		return
	}

	var req dto.SubscribeOffplanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	sub, err := h.offplanService.Subscribe(c.Request.Context(), userID, projectID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessWithMessage(c, http.StatusCreated, "Subscription created", sub)
}

// ListSubscriptions handles GET /api/v1/offplan/subscriptions (authenticated).
func (h *OffplanHandler) ListSubscriptions(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	subs, err := h.offplanService.ListSubscriptions(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, subs)
}

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

// SupportHandler serves the user-facing ticket routes (Phase 4.2).
// Every method is ownership-scoped: the service looks tickets up by
// (id, user_id), so another user's ticket is simply "not found".
//
// Endpoints (all behind Auth):
//
//	POST   /api/v1/support/tickets
//	GET    /api/v1/support/tickets?status=&page=&page_size=
//	GET    /api/v1/support/tickets/:id
//	POST   /api/v1/support/tickets/:id/replies
type SupportHandler struct {
	supportService services.SupportService
}

// NewSupportHandler creates the user-facing handler.
func NewSupportHandler(supportService services.SupportService) *SupportHandler {
	return &SupportHandler{supportService: supportService}
}

// parseTicketID converts the :id path param, answering 400 on garbage so a
// malformed UUID never reaches the service as a 500.
func parseTicketID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid ticket ID", "invalid_ticket_id")
		return uuid.Nil, false
	}
	return id, true
}

// pageParams normalises page/page_size with the module-wide defaults.
func pageParams(c *gin.Context) (page, size int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

// CreateTicket handles POST /api/v1/support/tickets
func (h *SupportHandler) CreateTicket(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req dto.CreateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Field-level 422 (field → messages) so the form can show which input
		// is wrong instead of one opaque banner.
		response.ValidationError(c, err)
		return
	}

	ticket, err := h.supportService.Create(c.Request.Context(), userID, &req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Ticket created successfully", ticket)
}

// ListTickets handles GET /api/v1/support/tickets
func (h *SupportHandler) ListTickets(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	page, size := pageParams(c)
	list, err := h.supportService.List(c.Request.Context(), userID, c.Query("status"), page, size)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Tickets retrieved successfully", list)
}

// GetTicket handles GET /api/v1/support/tickets/:id
func (h *SupportHandler) GetTicket(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, ok := parseTicketID(c)
	if !ok {
		return
	}

	ticket, err := h.supportService.Get(c.Request.Context(), userID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Ticket retrieved successfully", ticket)
}

// ReplyTicket handles POST /api/v1/support/tickets/:id/replies
func (h *SupportHandler) ReplyTicket(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, ok := parseTicketID(c)
	if !ok {
		return
	}

	var req dto.ReplyTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	reply, err := h.supportService.Reply(c.Request.Context(), userID, id, req.Message)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Reply added successfully", reply)
}

// ── Agent queue ─────────────────────────────────────────────────────────────

// AdminSupportHandler serves the staff/admin side of the ticket queue.
// RBAC (staff/admin/super_admin) is enforced at the route.
//
//	Endpoints:
//	  GET    /api/v1/admin/support/tickets
//	  GET    /api/v1/admin/support/tickets/:id
//	  POST   /api/v1/admin/support/tickets/:id/replies
//	  PATCH  /api/v1/admin/support/tickets/:id/status
type AdminSupportHandler struct {
	supportService services.SupportService
}

// NewAdminSupportHandler creates the agent-side handler.
func NewAdminSupportHandler(supportService services.SupportService) *AdminSupportHandler {
	return &AdminSupportHandler{supportService: supportService}
}

// AdminListTickets handles GET /api/v1/admin/support/tickets
func (h *AdminSupportHandler) AdminListTickets(c *gin.Context) {
	page, size := pageParams(c)
	list, err := h.supportService.AdminList(
		c.Request.Context(), c.Query("status"), c.Query("category"), page, size)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Tickets retrieved successfully", list)
}

// AdminGetTicket handles GET /api/v1/admin/support/tickets/:id
func (h *AdminSupportHandler) AdminGetTicket(c *gin.Context) {
	id, ok := parseTicketID(c)
	if !ok {
		return
	}

	ticket, err := h.supportService.AdminGet(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Ticket retrieved successfully", ticket)
}

// AdminReplyTicket handles POST /api/v1/admin/support/tickets/:id/replies
func (h *AdminSupportHandler) AdminReplyTicket(c *gin.Context) {
	agentID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, ok := parseTicketID(c)
	if !ok {
		return
	}

	var req dto.ReplyTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	reply, err := h.supportService.AdminReply(c.Request.Context(), agentID, id, req.Message)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Reply added successfully", reply)
}

// AdminUpdateStatus handles PATCH /api/v1/admin/support/tickets/:id/status
func (h *AdminSupportHandler) AdminUpdateStatus(c *gin.Context) {
	agentID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, ok := parseTicketID(c)
	if !ok {
		return
	}

	var req dto.UpdateTicketStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	ticket, err := h.supportService.AdminUpdateStatus(c.Request.Context(), agentID, id, req.Status)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Ticket status updated", ticket)
}

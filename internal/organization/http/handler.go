package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type OrganizationHandler struct {
	service organization.Service
}

func NewHandler(service organization.Service) *OrganizationHandler {
	return &OrganizationHandler{service: service}
}

// List retrieves a paginated list of active organizations.
// It supports standard pagination parameters.
func (h *OrganizationHandler) List(c *gin.Context) {
	var req ListOrganizationsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filter := organization.OrganizationFilter{
		Page:      req.Page,
		PageSize:  req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	if filter.SortBy == "" {
		filter.SortBy = "created_at"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "DESC"
	} else {
		filter.SortOrder = strings.ToUpper(filter.SortOrder)
	}

	orgs, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]OrganizationResponse, len(orgs))
	for i, o := range orgs {
		items[i] = NewOrganizationResponse(o)
	}

	resp := response.NewPageResponse(items, req.Page, req.PageSize, total)

	c.JSON(http.StatusOK, resp)
}

// Create adds a new organization to the system.
// Access Control: System Admin only.
func (h *OrganizationHandler) Create(c *gin.Context) {
	var req CreateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	org, err := h.service.Create(c.Request.Context(), req.Name, req.OwnerID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, NewOrganizationResponse(org))
}

// Get retrieves detailed information about a specific organization by its ID.
func (h *OrganizationHandler) Get(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	org, err := h.service.GetByID(c.Request.Context(), req.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewOrganizationResponse(org))
}

// Update modifies specific attributes of an organization.
// It supports partial updates via a JSON body.
// Access Control: System Admin only.
func (h *OrganizationHandler) Update(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Bind to HTTP DTO
	var body UpdateOrganizationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Map HTTP DTO to Service DTO
	req := organization.UpdateOrganizationRequest{
		Name:     body.Name,
		IsActive: body.IsActive,
		OwnerID:  body.OwnerID,
	}

	org, err := h.service.Update(c.Request.Context(), uri.ID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewOrganizationResponse(org))
}

// Delete performs a soft delete on an organization, marking it as inactive.
// Access Control: System Admin only.
func (h *OrganizationHandler) Delete(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	if err := h.service.Delete(c.Request.Context(), req.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListManagers retrieves managers of an organization.
func (h *OrganizationHandler) ListManagers(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// We can't filter by role anymore since we only return managers
	// but we assume ListMembersRequest might still be used for pagination or removed/simplified
	// For now, let's keep basic pagination.
	// NOTE: ListMembersRequest struct in DTO has SortBy which may include 'role', but now all are managers.

	userID := auth.GetUserID(c)
	hasPerm, err := h.service.CheckPermission(c.Request.Context(), uri.ID, userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	if !hasPerm {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		return
	}

	members, err := h.service.ListOrganizationManagers(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Manual pagination since service method changed (ListOrganizationManagers returns slice, no total/paging currently in repo)
	// We return simple list for now.

	items := make([]ManagerResponse, len(members))
	for i, m := range members {
		items[i] = NewManagerResponse(m)
	}

	c.JSON(http.StatusOK, gin.H{"data": items})
}

// AddManager adds a manager to an organization.
// Access Control: System Admin or Organization Owner.
func (h *OrganizationHandler) AddManager(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var body AddOrganizationManagerRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(body.UserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user UUID"})
		return
	}

	actorID := auth.GetUserID(c)
	// Permission check: Must be Owner or SysAdmin to add managers.
	// CheckIsOwner is strict owner check.
	isOwner, err := h.service.CheckIsOwner(c.Request.Context(), uri.ID, actorID)
	if err != nil {
		response.Error(c, err)
		return
	}
	if !isOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied: only owner can add managers"})
		return
	}

	if err := h.service.AddOrganizationManager(c.Request.Context(), uri.ID, body.UserID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusCreated)
}

// RemoveManager removes a manager from an organization.
// Access Control: System Admin or Organization Owner.
func (h *OrganizationHandler) RemoveManager(c *gin.Context) {
	var req OrgMemberRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	actorID := auth.GetUserID(c)
	isOwner, err := h.service.CheckIsOwner(c.Request.Context(), req.ID, actorID)
	if err != nil {
		response.Error(c, err)
		return
	}
	if !isOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied: only owner can remove managers"})
		return
	}

	if err := h.service.RemoveOrganizationManager(c.Request.Context(), req.ID, req.UserID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

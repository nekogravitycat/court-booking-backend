package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type OrganizationHandler struct {
	service organization.Service
}

func NewOrganizationHandler(service organization.Service) *OrganizationHandler {
	return &OrganizationHandler{service: service}
}

// List retrieves a paginated list of active organizations.
// It supports standard pagination parameters.
func (h *OrganizationHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := organization.OrganizationFilter{
		Page:     page,
		PageSize: pageSize,
	}

	orgs, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list organizations"})
		return
	}

	items := make([]OrganizationResponse, len(orgs))
	for i, o := range orgs {
		items[i] = NewOrganizationResponse(o)
	}

	resp := response.NewPageResponse(items, page, pageSize, total)

	c.JSON(http.StatusOK, resp)
}

// Create adds a new organization to the system.
// Access Control: System Admin only.
func (h *OrganizationHandler) Create(c *gin.Context) {
	var req CreateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	org, err := h.service.Create(c.Request.Context(), req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create organization"})
		return
	}

	c.JSON(http.StatusCreated, NewOrganizationResponse(org))
}

// Get retrieves detailed information about a specific organization by its ID.
func (h *OrganizationHandler) Get(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	org, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == organization.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get organization"})
		return
	}

	c.JSON(http.StatusOK, NewOrganizationResponse(org))
}

// Update modifies specific attributes of an organization.
// It supports partial updates via a JSON body.
// Access Control: System Admin only.
func (h *OrganizationHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	// Bind to HTTP DTO
	var body UpdateOrganizationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Map HTTP DTO to Service DTO
	req := organization.UpdateOrganizationRequest{
		Name:     body.Name,
		IsActive: body.IsActive,
	}

	org, err := h.service.Update(c.Request.Context(), id, req)
	if err != nil {
		if err == organization.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		// Handle validation error (e.g. empty name)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, NewOrganizationResponse(org))
}

// Delete performs a soft delete on an organization, marking it as inactive.
// Access Control: System Admin only.
func (h *OrganizationHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		if err == organization.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete organization"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListMembers retrieves members of an organization.
func (h *OrganizationHandler) ListMembers(c *gin.Context) {
	orgID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := organization.MemberFilter{
		Page:     page,
		PageSize: pageSize,
	}

	members, total, err := h.service.ListMembers(c.Request.Context(), orgID, filter)
	if err != nil {
		if err == organization.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
		return
	}

	items := make([]MemberResponse, len(members))
	for i, m := range members {
		items[i] = NewMemberResponse(m)
	}

	resp := response.NewPageResponse(items, page, pageSize, total)
	c.JSON(http.StatusOK, resp)
}

// AddMember adds a user to an organization.
// Access Control: System Admin (for now).
func (h *OrganizationHandler) AddMember(c *gin.Context) {
	orgID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	var body AddMemberRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req := organization.AddMemberRequest{
		UserID: body.UserID,
		Role:   body.Role,
	}

	if err := h.service.AddMember(c.Request.Context(), orgID, req); err != nil {
		switch err {
		case organization.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
		case organization.ErrUserAlreadyMember:
			c.JSON(http.StatusConflict, gin.H{"error": "user is already a member"})
		case organization.ErrUserNotFound:
			c.JSON(http.StatusBadRequest, gin.H{"error": "user not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add member"})
		}
		return
	}

	c.Status(http.StatusCreated)
}

// UpdateMemberRole modifies a member's role.
// Access Control: System Admin (for now).
func (h *OrganizationHandler) UpdateMemberRole(c *gin.Context) {
	orgID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var body UpdateMemberRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req := organization.UpdateMemberRequest{
		Role: body.Role,
	}

	if err := h.service.UpdateMemberRole(c.Request.Context(), orgID, userID, req); err != nil {
		if err == organization.ErrNotFound {
			// Could be org not found or user not member of org
			c.JSON(http.StatusNotFound, gin.H{"error": "member or organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update member role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// RemoveMember removes a user from an organization.
// Access Control: System Admin (for now).
func (h *OrganizationHandler) RemoveMember(c *gin.Context) {
	orgID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	if err := h.service.RemoveMember(c.Request.Context(), orgID, userID); err != nil {
		if err == organization.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "member or organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove member"})
		return
	}

	c.Status(http.StatusNoContent)
}

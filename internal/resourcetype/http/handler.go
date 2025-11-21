package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/resourcetype"
)

type Handler struct {
	service    resourcetype.Service
	orgService organization.Service
}

func NewHandler(service resourcetype.Service, orgService organization.Service) *Handler {
	return &Handler{
		service:    service,
		orgService: orgService,
	}
}

// checkPermission checks if the user is an admin or owner of the organization.
func (h *Handler) checkPermission(c *gin.Context, orgID string) bool {
	userID := auth.GetUserID(c)
	if userID == "" {
		return false
	}

	member, err := h.orgService.GetMember(c.Request.Context(), orgID, userID)
	if err != nil {
		return false
	}

	return member.Role == organization.RoleOwner || member.Role == organization.RoleAdmin
}

func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	orgID := c.Query("organization_id")

	filter := resourcetype.Filter{
		OrganizationID: orgID,
		Page:           page,
		PageSize:       pageSize,
	}

	rts, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list resource types"})
		return
	}

	items := make([]ResourceTypeResponse, len(rts))
	for i, rt := range rts {
		items[i] = NewResponse(rt)
	}

	resp := response.NewPageResponse(items, page, pageSize, total)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Create(c *gin.Context) {
	var body CreateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Permission check
	if !h.checkPermission(c, body.OrganizationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only organization admins can create resource types"})
		return
	}

	req := resourcetype.CreateRequest{
		OrganizationID: body.OrganizationID,
		Name:           body.Name,
		Description:    body.Description,
	}

	rt, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create resource type"})
		return
	}

	c.JSON(http.StatusCreated, NewResponse(rt))
}

func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	rt, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, resourcetype.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "resource type not found"})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get resource type"})
			return
		}
	}

	c.JSON(http.StatusOK, NewResponse(rt))
}

func (h *Handler) Update(c *gin.Context) {
	id := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	// Fetch existing to check Org ID for permissions
	existingRT, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, resourcetype.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "resource type not found"})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch resource type"})
			return
		}
	}

	// Permission check
	if !h.checkPermission(c, existingRT.OrganizationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
		return
	}

	var body UpdateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req := resourcetype.UpdateRequest{
		Name:        body.Name,
		Description: body.Description,
	}

	rt, err := h.service.Update(c.Request.Context(), id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update resource type"})
		return
	}

	c.JSON(http.StatusOK, NewResponse(rt))
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	// Fetch existing to check Org ID for permissions
	existingRT, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, resourcetype.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "resource type not found"})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch resource type"})
			return
		}
	}

	// Permission check
	if !h.checkPermission(c, existingRT.OrganizationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete resource type"})
		return
	}

	c.Status(http.StatusNoContent)
}

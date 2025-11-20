package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
)

type Handler struct {
	service    resource.Service
	locService location.Service
	orgService organization.Service
}

func NewHandler(service resource.Service, locService location.Service, orgService organization.Service) *Handler {
	return &Handler{
		service:    service,
		locService: locService,
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
	locationID := c.Query("location_id")
	resourceTypeID := c.Query("resource_type_id")

	filter := resource.Filter{
		LocationID:     locationID,
		ResourceTypeID: resourceTypeID,
		Page:           page,
		PageSize:       pageSize,
	}

	resources, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list resources"})
		return
	}

	items := make([]Response, len(resources))
	for i, r := range resources {
		items[i] = NewResponse(r)
	}

	resp := response.NewPageResponse(items, page, pageSize, total)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Create(c *gin.Context) {
	var body CreateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Permission Check Flow:
	// 1. Get Location to find OrganizationID
	loc, err := h.locService.GetByID(c.Request.Context(), body.LocationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid location"})
		return
	}

	// 2. Check User Permission for that Org
	if !h.checkPermission(c, loc.OrganizationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only organization admins can create resources"})
		return
	}

	req := resource.CreateRequest{
		Name:           body.Name,
		LocationID:     body.LocationID,
		ResourceTypeID: body.ResourceTypeID,
	}

	res, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, resource.ErrInvalidLocation),
			errors.Is(err, resource.ErrInvalidResourceType),
			errors.Is(err, resource.ErrOrgMismatch),
			errors.Is(err, resource.ErrEmptyName):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create resource"})
		}
		return
	}

	c.JSON(http.StatusCreated, NewResponse(res))
}

func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")

	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	res, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, resource.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get resource"})
		}
		return
	}

	c.JSON(http.StatusOK, NewResponse(res))
}

func (h *Handler) Update(c *gin.Context) {
	id := c.Param("id")

	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	// Permission Check Flow:
	// 1. Get Resource to find Location
	existingRes, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, resource.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch resource"})
			return
		}
	}

	// 2. Get Location to find Org
	loc, err := h.locService.GetByID(c.Request.Context(), existingRes.LocationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "associated location not found"})
		return
	}

	// 3. Check Permissions
	if !h.checkPermission(c, loc.OrganizationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
		return
	}

	var body UpdateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req := resource.UpdateRequest{
		Name: body.Name,
	}

	res, err := h.service.Update(c.Request.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, resource.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, resource.ErrEmptyName):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update resource"})
		}
		return
	}

	c.JSON(http.StatusOK, NewResponse(res))
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")

	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	// Permission Check Flow
	existingRes, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, resource.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch resource"})
		}
		return
	}

	loc, err := h.locService.GetByID(c.Request.Context(), existingRes.LocationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "associated location not found"})
		return
	}

	if !h.checkPermission(c, loc.OrganizationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: permission denied"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete resource"})
		return
	}

	c.Status(http.StatusNoContent)
}

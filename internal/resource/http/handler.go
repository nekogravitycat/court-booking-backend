package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
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
	var req ListResourcesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filter := resource.Filter{
		LocationID:     req.LocationID,
		ResourceTypeID: req.ResourceTypeID,
		Page:           req.Page,
		PageSize:       req.PageSize,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
	}

	if filter.SortBy == "" {
		filter.SortBy = "created_at"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "DESC"
	} else {
		filter.SortOrder = strings.ToUpper(filter.SortOrder)
	}

	resources, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]ResourceResponse, len(resources))
	for i, r := range resources {
		items[i] = NewResponse(r)
	}

	resp := response.NewPageResponse(items, req.Page, req.PageSize, total)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Create(c *gin.Context) {
	var body CreateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, NewResponse(res))
}

func (h *Handler) Get(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	res, err := h.service.GetByID(c.Request.Context(), req.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewResponse(res))
}

func (h *Handler) Update(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Permission Check Flow:
	// 1. Get Resource to find Location
	existingRes, err := h.service.GetByID(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
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

	var body UpdateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req := resource.UpdateRequest{
		Name: body.Name,
	}

	res, err := h.service.Update(c.Request.Context(), uri.ID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewResponse(res))
}

func (h *Handler) Delete(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Permission Check Flow
	existingRes, err := h.service.GetByID(c.Request.Context(), req.ID)
	if err != nil {
		response.Error(c, err)
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

	if err := h.service.Delete(c.Request.Context(), req.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

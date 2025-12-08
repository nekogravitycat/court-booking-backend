package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type LocationHandler struct {
	service     location.Service
	userService user.Service
	orgService  organization.Service
}

func NewHandler(service location.Service, userService user.Service, orgService organization.Service) *LocationHandler {
	return &LocationHandler{
		service:     service,
		userService: userService,
		orgService:  orgService,
	}
}

// List retrieves a paginated list of locations with optional filtering.
func (h *LocationHandler) List(c *gin.Context) {
	var req ListLocationsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Sorting logic
	sortBy := "created_at"
	sortOrder := "DESC"

	if req.SortBy != "" {
		sortBy = req.SortBy
	}
	if req.SortOrder != "" {
		sortOrder = strings.ToUpper(req.SortOrder)
	}

	// Parse CreatedAt times
	var createdAtFrom, createdAtTo time.Time
	if req.CreatedAtFrom != "" {
		createdAtFrom, _ = time.Parse(time.RFC3339, req.CreatedAtFrom)
	}
	if req.CreatedAtTo != "" {
		createdAtTo, _ = time.Parse(time.RFC3339, req.CreatedAtTo)
	}

	filter := location.LocationFilter{
		OrganizationID:       req.OrganizationID,
		Page:                 req.Page,
		PageSize:             req.PageSize,
		Name:                 req.Name,
		Opening:              req.Opening,
		CapacityMin:          req.CapacityMin,
		CapacityMax:          req.CapacityMax,
		OpeningHoursStartMin: req.OpeningHoursStartMin,
		OpeningHoursStartMax: req.OpeningHoursStartMax,
		OpeningHoursEndMin:   req.OpeningHoursEndMin,
		OpeningHoursEndMax:   req.OpeningHoursEndMax,
		CreatedAtFrom:        createdAtFrom,
		CreatedAtTo:          createdAtTo,
		SortBy:               sortBy,
		SortOrder:            sortOrder,
	}

	locs, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]LocationResponse, len(locs))
	for i, l := range locs {
		items[i] = NewLocationResponse(l)
	}

	resp := response.NewPageResponse(items, req.Page, req.PageSize, total)
	c.JSON(http.StatusOK, resp)
}

// Create adds a new location.
// It enforces strict permission checks: only Organization Admins or Owners can create locations.
func (h *LocationHandler) Create(c *gin.Context) {
	var body CreateLocationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Permission check: The user must be an Admin or Owner of the target organization.
	allowed, err := h.orgService.CheckPermission(c.Request.Context(), body.OrganizationID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only organization admins can create locations"})
		return
	}

	req := location.CreateLocationRequest{
		OrganizationID:    body.OrganizationID,
		Name:              body.Name,
		Capacity:          body.Capacity,
		OpeningHoursStart: body.OpeningHoursStart,
		OpeningHoursEnd:   body.OpeningHoursEnd,
		LocationInfo:      body.LocationInfo,
		Opening:           body.Opening,
		Rule:              body.Rule,
		Facility:          body.Facility,
		Description:       body.Description,
		Longitude:         body.Longitude,
		Latitude:          body.Latitude,
	}

	loc, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, NewLocationResponse(loc))
}

// Get retrieves specific location details.
func (h *LocationHandler) Get(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	loc, err := h.service.GetByID(c.Request.Context(), req.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewLocationResponse(loc))
}

// Update modifies specific attributes of a location.
// It enforces strict permission checks: only Organization Admins or Owners can update locations.
func (h *LocationHandler) Update(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Fetch the existing location to determine which organization it belongs to.
	existingLoc, err := h.service.GetByID(c.Request.Context(), uri.ID)
	if err != nil {
		switch {
		case errors.Is(err, location.ErrLocNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "location not found"})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch location for permission check"})
			return
		}
	}

	// Permission check: The user must be an Admin or Owner of that organization.
	allowed, err := h.orgService.CheckPermission(c.Request.Context(), existingLoc.OrganizationID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you do not have permission to update this location"})
		return
	}

	// Handle update logic.
	var body UpdateLocationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req := location.UpdateLocationRequest{
		Name:              body.Name,
		Capacity:          body.Capacity,
		OpeningHoursStart: body.OpeningHoursStart,
		OpeningHoursEnd:   body.OpeningHoursEnd,
		LocationInfo:      body.LocationInfo,
		Opening:           body.Opening,
		Rule:              body.Rule,
		Facility:          body.Facility,
		Description:       body.Description,
		Longitude:         body.Longitude,
		Latitude:          body.Latitude,
	}

	loc, err := h.service.Update(c.Request.Context(), uri.ID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewLocationResponse(loc))
}

// Delete removes a location.
// It enforces strict permission checks: only Organization Admins or Owners can delete locations.
func (h *LocationHandler) Delete(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Fetch the existing location to determine which organization it belongs to.
	existingLoc, err := h.service.GetByID(c.Request.Context(), req.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Permission check: The user must be an Admin or Owner of that organization.
	allowed, err := h.orgService.CheckPermission(c.Request.Context(), existingLoc.OrganizationID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you do not have permission to delete this location"})
		return
	}

	// Execute deletion.
	if err := h.service.Delete(c.Request.Context(), req.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

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
)

type LocationHandler struct {
	service    location.Service
	orgService organization.Service
}

func NewHandler(service location.Service, orgService organization.Service) *LocationHandler {
	return &LocationHandler{
		service:    service,
		orgService: orgService,
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
// It enforces strict permission checks: only Organization Managers or Owners can create locations.
func (h *LocationHandler) Create(c *gin.Context) {
	var body CreateLocationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	// Permission check: Organization Manager or Owner (or System Admin) can create locations.
	allowed, err := h.orgService.CheckPermission(c.Request.Context(), body.OrganizationID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only organization owners can create locations"})
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
// It enforces strict permission checks: only Organization Managers or Owners can update locations.
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

	// Permission check: The user must be a Manager (assigned to this location) or Owner.
	allowed, err := h.service.CheckLocationPermission(c.Request.Context(), existingLoc.OrganizationID, existingLoc.ID, auth.GetUserID(c))
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
// It enforces strict permission checks: only Organization Managers or Owners can delete locations.
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

	// Permission check: Only Organization Manager or Owner can delete locations.
	// Location Managers cannot delete.
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

// AddManager assigns an user as manager to a location.
// Only Organization Owner can assign managers to locations (or System Admin).
func (h *LocationHandler) AddManager(c *gin.Context) {
	var body struct {
		UserID string `json:"user_id" binding:"required,uuid"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Fetch location to check Org Owner permission
	loc, err := h.service.GetByID(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Permission: Owner or Org Manager can assign location managers
	allowed, err := h.orgService.CheckPermission(c.Request.Context(), loc.OrganizationID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only organization owners can assign location managers"})
		return
	}

	if err := h.service.AddLocationManager(c.Request.Context(), uri.ID, body.UserID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusCreated)
}

// RemoveManager removes a manager from a location.
// Only Organization Owner can remove managers from locations.
func (h *LocationHandler) RemoveManager(c *gin.Context) {
	var uri struct {
		ID     string `uri:"id" binding:"required,uuid"`
		UserID string `uri:"user_id" binding:"required,uuid"`
	}
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Fetch location
	loc, err := h.service.GetByID(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Permission: Owner or Org Admin
	allowed, err := h.orgService.CheckPermission(c.Request.Context(), loc.OrganizationID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only organization owners can remove location managers"})
		return
	}

	if err := h.service.RemoveLocationManager(c.Request.Context(), uri.ID, uri.UserID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

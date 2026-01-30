package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	filehttp "github.com/nekogravitycat/court-booking-backend/internal/file/http"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type LocationHandler struct {
	service     location.Service
	orgService  organization.Service
	fileService file.Service
	fileHandler *filehttp.Handler
}

func NewHandler(service location.Service, orgService organization.Service, fileService file.Service, fileHandler *filehttp.Handler) *LocationHandler {
	return &LocationHandler{
		service:     service,
		orgService:  orgService,
		fileService: fileService,
		fileHandler: fileHandler,
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
	allowed, err := h.orgService.IsManagerOrAbove(c.Request.Context(), body.OrganizationID, auth.GetUserID(c))
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

	// Permission check: The user must be a Manager (assigned to this location) or Owner.
	allowed, err := h.service.IsLocationManagerOrAbove(c.Request.Context(), uri.ID, auth.GetUserID(c))
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

// UploadCover uploads a cover image for a location.
func (h *LocationHandler) UploadCover(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Permission check
	allowed, err := h.service.IsLocationManagerOrAbove(c.Request.Context(), uri.ID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you do not have permission to upload cover for this location"})
		return
	}

	h.fileHandler.HandleFileUpload(c, filehttp.FileUploadConfig{
		MaxSizeBytes: 5 * 1024 * 1024, // 5MB
		AllowedTypes: []string{"image/jpeg", "image/png"},
		ResizeImage:  true,
		AfterUpload: func(ctx context.Context, fileID string) error {
			return h.service.UpdateCover(ctx, uri.ID, fileID)
		},
	})
}

// RemoveCover removes the cover image from a location.
func (h *LocationHandler) RemoveCover(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Permission check
	allowed, err := h.service.IsLocationManagerOrAbove(c.Request.Context(), uri.ID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you do not have permission to remove cover for this location"})
		return
	}

	if err := h.service.RemoveCover(c.Request.Context(), uri.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// Delete removes a location.
// It enforces strict permission checks: only Organization Managers or Owners can delete locations.
func (h *LocationHandler) Delete(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Permission check: Only Organization Manager or Owner can delete locations.
	// Location Managers cannot delete.
	allowed, err := h.service.IsOrganizationManagerOrAbove(c.Request.Context(), req.ID, auth.GetUserID(c))
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

	// Permission: Owner or Org Manager can assign location managers
	allowed, err := h.service.IsOrganizationManagerOrAbove(c.Request.Context(), uri.ID, auth.GetUserID(c))
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

	// Permission: Owner or Org Admin
	allowed, err := h.service.IsOrganizationManagerOrAbove(c.Request.Context(), uri.ID, auth.GetUserID(c))
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

// ListManagers retrieves the list of managers for a location.
func (h *LocationHandler) ListManagers(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var req ListManagersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	// Permission: Owner, Org Manager, or Location Manager (of this location)
	allowed, err := h.service.IsLocationManagerOrAbove(c.Request.Context(), uri.ID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you do not have permission to view managers for this location"})
		return
	}

	users, total, err := h.service.ListLocationManagers(c.Request.Context(), uri.ID, req.ListParams)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]ManagerResponse, len(users))
	for i, u := range users {
		items[i] = NewManagerResponse(u)
	}

	c.JSON(http.StatusOK, response.NewPageResponse(items, req.Page, req.PageSize, total))
}

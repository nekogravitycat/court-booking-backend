package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/booking"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	filehttp "github.com/nekogravitycat/court-booking-backend/internal/file/http"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
)

type Handler struct {
	service        resource.Service
	locService     location.Service
	orgService     organization.Service
	bookingService booking.Service
	fileService    file.Service
	fileHandler    *filehttp.Handler
}

func NewHandler(service resource.Service, locService location.Service, orgService organization.Service, bookingService booking.Service, fileService file.Service, fileHandler *filehttp.Handler) *Handler {
	return &Handler{
		service:        service,
		locService:     locService,
		orgService:     orgService,
		bookingService: bookingService,
		fileService:    fileService,
		fileHandler:    fileHandler,
	}
}

func (h *Handler) List(c *gin.Context) {
	var req ListResourcesRequest
	if !request.BindQuery(c, &req) {
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filter := resource.Filter{
		OrganizationID: req.OrganizationID,
		LocationID:     req.LocationID,
		ResourceType:   req.ResourceType,
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
	if !request.BindJSON(c, &body) {
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
	if err := h.orgService.RequireManager(c.Request.Context(), loc.OrganizationID, auth.GetUserID(c), "forbidden: only organization admins can create resources"); err != nil {
		response.Error(c, err)
		return
	}

	req := resource.CreateRequest{
		Name:         body.Name,
		Price:        body.Price,
		LocationID:   body.LocationID,
		ResourceType: body.ResourceType,
		SportID:      body.SportID,
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
	if !request.BindURI(c, &req) {
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
	if !request.BindURI(c, &uri) {
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
	if err := h.orgService.RequireManager(c.Request.Context(), loc.OrganizationID, auth.GetUserID(c), "forbidden: permission denied"); err != nil {
		response.Error(c, err)
		return
	}

	var body UpdateRequest
	if !request.BindJSON(c, &body) {
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req := resource.UpdateRequest{
		Name:    body.Name,
		Price:   body.Price,
		SportID: body.SportID,
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
	if !request.BindURI(c, &req) {
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

	if err := h.orgService.RequireManager(c.Request.Context(), loc.OrganizationID, auth.GetUserID(c), "forbidden: permission denied"); err != nil {
		response.Error(c, err)
		return
	}

	if err := h.service.Delete(c.Request.Context(), req.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// UploadCover uploads a cover image for a resource.
func (h *Handler) UploadCover(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	// Get resource to find location ID
	res, err := h.service.GetByID(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Permission check: Location manager or above
	currentUserID := auth.GetUserID(c)
	if err := h.locService.RequireLocationManager(c.Request.Context(), res.LocationID, currentUserID, "forbidden: you do not have permission to upload cover for this resource"); err != nil {
		response.Error(c, err)
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

// RemoveCover removes the cover image from a resource.
func (h *Handler) RemoveCover(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	// Get resource to find location ID
	res, err := h.service.GetByID(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Permission check: Location manager or above
	currentUserID := auth.GetUserID(c)
	if err := h.locService.RequireLocationManager(c.Request.Context(), res.LocationID, currentUserID, "forbidden: you do not have permission to remove cover for this resource"); err != nil {
		response.Error(c, err)
		return
	}

	if err := h.service.RemoveCover(c.Request.Context(), uri.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) GetAvailability(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	dateStr := c.Query("date")
	var date time.Time
	var err error

	if dateStr == "" {
		date = time.Now()
	} else {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, expected YYYY-MM-DD"})
			return
		}
	}

	slots, err := h.bookingService.GetAvailability(c.Request.Context(), uri.ID, date)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewAvailabilityResponse(date, slots))
}

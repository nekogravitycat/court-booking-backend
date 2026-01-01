package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/booking"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type Handler struct {
	service     booking.Service
	userService user.Service
	resService  resource.Service
	locService  location.Service
	orgService  organization.Service
}

func NewHandler(
	service booking.Service,
	userService user.Service,
	resService resource.Service,
	locService location.Service,
	orgService organization.Service,
) *Handler {
	return &Handler{
		service:     service,
		userService: userService,
		resService:  resService,
		locService:  locService,
		orgService:  orgService,
	}
}

// checkIsSysAdmin helper checks if the current user is a system admin
func (h *Handler) checkIsSysAdmin(c *gin.Context, userID string) bool {
	u, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		return false
	}
	return u.IsSystemAdmin
}

// checkIsOrgManager helper checks if the current user is an organization manager (owner or admin) for the resource's organization
func (h *Handler) checkIsOrgManager(c *gin.Context, resourceID string, userID string) bool {
	ctx := c.Request.Context()
	res, err := h.resService.GetByID(ctx, resourceID)
	if err != nil {
		return false
	}
	loc, err := h.locService.GetByID(ctx, res.LocationID)
	if err != nil {
		return false
	}
	member, err := h.orgService.GetOrganizationMember(ctx, loc.OrganizationID, userID)
	if err != nil {
		return false
	}
	return member.Role == organization.RoleOwner || member.Role == organization.RoleOrganizationManager
}

func (h *Handler) List(c *gin.Context) {
	var req ListBookingsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Access Control Logic
	currentUserID := auth.GetUserID(c)
	isSysAdmin := h.checkIsSysAdmin(c, currentUserID)

	filterUserID := currentUserID

	// If Admin, they can see all or filter by specific user
	if isSysAdmin {
		filterUserID = req.UserID // can be empty to show all
	}
	// If Normal User, forced to see only their own

	filter := booking.Filter{
		UserID:     filterUserID,
		ResourceID: req.ResourceID,
		Status:     req.Status,
		StartTime:  req.StartTimeFrom,
		EndTime:    req.StartTimeTo,
		Page:       req.Page,
		PageSize:   req.PageSize,
		SortBy:     req.SortBy,
		SortOrder:  req.SortOrder,
	}

	if filter.SortBy == "" {
		filter.SortBy = "start_time"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "DESC"
	} else {
		filter.SortOrder = strings.ToUpper(filter.SortOrder)
	}

	bookings, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]BookingResponse, len(bookings))
	for i, b := range bookings {
		items[i] = NewBookingResponse(b)
	}

	resp := response.NewPageResponse(items, req.Page, req.PageSize, total)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Create(c *gin.Context) {
	var body CreateBookingRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	req := booking.CreateRequest{
		UserID:     userID,
		ResourceID: body.ResourceID,
		StartTime:  body.StartTime,
		EndTime:    body.EndTime,
	}

	b, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, NewBookingResponse(b))
}

func (h *Handler) Get(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	b, err := h.service.GetByID(c.Request.Context(), req.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Access Check: User owns booking OR SysAdmin OR OrgManager
	userID := auth.GetUserID(c)

	isOwner := userID == b.UserID
	isSysAdmin := h.checkIsSysAdmin(c, userID)

	if !isOwner && !isSysAdmin {
		// Check if Org Manager
		if !h.checkIsOrgManager(c, b.ResourceID, userID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			return
		}
	}

	c.JSON(http.StatusOK, NewBookingResponse(b))
}

func (h *Handler) Update(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var body UpdateBookingRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	isSysAdmin := h.checkIsSysAdmin(c, userID)

	req := booking.UpdateRequest{
		StartTime: body.StartTime,
		EndTime:   body.EndTime,
		Status:    body.Status,
	}

	b, err := h.service.Update(c.Request.Context(), uri.ID, req, userID, isSysAdmin)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewBookingResponse(b))
}

func (h *Handler) Delete(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	isSysAdmin := h.checkIsSysAdmin(c, userID)

	err := h.service.Delete(c.Request.Context(), req.ID, userID, isSysAdmin)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

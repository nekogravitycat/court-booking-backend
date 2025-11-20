package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/booking"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
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
	member, err := h.orgService.GetMember(ctx, loc.OrganizationID, userID)
	if err != nil {
		return false
	}
	return member.Role == organization.RoleOwner || member.Role == organization.RoleAdmin
}

func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	resourceID := c.Query("resource_id")
	status := c.Query("status")
	queryUserID := c.Query("user_id")

	var startTime, endTime *time.Time
	if v := c.Query("start_time_from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			startTime = &t
		}
	}
	if v := c.Query("start_time_to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			endTime = &t
		}
	}

	// Access Control Logic
	currentUserID := auth.GetUserID(c)
	isSysAdmin := h.checkIsSysAdmin(c, currentUserID)

	filterUserID := currentUserID

	// If Admin, they can see all or filter by specific user
	if isSysAdmin {
		filterUserID = queryUserID // can be empty to show all
	}
	// If Normal User, forced to see only their own

	filter := booking.Filter{
		UserID:     filterUserID,
		ResourceID: resourceID,
		Status:     status,
		StartTime:  startTime,
		EndTime:    endTime,
		Page:       page,
		PageSize:   pageSize,
	}

	bookings, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list bookings"})
		return
	}

	items := make([]BookingResponse, len(bookings))
	for i, b := range bookings {
		items[i] = NewBookingResponse(b)
	}

	resp := response.NewPageResponse(items, page, pageSize, total)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Create(c *gin.Context) {
	var body CreateBookingBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
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
		switch err {
		case booking.ErrStartTimePast:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case booking.ErrResourceNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case booking.ErrInvalidTimeRange:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case booking.ErrTimeConflict:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create booking"})
		}
		return
	}

	c.JSON(http.StatusCreated, NewBookingResponse(b))
}

func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	b, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == booking.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "booking not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get booking"})
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
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	var body UpdateBookingBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	userID := auth.GetUserID(c)
	isSysAdmin := h.checkIsSysAdmin(c, userID)

	req := booking.UpdateRequest{
		StartTime: body.StartTime,
		EndTime:   body.EndTime,
		Status:    body.Status,
	}

	b, err := h.service.Update(c.Request.Context(), id, req, userID, isSysAdmin)
	if err != nil {
		switch err {
		case booking.ErrNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "booking not found"})
		case booking.ErrPermissionDenied:
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		case booking.ErrStartTimePast:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case booking.ErrTimeConflict:
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case booking.ErrInvalidTimeRange:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, NewBookingResponse(b))
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	userID := auth.GetUserID(c)
	isSysAdmin := h.checkIsSysAdmin(c, userID)

	err := h.service.Delete(c.Request.Context(), id, userID, isSysAdmin)
	if err != nil {
		if err == booking.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "booking not found"})
			return
		}
		if err == booking.ErrPermissionDenied {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete booking"})
		return
	}

	c.Status(http.StatusNoContent)
}

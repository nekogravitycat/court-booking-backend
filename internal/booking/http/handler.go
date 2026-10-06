package http

import (
	"errors"
	resHttp "github.com/nekogravitycat/court-booking-backend/internal/resource/http"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/booking"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type Handler struct {
	service booking.Service
}

func NewHandler(service booking.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *gin.Context) {
	var body CreateBookingRequest
	if !request.BindJSON(c, &body) {
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
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

func (h *Handler) List(c *gin.Context) {
	var req ListBookingsRequest
	if !request.BindQuery(c, &req) {
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Access Control Logic
	currentUserID := auth.GetUserID(c)
	isSysAdmin := auth.IsSystemAdmin(c)

	filterUserID := currentUserID

	// If Admin, they can see all or filter by specific user
	if isSysAdmin {
		filterUserID = req.UserID // can be empty to show all
	}
	// If Normal User, forced to see only their own

	filter := booking.Filter{
		UserID:         filterUserID,
		ResourceID:     req.ResourceID,
		OrganizationID: req.OrganizationID,
		Status:         req.Status,
		StartTime:      req.StartTimeFrom,
		EndTime:        req.StartTimeTo,
		Page:           req.Page,
		PageSize:       req.PageSize,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
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

func (h *Handler) Get(c *gin.Context) {
	var req request.ByIDRequest
	if !request.BindURI(c, &req) {
		return
	}

	// Access Check: User owns booking OR SysAdmin OR OrgManager
	b, err := h.service.GetForViewer(c.Request.Context(), req.ID, auth.GetUserID(c), auth.IsSystemAdmin(c))
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewBookingResponse(b))
}

func (h *Handler) Update(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	var body UpdateBookingRequest
	if !request.BindJSON(c, &body) {
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	isSysAdmin := auth.IsSystemAdmin(c)

	req := booking.UpdateRequest{
		StartTime:     body.StartTime,
		EndTime:       body.EndTime,
		Status:        body.Status,
		PaymentStatus: body.PaymentStatus,
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
	if !request.BindURI(c, &req) {
		return
	}

	userID := auth.GetUserID(c)
	isSysAdmin := auth.IsSystemAdmin(c)

	err := h.service.Delete(c.Request.Context(), req.ID, userID, isSysAdmin)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) CreateSeries(c *gin.Context) {
	var body CreateBookingSeriesRequest
	if !request.BindJSON(c, &body) {
		return
	}

	startDate, err := time.Parse("2006-01-02", body.StartDate)
	if err != nil {
		response.BadRequest(c, "invalid start_date, expected YYYY-MM-DD")
		return
	}
	weekdays := make([]time.Weekday, len(body.Weekdays))
	for i, d := range body.Weekdays {
		weekdays[i] = time.Weekday(d)
	}

	series, err := h.service.CreateSeries(c.Request.Context(), booking.CreateSeriesRequest{
		UserID:     auth.GetUserID(c),
		ResourceID: body.ResourceID,
		TermMonths: body.TermMonths,
		StartDate:  startDate,
		Weekdays:   weekdays,
		StartClock: body.StartTime,
		EndClock:   body.EndTime,
	})
	if err != nil {
		var conflict *booking.SeriesConflictError
		if errors.As(err, &conflict) {
			slots := make([]resHttp.TimeSlot, len(conflict.Conflicts))
			for i, s := range conflict.Conflicts {
				// Keep the location offset so the client sees the wall-clock time it asked for.
				slots[i] = resHttp.TimeSlot{StartTime: s.StartTime, EndTime: s.EndTime}
			}
			c.JSON(http.StatusConflict, ConflictResponse{Error: conflict.Error(), Conflicts: slots})
			return
		}
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, NewBookingSeriesResponse(series))
}

func (h *Handler) GetSeries(c *gin.Context) {
	var req request.ByIDRequest
	if !request.BindURI(c, &req) {
		return
	}

	series, err := h.service.GetSeriesForViewer(c.Request.Context(), req.ID, auth.GetUserID(c), auth.IsSystemAdmin(c))
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewBookingSeriesResponse(series))
}

// GetLocationAvailability returns the availability of every resource of a location for one date.
func (h *Handler) GetLocationAvailability(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	date := time.Now()
	if dateStr := c.Query("date"); dateStr != "" {
		var err error
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			response.BadRequest(c, "invalid date format, expected YYYY-MM-DD")
			return
		}
	}

	items, err := h.service.GetLocationAvailability(c.Request.Context(), uri.ID, date)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewLocationAvailabilityResponse(date, items))
}

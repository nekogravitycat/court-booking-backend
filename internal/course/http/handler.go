package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/course"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type Handler struct {
	service    course.Service
	payments   course.PaymentService
	attendance course.AttendanceService
}

func NewHandler(service course.Service, payments course.PaymentService, attendance course.AttendanceService) *Handler {
	return &Handler{service: service, payments: payments, attendance: attendance}
}

func viewer(c *gin.Context) course.Viewer {
	return course.Viewer{UserID: auth.GetUserID(c), IsAdmin: auth.IsSystemAdmin(c)}
}

var errLoginRequired = apperror.New(http.StatusUnauthorized, "login is required for this filter")

func (h *Handler) Create(c *gin.Context) {
	var req CreateCourseRequest
	if !request.BindJSON(c, &req) {
		return
	}
	out, err := h.service.Create(c.Request.Context(), auth.GetUserID(c), req.ToInput())
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewCourseResponse(out))
}

func (h *Handler) List(c *gin.Context) {
	var req ListCoursesRequest
	if !request.BindQuery(c, &req) {
		return
	}
	userID := auth.GetUserID(c)
	if (req.Managed || req.Enrolled) && userID == "" {
		response.Error(c, errLoginRequired)
		return
	}
	f := course.Filter{
		SportID: req.SportID, CourseType: req.CourseType, CoachID: req.CoachID, GroupID: req.GroupID,
		LocationID: req.LocationID, Keyword: req.Keyword, Status: req.Status, FreeOnly: req.FreeOnly,
		HasSeats: req.HasSeats, IncludeEnded: req.IncludeEnded, StartFrom: req.StartFrom, StartTo: req.StartTo,
		SortBy: req.SortBy, SortOrder: req.SortOrder, Page: req.Page, PageSize: req.PageSize,
	}
	if req.Managed {
		f.ManagedBy = userID
	}
	if req.Enrolled {
		f.EnrolledBy = userID
	}
	items, total, err := h.service.List(c.Request.Context(), f)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]CourseResponse, len(items))
	for i, it := range items {
		out[i] = NewCourseResponse(it)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, req.Page, req.PageSize, total))
}

func (h *Handler) Get(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	out, err := h.service.Get(c.Request.Context(), uri.ID, viewer(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewCourseResponse(out))
}

func (h *Handler) Update(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req UpdateCourseRequest
	if !request.BindJSON(c, &req) {
		return
	}
	out, err := h.service.Update(c.Request.Context(), uri.ID, viewer(c), req.ToInput())
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewCourseResponse(out))
}

func (h *Handler) AddSession(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req AddSessionRequest
	if !request.BindJSON(c, &req) {
		return
	}
	s, err := h.service.AddSession(c.Request.Context(), uri.ID, viewer(c),
		course.SessionInput{Start: req.StartAt, End: req.EndAt, VenueNote: req.VenueNote})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewSessionResponse(s))
}

func (h *Handler) UpdateSession(c *gin.Context) {
	var uri SessionURI
	if !request.BindURI(c, &uri) {
		return
	}
	var req UpdateSessionRequest
	if !request.BindJSON(c, &req) {
		return
	}
	s, err := h.service.UpdateSession(c.Request.Context(), uri.CourseID, uri.SessionID, viewer(c),
		course.SessionUpdate{Start: req.StartAt, End: req.EndAt, VenueNote: req.VenueNote, Cancel: req.Cancel, Reason: req.Reason})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewSessionResponse(s))
}

func (h *Handler) Enroll(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req EnrollRequest
	if c.Request.ContentLength != 0 && !request.BindJSON(c, &req) {
		return
	}
	e, err := h.service.Enroll(c.Request.Context(), uri.ID, auth.GetUserID(c), req.PaymentMethod)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewEnrollmentResponse(e))
}

func (h *Handler) ListCourseEnrollments(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ListEnrollmentsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.service.ListEnrollments(c.Request.Context(), uri.ID, viewer(c), course.EnrollmentFilter{
		Status: req.Status, PaymentStatus: req.PaymentStatus, Page: req.Page, PageSize: req.PageSize,
	})
	writeEnrollmentPage(c, items, req, total, err)
}

func (h *Handler) ListMyEnrollments(c *gin.Context) {
	var req ListEnrollmentsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.service.ListMyEnrollments(c.Request.Context(), auth.GetUserID(c), course.EnrollmentFilter{
		Status: req.Status, PaymentStatus: req.PaymentStatus, Page: req.Page, PageSize: req.PageSize,
	})
	writeEnrollmentPage(c, items, req, total, err)
}

func writeEnrollmentPage(c *gin.Context, items []*course.Enrollment, req ListEnrollmentsRequest, total int, err error) {
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]EnrollmentResponse, len(items))
	for i, it := range items {
		out[i] = NewEnrollmentResponse(it)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, req.Page, req.PageSize, total))
}

func (h *Handler) GetEnrollment(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	e, err := h.service.GetEnrollment(c.Request.Context(), uri.ID, viewer(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewEnrollmentResponse(e))
}

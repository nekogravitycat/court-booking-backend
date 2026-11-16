package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/course"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

func (h *Handler) RequestLeave(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req LeaveRequestBody
	if c.Request.ContentLength != 0 && !request.BindJSON(c, &req) {
		return
	}
	l, err := h.attendance.RequestLeave(c.Request.Context(), uri.ID, auth.GetUserID(c), req.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewLeaveResponse(l))
}

func (h *Handler) CancelLeave(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	if err := h.attendance.CancelLeave(c.Request.Context(), uri.ID, auth.GetUserID(c)); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) MakeupOptions(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	el, opts, err := h.attendance.MakeupOptions(c.Request.Context(), uri.ID, viewer(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewMakeupOptionsResponse(el, opts))
}

func (h *Handler) SelectMakeup(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req MakeupSelectionBody
	if !request.BindJSON(c, &req) {
		return
	}
	l, err := h.attendance.SelectMakeup(c.Request.Context(), uri.ID, auth.GetUserID(c), req.SessionID)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewLeaveResponse(l))
}

func (h *Handler) CancelMakeup(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	l, err := h.attendance.CancelMakeup(c.Request.Context(), uri.ID, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewLeaveResponse(l))
}

func (h *Handler) GetAttendance(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	sess, roster, err := h.attendance.Roster(c.Request.Context(), uri.ID, viewer(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewRosterResponse(sess, roster))
}

func (h *Handler) MarkAttendance(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req MarkAttendanceRequest
	if !request.BindJSON(c, &req) {
		return
	}
	recs := make([]course.AttendanceRecord, len(req.Records))
	for i, r := range req.Records {
		recs[i] = course.AttendanceRecord{EnrollmentID: r.EnrollmentID, Status: r.Status}
	}
	sess, roster, err := h.attendance.MarkAttendance(c.Request.Context(), uri.ID, viewer(c), recs)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewRosterResponse(sess, roster))
}

func (h *Handler) EnrollmentAttendance(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	items, sum, err := h.attendance.EnrollmentAttendance(c.Request.Context(), uri.ID, viewer(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewEnrollmentAttendanceResponse(items, sum))
}

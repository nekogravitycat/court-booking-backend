package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/course"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

func (h *Handler) ReportTransfer(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req TransferReportRequest
	if !request.BindJSON(c, &req) {
		return
	}
	t, err := h.payments.ReportTransfer(c.Request.Context(), uri.ID, auth.GetUserID(c), course.TransferInput{
		Last5: req.Last5, Amount: req.Amount, TransferredAt: req.TransferredAt, Note: req.Note,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewTransferReportResponse(t))
}

func writeReportPage(c *gin.Context, items []*course.TransferReport, page, pageSize, total int, err error) {
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]TransferReportResponse, len(items))
	for i, it := range items {
		out[i] = NewTransferReportResponse(it)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, page, pageSize, total))
}

func (h *Handler) ListEnrollmentReports(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ListReportsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.payments.ListEnrollmentReports(c.Request.Context(), uri.ID, viewer(c),
		course.TransferReportFilter{Status: req.Status, Page: req.Page, PageSize: req.PageSize})
	writeReportPage(c, items, req.Page, req.PageSize, total, err)
}

func (h *Handler) ListReports(c *gin.Context) {
	var req ListReportsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.payments.ListReports(c.Request.Context(), viewer(c),
		course.TransferReportFilter{CourseID: req.CourseID, Status: req.Status, Page: req.Page, PageSize: req.PageSize})
	writeReportPage(c, items, req.Page, req.PageSize, total, err)
}

func (h *Handler) ReviewReport(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ReviewTransferRequest
	if !request.BindJSON(c, &req) {
		return
	}
	t, err := h.payments.ReviewReport(c.Request.Context(), uri.ID, viewer(c), req.Decision == "approve", req.Note)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewTransferReportResponse(t))
}

func (h *Handler) UpdatePayment(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req UpdatePaymentRequest
	if !request.BindJSON(c, &req) {
		return
	}
	e, err := h.payments.SetPayment(c.Request.Context(), uri.ID, viewer(c), req.PaymentStatus)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewEnrollmentResponse(e))
}

func (h *Handler) RequestCancellation(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req CancellationBody
	if !request.BindJSON(c, &req) {
		return
	}
	q, err := h.payments.RequestCancellation(c.Request.Context(), uri.ID, viewer(c), course.CancelInput{SessionID: req.SessionID, Reason: req.Reason})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewCancellationResponse(q))
}

func writeCancellationPage(c *gin.Context, items []*course.CancellationRequest, page, pageSize, total int, err error) {
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]CancellationResponse, len(items))
	for i, it := range items {
		out[i] = NewCancellationResponse(it)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, page, pageSize, total))
}

func (h *Handler) ListEnrollmentCancellations(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ListCancellationsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.payments.ListEnrollmentCancellations(c.Request.Context(), uri.ID, viewer(c),
		course.CancellationFilter{Status: req.Status, Page: req.Page, PageSize: req.PageSize})
	writeCancellationPage(c, items, req.Page, req.PageSize, total, err)
}

func (h *Handler) ListCancellations(c *gin.Context) {
	var req ListCancellationsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.payments.ListCancellations(c.Request.Context(), viewer(c),
		course.CancellationFilter{CourseID: req.CourseID, Status: req.Status, Page: req.Page, PageSize: req.PageSize})
	writeCancellationPage(c, items, req.Page, req.PageSize, total, err)
}

func (h *Handler) ResolveCancellation(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ResolveCancellationRequest
	if !request.BindJSON(c, &req) {
		return
	}
	q, err := h.payments.ResolveCancellation(c.Request.Context(), uri.ID, viewer(c),
		course.ResolveInput{Resolution: req.Resolution, RefundAmount: req.RefundAmount, Note: req.Note})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewCancellationResponse(q))
}

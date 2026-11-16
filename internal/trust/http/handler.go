package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/trust"
)

type Handler struct {
	service trust.Service
}

func NewHandler(service trust.Service) *Handler {
	return &Handler{service: service}
}

func viewer(c *gin.Context) trust.Viewer {
	return trust.Viewer{UserID: auth.GetUserID(c), IsAdmin: auth.IsSystemAdmin(c)}
}

// ---- Reviews ----

func (h *Handler) CreateReview(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ReviewRequest
	if !request.BindJSON(c, &req) {
		return
	}
	rv, err := h.service.CreateReview(c.Request.Context(), uri.ID, auth.GetUserID(c),
		trust.ReviewInput{CoachID: req.CoachID, Rating: *req.Rating, Comment: req.Comment})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewReviewResponse(rv, false))
}

func (h *Handler) ListMyReviews(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	items, err := h.service.ListMyReviews(c.Request.Context(), uri.ID, viewer(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]ReviewResponse, len(items))
	for i, it := range items {
		out[i] = NewReviewResponse(it, false)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, 1, len(out), len(out)))
}

func (h *Handler) ListCoachReviews(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req request.ListParams
	if !request.BindQuery(c, &req) {
		return
	}
	items, sum, total, err := h.service.ListCoachReviews(c.Request.Context(), uri.ID, trust.ReviewFilter{Page: req.Page, PageSize: req.PageSize})
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]ReviewResponse, len(items))
	for i, it := range items {
		out[i] = NewReviewResponse(it, false)
	}
	c.JSON(http.StatusOK, CoachReviewsResponse{
		Summary: RatingSummaryResponse{Average: sum.Average, Count: sum.Count}, Items: out, Page: req.Page, PageSize: req.PageSize, Total: total,
	})
}

func (h *Handler) AdminListReviews(c *gin.Context) {
	var req ListReviewsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.service.AdminListReviews(c.Request.Context(),
		trust.ReviewFilter{CoachID: req.CoachID, HiddenOnly: req.Hidden, Page: req.Page, PageSize: req.PageSize})
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]ReviewResponse, len(items))
	for i, it := range items {
		out[i] = NewReviewResponse(it, true)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, req.Page, req.PageSize, total))
}

func (h *Handler) SetReviewVisibility(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ReviewVisibilityRequest
	if !request.BindJSON(c, &req) {
		return
	}
	rv, err := h.service.SetReviewVisibility(c.Request.Context(), uri.ID, *req.IsVisible, req.Reason, auth.GetUserID(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewReviewResponse(rv, true))
}

// ---- Complaints ----

func (h *Handler) CreateComplaint(c *gin.Context) {
	var req CreateComplaintRequest
	if !request.BindJSON(c, &req) {
		return
	}
	k, err := h.service.CreateComplaint(c.Request.Context(), auth.GetUserID(c), trust.ComplaintInput{
		EnrollmentID: req.EnrollmentID, TargetUserID: req.TargetUserID, Category: req.Category, Description: req.Description,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewComplaintResponse(k, false))
}

func (h *Handler) listComplaints(c *gin.Context, admin bool) {
	var req ListComplaintsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.service.ListComplaints(c.Request.Context(), viewer(c), trust.ComplaintFilter{
		Status: req.Status, Category: req.Category, TargetID: req.TargetID, Page: req.Page, PageSize: req.PageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]ComplaintResponse, len(items))
	for i, it := range items {
		out[i] = NewComplaintResponse(it, admin)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, req.Page, req.PageSize, total))
}

func (h *Handler) ListMyComplaints(c *gin.Context) {
	// Even an admin sees only their own complaints here; staff use /admin/complaints.
	v := viewer(c)
	v.IsAdmin = false
	var req ListComplaintsRequest
	if !request.BindQuery(c, &req) {
		return
	}
	items, total, err := h.service.ListComplaints(c.Request.Context(), v, trust.ComplaintFilter{
		Status: req.Status, Category: req.Category, Page: req.Page, PageSize: req.PageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]ComplaintResponse, len(items))
	for i, it := range items {
		out[i] = NewComplaintResponse(it, false)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, req.Page, req.PageSize, total))
}

func (h *Handler) AdminListComplaints(c *gin.Context) { h.listComplaints(c, true) }

func (h *Handler) GetMyComplaint(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	v := viewer(c)
	v.IsAdmin = false
	k, err := h.service.GetComplaint(c.Request.Context(), uri.ID, v)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewComplaintResponse(k, false))
}

func (h *Handler) AdminGetComplaint(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	k, err := h.service.GetComplaint(c.Request.Context(), uri.ID, trust.Viewer{UserID: auth.GetUserID(c), IsAdmin: true})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewComplaintResponse(k, true))
}

func (h *Handler) UpdateComplaint(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req UpdateComplaintRequest
	if !request.BindJSON(c, &req) {
		return
	}
	k, err := h.service.UpdateComplaint(c.Request.Context(), uri.ID, auth.GetUserID(c), req.Status, req.Note)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewComplaintResponse(k, true))
}

func (h *Handler) AddComplaintNote(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ComplaintNoteRequest
	if !request.BindJSON(c, &req) {
		return
	}
	e, err := h.service.AddComplaintNote(c.Request.Context(), uri.ID, auth.GetUserID(c), req.Note)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewComplaintEventResponse(e, true))
}

// ---- Reputation and suspension ----

func (h *Handler) GetReputation(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	rep, err := h.service.GetReputation(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewReputationResponse(rep))
}

func (h *Handler) AdjustReputation(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req ReputationEventRequest
	if !request.BindJSON(c, &req) {
		return
	}
	e, err := h.service.AdjustReputation(c.Request.Context(), uri.ID, auth.GetUserID(c), req.Delta, req.Reason, req.ComplaintID)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewReputationEventResponse(e))
}

func (h *Handler) Suspend(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req SuspendRequest
	if !request.BindJSON(c, &req) {
		return
	}
	s, err := h.service.Suspend(c.Request.Context(), uri.ID, auth.GetUserID(c), req.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewSuspensionResponse(s))
}

func (h *Handler) ListSuspensions(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	items, err := h.service.ListSuspensions(c.Request.Context(), uri.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	out := make([]SuspensionResponse, len(items))
	for i, it := range items {
		out[i] = NewSuspensionResponse(it)
	}
	c.JSON(http.StatusOK, response.NewPageResponse(out, 1, len(out), len(out)))
}

func (h *Handler) LiftSuspension(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}
	var req LiftSuspensionRequest
	if c.Request.ContentLength != 0 && !request.BindJSON(c, &req) {
		return
	}
	s, err := h.service.LiftSuspension(c.Request.Context(), uri.ID, auth.GetUserID(c), req.Note)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, NewSuspensionResponse(s))
}

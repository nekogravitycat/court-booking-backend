package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/skillrating"
)

type Handler struct {
	service skillrating.Service
}

func NewHandler(service skillrating.Service) *Handler {
	return &Handler{service: service}
}

// Rate records the host's skill-level rating of a participant.
func (h *Handler) Rate(c *gin.Context) {
	var uri RatingURI
	if !request.BindURI(c, &uri) {
		return
	}

	var body RateBody
	if !request.BindJSON(c, &body) {
		return
	}

	userID := auth.GetUserID(c)
	rating, err := h.service.Rate(c.Request.Context(), uri.GroupID, uri.UserID, body.SkillLevel, userID, auth.IsSystemAdmin(c))
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewRatingResponse(rating))
}

// Remove deletes the rating of a participant for a group.
func (h *Handler) Remove(c *gin.Context) {
	var uri RatingURI
	if !request.BindURI(c, &uri) {
		return
	}

	userID := auth.GetUserID(c)
	if err := h.service.Remove(c.Request.Context(), uri.GroupID, uri.UserID, userID, auth.IsSystemAdmin(c)); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListGroupRatings returns the ratings given for a group (host / admin only).
func (h *Handler) ListGroupRatings(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	userID := auth.GetUserID(c)
	ratings, err := h.service.ListByGroup(c.Request.Context(), uri.ID, userID, auth.IsSystemAdmin(c))
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]RatingResponse, len(ratings))
	for i, r := range ratings {
		items[i] = NewRatingResponse(r)
	}

	c.JSON(http.StatusOK, items)
}

// GetUserSummary returns a user's composite rating per sport.
func (h *Handler) GetUserSummary(c *gin.Context) {
	var uri request.ByIDRequest
	if !request.BindURI(c, &uri) {
		return
	}

	h.writeSummary(c, uri.ID)
}

// GetMySummary returns the caller's own composite rating per sport.
func (h *Handler) GetMySummary(c *gin.Context) {
	userID := auth.GetUserID(c)
	h.writeSummary(c, userID)
}

func (h *Handler) writeSummary(c *gin.Context, userID string) {
	summaries, err := h.service.Summary(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]SkillSummaryResponse, len(summaries))
	for i, s := range summaries {
		items[i] = NewSkillSummaryResponse(s)
	}

	c.JSON(http.StatusOK, items)
}

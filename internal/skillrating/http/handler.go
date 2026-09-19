package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/skillrating"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type Handler struct {
	service     skillrating.Service
	userService user.Service
}

func NewHandler(service skillrating.Service, userService user.Service) *Handler {
	return &Handler{service: service, userService: userService}
}

// isSysAdmin reports whether the authenticated user is a system admin.
func (h *Handler) isSysAdmin(c *gin.Context, userID string) bool {
	u, err := h.userService.GetByID(c.Request.Context(), userID)
	return err == nil && u.IsSystemAdmin
}

// Rate records the host's skill-level rating of a participant.
func (h *Handler) Rate(c *gin.Context) {
	var uri RatingURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var body RateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	rating, err := h.service.Rate(c.Request.Context(), uri.GroupID, uri.UserID, body.SkillLevel, userID, h.isSysAdmin(c, userID))
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, NewRatingResponse(rating))
}

// Remove deletes the rating of a participant for a group.
func (h *Handler) Remove(c *gin.Context) {
	var uri RatingURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if err := h.service.Remove(c.Request.Context(), uri.GroupID, uri.UserID, userID, h.isSysAdmin(c, userID)); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListGroupRatings returns the ratings given for a group (host / admin only).
func (h *Handler) ListGroupRatings(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	ratings, err := h.service.ListByGroup(c.Request.Context(), uri.ID, userID, h.isSysAdmin(c, userID))
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
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	h.writeSummary(c, uri.ID)
}

// GetMySummary returns the caller's own composite rating per sport.
func (h *Handler) GetMySummary(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

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

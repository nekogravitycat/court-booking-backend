package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/notification"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type Handler struct {
	service notification.Service
}

func NewHandler(service notification.Service) *Handler {
	return &Handler{service: service}
}

// List returns the current user's notifications, newest first.
func (h *Handler) List(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req ListNotificationsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	list, total, err := h.service.List(c.Request.Context(), notification.Filter{
		UserID:     userID,
		UnreadOnly: req.UnreadOnly,
		Page:       req.Page,
		PageSize:   req.PageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]NotificationResponse, len(list))
	for i, n := range list {
		items[i] = NewNotificationResponse(n)
	}

	c.JSON(http.StatusOK, response.NewPageResponse(items, req.Page, req.PageSize, total))
}

// UnreadCount returns how many unread notifications the current user has.
func (h *Handler) UnreadCount(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	count, err := h.service.CountUnread(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, UnreadCountResponse{Unread: count})
}

// MarkRead marks one of the current user's notifications as read.
func (h *Handler) MarkRead(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	if err := h.service.MarkRead(c.Request.Context(), userID, uri.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// MarkAllRead marks every unread notification of the current user as read.
func (h *Handler) MarkAllRead(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	updated, err := h.service.MarkAllRead(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, MarkAllReadResponse{Updated: updated})
}

// Send is restricted to system administrators by route middleware.
func (h *Handler) Send(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	var body SendNotificationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification request"})
		return
	}
	if err := h.service.SendManual(c.Request.Context(), auth.GetUserID(c), body.UserIDs, body.Title, body.Content); err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"sent": len(body.UserIDs)})
}

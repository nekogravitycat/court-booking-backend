package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/favorite"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type Handler struct {
	service favorite.Service
}

func NewHandler(service favorite.Service) *Handler {
	return &Handler{service: service}
}

// ListFavoriteHosts returns the current user's favorite hosts (nickname + avatar only).
func (h *Handler) ListFavoriteHosts(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	favorites, err := h.service.ListFavorites(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]FavoriteHostResponse, len(favorites))
	for i, f := range favorites {
		items[i] = NewFavoriteHostResponse(f)
	}

	c.JSON(http.StatusOK, items)
}

// AddFavoriteHost adds a host to the current user's favorites.
func (h *Handler) AddFavoriteHost(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var body FavoriteHostRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := h.service.AddFavorite(c.Request.Context(), userID, body.HostID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusCreated)
}

// RemoveFavoriteHost removes a host from the current user's favorites.
func (h *Handler) RemoveFavoriteHost(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var body FavoriteHostRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := h.service.RemoveFavorite(c.Request.Context(), userID, body.HostID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

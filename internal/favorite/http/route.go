package http

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware gin.HandlerFunc) {
	favoritesGroup := g.Group("/favorites")
	favoritesGroup.Use(authMiddleware)
	{
		hostGroup := favoritesGroup.Group("/host")
		{
			hostGroup.GET("", h.ListFavoriteHosts)
			hostGroup.POST("", h.AddFavoriteHost)
			hostGroup.DELETE("", h.RemoveFavoriteHost)
		}
	}
}

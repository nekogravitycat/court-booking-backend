package http

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware gin.HandlerFunc) {
	group := g.Group("/bookings")

	// === Authenticated Routes ===
	group.Use(authMiddleware)
	{
		group.GET("", h.List)
		group.GET("/:id", h.Get)
		group.POST("", h.Create)
		group.PATCH("/:id", h.Update)
		group.DELETE("/:id", h.Delete)
	}
}

package http

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware, adminMiddleware gin.HandlerFunc) {
	group := g.Group("/announcements")

	// === Authenticated Routes ===
	group.Use(authMiddleware)
	{
		group.GET("", h.List)
		group.GET("/:id", h.Get)
	}

	// === Administration Routes (System Admin Only) ===
	adminGroup := group.Group("")
	adminGroup.Use(adminMiddleware)
	{
		adminGroup.POST("", h.Create)
		adminGroup.PATCH("/:id", h.Update)
		adminGroup.DELETE("/:id", h.Delete)
	}
}

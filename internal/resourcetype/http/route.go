package http

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers resource-type related routes.
func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware gin.HandlerFunc) {
	group := g.Group("/resource-types")

	// === Authenticated Routes ===
	group.Use(authMiddleware)
	{
		group.GET("", h.List)          // List resource types
		group.GET("/:id", h.Get)       // Get resource type details
		group.POST("", h.Create)       // Create resource type
		group.PATCH("/:id", h.Update)  // Update resource type
		group.DELETE("/:id", h.Delete) // Delete resource type
	}
}

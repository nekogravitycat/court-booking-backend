package http

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(g *gin.RouterGroup, h *LocationHandler, authMiddleware gin.HandlerFunc) {
	group := g.Group("/locations")

	// === Authenticated Routes ===
	group.Use(authMiddleware)
	{
		group.GET("", h.List)          // List locations
		group.GET("/:id", h.Get)       // Get location details
		group.POST("", h.Create)       // Create location
		group.PATCH("/:id", h.Update)  // Update location
		group.DELETE("/:id", h.Delete) // Delete location

		// Location Managers
		group.POST("/:id/managers", h.AddManager)
		group.DELETE("/:id/managers/:user_id", h.RemoveManager)
	}
}

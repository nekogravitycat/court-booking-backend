package http

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers resource-related routes.
func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware gin.HandlerFunc) {
	group := g.Group("/resources")

	// === Authenticated Routes ===
	group.Use(authMiddleware)
	{
		group.GET("", h.List)                             // List resources
		group.GET("/:id", h.Get)                          // Get resource details
		group.POST("", h.Create)                          // Create resource
		group.PATCH("/:id", h.Update)                     // Update resource
		group.DELETE("/:id", h.Delete)                    // Delete resource
		group.PUT("/:id/cover", h.UploadCover)            // Upload cover image
		group.DELETE("/:id/cover", h.RemoveCover)         // Remove cover image
		group.GET("/:id/availability", h.GetAvailability) // Get availability
	}
}

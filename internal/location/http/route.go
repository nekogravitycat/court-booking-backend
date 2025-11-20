package http

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(g *gin.RouterGroup, h *LocationHandler, authMiddleware gin.HandlerFunc) {
	locationsGroup := g.Group("/locations")

	// Public or Authenticated Reading
	locationsGroup.GET("", h.List)
	locationsGroup.GET("/:id", h.Get)

	// Protected Write Operations
	// Ideally, you would have a middleware here checking Organization ownership/permissions
	protected := locationsGroup.Group("")
	protected.Use(authMiddleware)
	{
		protected.POST("", h.Create)
		protected.PATCH("/:id", h.Update)
		protected.DELETE("/:id", h.Delete)
	}
}

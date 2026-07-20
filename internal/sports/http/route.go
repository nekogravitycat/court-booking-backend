package http

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes wires the sport endpoints. Listing and reading are public
// (no authentication), while mutations require an authenticated system admin.
func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware, adminMiddleware gin.HandlerFunc) {
	// === Public Routes ===
	group := g.Group("/sports")
	{
		group.GET("", h.List)
		group.GET("/:id", h.Get)
	}

	// === Administration Routes (System Admin Only) ===
	// authMiddleware must run before adminMiddleware so the user id is available.
	adminGroup := group.Group("")
	adminGroup.Use(authMiddleware, adminMiddleware)
	{
		adminGroup.POST("", h.Create)
		adminGroup.PATCH("/:id", h.Update)
		adminGroup.DELETE("/:id", h.Delete)
	}
}

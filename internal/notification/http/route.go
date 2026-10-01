package http

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the private inbox and administrator-only delivery.
func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware, sysAdminMiddleware gin.HandlerFunc) {
	group := g.Group("/notifications")
	group.Use(authMiddleware)
	{
		group.POST("", sysAdminMiddleware, h.Send)
		group.GET("", h.List)
		group.GET("/unread-count", h.UnreadCount)
		group.POST("/read-all", h.MarkAllRead)
		group.PATCH("/:id/read", h.MarkRead)
	}
}

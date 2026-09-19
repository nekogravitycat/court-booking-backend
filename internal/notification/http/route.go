package http

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the notification inbox. Every route is scoped to the
// authenticated user; there is no way to read another user's notifications.
func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware gin.HandlerFunc) {
	group := g.Group("/notifications")
	group.Use(authMiddleware)
	{
		group.GET("", h.List)
		group.GET("/unread-count", h.UnreadCount)
		group.POST("/read-all", h.MarkAllRead)
		group.PATCH("/:id/read", h.MarkRead)
	}
}

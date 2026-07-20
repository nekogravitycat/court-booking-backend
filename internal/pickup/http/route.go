package http

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware, optionalAuthMiddleware gin.HandlerFunc) {
	// Public pickup group list (no auth required, trimmed + bookable-only).
	// Optional auth personalizes enrolled_status when a valid token is present.
	g.GET("/pickup-groups", optionalAuthMiddleware, h.ListGroups)

	// Public list of a specific host's pickup groups (optional auth, trimmed).
	g.GET("/hosts/:host_id/pickup-groups", optionalAuthMiddleware, h.ListGroupsByHost)

	// Authenticated pickup group routes
	groupsGroup := g.Group("/pickup-groups")
	groupsGroup.Use(authMiddleware)
	{
		groupsGroup.POST("", h.CreateGroup)
		groupsGroup.GET("/:id", h.GetGroup)
		groupsGroup.PATCH("/:id", h.UpdateGroup)
		groupsGroup.DELETE("/:id", h.DeleteGroup)
		groupsGroup.POST("/:id/orders", h.CreateOrder)
		groupsGroup.GET("/:id/orders", h.ListGroupOrders)
	}

	// Pickup order routes
	ordersGroup := g.Group("/pickup-orders")
	ordersGroup.Use(authMiddleware)
	{
		ordersGroup.GET("", h.ListMyOrders)
		ordersGroup.PATCH("/:id", h.UpdateOrder)
		ordersGroup.DELETE("/:id", h.DeleteOrder)
	}
}

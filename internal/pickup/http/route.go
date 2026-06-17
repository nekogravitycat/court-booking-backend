package http

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware gin.HandlerFunc) {
	// Public pickup group list (no auth, trimmed + bookable-only).
	g.GET("/pickup-groups", h.ListGroups)

	// Public list of a specific host's pickup groups (no auth, trimmed).
	g.GET("/hosts/:host_id/pickup-groups", h.ListGroupsByHost)

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
	}
}

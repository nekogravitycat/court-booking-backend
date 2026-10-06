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

	// Anonymous participant statistics for a group (public, counts only).
	g.GET("/pickup-groups/:id/participant-stats", h.GetParticipantStats)

	// Authenticated pickup group routes
	groupsGroup := g.Group("/pickup-groups")
	groupsGroup.Use(authMiddleware)
	{
		groupsGroup.POST("", h.CreateGroup)
		groupsGroup.GET("/:id", h.GetGroup)
		groupsGroup.PATCH("/:id", h.UpdateGroup)
		groupsGroup.DELETE("/:id", h.DeleteGroup)
		groupsGroup.POST("/:id/orders", h.CreateOrder)
		groupsGroup.POST("/:id/party-orders", h.CreatePartyOrder)
		groupsGroup.GET("/:id/orders", h.ListGroupOrders)
		groupsGroup.PUT("/:id/orders/:order_id/absence", h.MarkAbsence)
		groupsGroup.DELETE("/:id/orders/:order_id/absence", h.ClearAbsence)
	}

	// Batch creation of independent pickup groups (one per occurrence)
	g.POST("/pickup-group-series", authMiddleware, h.CreateGroupSeries)

	// Pickup participation statistics of a user (any authenticated user)
	g.GET("/users/:id/pickup-stats", authMiddleware, h.GetUserStats)

	// Pickup order routes
	ordersGroup := g.Group("/pickup-orders")
	ordersGroup.Use(authMiddleware)
	{
		ordersGroup.GET("", h.ListMyOrders)
		ordersGroup.PATCH("/:id", h.UpdateOrder)
		ordersGroup.DELETE("/:id", h.DeleteOrder)
	}
}

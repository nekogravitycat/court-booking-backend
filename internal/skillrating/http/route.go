package http

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the skill-rating endpoints. All of them require an
// authenticated user; host / admin checks are enforced in the service.
func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware gin.HandlerFunc) {
	// Host-side rating of participants of a group.
	groupRatings := g.Group("/pickup-groups/:id/ratings")
	groupRatings.Use(authMiddleware)
	{
		groupRatings.GET("", h.ListGroupRatings)
		groupRatings.PUT("/:user_id", h.Rate)
		groupRatings.DELETE("/:user_id", h.Remove)
	}

	// Composite rating of a user (any authenticated user may read it).
	g.GET("/me/skill-ratings", authMiddleware, h.GetMySummary)
	g.GET("/users/:id/skill-ratings", authMiddleware, h.GetUserSummary)
}

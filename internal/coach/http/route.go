package http

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the coach, coach group and coach application endpoints.
func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware, optionalAuthMiddleware, sysAdminMiddleware gin.HandlerFunc) {
	g.POST("/coach-applications", authMiddleware, h.Apply)

	// Public coach directory (the profile page personalizes for owner / admin).
	g.GET("/coaches", h.ListCoaches)
	me := g.Group("/coaches/me", authMiddleware)
	{
		me.GET("", h.GetMe)
		me.PATCH("", h.UpdateMe)
		me.GET("/invitations", h.ListMyInvitations)
		me.GET("/group", h.GetMyGroup)
	}
	g.GET("/coaches/:id", optionalAuthMiddleware, h.GetCoach)

	admin := g.Group("/admin/coach-applications", authMiddleware, sysAdminMiddleware)
	{
		admin.GET("", h.ListApplications)
		admin.POST("/:id/review", h.ReviewApplication)
	}

	groups := g.Group("/coach-groups", authMiddleware)
	{
		groups.POST("", h.CreateGroup)
		groups.GET("", h.ListGroups)
		groups.GET("/:id", h.GetGroup)
		groups.PATCH("/:id", h.UpdateGroup)
		groups.DELETE("/:id", h.DisbandGroup)
		groups.POST("/:id/invitations", h.Invite)
		groups.GET("/:id/invitations", h.ListGroupInvitations)
		groups.DELETE("/:id/invitations/:invitation_id", h.CancelInvitation)
		groups.PATCH("/:id/members/:user_id", h.UpdateMember)
		groups.DELETE("/:id/members/:user_id", h.RemoveMember)
	}

	invitations := g.Group("/coach-invitations", authMiddleware)
	{
		invitations.POST("/:id/accept", h.AcceptInvitation)
		invitations.POST("/:id/decline", h.DeclineInvitation)
	}
}

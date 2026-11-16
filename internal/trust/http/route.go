package http

import "github.com/gin-gonic/gin"

// RegisterRoutes wires reviews, complaints, reputation and suspension endpoints.
func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware, sysAdminMiddleware gin.HandlerFunc) {
	// Public reviews of a coach.
	g.GET("/coaches/:id/reviews", h.ListCoachReviews)

	g.POST("/enrollments/:id/review", authMiddleware, h.CreateReview)
	g.GET("/enrollments/:id/review", authMiddleware, h.ListMyReviews)

	complaints := g.Group("/complaints", authMiddleware)
	{
		complaints.POST("", h.CreateComplaint)
		complaints.GET("", h.ListMyComplaints)
		complaints.GET("/:id", h.GetMyComplaint)
	}

	admin := g.Group("/admin", authMiddleware, sysAdminMiddleware)
	{
		admin.GET("/reviews", h.AdminListReviews)
		admin.PATCH("/reviews/:id", h.SetReviewVisibility)

		admin.GET("/complaints", h.AdminListComplaints)
		admin.GET("/complaints/:id", h.AdminGetComplaint)
		admin.PATCH("/complaints/:id", h.UpdateComplaint)
		admin.POST("/complaints/:id/notes", h.AddComplaintNote)

		admin.GET("/coaches/:id/reputation", h.GetReputation)
		admin.POST("/coaches/:id/reputation-events", h.AdjustReputation)
		admin.GET("/coaches/:id/suspensions", h.ListSuspensions)
		admin.POST("/coaches/:id/suspensions", h.Suspend)
		admin.POST("/suspensions/:id/lift", h.LiftSuspension)
	}
}

package http

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the course, session and enrollment endpoints.
func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware, optionalAuthMiddleware gin.HandlerFunc) {
	// Public browsing; optional auth unlocks viewer-specific fields and filters.
	g.GET("/courses", optionalAuthMiddleware, h.List)
	g.GET("/courses/:id", optionalAuthMiddleware, h.Get)

	courses := g.Group("/courses", authMiddleware)
	{
		courses.POST("", h.Create)
		courses.PATCH("/:id", h.Update)
		courses.POST("/:id/sessions", h.AddSession)
		courses.PATCH("/:id/sessions/:session_id", h.UpdateSession)
		courses.POST("/:id/enrollments", h.Enroll)
		courses.GET("/:id/enrollments", h.ListCourseEnrollments)
	}

	enrollments := g.Group("/enrollments", authMiddleware)
	{
		enrollments.GET("", h.ListMyEnrollments)
		enrollments.GET("/:id", h.GetEnrollment)
		enrollments.PATCH("/:id/payment", h.UpdatePayment)
		enrollments.POST("/:id/transfer-reports", h.ReportTransfer)
		enrollments.GET("/:id/transfer-reports", h.ListEnrollmentReports)
		enrollments.POST("/:id/cancellation-requests", h.RequestCancellation)
		enrollments.GET("/:id/cancellation-requests", h.ListEnrollmentCancellations)
		enrollments.GET("/:id/attendance", h.EnrollmentAttendance)
	}

	reports := g.Group("/transfer-reports", authMiddleware)
	{
		reports.GET("", h.ListReports)
		reports.POST("/:id/review", h.ReviewReport)
	}

	cancellations := g.Group("/cancellation-requests", authMiddleware)
	{
		cancellations.GET("", h.ListCancellations)
		cancellations.POST("/:id/resolve", h.ResolveCancellation)
	}

	sessions := g.Group("/sessions", authMiddleware)
	{
		sessions.POST("/:id/leave-requests", h.RequestLeave)
		sessions.GET("/:id/attendance", h.GetAttendance)
		sessions.PATCH("/:id/attendance", h.MarkAttendance)
	}

	leaves := g.Group("/leave-requests", authMiddleware)
	{
		leaves.DELETE("/:id", h.CancelLeave)
		leaves.GET("/:id/makeup-options", h.MakeupOptions)
		leaves.POST("/:id/makeup-selection", h.SelectMakeup)
		leaves.DELETE("/:id/makeup-selection", h.CancelMakeup)
	}
}

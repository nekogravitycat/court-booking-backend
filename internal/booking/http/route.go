package http

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(g *gin.RouterGroup, h *Handler, authMiddleware gin.HandlerFunc) {
	group := g.Group("/bookings")

	// === Authenticated Routes ===
	group.Use(authMiddleware)
	{
		group.GET("", h.List)
		group.GET("/:id", h.Get)
		group.POST("", h.Create)
		group.PATCH("/:id", h.Update)
		group.DELETE("/:id", h.Delete)
	}

	// Booking series (seasonal rentals)
	series := g.Group("/booking-series")
	series.Use(authMiddleware)
	{
		series.POST("", h.CreateSeries)
		series.GET("/:id", h.GetSeries)
	}

	// Aggregated availability of every resource of a location
	g.GET("/locations/:id/availability", authMiddleware, h.GetLocationAvailability)
}

package http

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all user-related routes (including Auth).
func RegisterRoutes(g *gin.RouterGroup, h *UserHandler, authMiddleware, adminMiddleware gin.HandlerFunc) {
	// Public Routes
	authGroup := g.Group("/auth")
	{
		authGroup.POST("/register", h.Register)
		authGroup.POST("/login", h.Login)
	}

	// Authenticated Routes
	g.GET("/me", authMiddleware, h.Me)

	// Admin Routes
	usersGroup := g.Group("/users")
	usersGroup.Use(authMiddleware, adminMiddleware)
	{
		usersGroup.GET("", h.List)
		usersGroup.GET("/:id", h.Get)
		usersGroup.PATCH("/:id", h.Update)
	}
}

package http

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all user-related routes (including Auth).
func RegisterRoutes(g *gin.RouterGroup, h *UserHandler, authMiddleware, adminMiddleware gin.HandlerFunc) {
	// === Public Routes (Auth) ===
	authGroup := g.Group("/auth")
	{
		authGroup.POST("/register", h.Register) // Register new user
		authGroup.POST("/login", h.Login)       // Login and get token
	}

	// === Authenticated Routes ===
	g.GET("/me", authMiddleware, h.Me) // Get current user profile

	// === Administration Routes (System Admin Only) ===
	usersGroup := g.Group("/users")
	usersGroup.Use(authMiddleware, adminMiddleware)
	{
		usersGroup.GET("", h.List)          // List users
		usersGroup.GET("/:id", h.Get)       // Get user details
		usersGroup.PATCH("/:id", h.Update)  // Update user info
		usersGroup.DELETE("/:id", h.Delete) // Delete user
	}
}

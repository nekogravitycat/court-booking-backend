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

	// === Avatar Routes (Self or System Admin; enforced in the handler) ===
	avatarGroup := g.Group("/users")
	avatarGroup.Use(authMiddleware)
	{
		avatarGroup.PUT("/:id/avatar", h.UploadAvatar)    // Upload avatar
		avatarGroup.DELETE("/:id/avatar", h.RemoveAvatar) // Remove avatar
	}

	// === Administration Routes (System Admin Only) ===
	usersGroup := g.Group("/users")
	usersGroup.Use(authMiddleware, adminMiddleware)
	{
		usersGroup.GET("", h.List)          // List users
		usersGroup.GET("/:id", h.Get)       // Get user details
		usersGroup.PATCH("/:id", h.Update)  // Update user info
		usersGroup.DELETE("/:id", h.Delete) // Delete user
	}

	// === Pickup Host Role Management (System Admin Only) ===
	pickupHostsGroup := g.Group("/pickup-hosts")
	pickupHostsGroup.Use(authMiddleware, adminMiddleware)
	{
		pickupHostsGroup.GET("", h.ListPickupHosts)         // List pickup hosts
		pickupHostsGroup.POST("", h.AddPickupHost)          // Grant pickup host role
		pickupHostsGroup.DELETE("/:id", h.RemovePickupHost) // Revoke pickup host role
	}
}

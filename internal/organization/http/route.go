package http

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers organization-related routes.
func RegisterRoutes(g *gin.RouterGroup, h *OrganizationHandler, authMiddleware, adminMiddleware gin.HandlerFunc) {
	orgGroup := g.Group("/organizations")

	// === Authenticated Routes ===
	orgGroup.Use(authMiddleware)
	{
		orgGroup.GET("", h.List)    // List active organizations
		orgGroup.GET("/:id", h.Get) // Get organization details

		// --- Member Management ---
		// Permissions are handled inside the handlers to allow Owners/Admins
		orgGroup.GET("/:id/members", h.ListMembers)                 // List members
		orgGroup.POST("/:id/members", h.AddMember)                  // Add new member
		orgGroup.PATCH("/:id/members/:user_id", h.UpdateMemberRole) // Update member role
		orgGroup.DELETE("/:id/members/:user_id", h.RemoveMember)    // Remove member
	}

	// === Administration Routes (System Admin Only) ===
	adminGroup := orgGroup.Group("")
	adminGroup.Use(adminMiddleware)
	{
		// --- Organization Management ---
		adminGroup.POST("", h.Create)       // Create organization
		adminGroup.PATCH("/:id", h.Update)  // Update organization info
		adminGroup.DELETE("/:id", h.Delete) // Soft delete organization
	}
}

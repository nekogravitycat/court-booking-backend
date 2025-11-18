package http

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers organization-related routes.
func RegisterRoutes(g *gin.RouterGroup, h *OrganizationHandler, authMiddleware, adminMiddleware gin.HandlerFunc) {
	orgGroup := g.Group("/organizations")

	// ==============================
	// Public / Authenticated Routes
	// ==============================
	// Currently, listing and getting organizations are open to authenticated users.
	// We apply authMiddleware to the entire group or specific routes.
	orgGroup.Use(authMiddleware)
	{
		// GET /v1/organizations
		// List active organizations
		orgGroup.GET("", h.List)

		// GET /v1/organizations/:id
		// Get details of a specific organization
		orgGroup.GET("/:id", h.Get)
	}

	// ==============================
	// System Admin Routes
	// ==============================
	// Operations that modify organizations require system admin privileges.
	// Note: Currently restricted to System Admin, might allow Org Owner in the future.
	adminGroup := orgGroup.Group("")
	adminGroup.Use(adminMiddleware)
	{
		// POST /v1/organizations
		// Create a new organization
		adminGroup.POST("", h.Create)

		// PATCH /v1/organizations/:id
		// Update an organization's name
		adminGroup.PATCH("/:id", h.Update)

		// DELETE /v1/organizations/:id
		// Soft delete an organization
		adminGroup.DELETE("/:id", h.Delete)
	}
}

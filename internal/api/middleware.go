package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// RequireSystemAdmin ensures the authenticated user is a system admin.
// It MUST be used after auth.AuthRequired middleware.
func RequireSystemAdmin(userService user.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		// Check permissions
		u, err := userService.GetByID(c.Request.Context(), userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}

		if !u.IsSystemAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden: system admin access required"})
			return
		}

		c.Next()
	}
}

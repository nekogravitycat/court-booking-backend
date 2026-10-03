package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
)

// RequireSystemAdmin ensures the authenticated user is a system admin.
// It MUST be used after auth.AuthRequired middleware, which loads the admin flag.
func RequireSystemAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if auth.GetUserID(c) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		if !auth.IsSystemAdmin(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden: system admin access required"})
			return
		}

		c.Next()
	}
}

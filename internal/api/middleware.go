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

// RequireAllowedOrigin is the CSRF guard for endpoints authenticated by the
// session cookie (which the browser attaches automatically): the request's
// Origin header must be an allowed frontend origin. A missing Origin is
// rejected only when requireOrigin is set (production), since browsers always
// send Origin on cross-origin POSTs and non-browser clients carry no cookie.
func RequireAllowedOrigin(allowed func(origin string) bool, requireOrigin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if (origin == "" && requireOrigin) || (origin != "" && !allowed(origin)) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "origin not allowed"})
			return
		}
		c.Next()
	}
}

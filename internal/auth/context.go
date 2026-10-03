package auth

import "github.com/gin-gonic/gin"

// Gin context keys set by the auth middleware.
const (
	ctxUserID        = "userID"
	ctxIsSystemAdmin = "isSystemAdmin"
)

// GetUserID returns the authenticated user's ID or empty string.
func GetUserID(c *gin.Context) string {
	if v, ok := c.Get(ctxUserID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// IsSystemAdmin reports whether the authenticated user is a system admin. The flag is
// loaded once by the auth middleware, so handlers need no extra user lookup.
func IsSystemAdmin(c *gin.Context) bool {
	return c.GetBool(ctxIsSystemAdmin)
}

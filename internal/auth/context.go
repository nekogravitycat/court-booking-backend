package auth

import "github.com/gin-gonic/gin"

// GetUserID returns the authenticated user's ID or empty string.
func GetUserID(c *gin.Context) string {
	if v, ok := c.Get("userID"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetUserEmail returns the authenticated user's email or empty string.
func GetUserEmail(c *gin.Context) string {
	if v, ok := c.Get("userEmail"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

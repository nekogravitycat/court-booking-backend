package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ActiveStatusFunc reports whether the given user account is currently active
// (i.e. not suspended or soft-deleted). It is injected as a function so the auth
// package does not need to import the user package (which would create an import
// cycle, since user already depends on auth).
type ActiveStatusFunc func(ctx context.Context, userID string) (bool, error)

// AuthRequired is a Gin middleware that validates JWT from Authorization: Bearer <token>.
//
// When isActive is non-nil it additionally verifies, on every request, that the
// account is still active. This ensures suspended / soft-deleted users lose
// access immediately instead of remaining authorized until their access token
// expires (there is otherwise no token revocation mechanism).
func AuthRequired(jwtManager *JWTManager, isActive ActiveStatusFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing Authorization header",
			})
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid Authorization header format",
			})
			return
		}

		tokenStr := parts[1]

		claims, err := jwtManager.ParseAndValidate(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or expired token",
			})
			return
		}

		// Reject tokens belonging to accounts that have since been suspended or
		// soft-deleted. Treat lookup errors (including "not found") as unauthorized.
		if isActive != nil {
			active, err := isActive(c.Request.Context(), claims.Subject)
			if err != nil || !active {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "account is inactive or no longer exists",
				})
				return
			}
		}

		// Store user info into Gin context for later handlers.
		c.Set("userID", claims.Subject)

		c.Next()
	}
}

// AuthOptional is a Gin middleware for endpoints that are public but can tailor
// their response to the caller when a valid token happens to be present.
//
// Unlike AuthRequired it never aborts: a missing, malformed, or invalid
// Authorization header simply leaves the request unauthenticated. When a token
// validly parses, the user id is stored in the context so handlers can read it
// via GetUserID; otherwise GetUserID returns "".
func AuthOptional(jwtManager *JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.Next()
			return
		}

		claims, err := jwtManager.ParseAndValidate(parts[1])
		if err != nil {
			c.Next()
			return
		}

		c.Set("userID", claims.Subject)
		c.Next()
	}
}

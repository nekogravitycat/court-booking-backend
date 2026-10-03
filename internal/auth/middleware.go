package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Account is the per-request account state the auth middleware needs.
type Account struct {
	IsActive      bool
	IsSystemAdmin bool
}

// AccountLookupFunc loads the given user's current account state (the user is
// not suspended or soft-deleted, and whether they are a system admin). It is
// injected as a function so the auth package does not need to import the user
// package (which would create an import cycle, since user already depends on auth).
type AccountLookupFunc func(ctx context.Context, userID string) (Account, error)

// setIdentity stores the authenticated identity into the Gin context.
func setIdentity(c *gin.Context, userID string, a Account) {
	c.Set(ctxUserID, userID)
	c.Set(ctxIsSystemAdmin, a.IsSystemAdmin)
}

// AuthRequired is a Gin middleware that validates JWT from Authorization: Bearer <token>.
//
// When lookup is non-nil it additionally verifies, on every request, that the
// account is still active. This ensures suspended / soft-deleted users lose
// access immediately instead of remaining authorized until their access token
// expires (there is otherwise no token revocation mechanism).
func AuthRequired(jwtManager *JWTManager, lookup AccountLookupFunc) gin.HandlerFunc {
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
		var account Account
		if lookup != nil {
			account, err = lookup(c.Request.Context(), claims.Subject)
			if err != nil || !account.IsActive {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "account is inactive or no longer exists",
				})
				return
			}
		}

		// Store user info into Gin context for later handlers.
		setIdentity(c, claims.Subject, account)

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
//
// When lookup is non-nil, a token whose account is no longer active is
// treated as unauthenticated.
func AuthOptional(jwtManager *JWTManager, lookup AccountLookupFunc) gin.HandlerFunc {
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

		var account Account
		if lookup != nil {
			account, err = lookup(c.Request.Context(), claims.Subject)
			if err != nil || !account.IsActive {
				c.Next()
				return
			}
		}

		setIdentity(c, claims.Subject, account)
		c.Next()
	}
}

package auth

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SessionCookie describes the cookie that carries the session token.
//
// The cookie is host-only (no Domain attribute): the browser sends it solely to
// the API host, so every frontend sharing that API shares one login without
// exposing the token to sibling subdomains.
type SessionCookie struct {
	Name   string
	Secure bool
	MaxAge time.Duration
}

// NewSessionCookie returns the cookie settings for the environment. Production
// uses the __Host- prefix, which browsers only accept for Secure, host-only,
// Path=/ cookies; local development runs over plain HTTP, so it opts out.
func NewSessionCookie(isProduction bool, maxAge time.Duration) SessionCookie {
	if isProduction {
		return SessionCookie{Name: "__Host-session", Secure: true, MaxAge: maxAge}
	}
	return SessionCookie{Name: "session", Secure: false, MaxAge: maxAge}
}

func (sc SessionCookie) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sc.Name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   sc.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// Set writes the session cookie.
func (sc SessionCookie) Set(c *gin.Context, token string) {
	http.SetCookie(c.Writer, sc.cookie(token, int(sc.MaxAge.Seconds())))
}

// Clear expires the session cookie.
func (sc SessionCookie) Clear(c *gin.Context) {
	http.SetCookie(c.Writer, sc.cookie("", -1))
}

// Token returns the session token sent by the browser, or "".
func (sc SessionCookie) Token(c *gin.Context) string {
	v, err := c.Cookie(sc.Name)
	if err != nil {
		return ""
	}
	return v
}

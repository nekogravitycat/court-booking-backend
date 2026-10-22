package session

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

// ErrInvalidSession is returned when a session token is unknown, expired, or revoked.
var ErrInvalidSession = apperror.New(http.StatusUnauthorized, "session is invalid or expired")

// Session is a server-side login session shared by every frontend of a browser.
type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

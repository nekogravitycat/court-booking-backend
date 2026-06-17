package favorite

import (
	"net/http"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrAlreadyFavorited = apperror.New(http.StatusConflict, "host already in favorites")
	ErrFavoriteNotFound = apperror.New(http.StatusNotFound, "favorite not found")
	ErrHostNotFound     = apperror.New(http.StatusNotFound, "host not found")
	ErrNotPickupHost    = apperror.New(http.StatusBadRequest, "host is not a pickup host")
)

// FavoriteHost is a brief view of a favorited host: only the public-facing
// nickname and avatar are exposed, per the favorites requirement.
type FavoriteHost struct {
	HostID   string
	Nickname *string // host display name
	Avatar   *string // ID of the host's avatar image file
}

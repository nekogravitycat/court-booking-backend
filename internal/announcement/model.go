package announcement

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound        = apperror.New(http.StatusNotFound, "announcement not found")
	ErrTitleRequired   = apperror.New(http.StatusBadRequest, "title is required")
	ErrContentRequired = apperror.New(http.StatusBadRequest, "content is required")
)

// Announcement represents a system-wide news or update.
type Announcement struct {
	ID        string
	Title     string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Filter defines parameters for listing announcements.
type Filter struct {
	Keyword   string
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

package announcement

import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("announcement not found")
	ErrTitleRequired   = errors.New("title is required")
	ErrContentRequired = errors.New("content is required")
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
	Keyword  string
	Page     int
	PageSize int
}

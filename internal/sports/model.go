package sports

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound        = apperror.New(http.StatusNotFound, "sport not found")
	ErrCodeRequired    = apperror.New(http.StatusBadRequest, "sport code is required")
	ErrNameRequired    = apperror.New(http.StatusBadRequest, "sport name is required")
	ErrCodeAlreadyUsed = apperror.New(http.StatusConflict, "sport code already used")
)

// Sport is an admin-managed ball sport used to categorize pickup groups.
type Sport struct {
	ID        string
	Code      string // Stable machine key (uppercase), e.g. "BADMINTON"
	Name      string // Human-readable display name
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Filter defines parameters for listing sports.
type Filter struct {
	// ActiveOnly limits results to sports that have not been soft-deleted.
	ActiveOnly bool
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

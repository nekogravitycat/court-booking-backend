package skilllevel

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound        = apperror.New(http.StatusNotFound, "skill level not found")
	ErrNameRequired    = apperror.New(http.StatusBadRequest, "skill level name is required")
	ErrSportRequired   = apperror.New(http.StatusBadRequest, "sport id is required")
	ErrSportNotFound   = apperror.New(http.StatusNotFound, "sport not found")
	ErrNameAlreadyUsed = apperror.New(http.StatusConflict, "skill level name already used for this sport")
)

// SkillLevel is an admin-managed grading tier scoped to a single sport.
type SkillLevel struct {
	ID        string
	SportID   string
	Name      string // Grade label, e.g. "A" / "Beginner"
	SortOrder int    // Display ordering within a sport
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Filter defines parameters for listing skill levels.
type Filter struct {
	// SportID limits results to a single sport when set.
	SportID string
	// ActiveOnly limits results to skill levels that have not been soft-deleted.
	ActiveOnly bool
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

package skilllevel

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound         = apperror.New(http.StatusNotFound, "skill level not found")
	ErrLabelRequired    = apperror.New(http.StatusBadRequest, "skill level label is required")
	ErrLevelInvalid     = apperror.New(http.StatusBadRequest, "skill level must be a positive integer")
	ErrSportRequired    = apperror.New(http.StatusBadRequest, "sport id is required")
	ErrSportNotFound    = apperror.New(http.StatusNotFound, "sport not found")
	ErrLabelAlreadyUsed = apperror.New(http.StatusConflict, "skill level label already used for this sport")
	ErrLevelAlreadyUsed = apperror.New(http.StatusConflict, "skill level already defined for this sport")
)

// SkillLevel maps an integer skill level of a sport to a display label. Skill
// levels are stored and compared everywhere as plain integers (so they can be
// averaged); this table only supplies the human-readable label and defines
// which levels a sport recognizes. Each sport has its own scale.
type SkillLevel struct {
	ID        string
	SportID   string
	Level     int    // Positive integer; higher means more skilled
	Label     string // Display label, e.g. "Beginner"
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

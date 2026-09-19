package skillrating

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrPermissionDenied = apperror.New(http.StatusForbidden, "only the pickup host or a system admin can rate participants")
	ErrGroupNotEnded    = apperror.New(http.StatusConflict, "participants can only be rated after the pickup group has ended")
	ErrNotParticipant   = apperror.New(http.StatusBadRequest, "user is not a confirmed participant of this pickup group")
	ErrCannotRateSelf   = apperror.New(http.StatusBadRequest, "you cannot rate yourself")
	ErrRatingNotFound   = apperror.New(http.StatusNotFound, "rating not found")
	ErrSkillLevelNotSet = apperror.New(http.StatusBadRequest, "skill level is not defined for the group's sport")
)

// Rating is a host's skill-level judgement of one participant for one group.
type Rating struct {
	ID            string
	PickupGroupID string
	UserID        string // The rated participant
	SportID       string
	Level         int
	RatedBy       *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// SportSummary is a user's composite rating in one sport: the arithmetic mean of
// every rating they received for that sport.
type SportSummary struct {
	SportID     string
	SportCode   string
	SportName   string
	Average     float64
	RatingCount int
	// Level is the average rounded to the nearest integer level and Label its
	// display label (empty when the sport has no mapping row for that level).
	Level int
	Label string
}

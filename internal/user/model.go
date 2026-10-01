package user

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrSkillLevelNotSet     = apperror.New(http.StatusBadRequest, "skill level not set for this sport; set it via PUT /me/skill-levels/{sport_id}")
	ErrInvalidSkillLevel    = apperror.New(http.StatusBadRequest, "skill level must be defined and active for this sport")
	ErrSportInactive        = apperror.New(http.StatusBadRequest, "sport is inactive")
	ErrNotFound             = apperror.New(http.StatusNotFound, "user not found")
	ErrEmailAlreadyUsed     = apperror.New(http.StatusConflict, "email already used")
	ErrInvalidCredentials   = apperror.New(http.StatusUnauthorized, "invalid email or password")
	ErrInactiveUser         = apperror.New(http.StatusUnauthorized, "user is inactive")
	ErrEmailRequired        = apperror.New(http.StatusBadRequest, "email is required")
	ErrPasswordTooShort     = apperror.New(http.StatusBadRequest, "password is too short")
	ErrPasswordTooLong      = apperror.New(http.StatusBadRequest, "password is too long")
	ErrAlreadyPickupHost    = apperror.New(http.StatusConflict, "user is already a pickup host")
	ErrNotPickupHost        = apperror.New(http.StatusNotFound, "user is not a pickup host")
	ErrUsernameRequired     = apperror.New(http.StatusBadRequest, "username is required")
	ErrInvalidUsername      = apperror.New(http.StatusBadRequest, "username must be 4-15 characters of lowercase letters, digits, or underscore")
	ErrUsernameAlreadyUsed  = apperror.New(http.StatusConflict, "username already used")
	ErrCannotRevokeOwnAdmin = apperror.New(http.StatusForbidden, "cannot revoke your own system admin privilege")
	ErrInvalidGender        = apperror.New(http.StatusBadRequest, "gender must be one of male, female, other")
	ErrInvalidBirthDate     = apperror.New(http.StatusBadRequest, "birth_date must be a past date on or after 1900-01-01")
)

// SportSkillLevel is an account's self-reported level, separate from host ratings.
type SportSkillLevel struct {
	SportID    string
	SportName  string
	SkillLevel int
	Label      string
	IsActive   bool
	UpdatedAt  time.Time
}

// User represents a user in the system.
type User struct {
	ID            string // UUID
	Email         string
	Username      string // Unique, immutable handle (lowercase letters, digits, underscore)
	PasswordHash  string
	DisplayName   *string
	Phone         *string
	Gender        *string    // One of the Gender* constants; nil when not provided
	BirthDate     *time.Time // Calendar date (UTC midnight); nil when not provided
	Avatar        *string    // ID of avatar image file
	CreatedAt     time.Time
	LastLoginAt   *time.Time
	IsActive      bool
	IsSystemAdmin bool
	IsPickupHost  bool
	Organizations []UserOrganizationBrief
}

// UserFilter defines filter options for listing users.
type UserFilter struct {
	Email       string
	IDs         []string
	DisplayName string
	IsActive    *bool // Use pointer to distinguish between false and nil (not set)

	PickupHostsOnly bool // When true, only return users with the pickup host role

	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// UserOrganizationBrief holds minimal organization info for list views.
type UserOrganizationBrief struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Owner               bool     `json:"owner"`
	OrganizationManager bool     `json:"organization_manager"`
	LocationManager     []string `json:"location_manager"`
}

// Gender values accepted for users and party members.
const (
	GenderMale   = "male"
	GenderFemale = "female"
	GenderOther  = "other"
)

// IsValidGender reports whether g is one of the accepted gender values.
func IsValidGender(g string) bool {
	return g == GenderMale || g == GenderFemale || g == GenderOther
}

// minBirthDate is the earliest accepted birth date.
var minBirthDate = time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)

// ParseBirthDate parses a "YYYY-MM-DD" birth date and rejects future dates and
// dates before 1900-01-01. The result is the calendar date at UTC midnight.
func ParseBirthDate(s string) (time.Time, error) {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, ErrInvalidBirthDate
	}
	if d.Before(minBirthDate) || d.After(time.Now().UTC().AddDate(0, 0, 1)) {
		return time.Time{}, ErrInvalidBirthDate
	}
	return d, nil
}

// AgeOn returns the age in whole years of someone born on the calendar date
// birth, as of the calendar date of now. Only the year/month/day of each value
// is considered, so callers pass now already converted to the desired zone.
func AgeOn(birth, now time.Time) int {
	age := now.Year() - birth.Year()
	if now.Month() < birth.Month() || (now.Month() == birth.Month() && now.Day() < birth.Day()) {
		age--
	}
	return age
}

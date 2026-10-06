package location

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrOrganizationRoleConflict = apperror.New(http.StatusConflict, "user is already an organization manager or owner")
	ErrOrgNotFound              = apperror.New(http.StatusNotFound, "organization not found")
	ErrLocNotFound              = apperror.New(http.StatusNotFound, "location not found")
	ErrLocationInUse            = apperror.New(http.StatusConflict, "location still has resources with related bookings and cannot be deleted")
	ErrOrgIDRequired            = apperror.New(http.StatusBadRequest, "organization_id is required")
	ErrNameRequired             = apperror.New(http.StatusBadRequest, "name is required")
	ErrInvalidGeo               = apperror.New(http.StatusBadRequest, "invalid latitude or longitude")
	ErrInvalidOpeningHours      = apperror.New(http.StatusBadRequest, "opening hours start must be before end")
	ErrCapacityInvalid          = apperror.New(http.StatusBadRequest, "capacity must be greater than zero")
	ErrInvalidTimeRange         = apperror.New(http.StatusBadRequest, "start time must be before end time")
	ErrNotOrganizationMember    = apperror.New(http.StatusBadRequest, "user must be a member of the organization first")
	ErrUserNotFound             = apperror.New(http.StatusNotFound, "user not found")
	ErrInvalidTimezone          = apperror.New(http.StatusBadRequest, "invalid timezone; expected an IANA name such as Asia/Taipei")
	ErrInvalidBookingWindow     = apperror.New(http.StatusBadRequest, "minimum_booking_notice_minutes must be >= 0, maximum_booking_advance_days must be between 1 and 90, and the notice must not exceed the advance")
	ErrInvalidParking           = apperror.New(http.StatusBadRequest, "parking name, latitude and longitude must be provided together and be valid")
)

// Location represents a physical venue under an organization.
type Location struct {
	ID                string
	OrganizationID    string
	Name              string
	OrganizationName  string
	CreatedAt         time.Time
	Capacity          int64
	OpeningHoursStart string // Format: HH:MM:SS
	OpeningHoursEnd   string // Format: HH:MM:SS
	Timezone          string // IANA timezone name (e.g. "Asia/Taipei") for interpreting opening hours
	LocationInfo      string // Address
	Opening           bool   // Is currently open for business
	Rule              string
	Facility          string
	Description       string
	Longitude         float64
	Latitude          float64
	// Optional parking lot. The three fields are either all set or all nil.
	ParkingName      *string
	ParkingLatitude  *float64
	ParkingLongitude *float64
	Cover            *string // ID of cover image file

	// Booking window applied to ordinary (non-series) bookings.
	MinimumBookingNoticeMinutes int
	MaximumBookingAdvanceDays   int
}

// Booking window limits and defaults. MaxBookingAdvanceDays is the system-wide
// hard cap on how far ahead an ordinary booking may start.
const (
	DefaultMinimumBookingNoticeMinutes = 0
	DefaultMaximumBookingAdvanceDays   = 90
	MaxBookingAdvanceDays              = 90
)

// LocationFilter defines parameters for listing locations.
type LocationFilter struct {
	OrganizationID string
	Page           int
	PageSize       int

	// Filters

	Name                 string // Keyword search in location name
	Opening              *bool
	CapacityMin          *int64
	CapacityMax          *int64
	OpeningHoursStartMin string // Format: HH:MM:SS
	OpeningHoursStartMax string // Format: HH:MM:SS
	OpeningHoursEndMin   string // Format: HH:MM:SS
	OpeningHoursEndMax   string // Format: HH:MM:SS
	CreatedAtFrom        time.Time
	CreatedAtTo          time.Time

	// Sorting

	SortBy    string // "name", "capacity", "opening_hours_start", "opening_hours_end", "created_at"
	SortOrder string // "ASC" or "DESC"
}

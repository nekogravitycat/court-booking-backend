package location

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrOrgNotFound         = apperror.New(http.StatusNotFound, "organization not found")
	ErrLocNotFound         = apperror.New(http.StatusNotFound, "location not found")
	ErrOrgIDRequired       = apperror.New(http.StatusBadRequest, "organization_id is required")
	ErrNameRequired        = apperror.New(http.StatusBadRequest, "name is required")
	ErrInvalidGeo          = apperror.New(http.StatusBadRequest, "invalid latitude or longitude")
	ErrInvalidOpeningHours = apperror.New(http.StatusBadRequest, "opening hours start must be before end")
	ErrCapacityInvalid     = apperror.New(http.StatusBadRequest, "capacity must be greater than zero")
	ErrInvalidTimeRange    = apperror.New(http.StatusBadRequest, "start time must be before end time")
)

// Location represents a physical venue under an organization.
type Location struct {
	ID                string
	OrganizationID    string
	Name              string
	CreatedAt         time.Time
	Capacity          int64
	OpeningHoursStart string // Format: HH:MM:SS
	OpeningHoursEnd   string // Format: HH:MM:SS
	LocationInfo      string // Address
	Opening           bool   // Is currently open for business
	Rule              string
	Facility          string
	Description       string
	Longitude         float64
	Latitude          float64
}

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

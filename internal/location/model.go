package location

import (
	"errors"
	"time"
)

var (
	ErrOrgNotFound         = errors.New("organization not found")
	ErrLocNotFound         = errors.New("location not found")
	ErrOrgIDRequired       = errors.New("organization_id is required")
	ErrNameRequired        = errors.New("name is required")
	ErrInvalidGeo          = errors.New("invalid latitude or longitude")
	ErrInvalidOpeningHours = errors.New("opening hours start must be before end")
	ErrCapacityInvalid     = errors.New("capacity must be greater than zero")
	ErrInvalidTimeRange    = errors.New("start time must be before end time")
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

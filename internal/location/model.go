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
	Keyword        string // Search in Name or LocationInfo
	Page           int
	PageSize       int
}

package location

import (
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("location not found")
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

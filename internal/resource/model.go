package resource

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound            = apperror.New(http.StatusNotFound, "resource not found")
	ErrEmptyName           = apperror.New(http.StatusBadRequest, "name cannot be empty")
	ErrInvalidLocation     = apperror.New(http.StatusBadRequest, "invalid location_id")
	ErrInvalidResourceType = apperror.New(http.StatusBadRequest, "invalid resource_type")
)

// ValidResourceTypes defines the allowed resource type enum values
var ValidResourceTypes = []string{
	"badminton",
	"tennis",
	"basketball",
	"table_tennis",
	"volleyball",
	"football",
}

// Resource represents a bookable unit (e.g., Court A, Room 101).
type Resource struct {
	ID           string
	ResourceType string
	LocationID   string
	LocationName string
	Name         string
	CreatedAt    time.Time
}

// Filter defines parameters for listing resources.
type Filter struct {
	OrganizationID string
	LocationID     string
	ResourceType   string
	Page           int
	PageSize       int
	SortBy         string
	SortOrder      string
}

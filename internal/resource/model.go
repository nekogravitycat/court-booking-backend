package resource

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound            = apperror.New(http.StatusNotFound, "resource not found")
	ErrOrgMismatch         = apperror.New(http.StatusBadRequest, "location and resource type must belong to the same organization")
	ErrEmptyName           = apperror.New(http.StatusBadRequest, "name cannot be empty")
	ErrInvalidLocation     = apperror.New(http.StatusBadRequest, "invalid location_id")
	ErrInvalidResourceType = apperror.New(http.StatusBadRequest, "invalid resource_type_id")
)

// Resource represents a bookable unit (e.g., Court A, Room 101).
type Resource struct {
	ID             string
	ResourceTypeID string
	LocationID     string
	Name           string
	CreatedAt      time.Time
}

// Filter defines parameters for listing resources.
type Filter struct {
	LocationID     string
	ResourceTypeID string
	Page           int
	PageSize       int
	SortBy         string
	SortOrder      string
}

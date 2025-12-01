package resource

import (
	"errors"
	"time"
)

var (
	ErrNotFound            = errors.New("resource not found")
	ErrOrgMismatch         = errors.New("location and resource type must belong to the same organization")
	ErrEmptyName           = errors.New("name cannot be empty")
	ErrInvalidLocation     = errors.New("invalid location_id")
	ErrInvalidResourceType = errors.New("invalid resource_type_id")
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

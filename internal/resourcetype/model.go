package resourcetype

import (
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("resource type not found")
)

// ResourceType represents a category of resources (e.g., Badminton Court).
type ResourceType struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
	CreatedAt      time.Time
}

// Filter defines parameters for listing resource types.
type Filter struct {
	OrganizationID string
	Page           int
	PageSize       int
}

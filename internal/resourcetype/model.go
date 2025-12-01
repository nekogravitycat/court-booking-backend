package resourcetype

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound      = apperror.New(http.StatusNotFound, "resource type not found")
	ErrOrgIDRequired = apperror.New(http.StatusBadRequest, "organization_id is required")
	ErrNameRequired  = apperror.New(http.StatusBadRequest, "name is required")
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
	SortBy         string
	SortOrder      string
}

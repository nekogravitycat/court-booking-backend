package organization

import "time"

// Organization represents a venue owner or brand entity.
type Organization struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	IsActive  bool
}

// OrganizationFilter defines filter options for listing organizations.
type OrganizationFilter struct {
	Page     int
	PageSize int
}

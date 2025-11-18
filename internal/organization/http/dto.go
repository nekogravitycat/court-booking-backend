package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/organization"
)

// OrganizationResponse matches the OAS definition.
type OrganizationResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateOrganizationRequest is the payload for POST /organizations.
type CreateOrganizationRequest struct {
	Name string `json:"name" binding:"required"`
}

// UpdateOrganizationRequest is the payload for PATCH /organizations/:id.
type UpdateOrganizationRequest struct {
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

func NewOrganizationResponse(o *organization.Organization) OrganizationResponse {
	return OrganizationResponse{
		ID:        o.ID,
		Name:      o.Name,
		CreatedAt: o.CreatedAt,
	}
}

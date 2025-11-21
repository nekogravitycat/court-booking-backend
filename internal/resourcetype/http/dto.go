package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/resourcetype"
)

type ResourceTypeResponse struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
}

func NewResponse(rt *resourcetype.ResourceType) ResourceTypeResponse {
	return ResourceTypeResponse{
		ID:             rt.ID,
		OrganizationID: rt.OrganizationID,
		Name:           rt.Name,
		Description:    rt.Description,
		CreatedAt:      rt.CreatedAt,
	}
}

type CreateRequest struct {
	OrganizationID string `json:"organization_id" binding:"required,uuid"`
	Name           string `json:"name" binding:"required"`
	Description    string `json:"description"`
}

type UpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

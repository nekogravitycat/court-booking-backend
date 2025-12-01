package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/resourcetype"
)

// ListResourceTypesRequest defines query parameters for listing resource types.
type ListResourceTypesRequest struct {
	request.ListParams
	OrganizationID string `form:"organization_id" binding:"omitempty,uuid"`
	SortBy         string `form:"sort_by" binding:"omitempty,oneof=name created_at"`
}

// Validate performs custom validation for ListResourceTypesRequest.
func (r *ListResourceTypesRequest) Validate() error {
	return nil
}

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
	Name           string `json:"name" binding:"required,min=1,max=100"`
	Description    string `json:"description"`
}

// Validate performs custom validation for CreateRequest.
func (r *CreateRequest) Validate() error {
	return nil
}

type UpdateRequest struct {
	Name        *string `json:"name" binding:"omitempty,min=1,max=100"`
	Description *string `json:"description"`
}

// Validate performs custom validation for UpdateRequest.
func (r *UpdateRequest) Validate() error {
	return nil
}

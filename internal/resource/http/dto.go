package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
)

type ListResourcesRequest struct {
	request.ListParams
	LocationID     string `form:"location_id" binding:"omitempty,uuid"`
	ResourceTypeID string `form:"resource_type_id" binding:"omitempty,uuid"`
	SortBy         string `form:"sort_by" binding:"omitempty,oneof=name created_at"`
}

// Validate performs custom validation for ListResourcesRequest.
func (r *ListResourcesRequest) Validate() error {
	return nil
}

type ResourceResponse struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ResourceTypeID string    `json:"resource_type_id"`
	LocationID     string    `json:"location_id"`
	CreatedAt      time.Time `json:"created_at"`
}

func NewResponse(r *resource.Resource) ResourceResponse {
	return ResourceResponse{
		ID:             r.ID,
		Name:           r.Name,
		ResourceTypeID: r.ResourceTypeID,
		LocationID:     r.LocationID,
		CreatedAt:      r.CreatedAt,
	}
}

type CreateRequest struct {
	Name           string `json:"name" binding:"required"`
	LocationID     string `json:"location_id" binding:"required,uuid"`
	ResourceTypeID string `json:"resource_type_id" binding:"required,uuid"`
}

// Validate performs custom validation for CreateRequest.
func (r *CreateRequest) Validate() error {
	return nil
}

type UpdateRequest struct {
	Name *string `json:"name" binding:"omitempty,min=1,max=100"`
}

// Validate performs custom validation for UpdateRequest.
func (r *UpdateRequest) Validate() error {
	return nil
}

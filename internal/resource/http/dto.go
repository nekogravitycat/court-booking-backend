package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/resource"
)

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

type UpdateRequest struct {
	Name *string `json:"name" binding:"omitempty"`
}

package http

import (
	"time"

	locHttp "github.com/nekogravitycat/court-booking-backend/internal/location/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
	rtHttp "github.com/nekogravitycat/court-booking-backend/internal/resourcetype/http"
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
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	ResourceType rtHttp.ResourceTypeTag `json:"resource_type"`
	Location     locHttp.LocationTag    `json:"location"`
	CreatedAt    time.Time              `json:"created_at"`
}

// ResourceTag is a brief representation of a resource.
type ResourceTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewResponse(r *resource.Resource) ResourceResponse {
	return ResourceResponse{
		ID:           r.ID,
		Name:         r.Name,
		ResourceType: rtHttp.ResourceTypeTag{ID: r.ResourceTypeID, Name: r.ResourceTypeName},
		Location:     locHttp.LocationTag{ID: r.LocationID, Name: r.LocationName},
		CreatedAt:    r.CreatedAt,
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

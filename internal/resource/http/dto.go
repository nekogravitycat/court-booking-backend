package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/booking"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	locHttp "github.com/nekogravitycat/court-booking-backend/internal/location/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
)

type ListResourcesRequest struct {
	request.ListParams
	OrganizationID string `form:"organization_id" binding:"omitempty,uuid"`
	LocationID     string `form:"location_id" binding:"omitempty,uuid"`
	ResourceType   string `form:"resource_type" binding:"omitempty"`
	SortBy         string `form:"sort_by" binding:"omitempty,oneof=name created_at"`
}

// Validate performs custom validation for ListResourcesRequest.
func (r *ListResourcesRequest) Validate() error {
	return nil
}

type ResourceResponse struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Price          int                 `json:"price"`
	ResourceType   string              `json:"resource_type"`
	Location       locHttp.LocationTag `json:"location"`
	Cover          *string             `json:"cover"`           // URL to cover image
	CoverThumbnail *string             `json:"cover_thumbnail"` // URL to cover thumbnail
	CreatedAt      time.Time           `json:"created_at"`
}

// ResourceTag is a brief representation of a resource.
type ResourceTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewResponse(r *resource.Resource) ResourceResponse {
	var coverURL *string
	var coverThumbnailURL *string

	if r.Cover != nil {
		url := file.FileURL(*r.Cover)
		coverURL = &url

		thumbURL := file.ThumbnailURL(*r.Cover)
		coverThumbnailURL = &thumbURL
	}

	return ResourceResponse{
		ID:             r.ID,
		Name:           r.Name,
		Price:          r.Price,
		ResourceType:   r.ResourceType,
		Location:       locHttp.LocationTag{ID: r.LocationID, Name: r.LocationName},
		Cover:          coverURL,
		CoverThumbnail: coverThumbnailURL,
		CreatedAt:      r.CreatedAt.UTC(),
	}
}

type CreateRequest struct {
	Name         string `json:"name" binding:"required"`
	Price        int    `json:"price" binding:"min=0"`
	LocationID   string `json:"location_id" binding:"required,uuid"`
	ResourceType string `json:"resource_type" binding:"required"`
}

// Validate performs custom validation for CreateRequest.
func (r *CreateRequest) Validate() error {
	return nil
}

type UpdateRequest struct {
	Name  *string `json:"name" binding:"omitempty,min=1,max=100"`
	Price *int    `json:"price" binding:"omitempty,min=0"`
}

// Validate performs custom validation for UpdateRequest.
func (r *UpdateRequest) Validate() error {
	return nil
}

type TimeSlot struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

type AvailabilityResponse struct {
	Date  string     `json:"date"`
	Slots []TimeSlot `json:"slots"`
}

func NewAvailabilityResponse(date time.Time, slots []booking.TimeSlot) AvailabilityResponse {
	dtos := make([]TimeSlot, len(slots))
	for i, s := range slots {
		dtos[i] = TimeSlot{
			StartTime: s.StartTime.UTC(),
			EndTime:   s.EndTime.UTC(),
		}
	}
	return AvailabilityResponse{
		Date:  date.Format("2006-01-02"),
		Slots: dtos,
	}
}

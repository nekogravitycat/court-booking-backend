package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/booking"
	locHttp "github.com/nekogravitycat/court-booking-backend/internal/location/http"
	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	resHttp "github.com/nekogravitycat/court-booking-backend/internal/resource/http"
	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
)

// ListBookingsRequest defines query parameters for listing bookings.
type ListBookingsRequest struct {
	request.ListParams
	ResourceID    string     `form:"resource_id" binding:"omitempty,uuid"`
	Status        string     `form:"status" binding:"omitempty,oneof=pending confirmed cancelled"`
	UserID        string     `form:"user_id" binding:"omitempty,uuid"`
	StartTimeFrom *time.Time `form:"start_time_from" time_format:"2006-01-02T15:04:05Z07:00"`
	StartTimeTo   *time.Time `form:"start_time_to" time_format:"2006-01-02T15:04:05Z07:00"`
	SortBy        string     `form:"sort_by" binding:"omitempty,oneof=start_time end_time created_at status"`
}

// Validate performs custom validation for ListBookingsRequest.
func (r *ListBookingsRequest) Validate() error {
	if r.StartTimeFrom != nil && r.StartTimeTo != nil {
		if r.StartTimeFrom.After(*r.StartTimeTo) {
			return booking.ErrInvalidTimeRange
		}
	}
	return nil
}

type BookingResponse struct {
	ID           string                  `json:"id"`
	Resource     resHttp.ResourceTag     `json:"resource"`
	User         userHttp.UserTag        `json:"user"`
	Location     locHttp.LocationTag     `json:"location"`
	Organization orgHttp.OrganizationTag `json:"organization"`
	StartTime    time.Time               `json:"start_time"`
	EndTime      time.Time               `json:"end_time"`
	Status       string                  `json:"status"`
	CreatedAt    time.Time               `json:"created_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
}

func NewBookingResponse(b *booking.Booking) BookingResponse {
	return BookingResponse{
		ID:           b.ID,
		Resource:     resHttp.ResourceTag{ID: b.ResourceID, Name: b.ResourceName},
		User:         userHttp.UserTag{ID: b.UserID, Name: b.UserName},
		Location:     locHttp.LocationTag{ID: b.LocationID, Name: b.LocationName},
		Organization: orgHttp.OrganizationTag{ID: b.OrganizationID, Name: b.OrganizationName},
		StartTime:    b.StartTime,
		EndTime:      b.EndTime,
		Status:       string(b.Status),
		CreatedAt:    b.CreatedAt,
		UpdatedAt:    b.UpdatedAt,
	}
}

type CreateBookingRequest struct {
	ResourceID string    `json:"resource_id" binding:"required,uuid"`
	StartTime  time.Time `json:"start_time" binding:"required"`
	EndTime    time.Time `json:"end_time" binding:"required"`
}

// Validate performs custom validation for CreateBookingRequest.
func (r *CreateBookingRequest) Validate() error {
	if r.StartTime.After(r.EndTime) {
		return booking.ErrInvalidTimeRange
	}
	if r.StartTime.Before(time.Now()) {
		return booking.ErrStartTimePast
	}
	return nil
}

type UpdateBookingRequest struct {
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
	Status    *string    `json:"status" binding:"omitempty,oneof=pending confirmed cancelled"`
}

// Validate performs custom validation for UpdateBookingRequest.
func (r *UpdateBookingRequest) Validate() error {
	if r.StartTime != nil && r.EndTime != nil {
		if r.StartTime.After(*r.EndTime) {
			return booking.ErrInvalidTimeRange
		}
	}
	return nil
}

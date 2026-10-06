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
	ResourceID     string     `form:"resource_id" binding:"omitempty,uuid"`
	OrganizationID string     `form:"organization_id" binding:"omitempty,uuid"`
	Status         string     `form:"status" binding:"omitempty,oneof=pending confirmed cancelled cancel_request"`
	UserID         string     `form:"user_id" binding:"omitempty,uuid"`
	StartTimeFrom  *time.Time `form:"start_time_from" time_format:"2006-01-02T15:04:05Z07:00"`
	StartTimeTo    *time.Time `form:"start_time_to" time_format:"2006-01-02T15:04:05Z07:00"`
	SortBy         string     `form:"sort_by" binding:"omitempty,oneof=start_time end_time created_at status"`
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
	ID            string                  `json:"id"`
	Resource      resHttp.ResourceTag     `json:"resource"`
	SportID       *string                 `json:"sport_id"`
	User          userHttp.UserTag        `json:"user"`
	Location      locHttp.LocationTag     `json:"location"`
	Organization  orgHttp.OrganizationTag `json:"organization"`
	StartTime     time.Time               `json:"start_time"`
	EndTime       time.Time               `json:"end_time"`
	Status        string                  `json:"status"`
	PaymentStatus string                  `json:"payment_status"`
	// BookingSeriesID is null for an ordinary single booking.
	BookingSeriesID *string   `json:"booking_series_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func NewBookingResponse(b *booking.Booking) BookingResponse {
	return BookingResponse{
		ID:              b.ID,
		Resource:        resHttp.ResourceTag{ID: b.ResourceID, Name: b.ResourceName},
		SportID:         b.SportID,
		User:            userHttp.UserTag{ID: b.UserID, Name: b.UserName},
		Location:        locHttp.LocationTag{ID: b.LocationID, Name: b.LocationName},
		Organization:    orgHttp.OrganizationTag{ID: b.OrganizationID, Name: b.OrganizationName},
		StartTime:       b.StartTime.UTC(),
		EndTime:         b.EndTime.UTC(),
		Status:          string(b.Status),
		PaymentStatus:   string(b.PaymentStatus),
		BookingSeriesID: b.BookingSeriesID,
		CreatedAt:       b.CreatedAt.UTC(),
		UpdatedAt:       b.UpdatedAt.UTC(),
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
	StartTime     *time.Time `json:"start_time"`
	EndTime       *time.Time `json:"end_time"`
	Status        *string    `json:"status" binding:"omitempty,oneof=pending confirmed cancelled cancel_request"`
	PaymentStatus *string    `json:"payment_status" binding:"omitempty,oneof=done pending failed"`
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

// BookingSeriesResponse is a seasonal rental with all of its bookings.
type BookingSeriesResponse struct {
	ID         string              `json:"id"`
	User       userHttp.UserTag    `json:"user"`
	Resource   resHttp.ResourceTag `json:"resource"`
	TermMonths int                 `json:"term_months"`
	CreatedAt  time.Time           `json:"created_at"`
	Bookings   []BookingResponse   `json:"bookings"`
}

func NewBookingSeriesResponse(s *booking.BookingSeries) BookingSeriesResponse {
	resp := BookingSeriesResponse{
		ID:         s.ID,
		TermMonths: s.TermMonths,
		CreatedAt:  s.CreatedAt.UTC(),
		Bookings:   make([]BookingResponse, len(s.Bookings)),
	}
	for i, b := range s.Bookings {
		resp.Bookings[i] = NewBookingResponse(b)
	}
	if len(s.Bookings) > 0 {
		first := s.Bookings[0]
		resp.User = userHttp.UserTag{ID: first.UserID, Name: first.UserName}
		resp.Resource = resHttp.ResourceTag{ID: first.ResourceID, Name: first.ResourceName}
	}
	return resp
}

// CreateBookingSeriesRequest asks for the same daily time slot on the chosen
// weekdays (0 = Sunday ... 6 = Saturday) for term_months starting at start_date.
// Dates and times are wall-clock values in the resource location timezone.
type CreateBookingSeriesRequest struct {
	ResourceID string `json:"resource_id" binding:"required,uuid"`
	TermMonths int    `json:"term_months" binding:"required,oneof=3 6 12"`
	StartDate  string `json:"start_date" binding:"required,datetime=2006-01-02"`
	Weekdays   []int  `json:"weekdays" binding:"required,min=1,max=7,dive,min=0,max=6"`
	StartTime  string `json:"start_time" binding:"required,datetime=15:04"`
	EndTime    string `json:"end_time" binding:"required,datetime=15:04"`
}

// ConflictResponse is the 409 body of a series creation that overlaps existing bookings.
type ConflictResponse struct {
	Error     string             `json:"error"`
	Conflicts []resHttp.TimeSlot `json:"conflicts"`
}

// LocationAvailabilityResponse groups the availability of every resource of a location.
type LocationAvailabilityResponse struct {
	Date      string                         `json:"date"`
	Resources []ResourceAvailabilityResponse `json:"resources"`
}

type ResourceAvailabilityResponse struct {
	Resource resHttp.ResourceTag `json:"resource"`
	Slots    []resHttp.TimeSlot  `json:"slots"`
}

func NewLocationAvailabilityResponse(date time.Time, items []booking.ResourceAvailability) LocationAvailabilityResponse {
	resp := LocationAvailabilityResponse{
		Date:      date.Format("2006-01-02"),
		Resources: make([]ResourceAvailabilityResponse, len(items)),
	}
	for i, it := range items {
		resp.Resources[i] = ResourceAvailabilityResponse{
			Resource: resHttp.ResourceTag{ID: it.Resource.ID, Name: it.Resource.Name},
			// Same conversion as the single-resource availability response.
			Slots: resHttp.NewAvailabilityResponse(date, it.Slots).Slots,
		}
	}
	return resp
}

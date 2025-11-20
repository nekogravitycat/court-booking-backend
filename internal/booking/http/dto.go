package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/booking"
)

type BookingResponse struct {
	ID         string    `json:"id"`
	ResourceID string    `json:"resource_id"`
	UserID     string    `json:"user_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func NewBookingResponse(b *booking.Booking) BookingResponse {
	return BookingResponse{
		ID:         b.ID,
		ResourceID: b.ResourceID,
		UserID:     b.UserID,
		StartTime:  b.StartTime,
		EndTime:    b.EndTime,
		Status:     string(b.Status),
		CreatedAt:  b.CreatedAt,
		UpdatedAt:  b.UpdatedAt,
	}
}

type CreateBookingBody struct {
	ResourceID string    `json:"resource_id" binding:"required,uuid"`
	StartTime  time.Time `json:"start_time" binding:"required"`
	EndTime    time.Time `json:"end_time" binding:"required"`
}

type UpdateBookingBody struct {
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
	Status    *string    `json:"status" binding:"omitempty,oneof=pending confirmed cancelled"`
}

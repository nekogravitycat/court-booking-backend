package booking

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound                   = apperror.New(http.StatusNotFound, "booking not found")
	ErrTimeConflict               = apperror.New(http.StatusConflict, "time slot already booked")
	ErrInvalidTimeRange           = apperror.New(http.StatusBadRequest, "start time must be before end time")
	ErrInvalidStatus              = apperror.New(http.StatusBadRequest, "invalid booking status")
	ErrResourceNotFound           = apperror.New(http.StatusNotFound, "resource not found")
	ErrPermissionDenied           = apperror.New(http.StatusForbidden, "permission denied")
	ErrStartTimePast              = apperror.New(http.StatusBadRequest, "cannot create booking in the past")
	ErrCancellationRequiresReview = apperror.New(http.StatusForbidden, "cancelling a confirmed, paid, or cancellation-requested booking requires manager approval")
	ErrChangeRequiresReview       = apperror.New(http.StatusForbidden, "this booking can no longer be changed by its owner; contact a manager")
	ErrNotAligned                 = apperror.New(http.StatusBadRequest, "booking start and end must fall on a 30-minute boundary")
	ErrTooFarInAdvance            = apperror.New(http.StatusBadRequest, "booking start is too far in the future")
	ErrTooManyActiveBookings      = apperror.New(http.StatusConflict, "too many active upcoming bookings")
	ErrConcurrentUpdate           = apperror.New(http.StatusConflict, "booking was modified by someone else; reload and retry")
	ErrInvalidInput               = apperror.New(http.StatusBadRequest, "invalid input parameters")

	ErrLocationClosed      = apperror.New(http.StatusConflict, "location is not open for booking")
	ErrOutsideOpeningHours = apperror.New(http.StatusBadRequest, "booking must fall within the location's opening hours")
	ErrBookingTooLong      = apperror.New(http.StatusBadRequest, "booking duration exceeds the maximum allowed")
	ErrInvalidTimezone     = apperror.New(http.StatusInternalServerError, "location has an invalid timezone")
)

// MaxBookingDuration is a defensive upper bound on the length of a single
// booking. It prevents accidental or abusive multi-day/multi-month reservations
// that the opening-hours window alone would not catch. Tune as the business
// rules require.
const MaxBookingDuration = 24 * time.Hour

// Anti-abuse limits on bookings.
const (
	// MaxAdvanceBooking is how far ahead a booking may start.
	MaxAdvanceBooking = 90 * 24 * time.Hour
	// BookingSlotGranularity is the boundary (in the location's local clock)
	// that booking start and end times must align to.
	BookingSlotGranularity = 30 * time.Minute
	// MaxActiveBookingsPerUser caps a user's upcoming, non-cancelled bookings.
	MaxActiveBookingsPerUser = 10
)

type Status string

const (
	StatusPending       Status = "pending"
	StatusConfirmed     Status = "confirmed"
	StatusCancelled     Status = "cancelled"
	StatusCancelRequest Status = "cancel_request"
)

// IsValid reports whether the booking status is a recognized value.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusConfirmed, StatusCancelled, StatusCancelRequest:
		return true
	}
	return false
}

type PaymentStatus string

const (
	PaymentStatusDone    PaymentStatus = "done"
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusFailed  PaymentStatus = "failed"
)

// IsValid reports whether the payment status is a recognized value.
func (p PaymentStatus) IsValid() bool {
	switch p {
	case PaymentStatusDone, PaymentStatusPending, PaymentStatusFailed:
		return true
	}
	return false
}

type Booking struct {
	ID               string
	ResourceID       string
	ResourceName     string
	SportID          *string // Sport of the booked resource, nil if unset
	UserID           string
	UserName         string
	LocationID       string
	LocationName     string
	OrganizationID   string
	OrganizationName string
	StartTime        time.Time
	EndTime          time.Time
	Status           Status
	PaymentStatus    PaymentStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Filter struct {
	UserID         string
	ResourceID     string
	OrganizationID string
	Status         string
	StartTime      *time.Time // Filter bookings starting after this time
	EndTime        *time.Time // Filter bookings ending before this time
	Page           int
	PageSize       int
	SortBy         string
	SortOrder      string
}

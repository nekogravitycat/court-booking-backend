package booking

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound         = apperror.New(http.StatusNotFound, "booking not found")
	ErrTimeConflict     = apperror.New(http.StatusConflict, "time slot already booked")
	ErrInvalidTimeRange = apperror.New(http.StatusBadRequest, "start time must be before end time")
	ErrInvalidStatus    = apperror.New(http.StatusBadRequest, "invalid booking status")
	ErrResourceNotFound = apperror.New(http.StatusNotFound, "resource not found")
	ErrPermissionDenied = apperror.New(http.StatusForbidden, "permission denied")
	ErrStartTimePast    = apperror.New(http.StatusBadRequest, "cannot create booking in the past")
	ErrInvalidInput     = apperror.New(http.StatusBadRequest, "invalid input parameters")

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

// availabilityPageSize is the batch size used when paging through a day's
// bookings to compute availability, ensuring no bookings are silently dropped.
const availabilityPageSize = 1000

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

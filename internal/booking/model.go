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
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
	StatusCancelled Status = "cancelled"
)

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

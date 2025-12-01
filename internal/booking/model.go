package booking

import (
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("booking not found")
	ErrTimeConflict     = errors.New("time slot already booked")
	ErrInvalidTimeRange = errors.New("start time must be before end time")
	ErrInvalidStatus    = errors.New("invalid booking status")
	ErrResourceNotFound = errors.New("resource not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrStartTimePast    = errors.New("cannot create booking in the past")
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
	StatusCancelled Status = "cancelled"
)

type Booking struct {
	ID         string
	ResourceID string
	UserID     string
	StartTime  time.Time
	EndTime    time.Time
	Status     Status
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Filter struct {
	UserID     string
	ResourceID string
	Status     string
	StartTime  *time.Time // Filter bookings starting after this time
	EndTime    *time.Time // Filter bookings ending before this time
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

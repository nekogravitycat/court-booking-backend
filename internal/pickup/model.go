package pickup

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrGroupNotFound         = apperror.New(http.StatusNotFound, "pickup group not found")
	ErrOrderNotFound         = apperror.New(http.StatusNotFound, "pickup order not found")
	ErrGroupFullyBooked      = apperror.New(http.StatusConflict, "group is fully booked")
	ErrCapacityBelowEnrolled = apperror.New(http.StatusConflict, "capacity cannot be set below the current number of enrolled participants")
	ErrAlreadyEnrolled       = apperror.New(http.StatusConflict, "already enrolled in this group")
	ErrInvalidStatus         = apperror.New(http.StatusBadRequest, "invalid status")
	ErrInvalidTimeRange      = apperror.New(http.StatusBadRequest, "start time must be before end time")
	ErrPermissionDenied      = apperror.New(http.StatusForbidden, "permission denied")
	ErrGroupNotActive        = apperror.New(http.StatusBadRequest, "pickup group is not active")
)

type SkillLevel string

const (
	SkillLevelA SkillLevel = "A"
	SkillLevelB SkillLevel = "B"
	SkillLevelC SkillLevel = "C"
	SkillLevelD SkillLevel = "D"
)

type GroupStatus string

const (
	GroupStatusActive    GroupStatus = "active"
	GroupStatusCancelled GroupStatus = "cancelled"
	GroupStatusCompleted GroupStatus = "completed"
)

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

type OrderStatus string

const (
	OrderStatusPending       OrderStatus = "pending"
	OrderStatusConfirmed     OrderStatus = "confirmed"
	OrderStatusCancelled     OrderStatus = "cancelled"
	OrderStatusCancelRequest OrderStatus = "cancel_request"
)

// IsValid reports whether the order status is a recognized value.
func (s OrderStatus) IsValid() bool {
	switch s {
	case OrderStatusPending, OrderStatusConfirmed, OrderStatusCancelled, OrderStatusCancelRequest:
		return true
	}
	return false
}

type PickupGroup struct {
	ID              string
	HostID          string
	Title           string
	HostName        string
	HostPhone       string
	StartTime       time.Time
	EndTime         time.Time
	Fee             int
	Capacity        int
	LocationID      string
	SkillLevel      SkillLevel
	Status          GroupStatus
	Enable          bool
	CurrentEnrolled int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type PickupOrder struct {
	ID            string
	PickupGroupID string
	UserID        string
	BookerName    string
	BookerPhone   string
	Status        OrderStatus
	PaymentStatus PaymentStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type GroupFilter struct {
	Status     string
	SkillLevel string
	HostID     string
	// BookableOnly limits results to groups that can still be enrolled into:
	// status=active, enable=true, not yet ended, and not fully booked.
	BookableOnly bool
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

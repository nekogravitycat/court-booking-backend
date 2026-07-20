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
	ErrRejectedFromGroup     = apperror.New(http.StatusConflict, "you have been rejected from this group and cannot re-enroll")
	ErrInvalidStatus         = apperror.New(http.StatusBadRequest, "invalid status")
	ErrInvalidTimeRange      = apperror.New(http.StatusBadRequest, "start time must be before end time")
	ErrPermissionDenied      = apperror.New(http.StatusForbidden, "permission denied")
	ErrGroupNotActive        = apperror.New(http.StatusBadRequest, "pickup group is not active")
	ErrSportNotFound         = apperror.New(http.StatusNotFound, "sport not found")
	ErrSportInactive         = apperror.New(http.StatusBadRequest, "sport is not active")
	ErrSkillLevelNotFound    = apperror.New(http.StatusNotFound, "skill level not found")
	ErrSkillLevelMismatch    = apperror.New(http.StatusBadRequest, "skill level does not belong to the selected sport")
	ErrSkillLevelInactive    = apperror.New(http.StatusBadRequest, "skill level is not active")
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
	// OrderStatusRejected marks an enrollment the host has rejected. It is
	// excluded from the enrolled count and permanently blocks the user from
	// re-enrolling in the group (the row is retained rather than hard-deleted).
	OrderStatusRejected OrderStatus = "rejected"
)

// IsValid reports whether the order status is a recognized value.
func (s OrderStatus) IsValid() bool {
	switch s {
	case OrderStatusPending, OrderStatusConfirmed, OrderStatusCancelled, OrderStatusCancelRequest, OrderStatusRejected:
		return true
	}
	return false
}

// EnrolledStatusFree is the enrolled_status reported for a viewer that has no
// order in a group (or an anonymous viewer).
const EnrolledStatusFree = "free"

type PickupGroup struct {
	ID              string
	HostID          string
	Title           string
	StartTime       time.Time
	EndTime         time.Time
	Fee             int
	Capacity        int
	LocationID      string
	SportID         string
	SkillLevelID    string
	Status          GroupStatus
	Enable          bool
	CurrentEnrolled int
	CreatedAt       time.Time
	UpdatedAt       time.Time

	// Fields resolved via JOIN for display; not stored on pickup_groups.
	SportCode       string
	SportName       string
	SkillLevelName  string
	HostUsername    string
	HostDisplayName *string
	HostPhone       *string

	// EnrolledStatus is the requesting viewer's order status for this group.
	// It is only populated by list queries that receive a viewer id; it is the
	// empty string otherwise (the handler maps empty to "free").
	EnrolledStatus string
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
	Status       string
	SportID      string
	SkillLevelID string
	HostID       string
	// BookableOnly limits results to groups that can still be enrolled into:
	// status=active, enable=true, not yet ended, and not fully booked.
	BookableOnly bool
	// ViewerUserID, when set, resolves each group's enrolled_status for that user.
	ViewerUserID string
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

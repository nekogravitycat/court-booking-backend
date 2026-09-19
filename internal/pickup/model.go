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
	ErrSkillLevelNotFound    = apperror.New(http.StatusBadRequest, "skill level is not defined for the selected sport")
	ErrSkillLevelInactive    = apperror.New(http.StatusBadRequest, "skill level is not active")
	ErrInvalidPartySize      = apperror.New(http.StatusBadRequest, "party size must be between 2 and 50")
	ErrPartyMembersMismatch  = apperror.New(http.StatusBadRequest, "members must contain exactly party_size entries")
	ErrDistanceNeedsOrigin   = apperror.New(http.StatusBadRequest, "latitude and longitude are required to sort by distance")
	ErrFollowedNeedsAuth     = apperror.New(http.StatusUnauthorized, "authentication is required to filter by followed hosts")
	ErrTimeConflict          = apperror.New(http.StatusConflict, "time_conflict")
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
	Description     *string
	StartTime       time.Time
	EndTime         time.Time
	Fee             int
	Capacity        int
	LocationID      string
	SportID         string
	SkillLevel      int // Integer level; its label comes from the sport's skill_levels mapping
	Status          GroupStatus
	Enable          bool
	CurrentEnrolled int
	CreatedAt       time.Time
	UpdatedAt       time.Time

	// Fields resolved via JOIN for display; not stored on pickup_groups.
	SportCode       string
	SportName       string
	SkillLevelLabel string
	HostUsername    string
	HostDisplayName *string
	HostPhone       *string

	// EnrolledStatus is the requesting viewer's order status for this group.
	// It is only populated by list queries that receive a viewer id; it is the
	// empty string otherwise (the handler maps empty to "free").
	EnrolledStatus string

	// DistanceKm is the great-circle distance from the requested origin to the
	// group's location. It is only populated by list queries given an origin.
	DistanceKm *float64
}

type PickupOrder struct {
	ID            string
	PickupGroupID string
	UserID        string
	BookerName    string
	BookerPhone   string
	Status        OrderStatus
	PaymentStatus PaymentStatus
	// SkillLevel is the enrollee's self-reported level (for a party order, the
	// organizer's; each member carries their own).
	SkillLevel int
	// PartySize is the number of seats the order occupies (1 for a single enrollment).
	PartySize int
	// Members lists the anonymous seats of a party order (PartySize entries,
	// organizer included). It is empty for a single enrollment.
	Members   []OrderMember
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OrderMember is one anonymous seat of a party order.
type OrderMember struct {
	Gender     string
	SkillLevel int
}

type GroupFilter struct {
	Status     string
	SportID    string
	SkillLevel *int
	HostID     string
	// FeeMin / FeeMax bound the per-person fee (inclusive) when set.
	FeeMin *int
	FeeMax *int
	// FollowedOnly limits results to groups hosted by a host the viewer follows.
	// It requires ViewerUserID.
	FollowedOnly bool
	// Latitude / Longitude are the origin for distance computation and sorting.
	Latitude  *float64
	Longitude *float64
	// PubliclyVisibleOnly limits results to groups eligible for the public
	// listing: status=active, enable=true, and not yet ended. Fully booked
	// groups are still included so users can see (though not join) them.
	PubliclyVisibleOnly bool
	// ViewerUserID, when set, resolves each group's enrolled_status for that user.
	ViewerUserID string
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

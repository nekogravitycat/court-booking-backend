package pickup

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrGroupNotFound    = apperror.New(http.StatusNotFound, "pickup group not found")
	ErrOrderNotFound    = apperror.New(http.StatusNotFound, "pickup order not found")
	ErrGroupFullyBooked = apperror.New(http.StatusConflict, "group is fully booked")
	ErrAlreadyEnrolled  = apperror.New(http.StatusConflict, "already enrolled in this group")
	ErrInvalidStatus    = apperror.New(http.StatusBadRequest, "invalid status")
	ErrInvalidTimeRange = apperror.New(http.StatusBadRequest, "start time must be before end time")
	ErrPermissionDenied = apperror.New(http.StatusForbidden, "permission denied")
	ErrGroupNotActive   = apperror.New(http.StatusBadRequest, "pickup group is not active")
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
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusPaid      PaymentStatus = "paid"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusCancelled PaymentStatus = "cancelled"
)

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
	PaymentStatus PaymentStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type GroupFilter struct {
	Status     string
	SkillLevel string
	HostID     string
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

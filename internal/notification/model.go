package notification

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound = apperror.New(http.StatusNotFound, "notification not found")
)

// Notification types. The type is a stable machine key the client can use to
// pick an icon or deep link; title and content are ready-to-display text.
const (
	TypePickupOrderCreated         = "pickup_order_created"           // To the host: someone enrolled
	TypePickupOrderConfirmed       = "pickup_order_confirmed"         // To the booker: host confirmed
	TypePickupOrderRejected        = "pickup_order_rejected"          // To the booker: host rejected
	TypePickupOrderCancelledByHost = "pickup_order_cancelled_by_host" // To the booker: host cancelled
	TypePickupOrderCancelled       = "pickup_order_cancelled"         // To the host: booker cancelled
	TypePickupOrderCancelRequested = "pickup_order_cancel_requested"  // To the host: booker asked to cancel
	TypePickupPaymentUpdated       = "pickup_payment_updated"         // To the booker: payment status changed
	TypePickupGroupCancelled       = "pickup_group_cancelled"         // To enrolled users: group cancelled
	TypePickupGroupUpdated         = "pickup_group_updated"           // To enrolled users: time / place changed
	TypeSkillRated                 = "skill_rated"                    // To a participant: host rated their level
)

// Notification is one message in a user's inbox.
type Notification struct {
	ID            string
	UserID        string
	Type          string
	Title         string
	Content       string
	PickupGroupID *string
	PickupOrderID *string
	IsRead        bool
	CreatedAt     time.Time
}

// Filter defines parameters for listing a user's notifications.
type Filter struct {
	UserID     string
	UnreadOnly bool
	Page       int
	PageSize   int
}

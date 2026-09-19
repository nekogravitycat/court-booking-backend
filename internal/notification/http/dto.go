package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/notification"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

// ListNotificationsRequest defines query parameters for listing notifications.
type ListNotificationsRequest struct {
	request.ListParams
	UnreadOnly bool `form:"unread_only"`
}

type NotificationResponse struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	PickupGroupID *string   `json:"pickup_group_id"`
	PickupOrderID *string   `json:"pickup_order_id"`
	IsRead        bool      `json:"is_read"`
	CreatedAt     time.Time `json:"created_at"`
}

func NewNotificationResponse(n *notification.Notification) NotificationResponse {
	return NotificationResponse{
		ID:            n.ID,
		Type:          n.Type,
		Title:         n.Title,
		Content:       n.Content,
		PickupGroupID: n.PickupGroupID,
		PickupOrderID: n.PickupOrderID,
		IsRead:        n.IsRead,
		CreatedAt:     n.CreatedAt.UTC(),
	}
}

// UnreadCountResponse is returned by GET /notifications/unread-count.
type UnreadCountResponse struct {
	Unread int `json:"unread"`
}

// MarkAllReadResponse is returned by POST /notifications/read-all.
type MarkAllReadResponse struct {
	Updated int64 `json:"updated"`
}

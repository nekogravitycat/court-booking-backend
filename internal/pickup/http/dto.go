package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

// --- Request types ---

type ListGroupsRequest struct {
	request.ListParams
	Status     string `form:"status" binding:"omitempty,oneof=active cancelled completed"`
	SkillLevel string `form:"skill_level" binding:"omitempty,oneof=A B C D"`
	HostID     string `form:"host_id" binding:"omitempty,uuid"`
	SortBy     string `form:"sort_by" binding:"omitempty,oneof=start_time created_at"`
}

type GetGroupQuery struct {
	IncludeOrders bool `form:"include_orders"`
}

type CreateGroupBody struct {
	Title      string    `json:"title" binding:"required"`
	HostName   string    `json:"host_name"`
	HostPhone  string    `json:"host_phone"`
	StartTime  time.Time `json:"start_time" binding:"required"`
	EndTime    time.Time `json:"end_time" binding:"required"`
	Fee        int       `json:"fee" binding:"min=0"`
	Capacity   int       `json:"capacity" binding:"required,min=1"`
	LocationID string    `json:"location_id" binding:"required,uuid"`
	SkillLevel string    `json:"skill_level" binding:"required,oneof=A B C D"`
	Enable     *bool     `json:"enable"`
}

func (r *CreateGroupBody) Validate() error {
	if !r.EndTime.After(r.StartTime) {
		return pickup.ErrInvalidTimeRange
	}
	return nil
}

type UpdateOrderBody struct {
	PaymentStatus string `json:"payment_status" binding:"required,oneof=pending paid failed cancelled"`
}

// --- Response types ---

type PickupOrderResponse struct {
	ID            string    `json:"id"`
	PickupGroupID string    `json:"pickup_group_id"`
	UserID        string    `json:"user_id"`
	BookerName    string    `json:"booker_name"`
	BookerPhone   string    `json:"booker_phone"`
	PaymentStatus string    `json:"payment_status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NewPickupOrderResponse(o *pickup.PickupOrder) PickupOrderResponse {
	return PickupOrderResponse{
		ID:            o.ID,
		PickupGroupID: o.PickupGroupID,
		UserID:        o.UserID,
		BookerName:    o.BookerName,
		BookerPhone:   o.BookerPhone,
		PaymentStatus: string(o.PaymentStatus),
		CreatedAt:     o.CreatedAt.UTC(),
		UpdatedAt:     o.UpdatedAt.UTC(),
	}
}

type PickupGroupResponse struct {
	ID              string                `json:"id"`
	HostID          string                `json:"host_id"`
	Title           string                `json:"title"`
	HostName        string                `json:"host_name"`
	HostPhone       string                `json:"host_phone"`
	StartTime       time.Time             `json:"start_time"`
	EndTime         time.Time             `json:"end_time"`
	Fee             int                   `json:"fee"`
	Capacity        int                   `json:"capacity"`
	LocationID      string                `json:"location_id"`
	SkillLevel      string                `json:"skill_level"`
	Status          string                `json:"status"`
	Enable          bool                  `json:"enable"`
	CurrentEnrolled int                   `json:"current_enrolled"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
	Orders          *[]PickupOrderResponse `json:"orders,omitempty"`
}

// NewPickupGroupResponse builds a PickupGroupResponse.
// Pass a non-nil orders slice to include order details; nil omits the field entirely.
func NewPickupGroupResponse(g *pickup.PickupGroup, orders []*pickup.PickupOrder) PickupGroupResponse {
	resp := PickupGroupResponse{
		ID:              g.ID,
		HostID:          g.HostID,
		Title:           g.Title,
		HostName:        g.HostName,
		HostPhone:       g.HostPhone,
		StartTime:       g.StartTime.UTC(),
		EndTime:         g.EndTime.UTC(),
		Fee:             g.Fee,
		Capacity:        g.Capacity,
		LocationID:      g.LocationID,
		SkillLevel:      string(g.SkillLevel),
		Status:          string(g.Status),
		Enable:          g.Enable,
		CurrentEnrolled: g.CurrentEnrolled,
		CreatedAt:       g.CreatedAt.UTC(),
		UpdatedAt:       g.UpdatedAt.UTC(),
	}

	if orders != nil {
		orderResponses := make([]PickupOrderResponse, len(orders))
		for i, o := range orders {
			orderResponses[i] = NewPickupOrderResponse(o)
		}
		resp.Orders = &orderResponses
	}

	return resp
}

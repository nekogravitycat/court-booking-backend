package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	skillHttp "github.com/nekogravitycat/court-booking-backend/internal/skilllevel/http"
	sportsHttp "github.com/nekogravitycat/court-booking-backend/internal/sports/http"
)

// --- Request types ---

type ListGroupsRequest struct {
	request.ListParams
	Status       string `form:"status" binding:"omitempty,oneof=active cancelled completed"`
	SportID      string `form:"sport_id" binding:"omitempty,uuid"`
	SkillLevelID string `form:"skill_level_id" binding:"omitempty,uuid"`
	HostID       string `form:"host_id" binding:"omitempty,uuid"`
	SortBy       string `form:"sort_by" binding:"omitempty,oneof=start_time created_at"`
}

type GetGroupQuery struct {
	IncludeOrders bool `form:"include_orders"`
}

// HostGroupsURI binds the host_id path parameter for
// GET /hosts/{host_id}/pickup-groups.
type HostGroupsURI struct {
	HostID string `uri:"host_id" binding:"required,uuid"`
}

type CreateGroupBody struct {
	Title        string    `json:"title" binding:"required"`
	StartTime    time.Time `json:"start_time" binding:"required"`
	EndTime      time.Time `json:"end_time" binding:"required"`
	Fee          int       `json:"fee" binding:"min=0"`
	Capacity     int       `json:"capacity" binding:"required,min=1"`
	LocationID   string    `json:"location_id" binding:"required,uuid"`
	SportID      string    `json:"sport_id" binding:"required,uuid"`
	SkillLevelID string    `json:"skill_level_id" binding:"required,uuid"`
	Enable       *bool     `json:"enable"`
}

func (r *CreateGroupBody) Validate() error {
	if !r.EndTime.After(r.StartTime) {
		return pickup.ErrInvalidTimeRange
	}
	return nil
}

type UpdateOrderBody struct {
	Status        *string `json:"status" binding:"omitempty,oneof=pending confirmed cancelled cancel_request rejected"`
	PaymentStatus *string `json:"payment_status" binding:"omitempty,oneof=done pending failed"`
}

type UpdateGroupBody struct {
	Title        *string    `json:"title"`
	StartTime    *time.Time `json:"start_time"`
	EndTime      *time.Time `json:"end_time"`
	Fee          *int       `json:"fee" binding:"omitempty,min=0"`
	Capacity     *int       `json:"capacity" binding:"omitempty,min=1"`
	LocationID   *string    `json:"location_id" binding:"omitempty,uuid"`
	SportID      *string    `json:"sport_id" binding:"omitempty,uuid"`
	SkillLevelID *string    `json:"skill_level_id" binding:"omitempty,uuid"`
	Status       *string    `json:"status" binding:"omitempty,oneof=active cancelled completed"`
	Enable       *bool      `json:"enable"`
}

// --- Response types ---

type PickupOrderResponse struct {
	ID            string    `json:"id"`
	PickupGroupID string    `json:"pickup_group_id"`
	UserID        string    `json:"user_id"`
	BookerName    string    `json:"booker_name"`
	BookerPhone   string    `json:"booker_phone"`
	Status        string    `json:"status"`
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
		Status:        string(o.Status),
		PaymentStatus: string(o.PaymentStatus),
		CreatedAt:     o.CreatedAt.UTC(),
		UpdatedAt:     o.UpdatedAt.UTC(),
	}
}

// PickupHostTag is the host representation embedded in pickup group responses.
// Host details are resolved live from the users table (no snapshot).
type PickupHostTag struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name"`
	Phone       *string `json:"phone"`
}

// PickupGroupBrief is the trimmed, public-facing shape used by the list
// endpoints (GET /pickup-groups and GET /hosts/{host_id}/pickup-groups).
// The host phone is intentionally omitted from the public shape.
type PickupGroupBrief struct {
	ID              string                  `json:"id"`
	HostID          string                  `json:"host_id"`
	HostUsername    string                  `json:"host_username"`
	HostDisplayName *string                 `json:"host_display_name"`
	LocationID      string                  `json:"location_id"`
	Title           string                  `json:"title"`
	Sport           sportsHttp.SportTag     `json:"sport"`
	SkillLevel      skillHttp.SkillLevelTag `json:"skill_level"`
	StartTime       time.Time               `json:"start_time"`
	Fee             int                     `json:"fee"`
	// EnrolledStatus is the requesting user's status for this group: "free" when
	// not enrolled (or unauthenticated), otherwise their order status.
	EnrolledStatus string `json:"enrolled_status"`
}

func NewPickupGroupBrief(g *pickup.PickupGroup) PickupGroupBrief {
	enrolled := g.EnrolledStatus
	if enrolled == "" {
		enrolled = pickup.EnrolledStatusFree
	}
	return PickupGroupBrief{
		ID:              g.ID,
		HostID:          g.HostID,
		HostUsername:    g.HostUsername,
		HostDisplayName: g.HostDisplayName,
		LocationID:      g.LocationID,
		Title:           g.Title,
		Sport:           sportsHttp.SportTag{ID: g.SportID, Code: g.SportCode, Name: g.SportName},
		SkillLevel:      skillHttp.SkillLevelTag{ID: g.SkillLevelID, Name: g.SkillLevelName},
		StartTime:       g.StartTime.UTC(),
		Fee:             g.Fee,
		EnrolledStatus:  enrolled,
	}
}

type PickupGroupResponse struct {
	ID              string                  `json:"id"`
	Host            PickupHostTag           `json:"host"`
	Title           string                  `json:"title"`
	StartTime       time.Time               `json:"start_time"`
	EndTime         time.Time               `json:"end_time"`
	Fee             int                     `json:"fee"`
	Capacity        int                     `json:"capacity"`
	LocationID      string                  `json:"location_id"`
	Sport           sportsHttp.SportTag     `json:"sport"`
	SkillLevel      skillHttp.SkillLevelTag `json:"skill_level"`
	Status          string                  `json:"status"`
	Enable          bool                    `json:"enable"`
	CurrentEnrolled int                     `json:"current_enrolled"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
	Orders          *[]PickupOrderResponse  `json:"orders,omitempty"`
}

// NewPickupGroupResponse builds a PickupGroupResponse.
// Pass a non-nil orders slice to include order details; nil omits the field entirely.
func NewPickupGroupResponse(g *pickup.PickupGroup, orders []*pickup.PickupOrder) PickupGroupResponse {
	resp := PickupGroupResponse{
		ID:              g.ID,
		Host:            PickupHostTag{ID: g.HostID, Username: g.HostUsername, DisplayName: g.HostDisplayName, Phone: g.HostPhone},
		Title:           g.Title,
		StartTime:       g.StartTime.UTC(),
		EndTime:         g.EndTime.UTC(),
		Fee:             g.Fee,
		Capacity:        g.Capacity,
		LocationID:      g.LocationID,
		Sport:           sportsHttp.SportTag{ID: g.SportID, Code: g.SportCode, Name: g.SportName},
		SkillLevel:      skillHttp.SkillLevelTag{ID: g.SkillLevelID, Name: g.SkillLevelName},
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

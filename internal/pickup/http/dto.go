package http

import (
	"errors"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	skillHttp "github.com/nekogravitycat/court-booking-backend/internal/skilllevel/http"
	sportsHttp "github.com/nekogravitycat/court-booking-backend/internal/sports/http"
)

// --- Request types ---

type ListGroupsRequest struct {
	request.ListParams
	Status     string `form:"status" binding:"omitempty,oneof=active cancelled completed"`
	SportID    string `form:"sport_id" binding:"omitempty,uuid"`
	SkillLevel *int   `form:"skill_level" binding:"omitempty,min=1,max=100"`
	HostID     string `form:"host_id" binding:"omitempty,uuid"`
	SortBy     string `form:"sort_by" binding:"omitempty,oneof=start_time created_at skill_level distance"`

	// FeeMin / FeeMax bound the per-person fee (inclusive).
	FeeMin *int `form:"fee_min" binding:"omitempty,min=0"`
	FeeMax *int `form:"fee_max" binding:"omitempty,min=0"`

	// FollowedOnly restricts the list to hosts the caller follows (auth required).
	FollowedOnly bool `form:"followed_only"`

	// Latitude / Longitude are the caller origin; both are required together and
	// enable distance_km and sort_by=distance.
	Latitude  *float64 `form:"latitude" binding:"omitempty,min=-90,max=90"`
	Longitude *float64 `form:"longitude" binding:"omitempty,min=-180,max=180"`
}

// Validate performs cross-field validation for ListGroupsRequest.
func (r *ListGroupsRequest) Validate() error {
	if (r.Latitude == nil) != (r.Longitude == nil) {
		return errors.New("latitude and longitude must be provided together")
	}
	if r.FeeMin != nil && r.FeeMax != nil && *r.FeeMin > *r.FeeMax {
		return errors.New("fee_min must not exceed fee_max")
	}
	return nil
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
	Title       string    `json:"title" binding:"required,min=1,max=100"`
	Description *string   `json:"description" binding:"omitempty,max=100"`
	StartTime   time.Time `json:"start_time" binding:"required"`
	EndTime     time.Time `json:"end_time" binding:"required"`
	Fee         int       `json:"fee" binding:"min=0,max=100000"`
	Capacity    int       `json:"capacity" binding:"required,min=1,max=200"`
	LocationID  string    `json:"location_id" binding:"required,uuid"`
	SportID     string    `json:"sport_id" binding:"required,uuid"`
	SkillLevel  int       `json:"skill_level" binding:"required,min=1,max=100"`
	Enable      *bool     `json:"enable"`
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
	Title       *string    `json:"title" binding:"omitempty,min=1,max=100"`
	Description *string    `json:"description" binding:"omitempty,max=100"`
	StartTime   *time.Time `json:"start_time"`
	EndTime     *time.Time `json:"end_time"`
	Fee         *int       `json:"fee" binding:"omitempty,min=0,max=100000"`
	Capacity    *int       `json:"capacity" binding:"omitempty,min=1,max=200"`
	LocationID  *string    `json:"location_id" binding:"omitempty,uuid"`
	SportID     *string    `json:"sport_id" binding:"omitempty,uuid"`
	SkillLevel  *int       `json:"skill_level" binding:"omitempty,min=1,max=100"`
	Status      *string    `json:"status" binding:"omitempty,oneof=active cancelled completed"`
	Enable      *bool      `json:"enable"`
}

// --- Response types ---

type PickupOrderResponse struct {
	ID            string            `json:"id"`
	PickupGroupID string            `json:"pickup_group_id"`
	UserID        string            `json:"user_id"`
	BookerName    string            `json:"booker_name"`
	BookerPhone   string            `json:"booker_phone"`
	Status        string            `json:"status"`
	PaymentStatus string            `json:"payment_status"`
	SkillLevel    int               `json:"skill_level"`
	PartySize     int               `json:"party_size"`
	Members       []OrderMemberBody `json:"members,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

func NewPickupOrderResponse(o *pickup.PickupOrder) PickupOrderResponse {
	var members []OrderMemberBody
	for _, m := range o.Members {
		members = append(members, OrderMemberBody{Gender: m.Gender, SkillLevel: m.SkillLevel})
	}

	return PickupOrderResponse{
		ID:            o.ID,
		PickupGroupID: o.PickupGroupID,
		UserID:        o.UserID,
		BookerName:    o.BookerName,
		BookerPhone:   o.BookerPhone,
		Status:        string(o.Status),
		PaymentStatus: string(o.PaymentStatus),
		SkillLevel:    o.SkillLevel,
		PartySize:     o.PartySize,
		Members:       members,
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
	// DistanceKm is set only when latitude and longitude were supplied.
	DistanceKm *float64 `json:"distance_km"`
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
		SkillLevel:      skillHttp.SkillLevelTag{Level: g.SkillLevel, Label: g.SkillLevelLabel},
		StartTime:       g.StartTime.UTC(),
		Fee:             g.Fee,
		EnrolledStatus:  enrolled,
		DistanceKm:      g.DistanceKm,
	}
}

type PickupGroupResponse struct {
	ID              string                  `json:"id"`
	Host            PickupHostTag           `json:"host"`
	Title           string                  `json:"title"`
	Description     *string                 `json:"description"`
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
		Description:     g.Description,
		StartTime:       g.StartTime.UTC(),
		EndTime:         g.EndTime.UTC(),
		Fee:             g.Fee,
		Capacity:        g.Capacity,
		LocationID:      g.LocationID,
		Sport:           sportsHttp.SportTag{ID: g.SportID, Code: g.SportCode, Name: g.SportName},
		SkillLevel:      skillHttp.SkillLevelTag{Level: g.SkillLevel, Label: g.SkillLevelLabel},
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

// --- Enrollment (single / party) ---

// CreateOrderBody is the body of POST /pickup-groups/{id}/orders. SkillLevel is
// the enrollee's self-reported level on the group's sport scale.
type CreateOrderBody struct {
	SkillLevel int `json:"skill_level" binding:"required,min=1,max=100"`
}

// OrderMemberBody is one seat of a party enrollment.
type OrderMemberBody struct {
	Gender     string `json:"gender" binding:"required,oneof=male female other"`
	SkillLevel int    `json:"skill_level" binding:"required,min=1,max=100"`
}

// CreatePartyOrderBody is the body of POST /pickup-groups/{id}/party-orders.
// Members lists every seat (organizer included, as the first entry), so its
// length must equal PartySize.
type CreatePartyOrderBody struct {
	OrganizerName string            `json:"organizer_name" binding:"required,min=1,max=50"`
	PartySize     int               `json:"party_size" binding:"required,min=2,max=50"`
	Members       []OrderMemberBody `json:"members" binding:"required,min=2,max=50,dive"`
}

// --- Participant statistics ---

type GenderCountResponse struct {
	Gender string `json:"gender"`
	Count  int    `json:"count"`
}

type AgeGroupCountResponse struct {
	AgeGroup string `json:"age_group"`
	Count    int    `json:"count"`
}

type SkillLevelCountResponse struct {
	Level int    `json:"level"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// ParticipantStatsResponse is the anonymous breakdown of a group's enrolled seats.
type ParticipantStatsResponse struct {
	Total       int                       `json:"total"`
	Genders     []GenderCountResponse     `json:"genders"`
	AgeGroups   []AgeGroupCountResponse   `json:"age_groups"`
	SkillLevels []SkillLevelCountResponse `json:"skill_levels"`
}

func NewParticipantStatsResponse(s *pickup.ParticipantStats) ParticipantStatsResponse {
	resp := ParticipantStatsResponse{
		Total:       s.Total,
		Genders:     make([]GenderCountResponse, len(s.Genders)),
		AgeGroups:   make([]AgeGroupCountResponse, len(s.AgeGroups)),
		SkillLevels: make([]SkillLevelCountResponse, len(s.SkillLevels)),
	}
	for i, g := range s.Genders {
		resp.Genders[i] = GenderCountResponse{Gender: g.Gender, Count: g.Count}
	}
	for i, a := range s.AgeGroups {
		resp.AgeGroups[i] = AgeGroupCountResponse{AgeGroup: a.Group, Count: a.Count}
	}
	for i, l := range s.SkillLevels {
		resp.SkillLevels[i] = SkillLevelCountResponse{Level: l.Level, Label: l.Label, Count: l.Count}
	}
	return resp
}

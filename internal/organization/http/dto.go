package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/organization"
)

// OrganizationResponse matches the OAS definition.
type OrganizationResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateOrganizationRequest is the payload for POST /organizations.
type CreateOrganizationRequest struct {
	Name string `json:"name" binding:"required"`
}

// UpdateOrganizationRequest is the payload for PATCH /organizations/:id.
type UpdateOrganizationRequest struct {
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

func NewOrganizationResponse(o *organization.Organization) OrganizationResponse {
	return OrganizationResponse{
		ID:        o.ID,
		Name:      o.Name,
		CreatedAt: o.CreatedAt,
	}
}

// MemberResponse matches the desired output for a member in a list.
type MemberResponse struct {
	UserID      string  `json:"user_id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name"`
	Role        string  `json:"role"`
}

// AddMemberRequest defines payload for adding a member.
type AddMemberRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
	Role   string `json:"role" binding:"required,oneof=owner admin member"`
}

// UpdateMemberRequest defines payload for updating a member role.
type UpdateMemberRequest struct {
	Role string `json:"role" binding:"required,oneof=owner admin member"`
}

func NewMemberResponse(m *organization.Member) MemberResponse {
	return MemberResponse{
		UserID:      m.UserID,
		Email:       m.Email,
		DisplayName: m.DisplayName,
		Role:        m.Role,
	}
}

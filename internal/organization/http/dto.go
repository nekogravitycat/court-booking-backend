package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

// OrganizationResponse matches the OAS definition.
type OrganizationResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// OrganizationTag is a brief representation of an organization (ID and Name).
type OrganizationTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// OrgMemberRequest is a common struct for endpoints that require both OrgID and UserID path parameters.
type OrgMemberRequest struct {
	ID     string `uri:"id" binding:"required,uuid"`
	UserID string `uri:"user_id" binding:"required,uuid"`
}

// Validate performs custom validation for OrgMemberRequest.
func (r *OrgMemberRequest) Validate() error {
	return nil
}

// ListOrganizationsRequest defines query parameters for listing organizations.
type ListOrganizationsRequest struct {
	request.ListParams
	SortBy string `form:"sort_by" binding:"omitempty,oneof=name created_at"`
}

// Validate performs custom validation for ListOrganizationsRequest.
func (r *ListOrganizationsRequest) Validate() error {
	return nil
}

// CreateOrganizationRequest is the payload for POST /organizations.
type CreateOrganizationRequest struct {
	Name string `json:"name" binding:"required,min=1,max=100"`
}

// Validate performs custom validation for CreateOrganizationRequest.
func (r *CreateOrganizationRequest) Validate() error {
	return nil
}

// UpdateOrganizationRequest is the payload for PATCH /organizations/:id.
type UpdateOrganizationRequest struct {
	Name     *string `json:"name" binding:"omitempty,min=1,max=100"`
	IsActive *bool   `json:"is_active"`
}

// Validate performs custom validation for UpdateOrganizationRequest.
func (r *UpdateOrganizationRequest) Validate() error {
	return nil
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

// ListMembersRequest defines query parameters for listing members.
type ListMembersRequest struct {
	request.ListParams
	SortBy string `form:"sort_by" binding:"omitempty,oneof=role"`
}

// Validate performs custom validation for ListMembersRequest.
func (r *ListMembersRequest) Validate() error {
	return nil
}

// AddMemberRequest defines payload for adding a member.
type AddMemberRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
	Role   string `json:"role" binding:"required,oneof=manager"`
}

// Validate performs custom validation for AddMemberRequest.
func (r *AddMemberRequest) Validate() error {
	return nil
}

// UpdateMemberRequest defines payload for updating a member role.
type UpdateMemberRequest struct {
	Role string `json:"role" binding:"required,oneof=manager"`
}

// Validate performs custom validation for UpdateMemberRequest.
func (r *UpdateMemberRequest) Validate() error {
	return nil
}

func NewMemberResponse(m *organization.Member) MemberResponse {
	return MemberResponse{
		UserID:      m.UserID,
		Email:       m.Email,
		DisplayName: m.DisplayName,
		Role:        m.Role,
	}
}

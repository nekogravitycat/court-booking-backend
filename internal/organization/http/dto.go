package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// OrganizationResponse matches the OAS definition.
type OrganizationResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	IsActive  bool      `json:"is_active"`
}

type OrganizationBrief struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Owner               bool     `json:"owner"`
	OrganizationManager bool     `json:"organization_manager"`
	LocationManager     []string `json:"location_manager"`
}

// OrganizationTag is a brief representation of an organization with just ID and name.
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
	Name    string `json:"name" binding:"required,min=1,max=100"`
	OwnerID string `json:"owner_id" binding:"required,uuid"`
}

// Validate performs custom validation for CreateOrganizationRequest.
func (r *CreateOrganizationRequest) Validate() error {
	return nil
}

// UpdateOrganizationRequest is the payload for PATCH /organizations/:id.
type UpdateOrganizationRequest struct {
	Name     *string `json:"name" binding:"omitempty,min=1,max=100"`
	IsActive *bool   `json:"is_active"`
	OwnerID  *string `json:"owner_id" binding:"omitempty,uuid"`
}

// Validate performs custom validation for UpdateOrganizationRequest.
func (r *UpdateOrganizationRequest) Validate() error {
	return nil
}

func NewOrganizationResponse(o *organization.Organization) OrganizationResponse {
	return OrganizationResponse{
		ID:        o.ID,
		Name:      o.Name,
		OwnerID:   o.OwnerID,
		CreatedAt: o.CreatedAt,
		IsActive:  o.IsActive,
	}
}

// ManagerResponse matches the User entity structure for manager list.
type ManagerResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName *string   `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
	IsActive    bool      `json:"is_active"`
}

// ListManagerRequest defines query parameters for listing managers.
type ListManagerRequest struct {
	request.ListParams
	SortBy string `form:"sort_by" binding:"omitempty,oneof=name email created_at"`
}

// Validate performs custom validation for ListManagerRequest.
func (r *ListManagerRequest) Validate() error {
	return nil
}

// AddOrganizationManagerRequest defines payload for adding a manager.
type AddOrganizationManagerRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
}

// Validate performs custom validation for AddOrganizationManagerRequest.
func (r *AddOrganizationManagerRequest) Validate() error {
	return nil
}

// AddOrganizationMemberRequest defines payload for adding a member.
type AddOrganizationMemberRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// Validate performs custom validation for AddOrganizationMemberRequest.
func (r *AddOrganizationMemberRequest) Validate() error {
	return nil
}

// ListMemberRequest defines query parameters for listing members.
// Similar to ListManagerRequest.
type ListMemberRequest struct {
	request.ListParams
	SortBy string `form:"sort_by" binding:"omitempty,oneof=name email created_at"`
}

// Validate performs custom validation for ListMemberRequest.
func (r *ListMemberRequest) Validate() error {
	return nil
}

func NewManagerResponse(u *user.User) ManagerResponse {
	return ManagerResponse{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		CreatedAt:   u.CreatedAt,
		IsActive:    u.IsActive,
	}
}

// MemberResponse is a type alias for ManagerResponse as they share the same User structure.
type MemberResponse = ManagerResponse

// NewMemberResponse creates a MemberResponse from a User.
func NewMemberResponse(u *user.User) MemberResponse {
	return NewManagerResponse(u)
}

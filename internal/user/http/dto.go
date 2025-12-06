package http

import (
	"time"

	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// ListUsersRequest defines query parameters for listing users.
type ListUsersRequest struct {
	request.ListParams
	Email       string `form:"email"`
	DisplayName string `form:"display_name"`
	IsActive    *bool  `form:"is_active"`
	SortBy      string `form:"sort_by" binding:"omitempty,oneof=name email created_at"`
}

// Validate performs custom validation for ListUsersRequest.
func (r *ListUsersRequest) Validate() error {
	return nil
}

// UserResponse is the shape of user data returned in API responses.
type UserResponse struct {
	ID            string                    `json:"id"`
	Email         string                    `json:"email"`
	DisplayName   *string                   `json:"display_name"`
	CreatedAt     time.Time                 `json:"created_at"`
	LastLoginAt   *time.Time                `json:"last_login_at"`
	IsActive      bool                      `json:"is_active"`
	IsSystemAdmin bool                      `json:"is_system_admin"`
	Organizations []orgHttp.OrganizationTag `json:"organizations"`
}

// UserTag is a brief representation of a user.
type UserTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NewUserResponse converts domain user.User to UserResponse used by the API.
func NewUserResponse(u *user.User) UserResponse {
	// Make a copy of time fields to avoid accidental mutation from outside.
	createdAt := u.CreatedAt
	var lastLoginAt *time.Time
	if u.LastLoginAt != nil {
		ll := *u.LastLoginAt
		lastLoginAt = &ll
	}

	// Map the organizations
	orgs := make([]orgHttp.OrganizationTag, 0, len(u.Organizations))
	if u.Organizations != nil {
		for _, org := range u.Organizations {
			orgs = append(orgs, orgHttp.OrganizationTag{
				ID:   org.ID,
				Name: org.Name,
			})
		}
	}

	return UserResponse{
		ID:            u.ID,
		Email:         u.Email,
		DisplayName:   u.DisplayName,
		CreatedAt:     createdAt,
		LastLoginAt:   lastLoginAt,
		IsActive:      u.IsActive,
		IsSystemAdmin: u.IsSystemAdmin,
		Organizations: orgs,
	}
}

// RegisterRequest defines the payload for user registration.
type RegisterRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	DisplayName string `json:"display_name" binding:"required"`
}

// Validate performs custom validation for RegisterRequest.
func (r *RegisterRequest) Validate() error {
	return nil
}

// LoginRequest defines the payload for user login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Validate performs custom validation for LoginRequest.
func (r *LoginRequest) Validate() error {
	return nil
}

// UpdateUserRequest defines fields allowed to be updated via PATCH /users/:id.
// Use pointers to distinguish between "field not sent" and "field sent as false/empty".
type UpdateUserRequest struct {
	DisplayName   *string `json:"display_name"`
	IsActive      *bool   `json:"is_active"`
	IsSystemAdmin *bool   `json:"is_system_admin"`
}

// Validate performs custom validation for UpdateUserRequest.
func (r *UpdateUserRequest) Validate() error {
	return nil
}

// LoginResponse returns the token and user info.
type LoginResponse struct {
	AccessToken string       `json:"access_token"`
	User        UserResponse `json:"user"`
}

// MeResponse returns the current user info.
type MeResponse struct {
	User UserResponse `json:"user"`
}

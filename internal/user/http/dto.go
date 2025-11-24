package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// UpdateUserRequest defines fields allowed to be updated via PATCH /users/:id.
// Use pointers to distinguish between "field not sent" and "field sent as false/empty".
type UpdateUserRequest struct {
	DisplayName   *string `json:"display_name"`
	IsActive      *bool   `json:"is_active"`
	IsSystemAdmin *bool   `json:"is_system_admin"`
}

// UserResponse is the shape of user data returned in API responses.
type UserResponse struct {
	ID            string                      `json:"id"`
	Email         string                      `json:"email"`
	DisplayName   *string                     `json:"display_name"`
	CreatedAt     time.Time                   `json:"created_at"`
	LastLoginAt   *time.Time                  `json:"last_login_at"`
	IsActive      bool                        `json:"is_active"`
	IsSystemAdmin bool                        `json:"is_system_admin"`
	Organizations []OrganizationBriefResponse `json:"organizations"`
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
	orgs := make([]OrganizationBriefResponse, 0, len(u.Organizations))
	if u.Organizations != nil {
		for _, org := range u.Organizations {
			orgs = append(orgs, OrganizationBriefResponse{
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

// LoginRequest defines the payload for user login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
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

// OrganizationBriefResponse is a nested struct for user list.
type OrganizationBriefResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

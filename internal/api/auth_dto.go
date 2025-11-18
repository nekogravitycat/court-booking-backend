package api

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// RegisterRequest is the payload for POST /v1/auth/register.
type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// LoginRequest is the payload for POST /v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserResponse is the shape of user data returned in API responses.
type UserResponse struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	DisplayName   *string    `json:"display_name,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
	IsActive      bool       `json:"is_active"`
	IsSystemAdmin bool       `json:"is_system_admin"`
}

// RegisterResponse is the response for POST /v1/auth/register.
type RegisterResponse struct {
	User UserResponse `json:"user"`
}

// LoginResponse is the response for POST /v1/auth/login.
type LoginResponse struct {
	AccessToken string       `json:"access_token"`
	User        UserResponse `json:"user"`
}

// MeResponse is the response for GET /v1/me.
type MeResponse struct {
	User UserResponse `json:"user"`
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

	return UserResponse{
		ID:            u.ID,
		Email:         u.Email,
		DisplayName:   u.DisplayName,
		CreatedAt:     createdAt,
		LastLoginAt:   lastLoginAt,
		IsActive:      u.IsActive,
		IsSystemAdmin: u.IsSystemAdmin,
	}
}

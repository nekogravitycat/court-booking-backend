package user

import (
	"errors"
	"time"
)

var (
	ErrNotFound           = errors.New("user not found")
	ErrEmailAlreadyUsed   = errors.New("email already used")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInactiveUser       = errors.New("user is inactive")
	ErrEmailRequired      = errors.New("email is required")
	ErrPasswordTooShort   = errors.New("password is too short")
)

// User represents a user in the system.
type User struct {
	ID            string // UUID
	Email         string
	PasswordHash  string
	DisplayName   *string
	CreatedAt     time.Time
	LastLoginAt   *time.Time
	IsActive      bool
	IsSystemAdmin bool
	Organizations []UserOrganizationBrief
}

// UserFilter defines filter options for listing users.
type UserFilter struct {
	Email       string
	DisplayName string
	IsActive    *bool // Use pointer to distinguish between false and nil (not set)

	Page     int
	PageSize int
	Sort     string // simple string for now, e.g., "created_at desc"
}

// UserOrganizationBrief holds minimal organization info for list views.
type UserOrganizationBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

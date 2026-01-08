package user

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound           = apperror.New(http.StatusNotFound, "user not found")
	ErrEmailAlreadyUsed   = apperror.New(http.StatusConflict, "email already used")
	ErrInvalidCredentials = apperror.New(http.StatusUnauthorized, "invalid email or password")
	ErrInactiveUser       = apperror.New(http.StatusUnauthorized, "user is inactive")
	ErrEmailRequired      = apperror.New(http.StatusBadRequest, "email is required")
	ErrPasswordTooShort   = apperror.New(http.StatusBadRequest, "password is too short")
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
	IDs         []string
	DisplayName string
	IsActive    *bool // Use pointer to distinguish between false and nil (not set)

	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// UserOrganizationBrief holds minimal organization info for list views.
type UserOrganizationBrief struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Owner               bool     `json:"owner"`
	OrganizationManager bool     `json:"organization_manager"`
	LocationManager     []string `json:"location_manager"`
}

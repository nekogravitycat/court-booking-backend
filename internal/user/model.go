package user

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound             = apperror.New(http.StatusNotFound, "user not found")
	ErrEmailAlreadyUsed     = apperror.New(http.StatusConflict, "email already used")
	ErrInvalidCredentials   = apperror.New(http.StatusUnauthorized, "invalid email or password")
	ErrInactiveUser         = apperror.New(http.StatusUnauthorized, "user is inactive")
	ErrEmailRequired        = apperror.New(http.StatusBadRequest, "email is required")
	ErrPasswordTooShort     = apperror.New(http.StatusBadRequest, "password is too short")
	ErrPasswordTooLong      = apperror.New(http.StatusBadRequest, "password is too long")
	ErrAlreadyPickupHost    = apperror.New(http.StatusConflict, "user is already a pickup host")
	ErrNotPickupHost        = apperror.New(http.StatusNotFound, "user is not a pickup host")
	ErrUsernameRequired     = apperror.New(http.StatusBadRequest, "username is required")
	ErrInvalidUsername      = apperror.New(http.StatusBadRequest, "username must be 4-15 characters of lowercase letters, digits, or underscore")
	ErrUsernameAlreadyUsed  = apperror.New(http.StatusConflict, "username already used")
	ErrCannotRevokeOwnAdmin = apperror.New(http.StatusForbidden, "cannot revoke your own system admin privilege")
)

// User represents a user in the system.
type User struct {
	ID            string // UUID
	Email         string
	Username      string // Unique, immutable handle (lowercase letters, digits, underscore)
	PasswordHash  string
	DisplayName   *string
	Phone         *string
	Avatar        *string // ID of avatar image file
	CreatedAt     time.Time
	LastLoginAt   *time.Time
	IsActive      bool
	IsSystemAdmin bool
	IsPickupHost  bool
	Organizations []UserOrganizationBrief
}

// UserFilter defines filter options for listing users.
type UserFilter struct {
	Email       string
	IDs         []string
	DisplayName string
	IsActive    *bool // Use pointer to distinguish between false and nil (not set)

	PickupHostsOnly bool // When true, only return users with the pickup host role

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

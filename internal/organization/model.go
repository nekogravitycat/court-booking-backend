package organization

import (
	"errors"
	"time"
)

var (
	ErrUserAlreadyMember = errors.New("user is already a member of this organization")
	ErrUserNotFound      = errors.New("user not found")
	ErrUserNotMember     = errors.New("user is not a member of the organization")
	ErrOrgNotFound       = errors.New("organization not found")
	ErrNameRequired      = errors.New("organization name is required")
	ErrUserIDRequired    = errors.New("user_id is required")
	ErrInvalidRole       = errors.New("invalid role")
)

// Organization represents a venue owner or brand entity.
type Organization struct {
	ID        string
	Name      string
	CreatedAt time.Time
	IsActive  bool
}

// OrganizationFilter defines filter options for listing organizations.
type OrganizationFilter struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// Define roles matching the database enum
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Member represents a user with a specific role within an organization.
// It joins data from organization_permissions and users tables.
type Member struct {
	UserID      string
	Email       string
	DisplayName *string
	Role        string
}

// MemberFilter defines filter options for listing members.
type MemberFilter struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

package organization

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrUserAlreadyMember = apperror.New(http.StatusConflict, "user is already a member of this organization")
	ErrUserNotFound      = apperror.New(http.StatusNotFound, "user not found")
	ErrUserNotMember     = apperror.New(http.StatusNotFound, "user is not a member of the organization")
	ErrOrgNotFound       = apperror.New(http.StatusNotFound, "organization not found")
	ErrNameRequired      = apperror.New(http.StatusBadRequest, "organization name is required")
	ErrUserIDRequired    = apperror.New(http.StatusBadRequest, "user_id is required")
	ErrInvalidRole       = apperror.New(http.StatusBadRequest, "invalid role")
)

// Organization represents a venue owner or brand entity.
type Organization struct {
	ID        string
	OwnerID   string
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

// Define roles
const (
	RoleOrganizationManager = "manager"
	RoleLocationManager     = "location_manager"
)

// Member represents a user with a specific relation to an organization (Manager).
// It joins data from organization_managers and users tables.
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

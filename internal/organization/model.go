package organization

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrOrgInactive          = apperror.New(http.StatusForbidden, "organization is inactive")
	ErrOwnerRoleConflict    = apperror.New(http.StatusConflict, "user is already the owner of this organization")
	ErrLocationRoleConflict = apperror.New(http.StatusConflict, "user is already a location manager in this organization")
	ErrMemberRequired       = apperror.New(http.StatusBadRequest, "user must be a member of the organization first")
	ErrUserAlreadyMember    = apperror.New(http.StatusConflict, "user is already a member of this organization")
	ErrOwnerInactive        = apperror.New(http.StatusBadRequest, "owner must be an active user")
	ErrUserNotFound         = apperror.New(http.StatusNotFound, "user not found")
	ErrUserNotMember        = apperror.New(http.StatusNotFound, "user is not a member of the organization")
	ErrOrgNotFound          = apperror.New(http.StatusNotFound, "organization not found")
	ErrNameRequired         = apperror.New(http.StatusBadRequest, "organization name is required")
	ErrUserIDRequired       = apperror.New(http.StatusBadRequest, "user_id is required")
	ErrInvalidRole          = apperror.New(http.StatusBadRequest, "invalid role")
)

// Organization represents a venue owner or brand entity.
type Organization struct {
	ID        string
	OwnerID   string
	Name      string
	Cover     *string // ID of cover image file
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

// ManagerFilter defines filter options for listing organization managers.
type ManagerFilter struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

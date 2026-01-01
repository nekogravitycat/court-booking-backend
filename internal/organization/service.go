package organization

import (
	"context"
	"errors"
	"strings"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// UpdateOrganizationRequest defines the fields that can be updated.
type UpdateOrganizationRequest struct {
	Name     *string
	IsActive *bool
}

// AddMemberRequest defines fields for adding a member.
type AddMemberRequest struct {
	UserID string
	Role   string
}

// UpdateMemberRequest defines fields for updating a member.
type UpdateMemberRequest struct {
	Role string
}

// Service defines business logic for organizations.
type Service interface {
	// Organization methods
	Create(ctx context.Context, name string) (*Organization, error)
	GetByID(ctx context.Context, id string) (*Organization, error)
	List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error)
	Update(ctx context.Context, id string, req UpdateOrganizationRequest) (*Organization, error)
	Delete(ctx context.Context, id string) error
	// Member methods
	GetOrganizationMember(ctx context.Context, orgID string, userID string) (*Member, error)
	AddOrganizationManager(ctx context.Context, orgID string, userID string) error
	RemoveOrganizationMember(ctx context.Context, orgID string, userID string) error
	UpdateOrganizationMemberRole(ctx context.Context, orgID string, userID string, req UpdateMemberRequest) error
	ListOrganizationMembers(ctx context.Context, orgID string, filter MemberFilter) ([]*Member, int, error)
	// Permission methods
	CheckPermission(ctx context.Context, orgID string, userID string) (bool, error)
	CheckLocationPermission(ctx context.Context, orgID string, locationID string, userID string) (bool, error)
	CheckIsOwner(ctx context.Context, orgID string, userID string) (bool, error)
	// Location Manager management
	AddLocationManager(ctx context.Context, locationID string, userID string) error
	RemoveLocationManager(ctx context.Context, locationID string, userID string) error
	ListLocationManagers(ctx context.Context, locationID string) ([]string, error)
}

type service struct {
	repo        Repository
	userService user.Service
}

// NewService creates a new organization service.
func NewService(repo Repository, userService user.Service) Service {
	return &service{repo: repo, userService: userService}
}

// ------------------------
//   Organization methods
// ------------------------

func (s *service) Create(ctx context.Context, name string) (*Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}

	org := &Organization{
		Name:     name,
		IsActive: true,
	}

	if err := s.repo.Create(ctx, org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*Organization, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateOrganizationRequest) (*Organization, error) {
	if req.Name != nil {
		*req.Name = strings.TrimSpace(*req.Name)
		if *req.Name == "" {
			return nil, ErrNameRequired
		}
	}

	// Check existence
	org, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates if provided
	if req.Name != nil {
		newName := strings.TrimSpace(*req.Name)
		if newName == "" {
			return nil, ErrNameRequired
		}
		org.Name = newName
	}
	if req.IsActive != nil {
		org.IsActive = *req.IsActive
	}

	// Save updates
	if err := s.repo.Update(ctx, org); err != nil {
		return nil, err
	}

	return org, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	// Check existence
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// ------------------------
//     Member methods
// ------------------------

func (s *service) GetOrganizationMember(ctx context.Context, orgID string, userID string) (*Member, error) {
	return s.repo.GetMember(ctx, orgID, userID)
}

func (s *service) AddOrganizationManager(ctx context.Context, orgID string, userID string) error {
	role := RoleOrganizationManager

	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return err
	}

	// Verify user exists
	if _, err := s.userService.GetByID(ctx, userID); err != nil {
		switch {
		case errors.Is(err, user.ErrNotFound):
			return ErrUserNotFound
		default:
			return err
		}
	}

	// Mutual Exclusion Check: User cannot be both Org Manager/Owner and Location Manager
	isLoMgr, err := s.repo.IsLocationManagerInOrg(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if isLoMgr {
		return apperror.New(409, "user is already a location manager in this organization; remove location manager privileges first")
	}

	return s.repo.AddMember(ctx, orgID, userID, role)
}

func (s *service) RemoveOrganizationMember(ctx context.Context, orgID string, userID string) error {
	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return err
	}
	return s.repo.RemoveMember(ctx, orgID, userID)
}

func (s *service) UpdateOrganizationMemberRole(ctx context.Context, orgID string, userID string, req UpdateMemberRequest) error {
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if req.Role != RoleOrganizationManager {
		return apperror.New(400, "invalid role: only 'manager' can be assigned via this endpoint")
	}

	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return err
	}

	return s.repo.UpdateMemberRole(ctx, orgID, userID, req.Role)
}

func (s *service) ListOrganizationMembers(ctx context.Context, orgID string, filter MemberFilter) ([]*Member, int, error) {
	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return nil, 0, err
	}
	return s.repo.ListMembers(ctx, orgID, filter)
}

// ------------------------
//     Permission methods
// ------------------------

// CheckPermission verifies if the user is an Owner or Manager of the organization.
// This checks general organization membership.
func (s *service) CheckPermission(ctx context.Context, orgID string, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	// Check System Admin
	user, err := s.userService.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	if user.IsSystemAdmin {
		return true, nil
	}

	// Check Organization Role
	member, err := s.repo.GetMember(ctx, orgID, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotMember) {
			return false, nil
		}
		return false, err
	}

	// Both Owner and Admin are valid members of the Org.
	// However, for specific location actions, CheckLocationPermission should be used.
	if member.Role == RoleOwner || member.Role == RoleOrganizationManager {
		return true, nil
	}

	return false, nil
}

// CheckLocationPermission verifies if the user has permission for a specific location.
// Owner: Has access to all locations.
// Manager: Has access only if assigned to the location.
func (s *service) CheckLocationPermission(ctx context.Context, orgID string, locationID string, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	// 1. Check System Admin
	user, err := s.userService.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	if user.IsSystemAdmin {
		return true, nil
	}

	// 2. Check Org Level Permission (Owner or Admin)
	// Owners and Org Admins have full access to all locations.
	isOrgStaff, err := s.CheckPermission(ctx, orgID, userID)
	if err != nil {
		return false, err
	}
	if isOrgStaff {
		return true, nil
	}

	// 3. Check Location Manager
	// If not Org Staff, check if assigned specifically to this location.
	if locationID == "" {
		return false, nil
	}
	isAdmin, err := s.repo.IsLocationManager(ctx, locationID, userID)
	if err != nil {
		return false, err
	}
	return isAdmin, nil
}

// CheckIsOwner verifies if the user is an Owner of the organization.
func (s *service) CheckIsOwner(ctx context.Context, orgID string, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	// 1. Check System Admin (God mode)
	user, err := s.userService.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	if user.IsSystemAdmin {
		return true, nil
	}

	// 2. Check Member Role
	member, err := s.repo.GetMember(ctx, orgID, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotMember) {
			return false, nil
		}
		return false, err
	}

	if member.Role == RoleOwner {
		return true, nil
	}

	return false, nil
}

// AddLocationManager assigns a manager to a location
func (s *service) AddLocationManager(ctx context.Context, locationID string, userID string) error {
	// Get Org ID to check for conflicts
	orgID, err := s.repo.GetOrgIDByLocationID(ctx, locationID)
	if err != nil {
		return err
	}

	// Mutual Exclusion Check: User cannot be Org Admin/Owner and Location Admin
	isOrgStaff, err := s.CheckPermission(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if isOrgStaff {
		return apperror.New(409, "user is already an organization manager or owner; cannot add as location manager")
	}

	return s.repo.AddLocationManager(ctx, locationID, userID)
}

// RemoveLocationManager removes a manager from a location
func (s *service) RemoveLocationManager(ctx context.Context, locationID string, userID string) error {
	return s.repo.RemoveLocationManager(ctx, locationID, userID)
}

// ListLocationManagers lists users who are managers of a location
func (s *service) ListLocationManagers(ctx context.Context, locationID string) ([]string, error) {
	return s.repo.ListLocationManagers(ctx, locationID)
}

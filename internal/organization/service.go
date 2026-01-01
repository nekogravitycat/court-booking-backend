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
	OwnerID  *string
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
	Create(ctx context.Context, name string, ownerID string) (*Organization, error)
	GetByID(ctx context.Context, id string) (*Organization, error)
	List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error)
	Update(ctx context.Context, id string, req UpdateOrganizationRequest) (*Organization, error)
	Delete(ctx context.Context, id string) error
	// Organization Manager methods
	AddOrganizationManager(ctx context.Context, orgID string, userID string) error
	RemoveOrganizationManager(ctx context.Context, orgID string, userID string) error
	ListOrganizationManagers(ctx context.Context, orgID string) ([]*user.User, error)
	// Permission methods
	CheckPermission(ctx context.Context, orgID string, userID string) (bool, error)
	CheckIsOwner(ctx context.Context, orgID string, userID string) (bool, error)
}

// LocationManagerChecker defines the method required to check location manager status.
// This interface allows OrganizationService to communicate with Location module without direct import.
type LocationManagerChecker interface {
	IsLocationManagerInOrg(ctx context.Context, orgID string, userID string) (bool, error)
}

type service struct {
	repo        Repository
	userService user.Service
	locChecker  LocationManagerChecker
}

// NewService creates a new organization service.
func NewService(repo Repository, userService user.Service, locChecker LocationManagerChecker) Service {
	return &service{repo: repo, userService: userService, locChecker: locChecker}
}

// ------------------------
//   Organization methods
// ------------------------

func (s *service) Create(ctx context.Context, name string, ownerID string) (*Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}
	if ownerID == "" {
		return nil, ErrUserIDRequired
	}

	// Verify owner exists
	if _, err := s.userService.GetByID(ctx, ownerID); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	org := &Organization{
		Name:     name,
		OwnerID:  ownerID,
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
	if req.OwnerID != nil {
		newOwnerID := *req.OwnerID
		// Verify new owner exists
		if _, err := s.userService.GetByID(ctx, newOwnerID); err != nil {
			if errors.Is(err, user.ErrNotFound) {
				return nil, ErrUserNotFound
			}
			return nil, err
		}

		// Mutual Exclusion Check: User cannot be both Org Owner and Location Manager
		isLoMgr, err := s.locChecker.IsLocationManagerInOrg(ctx, id, newOwnerID)
		if err != nil {
			return nil, err
		}
		if isLoMgr {
			return nil, apperror.New(409, "user is already a location manager in this organization; remove location manager privileges first")
		}

		org.OwnerID = newOwnerID
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

// -----------------------------
//   Organization Manager methods
// -----------------------------

func (s *service) AddOrganizationManager(ctx context.Context, orgID string, userID string) error {
	// Verify organization exists (get org to check owner)
	org, err := s.repo.GetByID(ctx, orgID)
	if err != nil {
		return err
	}
	if org.OwnerID == userID {
		return apperror.New(409, "user is already the owner of this organization")
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
	isLoMgr, err := s.locChecker.IsLocationManagerInOrg(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if isLoMgr {
		return apperror.New(409, "user is already a location manager in this organization; remove location manager privileges first")
	}

	return s.repo.AddOrganizationManager(ctx, orgID, userID)
}

func (s *service) RemoveOrganizationManager(ctx context.Context, orgID string, userID string) error {
	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return err
	}
	return s.repo.RemoveOrganizationManager(ctx, orgID, userID)
}

func (s *service) ListOrganizationManagers(ctx context.Context, orgID string) ([]*user.User, error) {
	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return nil, err
	}

	members, err := s.repo.ListOrganizationManagers(ctx, orgID)
	if err != nil {
		return nil, err
	}

	if len(members) == 0 {
		return []*user.User{}, nil
	}

	userIDs := make([]string, len(members))
	for i, u := range members {
		userIDs[i] = u.ID
	}

	users, _, err := s.userService.List(ctx, user.UserFilter{
		IDs: userIDs,
	})
	if err != nil {
		return nil, err
	}

	return users, nil
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

	// Check Organization Owner
	org, err := s.repo.GetByID(ctx, orgID)
	if err != nil {
		return false, err
	}
	if org.OwnerID == userID {
		return true, nil
	}

	// Check Organization Manager
	isManager, err := s.repo.IsOrganizationManager(ctx, orgID, userID)
	if err != nil {
		return false, err
	}
	return isManager, nil
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

	// 2. Check Owner
	org, err := s.repo.GetByID(ctx, orgID)
	if err != nil {
		return false, err
	}
	if org.OwnerID == userID {
		return true, nil
	}

	return false, nil
}

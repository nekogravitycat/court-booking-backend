package organization

import (
	"context"
	"errors"
	"strings"

	"github.com/nekogravitycat/court-booking-backend/internal/file"
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
	CheckOperation(ctx context.Context, orgID, userID string) error
	// Organization methods
	Create(ctx context.Context, name string, ownerID string) (*Organization, error)
	GetByID(ctx context.Context, id string) (*Organization, error)
	List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error)
	Update(ctx context.Context, id string, req UpdateOrganizationRequest) (*Organization, error)
	UpdateCover(ctx context.Context, id string, fileID string) error
	RemoveCover(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	// Organization Manager methods
	AddOrganizationManager(ctx context.Context, orgID string, userID string) error
	RemoveOrganizationManager(ctx context.Context, orgID string, userID string) error
	ListOrganizationManagers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error)
	// Organization Member methods
	AddMember(ctx context.Context, orgID string, email string) error
	RemoveMember(ctx context.Context, orgID string, userID string) error
	ListMembers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error)
	// Permission methods
	IsOwnerOrAbove(ctx context.Context, orgID string, userID string) (bool, error)
	IsManagerOrAbove(ctx context.Context, orgID string, userID string) (bool, error)
	// RequireOwner and RequireManager gate an operation: CheckOperation, then the role check,
	// failing with a 403 carrying forbiddenMsg.
	RequireOwner(ctx context.Context, orgID, userID, forbiddenMsg string) error
	RequireManager(ctx context.Context, orgID, userID, forbiddenMsg string) error
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
	fileService file.Service
}

// NewService creates a new organization service.
func NewService(repo Repository, userService user.Service, locChecker LocationManagerChecker, fileService file.Service) Service {
	return &service{repo: repo, userService: userService, locChecker: locChecker, fileService: fileService}
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
	owner, err := s.userService.GetByID(ctx, ownerID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if !owner.IsActive {
		return nil, ErrOwnerInactive
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
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, ErrNameRequired
		}
		req.Name = &name
	}
	if req.OwnerID != nil {
		newOwner, err := s.userService.GetByID(ctx, *req.OwnerID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				return nil, ErrUserNotFound
			}
			return nil, err
		}
		if !newOwner.IsActive {
			return nil, ErrOwnerInactive
		}
	}
	if err := s.repo.UpdateDetails(ctx, id, req); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}

func (s *service) Delete(ctx context.Context, id string) error {
	// Check existence
	org, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// Clean up cover file after the organization is deactivated.
	file.ReleaseReplaced(ctx, s.fileService, org.Cover, "")

	return nil
}

func (s *service) UpdateCover(ctx context.Context, id string, fileID string) error {
	// Persist the new reference first; only delete the old file once the new
	// reference is durably stored, to avoid orphaned files / dangling references.
	oldCover, err := s.repo.SetCover(ctx, id, &fileID)
	if err != nil {
		return err
	}
	file.ReleaseReplaced(ctx, s.fileService, oldCover, fileID)
	return nil
}

func (s *service) RemoveCover(ctx context.Context, id string) error {
	// Clear the reference first, then delete the file, keeping the database
	// consistent even if the storage delete fails (best effort).
	oldCover, err := s.repo.SetCover(ctx, id, nil)
	if err != nil {
		return err
	}
	file.ReleaseReplaced(ctx, s.fileService, oldCover, "")
	return nil
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

	// Membership Check: User must be a member first
	isMember, err := s.repo.IsMember(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return apperror.New(400, "user must be a member of the organization first")
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

func (s *service) ListOrganizationManagers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error) {
	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return nil, 0, err
	}

	return s.repo.ListOrganizationManagers(ctx, orgID, filter)
}

// -----------------------------
//   Organization Member methods
// -----------------------------

func (s *service) AddMember(ctx context.Context, orgID string, email string) error {
	// Verify organization exists
	org, err := s.repo.GetByID(ctx, orgID)
	if err != nil {
		return err
	}

	// Lookup user by email
	foundUser, err := s.userService.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return ErrUserNotFound
		}
		return err
	}

	userID := foundUser.ID

	// Mutual Exclusion: Owner cannot be a member
	if org.OwnerID == userID {
		return apperror.New(409, "user is the owner of this organization")
	}

	return s.repo.AddMember(ctx, orgID, userID)
}

func (s *service) RemoveMember(ctx context.Context, orgID string, userID string) error {
	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return err
	}
	return s.repo.RemoveMember(ctx, orgID, userID)
}

func (s *service) ListMembers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error) {
	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return nil, 0, err
	}
	return s.repo.ListMembers(ctx, orgID, filter)
}

// ------------------------
//     Permission methods
// ------------------------

// IsOwnerOrAbove verifies if the user is an Owner of the organization or SysAdmin.
func (s *service) IsOwnerOrAbove(ctx context.Context, orgID string, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	// 1. Check System Admin (God mode)
	account, err := s.userService.GetAccount(ctx, userID)
	if err != nil {
		return false, err
	}
	if account.IsSystemAdmin {
		return true, nil
	}

	// 2. Check Owner
	org, err := s.repo.GetByID(ctx, orgID)
	if err != nil {
		return false, err
	}
	return org.OwnerID == userID, nil
}

// IsManagerOrAbove verifies if the user is an Owner or Manager of the organization, or SysAdmin.
func (s *service) IsManagerOrAbove(ctx context.Context, orgID string, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	// Check if user is owner or above
	if isOwner, err := s.IsOwnerOrAbove(ctx, orgID, userID); err != nil {
		return false, err
	} else if isOwner {
		return true, nil
	}

	// Check if user is organization manager
	return s.repo.IsOrganizationManager(ctx, orgID, userID)
}

// RequireOwner fails unless the organization accepts new operations and the user is its owner or a SysAdmin.
func (s *service) RequireOwner(ctx context.Context, orgID, userID, forbiddenMsg string) error {
	return s.require(ctx, orgID, userID, forbiddenMsg, s.IsOwnerOrAbove)
}

// RequireManager fails unless the organization accepts new operations and the user is an owner, manager or SysAdmin.
func (s *service) RequireManager(ctx context.Context, orgID, userID, forbiddenMsg string) error {
	return s.require(ctx, orgID, userID, forbiddenMsg, s.IsManagerOrAbove)
}

func (s *service) require(ctx context.Context, orgID, userID, forbiddenMsg string, check func(context.Context, string, string) (bool, error)) error {
	if err := s.CheckOperation(ctx, orgID, userID); err != nil {
		return err
	}
	allowed, err := check(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if !allowed {
		return apperror.Forbidden(forbiddenMsg)
	}
	return nil
}

// CheckOperation preserves historical reads while blocking new operations for
// inactive organizations. System administrators may restore/manage them.
func (s *service) CheckOperation(ctx context.Context, orgID, userID string) error {
	org, err := s.repo.GetByID(ctx, orgID)
	if err != nil {
		return err
	}
	if org.IsActive {
		return nil
	}
	account, err := s.userService.GetAccount(ctx, userID)
	if err != nil {
		return err
	}
	if account.IsSystemAdmin {
		return nil
	}
	return ErrOrgInactive
}

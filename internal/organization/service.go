package organization

import (
	"context"
	"strings"
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
	GetMember(ctx context.Context, orgID string, userID string) (*Member, error)
	AddMember(ctx context.Context, orgID string, req AddMemberRequest) error
	RemoveMember(ctx context.Context, orgID string, userID string) error
	UpdateMemberRole(ctx context.Context, orgID string, userID string, req UpdateMemberRequest) error
	ListMembers(ctx context.Context, orgID string, filter MemberFilter) ([]*Member, int, error)
}

type service struct {
	repo Repository
}

// NewService creates a new organization service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
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

func (s *service) GetMember(ctx context.Context, orgID string, userID string) (*Member, error) {
	return s.repo.GetMember(ctx, orgID, userID)
}

func (s *service) AddMember(ctx context.Context, orgID string, req AddMemberRequest) error {
	if req.UserID == "" {
		return ErrUserIDRequired
	}

	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if !isValidRole(req.Role) {
		return ErrInvalidRole
	}

	// Verify organization exists first
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return err
	}

	return s.repo.AddMember(ctx, orgID, req.UserID, req.Role)
}

func (s *service) RemoveMember(ctx context.Context, orgID string, userID string) error {
	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return err
	}
	return s.repo.RemoveMember(ctx, orgID, userID)
}

func (s *service) UpdateMemberRole(ctx context.Context, orgID string, userID string, req UpdateMemberRequest) error {
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if !isValidRole(req.Role) {
		return ErrInvalidRole
	}

	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return err
	}

	return s.repo.UpdateMemberRole(ctx, orgID, userID, req.Role)
}

func (s *service) ListMembers(ctx context.Context, orgID string, filter MemberFilter) ([]*Member, int, error) {
	// Verify organization exists
	if _, err := s.repo.GetByID(ctx, orgID); err != nil {
		return nil, 0, err
	}
	return s.repo.ListMembers(ctx, orgID, filter)
}

func isValidRole(r string) bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember:
		return true
	}
	return false
}

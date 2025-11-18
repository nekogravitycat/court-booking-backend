package organization

import (
	"context"
	"errors"
	"strings"
)

// Service defines business logic for organizations.
type Service interface {
	Create(ctx context.Context, name string) (*Organization, error)
	GetByID(ctx context.Context, id int64) (*Organization, error)
	List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error)
	Update(ctx context.Context, id int64, name string) (*Organization, error)
	Delete(ctx context.Context, id int64) error
}

type service struct {
	repo Repository
}

// NewService creates a new organization service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, name string) (*Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("organization name is required")
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

func (s *service) GetByID(ctx context.Context, id int64) (*Organization, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id int64, name string) (*Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("organization name cannot be empty")
	}

	// Check existence
	org, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	org.Name = name
	if err := s.repo.Update(ctx, org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *service) Delete(ctx context.Context, id int64) error {
	// Check existence
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

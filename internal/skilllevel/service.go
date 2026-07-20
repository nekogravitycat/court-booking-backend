package skilllevel

import (
	"context"
	"strings"
)

type CreateRequest struct {
	SportID   string
	Name      string
	SortOrder int
}

type UpdateRequest struct {
	Name      *string
	SortOrder *int
	IsActive  *bool
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*SkillLevel, error)
	GetByID(ctx context.Context, id string) (*SkillLevel, error)
	List(ctx context.Context, filter Filter) ([]*SkillLevel, int, error)
	Update(ctx context.Context, id string, req UpdateRequest) (*SkillLevel, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*SkillLevel, error) {
	if strings.TrimSpace(req.SportID) == "" {
		return nil, ErrSportRequired
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrNameRequired
	}

	sl := &SkillLevel{
		SportID:   req.SportID,
		Name:      name,
		SortOrder: req.SortOrder,
		IsActive:  true,
	}
	if err := s.repo.Create(ctx, sl); err != nil {
		return nil, err
	}
	return sl, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*SkillLevel, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter Filter) ([]*SkillLevel, int, error) {
	return s.repo.List(ctx, filter)
}

// Update mutates a skill level's name, ordering, and active flag. The owning
// sport is fixed at creation time and cannot be reassigned.
func (s *service) Update(ctx context.Context, id string, req UpdateRequest) (*SkillLevel, error) {
	sl, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, ErrNameRequired
		}
		sl.Name = name
	}
	if req.SortOrder != nil {
		sl.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		sl.IsActive = *req.IsActive
	}

	if err := s.repo.Update(ctx, sl); err != nil {
		return nil, err
	}
	return sl, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

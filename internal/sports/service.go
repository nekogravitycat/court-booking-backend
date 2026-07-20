package sports

import (
	"context"
	"strings"
)

type CreateRequest struct {
	Code string
	Name string
}

type UpdateRequest struct {
	Code     *string
	Name     *string
	IsActive *bool
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*Sport, error)
	GetByID(ctx context.Context, id string) (*Sport, error)
	List(ctx context.Context, filter Filter) ([]*Sport, int, error)
	Update(ctx context.Context, id string, req UpdateRequest) (*Sport, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// normalizeCode trims spaces and uppercases the code so it stays a stable,
// case-insensitive machine key.
func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*Sport, error) {
	code := normalizeCode(req.Code)
	if code == "" {
		return nil, ErrCodeRequired
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrNameRequired
	}

	sp := &Sport{
		Code:     code,
		Name:     name,
		IsActive: true,
	}
	if err := s.repo.Create(ctx, sp); err != nil {
		return nil, err
	}
	return sp, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*Sport, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter Filter) ([]*Sport, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateRequest) (*Sport, error) {
	sp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Code != nil {
		code := normalizeCode(*req.Code)
		if code == "" {
			return nil, ErrCodeRequired
		}
		sp.Code = code
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, ErrNameRequired
		}
		sp.Name = name
	}
	if req.IsActive != nil {
		sp.IsActive = *req.IsActive
	}

	if err := s.repo.Update(ctx, sp); err != nil {
		return nil, err
	}
	return sp, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

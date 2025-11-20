package announcement

import (
	"context"
	"strings"
)

type CreateRequest struct {
	Title   string
	Content string
}

type UpdateRequest struct {
	Title   *string
	Content *string
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*Announcement, error)
	GetByID(ctx context.Context, id string) (*Announcement, error)
	List(ctx context.Context, filter Filter) ([]*Announcement, int, error)
	Update(ctx context.Context, id string, req UpdateRequest) (*Announcement, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*Announcement, error) {
	if strings.TrimSpace(req.Title) == "" {
		return nil, ErrTitleRequired
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, ErrContentRequired
	}

	a := &Announcement{
		Title:   req.Title,
		Content: req.Content,
	}

	if err := s.repo.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*Announcement, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter Filter) ([]*Announcement, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateRequest) (*Announcement, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			return nil, ErrTitleRequired
		}
		a.Title = *req.Title
	}

	if req.Content != nil {
		if strings.TrimSpace(*req.Content) == "" {
			return nil, ErrContentRequired
		}
		a.Content = *req.Content
	}

	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	// Check existence first
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

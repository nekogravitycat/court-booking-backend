package resourcetype

import (
	"context"
	"strings"
)

type CreateRequest struct {
	OrganizationID string
	Name           string
	Description    string
}

type UpdateRequest struct {
	Name        *string
	Description *string
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*ResourceType, error)
	GetByID(ctx context.Context, id string) (*ResourceType, error)
	List(ctx context.Context, filter Filter) ([]*ResourceType, int, error)
	Update(ctx context.Context, id string, req UpdateRequest) (*ResourceType, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*ResourceType, error) {
	if req.OrganizationID == "" {
		return nil, ErrOrgIDRequired
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, ErrNameRequired
	}

	rt := &ResourceType{
		OrganizationID: req.OrganizationID,
		Name:           req.Name,
		Description:    req.Description,
	}

	if err := s.repo.Create(ctx, rt); err != nil {
		return nil, err
	}
	return rt, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*ResourceType, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter Filter) ([]*ResourceType, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateRequest) (*ResourceType, error) {
	rt, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return nil, ErrNameRequired
		}
		rt.Name = *req.Name
	}
	if req.Description != nil {
		rt.Description = *req.Description
	}

	if err := s.repo.Update(ctx, rt); err != nil {
		return nil, err
	}
	return rt, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	// Check existence
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

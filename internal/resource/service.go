package resource

import (
	"context"
	"strings"

	"github.com/nekogravitycat/court-booking-backend/internal/location"
)

type CreateRequest struct {
	Name         string
	LocationID   string
	ResourceType string
}

type UpdateRequest struct {
	Name *string
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*Resource, error)
	GetByID(ctx context.Context, id string) (*Resource, error)
	List(ctx context.Context, filter Filter) ([]*Resource, int, error)
	Update(ctx context.Context, id string, req UpdateRequest) (*Resource, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo       Repository
	locService location.Service
}

func NewService(repo Repository, locService location.Service) Service {
	return &service{
		repo:       repo,
		locService: locService,
	}
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*Resource, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, ErrEmptyName
	}
	if req.LocationID == "" {
		return nil, ErrInvalidLocation
	}
	if req.ResourceType == "" {
		return nil, ErrInvalidResourceType
	}

	// Validate resource type is a valid enum value
	validType := false
	for _, t := range ValidResourceTypes {
		if req.ResourceType == t {
			validType = true
			break
		}
	}
	if !validType {
		return nil, ErrInvalidResourceType
	}

	// Validation: Check if Location exists
	_, err := s.locService.GetByID(ctx, req.LocationID)
	if err != nil {
		return nil, ErrInvalidLocation
	}

	res := &Resource{
		Name:         req.Name,
		LocationID:   req.LocationID,
		ResourceType: req.ResourceType,
	}

	if err := s.repo.Create(ctx, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*Resource, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter Filter) ([]*Resource, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateRequest) (*Resource, error) {
	res, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return nil, ErrEmptyName
		}
		res.Name = *req.Name
	}

	if err := s.repo.Update(ctx, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

package resource

import (
	"context"
	"net/http"
	"strings"

	"github.com/nekogravitycat/court-booking-backend/internal/file"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

type CreateRequest struct {
	Name         string
	Price        int
	LocationID   string
	ResourceType string
}

type UpdateRequest struct {
	Name  *string
	Price *int
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*Resource, error)
	GetByID(ctx context.Context, id string) (*Resource, error)
	List(ctx context.Context, filter Filter) ([]*Resource, int, error)
	Update(ctx context.Context, id string, req UpdateRequest) (*Resource, error)
	UpdateCover(ctx context.Context, id string, fileID string) error
	RemoveCover(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo        Repository
	locService  location.Service
	fileService file.Service
}

func NewService(repo Repository, locService location.Service, fileService file.Service) Service {
	return &service{
		repo:        repo,
		locService:  locService,
		fileService: fileService,
	}
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*Resource, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, ErrEmptyName
	}
	if req.Price < 0 {
		return nil, apperror.New(http.StatusBadRequest, "price cannot be negative")
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
		Price:        req.Price,
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
	if req.Price != nil {
		if *req.Price < 0 {
			return nil, apperror.New(http.StatusBadRequest, "price cannot be negative")
		}
		res.Price = *req.Price
	}

	if err := s.repo.Update(ctx, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	res, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Clean up cover file if exists
	if res.Cover != nil && *res.Cover != "" {
		_ = s.fileService.Delete(ctx, *res.Cover)
	}

	return s.repo.Delete(ctx, id)
}

func (s *service) UpdateCover(ctx context.Context, id string, fileID string) error {
	res, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Clean up old cover if exists
	if res.Cover != nil && *res.Cover != "" {
		// Best effort delete, don't block update if fail
		_ = s.fileService.Delete(ctx, *res.Cover)
	}

	res.Cover = &fileID
	return s.repo.Update(ctx, res)
}

func (s *service) RemoveCover(ctx context.Context, id string) error {
	res, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete the old cover file if exists
	if res.Cover != nil && *res.Cover != "" {
		if err := s.fileService.Delete(ctx, *res.Cover); err != nil {
			// Log error but don't block the removal
			_ = err
		}
	}

	// Set cover to nil
	res.Cover = nil
	return s.repo.Update(ctx, res)
}

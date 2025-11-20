package location

import (
	"context"
	"errors"
	"strings"
)

// CreateLocationRequest carries data to create a location.
type CreateLocationRequest struct {
	OrganizationID    string
	Name              string
	Capacity          int64
	OpeningHoursStart string
	OpeningHoursEnd   string
	LocationInfo      string
	Opening           bool
	Rule              string
	Facility          string
	Description       string
	Longitude         float64
	Latitude          float64
}

// UpdateLocationRequest carries data for partial updates.
type UpdateLocationRequest struct {
	Name              *string
	Capacity          *int64
	OpeningHoursStart *string
	OpeningHoursEnd   *string
	LocationInfo      *string
	Opening           *bool
	Rule              *string
	Facility          *string
	Description       *string
	Longitude         *float64
	Latitude          *float64
}

type Service interface {
	Create(ctx context.Context, req CreateLocationRequest) (*Location, error)
	GetByID(ctx context.Context, id string) (*Location, error)
	List(ctx context.Context, filter LocationFilter) ([]*Location, int, error)
	Update(ctx context.Context, id string, req UpdateLocationRequest) (*Location, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, req CreateLocationRequest) (*Location, error) {
	if req.OrganizationID == "" {
		return nil, errors.New("organization_id is required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("name is required")
	}
	// Additional validation (e.g. time format check) can be added here.

	loc := &Location{
		OrganizationID:    req.OrganizationID,
		Name:              req.Name,
		Capacity:          req.Capacity,
		OpeningHoursStart: req.OpeningHoursStart,
		OpeningHoursEnd:   req.OpeningHoursEnd,
		LocationInfo:      req.LocationInfo,
		Opening:           req.Opening,
		Rule:              req.Rule,
		Facility:          req.Facility,
		Description:       req.Description,
		Longitude:         req.Longitude,
		Latitude:          req.Latitude,
	}

	if err := s.repo.Create(ctx, loc); err != nil {
		return nil, err
	}
	return loc, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*Location, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter LocationFilter) ([]*Location, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateLocationRequest) (*Location, error) {
	loc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply non-nil fields
	if req.Name != nil {
		loc.Name = *req.Name
	}
	if req.Capacity != nil {
		loc.Capacity = *req.Capacity
	}
	if req.OpeningHoursStart != nil {
		loc.OpeningHoursStart = *req.OpeningHoursStart
	}
	if req.OpeningHoursEnd != nil {
		loc.OpeningHoursEnd = *req.OpeningHoursEnd
	}
	if req.LocationInfo != nil {
		loc.LocationInfo = *req.LocationInfo
	}
	if req.Opening != nil {
		loc.Opening = *req.Opening
	}
	if req.Rule != nil {
		loc.Rule = *req.Rule
	}
	if req.Facility != nil {
		loc.Facility = *req.Facility
	}
	if req.Description != nil {
		loc.Description = *req.Description
	}
	if req.Longitude != nil {
		loc.Longitude = *req.Longitude
	}
	if req.Latitude != nil {
		loc.Latitude = *req.Latitude
	}

	if err := s.repo.Update(ctx, loc); err != nil {
		return nil, err
	}
	return loc, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	// Check existence
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

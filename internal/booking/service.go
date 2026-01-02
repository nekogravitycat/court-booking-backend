package booking

import (
	"context"
	"errors"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
)

type CreateRequest struct {
	UserID     string
	ResourceID string
	StartTime  time.Time
	EndTime    time.Time
}

type UpdateRequest struct {
	StartTime *time.Time
	EndTime   *time.Time
	Status    *string
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*Booking, error)
	GetByID(ctx context.Context, id string) (*Booking, error)
	List(ctx context.Context, filter Filter) ([]*Booking, int, error)
	Update(ctx context.Context, id string, req UpdateRequest, updaterUserID string, isSysAdmin bool) (*Booking, error)
	Delete(ctx context.Context, id string, deleterUserID string, isSysAdmin bool) error
}

type service struct {
	repo       Repository
	resService resource.Service
	locService location.Service
	orgService organization.Service
}

func NewService(repo Repository, resService resource.Service, locService location.Service, orgService organization.Service) Service {
	return &service{
		repo:       repo,
		resService: resService,
		locService: locService,
		orgService: orgService,
	}
}

// isOrgManager checks if the user is an Owner or Admin of the organization that owns the resource
func (s *service) isOrgManager(ctx context.Context, resourceID string, userID string) (bool, error) {
	// 1. Get Resource
	res, err := s.resService.GetByID(ctx, resourceID)
	if err != nil {
		return false, err
	}
	// 2. Get Location
	loc, err := s.locService.GetByID(ctx, res.LocationID)
	if err != nil {
		return false, err
	}
	// 3. Check Permission using Org Service
	return s.orgService.IsManagerOrAbove(ctx, loc.OrganizationID, userID)
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*Booking, error) {
	// 1. Validate Time Range
	if req.EndTime.Before(req.StartTime) || req.EndTime.Equal(req.StartTime) {
		return nil, ErrInvalidTimeRange
	}
	// Strict check: StartTime cannot be in the past
	if req.StartTime.Before(time.Now().UTC()) {
		return nil, ErrStartTimePast
	}

	// 2. Validate Resource Exists
	if _, err := s.resService.GetByID(ctx, req.ResourceID); err != nil {
		switch {
		case errors.Is(err, resource.ErrNotFound):
			return nil, ErrResourceNotFound
		default:
			return nil, err
		}
	}

	// 3. Check for Overlaps
	hasOverlap, err := s.repo.HasOverlap(ctx, req.ResourceID, req.StartTime, req.EndTime, "")
	if err != nil {
		return nil, err
	}
	if hasOverlap {
		return nil, ErrTimeConflict
	}

	// 4. Create Booking
	booking := &Booking{
		ResourceID: req.ResourceID,
		UserID:     req.UserID,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Status:     StatusPending, // Default status
	}

	if err := s.repo.Create(ctx, booking); err != nil {
		return nil, err
	}

	return booking, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*Booking, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, filter Filter) ([]*Booking, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateRequest, updaterUserID string, isSysAdmin bool) (*Booking, error) {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Permission Check Logic:
	// 1. System Admin -> Allowed
	// 2. Owner of Booking -> Allowed (with restrictions on Status)
	// 3. Org Owner/Admin -> Allowed

	isBookingOwner := b.UserID == updaterUserID
	isOrgMgr := false

	if !isSysAdmin && !isBookingOwner {
		// Lazy check: only query DB if not already authorized
		var err error
		isOrgMgr, err = s.isOrgManager(ctx, b.ResourceID, updaterUserID)
		if err != nil {
			return nil, err // Internal error (e.g. DB down)
		}
	}

	if !isSysAdmin && !isBookingOwner && !isOrgMgr {
		return nil, ErrPermissionDenied
	}

	// Prepare new values
	newStart := b.StartTime
	newEnd := b.EndTime
	timeChanged := false

	if req.StartTime != nil {
		newStart = *req.StartTime
		timeChanged = true
	}
	if req.EndTime != nil {
		newEnd = *req.EndTime
		timeChanged = true
	}

	if timeChanged {
		if newEnd.Before(newStart) || newEnd.Equal(newStart) {
			return nil, ErrInvalidTimeRange
		}

		// Check past time for updates
		if req.StartTime != nil && req.StartTime.Before(time.Now().UTC()) {
			return nil, ErrStartTimePast
		}

		// Check Overlap excluding current booking
		hasOverlap, err := s.repo.HasOverlap(ctx, b.ResourceID, newStart, newEnd, b.ID)
		if err != nil {
			return nil, err
		}
		if hasOverlap {
			return nil, ErrTimeConflict
		}
		b.StartTime = newStart
		b.EndTime = newEnd
	}

	if req.Status != nil {
		st := Status(*req.Status)
		if st != StatusPending && st != StatusConfirmed && st != StatusCancelled {
			return nil, ErrInvalidStatus
		}

		// Business Logic: Normal User (Booking Owner) can ONLY Cancel
		// SysAdmin or OrgManager can do anything
		if isBookingOwner && !isSysAdmin && !isOrgMgr {
			if st != StatusCancelled {
				return nil, ErrPermissionDenied
			}
		}
		b.Status = st
	}

	if err := s.repo.Update(ctx, b); err != nil {
		return nil, err
	}

	return b, nil
}

func (s *service) Delete(ctx context.Context, id string, deleterUserID string, isSysAdmin bool) error {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Permission Check
	isBookingOwner := b.UserID == deleterUserID
	isOrgMgr := false

	if !isSysAdmin && !isBookingOwner {
		var err error
		isOrgMgr, err = s.isOrgManager(ctx, b.ResourceID, deleterUserID)
		if err != nil {
			return err
		}
	}

	if !isSysAdmin && !isBookingOwner && !isOrgMgr {
		return ErrPermissionDenied
	}

	return s.repo.Delete(ctx, id)
}

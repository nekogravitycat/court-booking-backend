package booking

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
)

// TimeSlot represents a time range where a resource is available.
type TimeSlot struct {
	StartTime time.Time
	EndTime   time.Time
}

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
	GetAvailability(ctx context.Context, resourceID string, date time.Time) ([]TimeSlot, error)
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

func (s *service) GetAvailability(ctx context.Context, resourceID string, date time.Time) ([]TimeSlot, error) {
	// Get Resource to find Location
	res, err := s.resService.GetByID(ctx, resourceID)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			return nil, ErrResourceNotFound
		}
		return nil, err
	}

	// Get Location for Opening Hours
	loc, err := s.locService.GetByID(ctx, res.LocationID)
	if err != nil {
		return nil, err
	}

	// List Bookings for the day
	// We need bookings that overlap with the day:
	// Start < EndOfDay AND End > StartOfDay
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	bookings, _, err := s.repo.List(ctx, Filter{
		ResourceID: resourceID,
		StartTime:  &startOfDay, // Filter where EndTime >= StartOfDay (handled by repo logic: EndTime > filter.StartTime)
		EndTime:    &endOfDay,   // Filter where StartTime <= EndOfDay (handled by repo logic: StartTime < filter.EndTime)
		Page:       1,
		PageSize:   1000, // Fetch all relevant bookings
		SortBy:     "start_time",
		SortOrder:  "ASC",
	})
	if err != nil {
		return nil, err
	}

	// Calculate Slots
	return CalculateAvailability(date, loc.OpeningHoursStart, loc.OpeningHoursEnd, bookings)
}

// CalculateAvailability computes available time slots given the operating hours and existing bookings.
//
// Algorithm Design:
//  1. Normalization: Opening and closing times are parsed and normalized to the specific date requested.
//  2. Sorting: Bookings are sorted by start time to allow for a linear pass.
//  3. Linear Scan: We iterate through the sorted bookings, maintaining a 'currentStart' pointer that tracks
//     the beginning of the next potential available slot.
//     - For each booking, we verify if there is a gap between 'currentStart' and the booking's start time.
//     - If a gap exists, it is recorded as an available TimeSlot.
//     - 'currentStart' is then advanced to the end of the current booking.
//  4. Final Slot: After processing all bookings, if 'currentStart' is still before the closing time,
//     the remaining time is added as the final available slot.
func CalculateAvailability(date time.Time, openStr, closeStr string, bookings []*Booking) ([]TimeSlot, error) {
	// 1. Parse Opening and Closing Times
	layout := "15:04:05"
	if len(openStr) == 5 {
		layout = "15:04"
	}

	openTime, err := time.Parse(layout, openStr)
	if err != nil {
		return nil, err
	}
	// Normalizing to the given date
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), openTime.Hour(), openTime.Minute(), openTime.Second(), 0, time.UTC)

	if len(closeStr) == 5 {
		layout = "15:04"
	}
	closeTime, err := time.Parse(layout, closeStr)
	if err != nil {
		return nil, err
	}
	endOfDay := time.Date(date.Year(), date.Month(), date.Day(), closeTime.Hour(), closeTime.Minute(), closeTime.Second(), 0, time.UTC)

	if endOfDay.Before(startOfDay) {
		// Handle case where closing time is past midnight (next day) - simplistic for now, assume same day
		// For this specific requirement, let's assume valid business hours within a day or handle error
		// Logic in Location service ensures End > Start, but that's just time comparison.
		// Here we map to a specific date.
		return nil, ErrInvalidTimeRange
	}

	// 2. Sort bookings by start time
	sort.Slice(bookings, func(i, j int) bool {
		return bookings[i].StartTime.Before(bookings[j].StartTime)
	})

	var availableSlots []TimeSlot
	currentStart := startOfDay

	for _, book := range bookings {
		// Ignore cancelled bookings
		if book.Status == StatusCancelled {
			continue
		}

		// Adjust booking times to be within the operating day (clamping)
		bookStart := book.StartTime
		bookEnd := book.EndTime
		if bookEnd.Before(currentStart) {
			continue // Already passed this booking
		}
		if bookStart.After(endOfDay) {
			break // Booking is after closing, no need to check further
		}

		// Clamp booking start time to current processing start time
		/*
			This logic handles overlapping bookings.

			As we iterate through the bookings, currentStart tracks the end of the previous booking (or the opening time).
			If the current booking starts before the previous one ended (an overlap), bookStart would be less than currentStart.

			This line effectively "trims" the start of the current booking to ignore the part that overlaps with the previous one,
			ensuring we don't start checking for available slots "backwards" in time.

			For example:

			Booking A: 10:00 - 11:00 (currentStart becomes 11:00).
			Booking B: 10:30 - 11:30.
			When processing B, bookStart (10:30) is before currentStart (11:00).
			We clamp bookStart to 11:00.
			The next check if bookStart.After(currentStart) is 11:00 > 11:00 (False), so no "free slot" is created (correctly).
			currentStart is then updated to 11:30.
		*/
		if bookStart.Before(currentStart) {
			bookStart = currentStart
		}
		// Clamp booking end time to end of business day
		if bookEnd.After(endOfDay) {
			bookEnd = endOfDay
		}

		// If there is a gap between currentStart and booking start, that's an available slot
		if bookStart.After(currentStart) {
			availableSlots = append(availableSlots, TimeSlot{
				StartTime: currentStart,
				EndTime:   bookStart,
			})
		}

		// Move current pointer to end of this booking
		if bookEnd.After(currentStart) {
			currentStart = bookEnd
		}
	}

	// 3. Add final slot if there is time remaining until close
	if currentStart.Before(endOfDay) {
		availableSlots = append(availableSlots, TimeSlot{
			StartTime: currentStart,
			EndTime:   endOfDay,
		})
	}

	return availableSlots, nil
}

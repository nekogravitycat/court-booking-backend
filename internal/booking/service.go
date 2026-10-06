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
	StartTime     *time.Time
	EndTime       *time.Time
	Status        *string
	PaymentStatus *string
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*Booking, error)
	GetByID(ctx context.Context, id string) (*Booking, error)
	// GetForViewer returns the booking if the viewer owns it, manages its organization, or is a system admin.
	GetForViewer(ctx context.Context, id string, viewerID string, isSysAdmin bool) (*Booking, error)
	List(ctx context.Context, filter Filter) ([]*Booking, int, error)
	Update(ctx context.Context, id string, req UpdateRequest, updaterUserID string, isSysAdmin bool) (*Booking, error)
	Delete(ctx context.Context, id string, deleterUserID string, isSysAdmin bool) error
	GetAvailability(ctx context.Context, resourceID string, date time.Time) ([]TimeSlot, error)
	// GetLocationAvailability returns the availability of every resource of a location for one date.
	GetLocationAvailability(ctx context.Context, locationID string, date time.Time) ([]ResourceAvailability, error)
	// CreateSeries creates a Booking Series (seasonal rental) all-or-nothing.
	CreateSeries(ctx context.Context, req CreateSeriesRequest) (*BookingSeries, error)
	// GetSeriesForViewer returns the series if the viewer owns it, manages its organization, or is a system admin.
	GetSeriesForViewer(ctx context.Context, id string, viewerID string, isSysAdmin bool) (*BookingSeries, error)
	// OnUserDeactivated cancels a deactivated user's upcoming bookings.
	OnUserDeactivated(ctx context.Context, userID string) error
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

// authorize resolves the caller's relation to a booking and rejects callers who are neither a
// system admin, the booking's owner, nor a manager of the booking's organization. isOrgMgr is
// only evaluated for non-admins; management privileges also apply to a manager's own booking.
func (s *service) authorize(ctx context.Context, b *Booking, userID string, isSysAdmin bool) (isOwner, isOrgMgr bool, err error) {
	isOwner = b.UserID == userID
	if !isSysAdmin {
		isOrgMgr, err = s.orgService.IsManagerOrAbove(ctx, b.OrganizationID, userID)
		if err != nil {
			return false, false, err // Internal error (e.g. DB down)
		}
	}
	if !isSysAdmin && !isOwner && !isOrgMgr {
		return false, false, ErrPermissionDenied
	}
	return isOwner, isOrgMgr, nil
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
	res, err := s.resService.GetByID(ctx, req.ResourceID)
	if err != nil {
		switch {
		case errors.Is(err, resource.ErrNotFound):
			return nil, ErrResourceNotFound
		default:
			return nil, err
		}
	}

	// 2b. Validate the booking against the location's operating constraints
	// (open flag, opening hours in the location timezone, max duration).
	loc, err := s.locService.GetByID(ctx, res.LocationID)
	if err != nil {
		return nil, err
	}
	if err := s.orgService.CheckOperation(ctx, loc.OrganizationID, req.UserID); err != nil {
		return nil, err
	}
	if err := validateBookingWindow(loc, req.StartTime, req.EndTime); err != nil {
		return nil, err
	}

	// 2c. Cap how many upcoming bookings one user can hold, so a single account
	// cannot sweep the calendar.
	active, err := s.repo.CountUpcomingActiveByUser(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if active >= MaxActiveBookingsPerUser {
		return nil, ErrTooManyActiveBookings
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

	// 5. Fetch full booking details (joins) for response
	return s.repo.GetByID(ctx, booking.ID)
}

func (s *service) GetByID(ctx context.Context, id string) (*Booking, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) GetForViewer(ctx context.Context, id string, viewerID string, isSysAdmin bool) (*Booking, error) {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.authorize(ctx, b, viewerID, isSysAdmin); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *service) List(ctx context.Context, filter Filter) ([]*Booking, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateRequest, updaterUserID string, isSysAdmin bool) (*Booking, error) {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.orgService.CheckOperation(ctx, b.OrganizationID, updaterUserID); err != nil {
		return nil, err
	}

	// Permission Check Logic:
	// 1. System Admin -> Allowed
	// 2. Owner of Booking -> Allowed (with restrictions on Status)
	// 3. Org Owner/Admin -> Allowed
	isBookingOwner, isOrgMgr, err := s.authorize(ctx, b, updaterUserID, isSysAdmin)
	if err != nil {
		return nil, err
	}

	// A plain owner (no management privilege) is restricted the same way a
	// pickup booker is: a confirmed, paid, started, or cancelled booking can
	// only be handled by a manager.
	plainOwner := isBookingOwner && !isSysAdmin && !isOrgMgr

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

	if timeChanged && plainOwner {
		if b.Status != StatusPending || b.PaymentStatus == PaymentStatusDone || !b.StartTime.After(time.Now().UTC()) {
			return nil, ErrChangeRequiresReview
		}
	}

	if timeChanged {
		if newEnd.Before(newStart) || newEnd.Equal(newStart) {
			return nil, ErrInvalidTimeRange
		}

		// Check past time for updates
		if req.StartTime != nil && req.StartTime.Before(time.Now().UTC()) {
			return nil, ErrStartTimePast
		}

		// Validate the new time range against the location's operating
		// constraints (open flag, opening hours, max duration).
		loc, err := s.locService.GetByID(ctx, b.LocationID)
		if err != nil {
			return nil, err
		}
		if err := validateBookingWindow(loc, newStart, newEnd); err != nil {
			return nil, err
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
		if !st.IsValid() {
			return nil, ErrInvalidStatus
		}

		// Business Logic: Normal User (Booking Owner) can only cancel or
		// request cancellation. SysAdmin or OrgManager can do anything.
		if plainOwner {
			if st != StatusCancelled && st != StatusCancelRequest {
				return nil, ErrPermissionDenied
			}
			// A cancelled booking cannot be revived by its owner; they must
			// create a new one.
			if b.Status == StatusCancelled {
				return nil, ErrPermissionDenied
			}
			if st == StatusCancelled && (b.Status == StatusConfirmed || b.Status == StatusCancelRequest || b.PaymentStatus == PaymentStatusDone) {
				return nil, ErrCancellationRequiresReview
			}
		}
		b.Status = st
	}

	if req.PaymentStatus != nil {
		// Payment status is manager-only (SysAdmin or OrgManager).
		if !isSysAdmin && !isOrgMgr {
			return nil, ErrPermissionDenied
		}
		ps := PaymentStatus(*req.PaymentStatus)
		if !ps.IsValid() {
			return nil, ErrInvalidStatus
		}
		b.PaymentStatus = ps
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

	if err := s.orgService.CheckOperation(ctx, b.OrganizationID, deleterUserID); err != nil {
		return err
	}

	isBookingOwner, isOrgMgr, err := s.authorize(ctx, b, deleterUserID, isSysAdmin)
	if err != nil {
		return err
	}

	// Hard-deleting would erase the payment / approval record, so a plain owner
	// may only delete a booking that is still unpaid and unconfirmed.
	if isBookingOwner && !isSysAdmin && !isOrgMgr {
		if b.Status == StatusConfirmed || b.Status == StatusCancelRequest || b.PaymentStatus == PaymentStatusDone {
			return ErrCancellationRequiresReview
		}
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

	tz, err := loadLocationTZ(loc.Timezone)
	if err != nil {
		return nil, err
	}
	from, to := dayRange(date, tz)

	bookings, err := s.repo.ListOccupied(ctx, resourceID, from, to)
	if err != nil {
		return nil, err
	}

	return ComputeAvailability(date, tz, loc, bookings, time.Now())
}

// loadLocationTZ resolves an IANA timezone name to a *time.Location. An empty
// name falls back to UTC. Location timezones are validated at create/update
// time, so a failure here indicates corrupt data and surfaces as an error.
func loadLocationTZ(tz string) (*time.Location, error) {
	if tz == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, ErrInvalidTimezone
	}
	return loc, nil
}

// parseOpeningTime parses an "HH:MM:SS" or "HH:MM" wall-clock string.
func parseOpeningTime(s string) (time.Time, error) {
	layout := "15:04:05"
	if len(s) == 5 {
		layout = "15:04"
	}
	return time.Parse(layout, s)
}

// validateBookingWindow enforces every constraint on an ordinary single booking:
// the occurrence rules (validateOccurrence) plus the location's booking window
// (validateAdvanceWindow).
func validateBookingWindow(loc *location.Location, start, end time.Time) error {
	if err := validateOccurrence(loc, start, end); err != nil {
		return err
	}
	return validateAdvanceWindow(loc, start, time.Now())
}

// validateAdvanceWindow enforces the location's booking window: the start must
// be at least minimum_booking_notice after now and at most
// maximum_booking_advance after now. It is shared in spirit with
// ComputeAvailability, which derives the same bounds from bookingWindowBounds.
// Booking Series do not use it.
func validateAdvanceWindow(loc *location.Location, start, now time.Time) error {
	if start.Before(now) {
		return ErrStartTimePast
	}
	if start.Before(now.Add(noticeOf(loc))) {
		return ErrTooSoon
	}
	if start.After(now.Add(advanceOf(loc))) {
		return ErrTooFarInAdvance
	}
	return nil
}

// validateOccurrence enforces the location's operating constraints on one time
// range, independent of when it is booked:
//   - the location must currently be open for business;
//   - the duration must not exceed MaxBookingDuration;
//   - start and end must align to BookingSlotGranularity;
//   - the range must fall within the daily opening hours, interpreted in the
//     location's timezone (so non-UTC venues are handled correctly).
func validateOccurrence(loc *location.Location, start, end time.Time) error {
	if !loc.Opening {
		return ErrLocationClosed
	}
	if end.Sub(start) > MaxBookingDuration {
		return ErrBookingTooLong
	}

	tz, err := loadLocationTZ(loc.Timezone)
	if err != nil {
		return err
	}

	openT, err := parseOpeningTime(loc.OpeningHoursStart)
	if err != nil {
		return ErrInvalidTimeRange
	}
	closeT, err := parseOpeningTime(loc.OpeningHoursEnd)
	if err != nil {
		return ErrInvalidTimeRange
	}

	startLocal := start.In(tz)
	endLocal := end.In(tz)
	if !isAligned(startLocal) || !isAligned(endLocal) {
		return ErrNotAligned
	}

	// Anchor the opening hours to the booking's local calendar day. A booking
	// must start no earlier than opening and end no later than closing on the
	// same local day; cross-midnight bookings are therefore rejected (matching
	// the single-day opening-hours model enforced on the location).
	y, m, d := startLocal.Date()
	openAt := time.Date(y, m, d, openT.Hour(), openT.Minute(), openT.Second(), 0, tz)
	closeAt := time.Date(y, m, d, closeT.Hour(), closeT.Minute(), closeT.Second(), 0, tz)

	if startLocal.Before(openAt) || endLocal.After(closeAt) {
		return ErrOutsideOpeningHours
	}
	return nil
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
//
// The opening hours are interpreted in the supplied timezone (tz), so the
// computed slots line up with the location's local wall-clock hours rather than
// UTC. Pass time.UTC for UTC behaviour.
func CalculateAvailability(date time.Time, tz *time.Location, openStr, closeStr string, bookings []*Booking) ([]TimeSlot, error) {
	if tz == nil {
		tz = time.UTC
	}

	// 1. Parse Opening and Closing Times
	openTime, err := parseOpeningTime(openStr)
	if err != nil {
		return nil, err
	}
	// Normalizing to the given date, in the location's timezone
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), openTime.Hour(), openTime.Minute(), openTime.Second(), 0, tz)

	closeTime, err := parseOpeningTime(closeStr)
	if err != nil {
		return nil, err
	}
	endOfDay := time.Date(date.Year(), date.Month(), date.Day(), closeTime.Hour(), closeTime.Minute(), closeTime.Second(), 0, tz)

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

// OnUserDeactivated implements user.DeactivationHook: a deactivated user's
// upcoming bookings are cancelled so they stop holding time slots.
func (s *service) OnUserDeactivated(ctx context.Context, userID string) error {
	return s.repo.CancelUpcomingByUser(ctx, userID)
}

// isAligned reports whether t falls exactly on a BookingSlotGranularity
// boundary of its (location-local) wall clock.
func isAligned(t time.Time) bool {
	if t.Second() != 0 || t.Nanosecond() != 0 {
		return false
	}
	return t.Minute()%int(BookingSlotGranularity/time.Minute) == 0
}

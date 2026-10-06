package booking

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/resource"
)

// CreateSeriesRequest describes a seasonal rental: the same daily time slot on
// the chosen weekdays for TermMonths starting at StartDate.
type CreateSeriesRequest struct {
	UserID     string
	ResourceID string
	TermMonths int
	// StartDate is the first calendar day of the term; only its year, month and
	// day are used, interpreted in the location timezone.
	StartDate  time.Time
	Weekdays   []time.Weekday
	StartClock string // "HH:MM" wall clock in the location timezone
	EndClock   string
}

func (s *service) CreateSeries(ctx context.Context, req CreateSeriesRequest) (*BookingSeries, error) {
	if !slices.Contains(SeriesTermMonths, req.TermMonths) {
		return nil, ErrInvalidTermMonths
	}
	if len(req.Weekdays) == 0 {
		return nil, ErrInvalidSeriesInput
	}
	for _, d := range req.Weekdays {
		if d < time.Sunday || d > time.Saturday {
			return nil, ErrInvalidSeriesInput
		}
	}
	startClock, err := parseOpeningTime(req.StartClock)
	if err != nil {
		return nil, ErrInvalidSeriesInput
	}
	endClock, err := parseOpeningTime(req.EndClock)
	if err != nil {
		return nil, ErrInvalidSeriesInput
	}
	if !endClock.After(startClock) {
		return nil, ErrInvalidTimeRange
	}

	res, err := s.resService.GetByID(ctx, req.ResourceID)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			return nil, ErrResourceNotFound
		}
		return nil, err
	}
	loc, err := s.locService.GetByID(ctx, res.LocationID)
	if err != nil {
		return nil, err
	}
	if err := s.orgService.CheckOperation(ctx, loc.OrganizationID, req.UserID); err != nil {
		return nil, err
	}
	tz, err := loadLocationTZ(loc.Timezone)
	if err != nil {
		return nil, err
	}

	occurrences := expandSeries(tz, req, startClock, endClock)
	if len(occurrences) == 0 {
		return nil, ErrSeriesNoOccurrences
	}
	if len(occurrences) > MaxSeriesBookings {
		return nil, ErrSeriesTooManyBookings
	}

	// Series skip the ordinary booking window and the per-user cap, but each
	// occurrence must still be in the future and respect the opening hours.
	now := time.Now()
	for _, o := range occurrences {
		if o.StartTime.Before(now) {
			return nil, ErrStartTimePast
		}
		if err := validateOccurrence(loc, o.StartTime, o.EndTime); err != nil {
			return nil, err
		}
	}

	if err := s.checkSeriesConflicts(ctx, req.ResourceID, occurrences); err != nil {
		return nil, err
	}

	series := &BookingSeries{UserID: req.UserID, ResourceID: req.ResourceID, TermMonths: req.TermMonths}
	bookings := make([]*Booking, len(occurrences))
	for i, o := range occurrences {
		bookings[i] = &Booking{
			ResourceID: req.ResourceID,
			UserID:     req.UserID,
			StartTime:  o.StartTime,
			EndTime:    o.EndTime,
			Status:     StatusPending,
		}
	}
	if err := s.repo.CreateSeries(ctx, series, bookings); err != nil {
		if errors.Is(err, ErrTimeConflict) {
			// Lost a race with a concurrent booking: report which occurrences clash now.
			if cerr := s.checkSeriesConflicts(ctx, req.ResourceID, occurrences); cerr != nil {
				return nil, cerr
			}
		}
		return nil, err
	}

	return s.repo.GetSeriesByID(ctx, series.ID)
}

// checkSeriesConflicts returns a SeriesConflictError listing the occurrences
// that overlap an existing booking of the resource, or nil when none do.
func (s *service) checkSeriesConflicts(ctx context.Context, resourceID string, occurrences []TimeSlot) error {
	first, last := occurrences[0], occurrences[len(occurrences)-1]
	existing, err := s.repo.ListOccupied(ctx, resourceID, first.StartTime, last.EndTime)
	if err != nil {
		return err
	}
	var conflicts []TimeSlot
	for _, o := range occurrences {
		for _, b := range existing {
			if o.StartTime.Before(b.EndTime) && o.EndTime.After(b.StartTime) {
				conflicts = append(conflicts, o)
				break
			}
		}
	}
	if len(conflicts) > 0 {
		return &SeriesConflictError{Conflicts: conflicts}
	}
	return nil
}

// expandSeries lists the concrete occurrences of a series in chronological
// order: every selected weekday in [StartDate, StartDate + TermMonths).
func expandSeries(tz *time.Location, req CreateSeriesRequest, startClock, endClock time.Time) []TimeSlot {
	y, m, d := req.StartDate.Date()
	termEnd := addMonthsClamped(y, m, d, req.TermMonths)

	var out []TimeSlot
	for day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC); day.Before(termEnd); day = day.AddDate(0, 0, 1) {
		if !slices.Contains(req.Weekdays, day.Weekday()) {
			continue
		}
		dy, dm, dd := day.Date()
		out = append(out, TimeSlot{
			StartTime: time.Date(dy, dm, dd, startClock.Hour(), startClock.Minute(), 0, 0, tz),
			EndTime:   time.Date(dy, dm, dd, endClock.Hour(), endClock.Minute(), 0, 0, tz),
		})
	}
	return out
}

// addMonthsClamped adds n months to the date, clamping the day to the target
// month length (31 Aug + 6 months = 28/29 Feb). The result is a UTC midnight
// used only for calendar-date comparison.
func addMonthsClamped(y int, m time.Month, d, n int) time.Time {
	target := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if d > last {
		d = last
	}
	return time.Date(target.Year(), target.Month(), d, 0, 0, 0, 0, time.UTC)
}

func (s *service) GetSeriesForViewer(ctx context.Context, id string, viewerID string, isSysAdmin bool) (*BookingSeries, error) {
	series, err := s.repo.GetSeriesByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(series.Bookings) == 0 {
		return nil, ErrSeriesNotFound
	}
	if _, _, err := s.authorize(ctx, series.Bookings[0], viewerID, isSysAdmin); err != nil {
		return nil, err
	}
	return series, nil
}

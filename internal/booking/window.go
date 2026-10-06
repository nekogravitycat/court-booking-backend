package booking

import (
	"context"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
)

// ResourceAvailability is one resource with its available slots for a date.
type ResourceAvailability struct {
	Resource *resource.Resource
	Slots    []TimeSlot
}

func noticeOf(loc *location.Location) time.Duration {
	return time.Duration(loc.MinimumBookingNoticeMinutes) * time.Minute
}

func advanceOf(loc *location.Location) time.Duration {
	return time.Duration(loc.MaximumBookingAdvanceDays) * 24 * time.Hour
}

// bookingWindowBounds returns the earliest start and the end of the latest
// bookable slot for an ordinary booking made at now. Starts are aligned to
// BookingSlotGranularity in the location's local clock, so the earliest start
// is now + notice rounded up, and the latest slot is the one that starts at
// now + advance rounded down (it ends one granule later).
func bookingWindowBounds(loc *location.Location, tz *time.Location, now time.Time) (earliest, latestEnd time.Time) {
	earliest = ceilToGranularity(now.Add(noticeOf(loc)), tz)
	latestEnd = floorToGranularity(now.Add(advanceOf(loc)), tz).Add(BookingSlotGranularity)
	return earliest, latestEnd
}

func floorToGranularity(t time.Time, tz *time.Location) time.Time {
	l := t.In(tz)
	step := int(BookingSlotGranularity / time.Minute)
	return time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute()/step*step, 0, 0, tz)
}

func ceilToGranularity(t time.Time, tz *time.Location) time.Time {
	f := floorToGranularity(t, tz)
	if f.Before(t) {
		return f.Add(BookingSlotGranularity)
	}
	return f
}

// dayRange returns the [start, end) instants of the calendar day of date in tz.
func dayRange(date time.Time, tz *time.Location) (time.Time, time.Time) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, tz)
	return start, start.Add(24 * time.Hour)
}

// ComputeAvailability is the single availability calculation shared by the
// resource and location availability APIs: opening hours and occupied bookings
// (CalculateAvailability) restricted to the location's booking window.
func ComputeAvailability(date time.Time, tz *time.Location, loc *location.Location, bookings []*Booking, now time.Time) ([]TimeSlot, error) {
	slots, err := CalculateAvailability(date, tz, loc.OpeningHoursStart, loc.OpeningHoursEnd, bookings)
	if err != nil {
		return nil, err
	}
	earliest, latestEnd := bookingWindowBounds(loc, tz, now)
	return clipSlots(slots, earliest, latestEnd), nil
}

// clipSlots restricts slots to [from, to], dropping those left empty.
func clipSlots(slots []TimeSlot, from, to time.Time) []TimeSlot {
	var out []TimeSlot
	for _, s := range slots {
		if s.StartTime.Before(from) {
			s.StartTime = from
		}
		if s.EndTime.After(to) {
			s.EndTime = to
		}
		if s.StartTime.Before(s.EndTime) {
			out = append(out, s)
		}
	}
	return out
}

func (s *service) GetLocationAvailability(ctx context.Context, locationID string, date time.Time) ([]ResourceAvailability, error) {
	loc, err := s.locService.GetByID(ctx, locationID)
	if err != nil {
		return nil, err
	}
	tz, err := loadLocationTZ(loc.Timezone)
	if err != nil {
		return nil, err
	}
	resources, err := s.resService.ListByLocation(ctx, locationID)
	if err != nil {
		return nil, err
	}

	from, to := dayRange(date, tz)
	byResource, err := s.repo.ListOccupiedByLocation(ctx, locationID, from, to)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	out := make([]ResourceAvailability, 0, len(resources))
	for _, res := range resources {
		slots, err := ComputeAvailability(date, tz, loc, byResource[res.ID], now)
		if err != nil {
			return nil, err
		}
		out = append(out, ResourceAvailability{Resource: res, Slots: slots})
	}
	return out, nil
}

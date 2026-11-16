package course

import (
	"fmt"
	"sort"
	"time"
)

// TimeRange is one class meeting's start and end.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// Schedule describes a recurring weekly pattern. Weekdays follow Go / JavaScript
// numbering (0 = Sunday ... 6 = Saturday). Times are wall-clock "HH:MM" in the
// course time zone, so a 19:00 class stays at 19:00 across daylight saving.
type Schedule struct {
	StartDate string   // YYYY-MM-DD, inclusive
	EndDate   string   // YYYY-MM-DD, inclusive
	Weekdays  []int    // At least one
	StartTime string   // HH:MM
	EndTime   string   // HH:MM, after StartTime on the same day
	SkipDates []string // YYYY-MM-DD dates to leave out (holidays)
}

// ExpandSchedule turns a weekly pattern into concrete meetings in loc, ordered
// by start time. It does not check against the current time.
func ExpandSchedule(s Schedule, loc *time.Location) ([]TimeRange, error) {
	start, err := time.ParseInLocation("2006-01-02", s.StartDate, loc)
	if err != nil {
		return nil, ErrInvalidSchedule
	}
	end, err := time.ParseInLocation("2006-01-02", s.EndDate, loc)
	if err != nil || end.Before(start) {
		return nil, ErrInvalidSchedule
	}
	if end.Sub(start) > MaxScheduleDays*24*time.Hour {
		return nil, ErrInvalidSchedule
	}
	sh, sm, err := parseClock(s.StartTime)
	if err != nil {
		return nil, err
	}
	eh, em, err := parseClock(s.EndTime)
	if err != nil {
		return nil, err
	}
	if eh*60+em <= sh*60+sm {
		return nil, ErrInvalidSchedule
	}
	if len(s.Weekdays) == 0 {
		return nil, ErrInvalidSchedule
	}
	days := make(map[time.Weekday]bool, len(s.Weekdays))
	for _, d := range s.Weekdays {
		if d < 0 || d > 6 {
			return nil, ErrInvalidSchedule
		}
		days[time.Weekday(d)] = true
	}
	skip := make(map[string]bool, len(s.SkipDates))
	for _, d := range s.SkipDates {
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return nil, ErrInvalidSchedule
		}
		skip[d] = true
	}

	var out []TimeRange
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if !days[d.Weekday()] || skip[d.Format("2006-01-02")] {
			continue
		}
		y, m, day := d.Date()
		out = append(out, TimeRange{
			Start: time.Date(y, m, day, sh, sm, 0, 0, loc),
			End:   time.Date(y, m, day, eh, em, 0, 0, loc),
		})
		if len(out) > MaxSessions {
			return nil, ErrTooManySessions
		}
	}
	if len(out) == 0 {
		return nil, ErrInvalidSchedule
	}
	return out, nil
}

func parseClock(s string) (int, int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, ErrInvalidSchedule
	}
	return t.Hour(), t.Minute(), nil
}

// validateRanges sorts the meetings and checks count, ordering, overlap, and
// (when now is non-zero) that every meeting starts in the future.
func validateRanges(rs []TimeRange, now time.Time) ([]TimeRange, error) {
	if len(rs) == 0 {
		return nil, ErrScheduleRequired
	}
	if len(rs) > MaxSessions {
		return nil, ErrTooManySessions
	}
	out := append([]TimeRange(nil), rs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	for i, r := range out {
		if !r.End.After(r.Start) {
			return nil, fmt.Errorf("%w", ErrInvalidSchedule)
		}
		if !now.IsZero() && !r.Start.After(now) {
			return nil, ErrSessionInPast
		}
		if i > 0 && r.Start.Before(out[i-1].End) {
			return nil, ErrSessionsOverlap
		}
	}
	return out, nil
}

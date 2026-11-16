package course

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandSchedule(t *testing.T) {
	taipei, _ := time.LoadLocation("Asia/Taipei")

	t.Run("weekly pattern with skipped date", func(t *testing.T) {
		got, err := ExpandSchedule(Schedule{
			StartDate: "2027-01-04", EndDate: "2027-01-24", Weekdays: []int{1, 3},
			StartTime: "19:00", EndTime: "21:00", SkipDates: []string{"2027-01-13"},
		}, taipei)
		require.NoError(t, err)
		// Mondays 4, 11, 18 and Wednesdays 6, 20 (13 skipped).
		require.Len(t, got, 5)
		assert.Equal(t, "2027-01-04 19:00", got[0].Start.Format("2006-01-02 15:04"))
		assert.Equal(t, "2027-01-06 21:00", got[1].End.Format("2006-01-02 15:04"))
		assert.Equal(t, "2027-01-20", got[4].Start.Format("2006-01-02"))
	})

	t.Run("wall clock is kept across daylight saving", func(t *testing.T) {
		ny, err := time.LoadLocation("America/New_York")
		require.NoError(t, err)
		got, err := ExpandSchedule(Schedule{
			StartDate: "2027-03-08", EndDate: "2027-03-15", Weekdays: []int{1},
			StartTime: "19:00", EndTime: "20:00",
		}, ny)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, 19, got[0].Start.Hour())
		assert.Equal(t, 19, got[1].Start.Hour())
		assert.Equal(t, 6*24*time.Hour+23*time.Hour, got[1].Start.Sub(got[0].Start)-0, "one calendar week minus the DST hour")
	})

	for name, s := range map[string]Schedule{
		"bad start date":  {StartDate: "x", EndDate: "2027-01-10", Weekdays: []int{1}, StartTime: "10:00", EndTime: "11:00"},
		"end before":      {StartDate: "2027-01-10", EndDate: "2027-01-01", Weekdays: []int{1}, StartTime: "10:00", EndTime: "11:00"},
		"no weekdays":     {StartDate: "2027-01-01", EndDate: "2027-01-10", StartTime: "10:00", EndTime: "11:00"},
		"weekday range":   {StartDate: "2027-01-01", EndDate: "2027-01-10", Weekdays: []int{7}, StartTime: "10:00", EndTime: "11:00"},
		"end before time": {StartDate: "2027-01-01", EndDate: "2027-01-10", Weekdays: []int{1}, StartTime: "11:00", EndTime: "10:00"},
		"bad clock":       {StartDate: "2027-01-01", EndDate: "2027-01-10", Weekdays: []int{1}, StartTime: "25:00", EndTime: "26:00"},
		"bad skip":        {StartDate: "2027-01-01", EndDate: "2027-01-10", Weekdays: []int{1}, StartTime: "10:00", EndTime: "11:00", SkipDates: []string{"nope"}},
		"too long":        {StartDate: "2027-01-01", EndDate: "2029-01-01", Weekdays: []int{1}, StartTime: "10:00", EndTime: "11:00"},
		"no matching day": {StartDate: "2027-01-04", EndDate: "2027-01-04", Weekdays: []int{3}, StartTime: "10:00", EndTime: "11:00"},
	} {
		_, err := ExpandSchedule(s, taipei)
		assert.Error(t, err, name)
	}

	t.Run("too many sessions", func(t *testing.T) {
		_, err := ExpandSchedule(Schedule{
			StartDate: "2027-01-01", EndDate: "2027-12-31", Weekdays: []int{0, 1, 2, 3, 4, 5, 6},
			StartTime: "10:00", EndTime: "11:00",
		}, taipei)
		assert.ErrorIs(t, err, ErrTooManySessions)
	})
}

func TestValidateRanges(t *testing.T) {
	base := time.Date(2027, 1, 1, 10, 0, 0, 0, time.UTC)
	now := base.Add(-time.Hour)
	r := func(h int) TimeRange {
		return TimeRange{base.Add(time.Duration(h) * time.Hour), base.Add(time.Duration(h+1) * time.Hour)}
	}

	got, err := validateRanges([]TimeRange{r(5), r(0)}, now)
	require.NoError(t, err)
	assert.True(t, got[0].Start.Before(got[1].Start), "sorted")

	_, err = validateRanges(nil, now)
	assert.ErrorIs(t, err, ErrScheduleRequired)
	_, err = validateRanges([]TimeRange{r(0), {r(0).Start.Add(30 * time.Minute), r(0).End.Add(time.Hour)}}, now)
	assert.ErrorIs(t, err, ErrSessionsOverlap)
	_, err = validateRanges([]TimeRange{r(0)}, base.Add(time.Minute))
	assert.ErrorIs(t, err, ErrSessionInPast)
	_, err = validateRanges([]TimeRange{{base, base}}, now)
	assert.Error(t, err)
}

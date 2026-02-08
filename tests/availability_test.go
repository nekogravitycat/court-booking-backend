package tests

import (
	"reflect"
	"testing"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/booking"
)

func TestCalculateAvailability(t *testing.T) {
	// Base date for testing: 2026-02-08
	baseDate := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		date     time.Time
		openStr  string
		closeStr string
		bookings []*booking.Booking
		want     []booking.TimeSlot
		wantErr  bool
	}{
		{
			name:     "No bookings, full day available",
			date:     baseDate,
			openStr:  "09:00:00",
			closeStr: "18:00:00",
			bookings: []*booking.Booking{},
			want: []booking.TimeSlot{
				{
					StartTime: time.Date(2026, 2, 8, 9, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 18, 0, 0, 0, time.UTC),
				},
			},
			wantErr: false,
		},
		{
			name:     "One booking in the middle",
			date:     baseDate,
			openStr:  "09:00",
			closeStr: "18:00",
			bookings: []*booking.Booking{
				{
					StartTime: time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 13, 0, 0, 0, time.UTC),
					Status:    booking.StatusConfirmed,
				},
			},
			want: []booking.TimeSlot{
				{
					StartTime: time.Date(2026, 2, 8, 9, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC),
				},
				{
					StartTime: time.Date(2026, 2, 8, 13, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 18, 0, 0, 0, time.UTC),
				},
			},
			wantErr: false,
		},
		{
			name:     "Booking pending is considered confirmed (unavailable)",
			date:     baseDate,
			openStr:  "09:00",
			closeStr: "18:00",
			bookings: []*booking.Booking{
				{
					StartTime: time.Date(2026, 2, 8, 10, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 11, 0, 0, 0, time.UTC),
					Status:    booking.StatusPending,
				},
			},
			want: []booking.TimeSlot{
				{
					StartTime: time.Date(2026, 2, 8, 9, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 10, 0, 0, 0, time.UTC),
				},
				{
					StartTime: time.Date(2026, 2, 8, 11, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 18, 0, 0, 0, time.UTC),
				},
			},
			wantErr: false,
		},
		{
			name:     "Cancelled booking is ignored",
			date:     baseDate,
			openStr:  "09:00",
			closeStr: "18:00",
			bookings: []*booking.Booking{
				{
					StartTime: time.Date(2026, 2, 8, 10, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 11, 0, 0, 0, time.UTC),
					Status:    booking.StatusCancelled,
				},
			},
			want: []booking.TimeSlot{
				{
					StartTime: time.Date(2026, 2, 8, 9, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 18, 0, 0, 0, time.UTC),
				},
			},
			wantErr: false,
		},
		{
			name:     "Booking covers entire day",
			date:     baseDate,
			openStr:  "09:00",
			closeStr: "18:00",
			bookings: []*booking.Booking{
				{
					StartTime: time.Date(2026, 2, 8, 9, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 18, 0, 0, 0, time.UTC),
					Status:    booking.StatusConfirmed,
				},
			},
			want:    nil, // Empty slice or nil depending on implementation, let's allow nil
			wantErr: false,
		},
		{
			name:     "Overlapping / Unsorted bookings",
			date:     baseDate,
			openStr:  "09:00",
			closeStr: "18:00",
			bookings: []*booking.Booking{
				{
					StartTime: time.Date(2026, 2, 8, 14, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 16, 0, 0, 0, time.UTC),
					Status:    booking.StatusConfirmed,
				},
				{
					StartTime: time.Date(2026, 2, 8, 10, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC),
					Status:    booking.StatusConfirmed,
				},
			},
			want: []booking.TimeSlot{
				{
					StartTime: time.Date(2026, 2, 8, 9, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 10, 0, 0, 0, time.UTC),
				},
				{
					StartTime: time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 14, 0, 0, 0, time.UTC),
				},
				{
					StartTime: time.Date(2026, 2, 8, 16, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 2, 8, 18, 0, 0, 0, time.UTC),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := booking.CalculateAvailability(tt.date, tt.openStr, tt.closeStr, tt.bookings)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateAvailability() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CalculateAvailability() = %v, want %v", got, tt.want)
			}
		})
	}
}

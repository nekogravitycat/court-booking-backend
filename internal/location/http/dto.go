package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/location"
	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type LocationResponse struct {
	ID                string                  `json:"id"`
	Organization      orgHttp.OrganizationTag `json:"organization"`
	Name              string                  `json:"name"`
	CreatedAt         time.Time               `json:"created_at"`
	Capacity          int64                   `json:"capacity"`
	OpeningHoursStart string                  `json:"opening_hours_start"`
	OpeningHoursEnd   string                  `json:"opening_hours_end"`
	LocationInfo      string                  `json:"location_info"`
	Opening           bool                    `json:"opening"`
	Rule              string                  `json:"rule"`
	Facility          string                  `json:"facility"`
	Description       string                  `json:"description"`
	Longitude         float64                 `json:"longitude"`
	Latitude          float64                 `json:"latitude"`
}

// LocationTag is a brief representation of a location.
type LocationTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewLocationResponse(l *location.Location) LocationResponse {
	return LocationResponse{
		ID:                l.ID,
		Organization:      orgHttp.OrganizationTag{ID: l.OrganizationID, Name: l.OrganizationName},
		Name:              l.Name,
		CreatedAt:         l.CreatedAt,
		Capacity:          l.Capacity,
		OpeningHoursStart: l.OpeningHoursStart,
		OpeningHoursEnd:   l.OpeningHoursEnd,
		LocationInfo:      l.LocationInfo,
		Opening:           l.Opening,
		Rule:              l.Rule,
		Facility:          l.Facility,
		Description:       l.Description,
		Longitude:         l.Longitude,
		Latitude:          l.Latitude,
	}
}

type ManagerResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName *string   `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
	IsActive    bool      `json:"is_active"`
}

func NewManagerResponse(u *user.User) ManagerResponse {
	return ManagerResponse{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		CreatedAt:   u.CreatedAt,
		IsActive:    u.IsActive,
	}
}

type CreateLocationRequest struct {
	OrganizationID    string  `json:"organization_id" binding:"required,uuid"`
	Name              string  `json:"name" binding:"required"`
	Capacity          int64   `json:"capacity" binding:"required"`
	OpeningHoursStart string  `json:"opening_hours_start" binding:"required"`
	OpeningHoursEnd   string  `json:"opening_hours_end" binding:"required"`
	LocationInfo      string  `json:"location_info" binding:"required"`
	Opening           bool    `json:"opening"`
	Rule              string  `json:"rule"`
	Facility          string  `json:"facility"`
	Description       string  `json:"description"`
	Longitude         float64 `json:"longitude" binding:"required,min=-180,max=180"`
	Latitude          float64 `json:"latitude" binding:"required,min=-90,max=90"`
}

type UpdateLocationRequest struct {
	Name              *string  `json:"name"`
	Capacity          *int64   `json:"capacity"`
	OpeningHoursStart *string  `json:"opening_hours_start"`
	OpeningHoursEnd   *string  `json:"opening_hours_end"`
	LocationInfo      *string  `json:"location_info"`
	Opening           *bool    `json:"opening"`
	Rule              *string  `json:"rule"`
	Facility          *string  `json:"facility"`
	Description       *string  `json:"description"`
	Longitude         *float64 `json:"longitude" binding:"omitempty,min=-180,max=180"`
	Latitude          *float64 `json:"latitude" binding:"omitempty,min=-90,max=90"`
}

type ListLocationsRequest struct {
	request.ListParams
	OrganizationID string `form:"organization_id" binding:"omitempty,uuid"`

	// Filters
	Name                 string `form:"name"`
	Opening              *bool  `form:"opening"`
	CapacityMin          *int64 `form:"capacity_min"`
	CapacityMax          *int64 `form:"capacity_max"`
	OpeningHoursStartMin string `form:"opening_hours_start_min"`
	OpeningHoursStartMax string `form:"opening_hours_start_max"`
	OpeningHoursEndMin   string `form:"opening_hours_end_min"`
	OpeningHoursEndMax   string `form:"opening_hours_end_max"`
	CreatedAtFrom        string `form:"created_at_from" binding:"omitempty,datetime=2006-01-02T15:04:05Z07:00"`
	CreatedAtTo          string `form:"created_at_to" binding:"omitempty,datetime=2006-01-02T15:04:05Z07:00"`

	// Sorting
	SortBy string `form:"sort_by" binding:"omitempty,oneof=capacity opening_hours_start opening_hours_end created_at"`
}

func (r *ListLocationsRequest) Validate() error {
	// 1. Capacity Range
	if r.CapacityMin != nil && r.CapacityMax != nil {
		if *r.CapacityMin > *r.CapacityMax {
			return location.ErrCapacityInvalid
		}
	}

	// 2. Opening Hours Range
	if r.OpeningHoursStartMin != "" {
		if _, err := time.Parse(time.TimeOnly, r.OpeningHoursStartMin); err != nil {
			return location.ErrInvalidOpeningHours
		}
	}
	if r.OpeningHoursStartMax != "" {
		if _, err := time.Parse(time.TimeOnly, r.OpeningHoursStartMax); err != nil {
			return location.ErrInvalidOpeningHours
		}
	}
	if r.OpeningHoursStartMin != "" && r.OpeningHoursStartMax != "" {
		if r.OpeningHoursStartMin > r.OpeningHoursStartMax {
			return location.ErrInvalidOpeningHours
		}
	}

	if r.OpeningHoursEndMin != "" {
		if _, err := time.Parse(time.TimeOnly, r.OpeningHoursEndMin); err != nil {
			return location.ErrInvalidOpeningHours
		}
	}
	if r.OpeningHoursEndMax != "" {
		if _, err := time.Parse(time.TimeOnly, r.OpeningHoursEndMax); err != nil {
			return location.ErrInvalidOpeningHours
		}
	}
	if r.OpeningHoursEndMin != "" && r.OpeningHoursEndMax != "" {
		if r.OpeningHoursEndMin > r.OpeningHoursEndMax {
			return location.ErrInvalidOpeningHours
		}
	}

	// 3. CreatedAt Range
	if r.CreatedAtFrom != "" && r.CreatedAtTo != "" {
		t1, _ := time.Parse(time.RFC3339, r.CreatedAtFrom)
		t2, _ := time.Parse(time.RFC3339, r.CreatedAtTo)
		if t1.After(t2) {
			return location.ErrInvalidTimeRange
		}
	}

	return nil
}

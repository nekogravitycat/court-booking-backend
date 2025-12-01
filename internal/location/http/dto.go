package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/location"
)

type LocationResponse struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"organization_id"`
	Name              string    `json:"name"`
	CreatedAt         time.Time `json:"created_at"`
	Capacity          int64     `json:"capacity"`
	OpeningHoursStart string    `json:"opening_hours_start"`
	OpeningHoursEnd   string    `json:"opening_hours_end"`
	LocationInfo      string    `json:"location_info"`
	Opening           bool      `json:"opening"`
	Rule              string    `json:"rule"`
	Facility          string    `json:"facility"`
	Description       string    `json:"description"`
	Longitude         float64   `json:"longitude"`
	Latitude          float64   `json:"latitude"`
}

func NewLocationResponse(l *location.Location) LocationResponse {
	return LocationResponse{
		ID:                l.ID,
		OrganizationID:    l.OrganizationID,
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
	Page           int    `form:"page,default=1" binding:"min=1"`
	PageSize       int    `form:"page_size,default=20" binding:"min=1,max=100"`
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
	SortBy    string `form:"sort"`
	SortOrder string `form:"sort_order"` // Optional, or inferred from sort param if using "-field" syntax
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

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

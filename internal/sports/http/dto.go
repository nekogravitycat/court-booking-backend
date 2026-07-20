package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/sports"
)

// ListSportsRequest defines query parameters for listing sports.
type ListSportsRequest struct {
	request.ListParams
	ActiveOnly bool   `form:"active_only"`
	SortBy     string `form:"sort_by" binding:"omitempty,oneof=code name created_at"`
}

// Validate performs custom validation for ListSportsRequest.
func (r *ListSportsRequest) Validate() error { return nil }

type CreateSportBody struct {
	Code string `json:"code" binding:"required,min=1,max=50"`
	Name string `json:"name" binding:"required,min=1,max=100"`
}

// Validate performs custom validation for CreateSportBody.
func (r *CreateSportBody) Validate() error { return nil }

type UpdateSportBody struct {
	Code     *string `json:"code" binding:"omitempty,min=1,max=50"`
	Name     *string `json:"name" binding:"omitempty,min=1,max=100"`
	IsActive *bool   `json:"is_active"`
}

// Validate performs custom validation for UpdateSportBody.
func (r *UpdateSportBody) Validate() error { return nil }

// SportResponse is the full representation of a sport.
type SportResponse struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewSportResponse(s *sports.Sport) SportResponse {
	return SportResponse{
		ID:        s.ID,
		Code:      s.Code,
		Name:      s.Name,
		IsActive:  s.IsActive,
		CreatedAt: s.CreatedAt.UTC(),
		UpdatedAt: s.UpdatedAt.UTC(),
	}
}

// SportTag is a brief representation of a sport, used when embedding into other
// responses (e.g. pickup groups).
type SportTag struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

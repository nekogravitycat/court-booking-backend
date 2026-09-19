package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/skilllevel"
)

// ListSkillLevelsRequest defines query parameters for listing skill levels.
type ListSkillLevelsRequest struct {
	request.ListParams
	SportID    string `form:"sport_id" binding:"omitempty,uuid"`
	ActiveOnly bool   `form:"active_only"`
	SortBy     string `form:"sort_by" binding:"omitempty,oneof=level label created_at"`
}

// Validate performs custom validation for ListSkillLevelsRequest.
func (r *ListSkillLevelsRequest) Validate() error { return nil }

type CreateSkillLevelBody struct {
	SportID string `json:"sport_id" binding:"required,uuid"`
	Level   int    `json:"level" binding:"required,min=1,max=100"`
	Label   string `json:"label" binding:"required,min=1,max=100"`
}

// Validate performs custom validation for CreateSkillLevelBody.
func (r *CreateSkillLevelBody) Validate() error { return nil }

type UpdateSkillLevelBody struct {
	Label    *string `json:"label" binding:"omitempty,min=1,max=100"`
	IsActive *bool   `json:"is_active"`
}

// Validate performs custom validation for UpdateSkillLevelBody.
func (r *UpdateSkillLevelBody) Validate() error { return nil }

// SkillLevelResponse is the full representation of a skill level.
type SkillLevelResponse struct {
	ID        string    `json:"id"`
	SportID   string    `json:"sport_id"`
	Level     int       `json:"level"`
	Label     string    `json:"label"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewSkillLevelResponse(s *skilllevel.SkillLevel) SkillLevelResponse {
	return SkillLevelResponse{
		ID:        s.ID,
		SportID:   s.SportID,
		Level:     s.Level,
		Label:     s.Label,
		IsActive:  s.IsActive,
		CreatedAt: s.CreatedAt.UTC(),
		UpdatedAt: s.UpdatedAt.UTC(),
	}
}

// SkillLevelTag is a brief representation of a skill level, used when embedding
// into other responses (e.g. pickup groups).
type SkillLevelTag struct {
	Level int    `json:"level"`
	Label string `json:"label"`
}

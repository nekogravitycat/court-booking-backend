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
	SortBy     string `form:"sort_by" binding:"omitempty,oneof=sort_order name created_at"`
}

// Validate performs custom validation for ListSkillLevelsRequest.
func (r *ListSkillLevelsRequest) Validate() error { return nil }

type CreateSkillLevelBody struct {
	SportID   string `json:"sport_id" binding:"required,uuid"`
	Name      string `json:"name" binding:"required,min=1,max=100"`
	SortOrder int    `json:"sort_order"`
}

// Validate performs custom validation for CreateSkillLevelBody.
func (r *CreateSkillLevelBody) Validate() error { return nil }

type UpdateSkillLevelBody struct {
	Name      *string `json:"name" binding:"omitempty,min=1,max=100"`
	SortOrder *int    `json:"sort_order"`
	IsActive  *bool   `json:"is_active"`
}

// Validate performs custom validation for UpdateSkillLevelBody.
func (r *UpdateSkillLevelBody) Validate() error { return nil }

// SkillLevelResponse is the full representation of a skill level.
type SkillLevelResponse struct {
	ID        string    `json:"id"`
	SportID   string    `json:"sport_id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewSkillLevelResponse(s *skilllevel.SkillLevel) SkillLevelResponse {
	return SkillLevelResponse{
		ID:        s.ID,
		SportID:   s.SportID,
		Name:      s.Name,
		SortOrder: s.SortOrder,
		IsActive:  s.IsActive,
		CreatedAt: s.CreatedAt.UTC(),
		UpdatedAt: s.UpdatedAt.UTC(),
	}
}

// SkillLevelTag is a brief representation of a skill level, used when embedding
// into other responses (e.g. pickup groups).
type SkillLevelTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

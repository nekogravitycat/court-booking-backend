package http

import (
	"math"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/skillrating"
	sportsHttp "github.com/nekogravitycat/court-booking-backend/internal/sports/http"
)

// RatingURI binds the group and participant path parameters.
type RatingURI struct {
	GroupID string `uri:"id" binding:"required,uuid"`
	UserID  string `uri:"user_id" binding:"required,uuid"`
}

// RateBody is the body of PUT /pickup-groups/{id}/ratings/{user_id}.
type RateBody struct {
	SkillLevel int `json:"skill_level" binding:"required,min=1,max=100"`
}

type RatingResponse struct {
	ID            string    `json:"id"`
	PickupGroupID string    `json:"pickup_group_id"`
	UserID        string    `json:"user_id"`
	SkillLevel    int       `json:"skill_level"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NewRatingResponse(r *skillrating.Rating) RatingResponse {
	return RatingResponse{
		ID:            r.ID,
		PickupGroupID: r.PickupGroupID,
		UserID:        r.UserID,
		SkillLevel:    r.Level,
		CreatedAt:     r.CreatedAt.UTC(),
		UpdatedAt:     r.UpdatedAt.UTC(),
	}
}

// SkillSummaryResponse is a user's composite rating in one sport.
type SkillSummaryResponse struct {
	Sport       sportsHttp.SportTag `json:"sport"`
	Average     float64             `json:"average"`      // Arithmetic mean, two decimals
	RatingCount int                 `json:"rating_count"` // Number of ratings behind the average
	Level       int                 `json:"level"`        // Average rounded to the nearest level
	Label       string              `json:"label"`        // Label of that level in the sport scale
}

func NewSkillSummaryResponse(s *skillrating.SportSummary) SkillSummaryResponse {
	return SkillSummaryResponse{
		Sport:       sportsHttp.SportTag{ID: s.SportID, Code: s.SportCode, Name: s.SportName},
		Average:     math.Round(s.Average*100) / 100,
		RatingCount: s.RatingCount,
		Level:       s.Level,
		Label:       s.Label,
	}
}

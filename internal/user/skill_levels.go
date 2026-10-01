package user

import (
	"context"
	"errors"

	"github.com/nekogravitycat/court-booking-backend/internal/skilllevel"
)

func (s *service) GetSkillLevel(ctx context.Context, userID, sportID string) (*SportSkillLevel, error) {
	return s.repo.GetSkillLevel(ctx, userID, sportID)
}

func (s *service) ListSkillLevels(ctx context.Context, userID string) ([]*SportSkillLevel, error) {
	return s.repo.ListSkillLevels(ctx, userID)
}

// SetSkillLevel validates against the active sport scale before replacing the
// account's declaration. Host ratings are stored separately and are unaffected.
func (s *service) SetSkillLevel(ctx context.Context, userID, sportID string, level int) (*SportSkillLevel, error) {
	if level < 1 || level > 100 {
		return nil, ErrInvalidSkillLevel
	}
	sport, err := s.sportsService.GetByID(ctx, sportID)
	if err != nil {
		return nil, err
	}
	if !sport.IsActive {
		return nil, ErrSportInactive
	}
	sl, err := s.skillLevelService.GetBySportAndLevel(ctx, sportID, level)
	if err != nil {
		// An undefined level is invalid input, rather than a missing profile.
		if errors.Is(err, skilllevel.ErrNotFound) {
			return nil, ErrInvalidSkillLevel
		}
		return nil, err
	}
	if !sl.IsActive {
		return nil, ErrInvalidSkillLevel
	}
	return s.repo.SetSkillLevel(ctx, userID, sportID, level)
}

func (s *service) DeleteSkillLevel(ctx context.Context, userID, sportID string) error {
	return s.repo.DeleteSkillLevel(ctx, userID, sportID)
}

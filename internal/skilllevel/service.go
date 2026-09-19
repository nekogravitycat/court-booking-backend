package skilllevel

import (
	"context"
	"strings"
)

type CreateRequest struct {
	SportID string
	Level   int
	Label   string
}

type UpdateRequest struct {
	Label    *string
	IsActive *bool
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*SkillLevel, error)
	GetByID(ctx context.Context, id string) (*SkillLevel, error)
	// GetBySportAndLevel returns the mapping row for a sport's integer level, or
	// ErrNotFound when the sport has no such level.
	GetBySportAndLevel(ctx context.Context, sportID string, level int) (*SkillLevel, error)
	// LabelsBySport returns level -> label for every level of the sport
	// (including inactive ones, so historical data still resolves a label).
	LabelsBySport(ctx context.Context, sportID string) (map[int]string, error)
	List(ctx context.Context, filter Filter) ([]*SkillLevel, int, error)
	Update(ctx context.Context, id string, req UpdateRequest) (*SkillLevel, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*SkillLevel, error) {
	if strings.TrimSpace(req.SportID) == "" {
		return nil, ErrSportRequired
	}
	if req.Level < 1 {
		return nil, ErrLevelInvalid
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		return nil, ErrLabelRequired
	}

	sl := &SkillLevel{
		SportID:  req.SportID,
		Level:    req.Level,
		Label:    label,
		IsActive: true,
	}
	if err := s.repo.Create(ctx, sl); err != nil {
		return nil, err
	}
	return sl, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*SkillLevel, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) GetBySportAndLevel(ctx context.Context, sportID string, level int) (*SkillLevel, error) {
	return s.repo.GetBySportAndLevel(ctx, sportID, level)
}

func (s *service) LabelsBySport(ctx context.Context, sportID string) (map[int]string, error) {
	return s.repo.LabelsBySport(ctx, sportID)
}

func (s *service) List(ctx context.Context, filter Filter) ([]*SkillLevel, int, error) {
	return s.repo.List(ctx, filter)
}

// Update mutates a skill level's label and active flag. The owning sport and the
// integer level are fixed at creation time: changing either would silently
// change the meaning of data already recorded against that level.
func (s *service) Update(ctx context.Context, id string, req UpdateRequest) (*SkillLevel, error) {
	sl, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Label != nil {
		label := strings.TrimSpace(*req.Label)
		if label == "" {
			return nil, ErrLabelRequired
		}
		sl.Label = label
	}
	if req.IsActive != nil {
		sl.IsActive = *req.IsActive
	}

	if err := s.repo.Update(ctx, sl); err != nil {
		return nil, err
	}
	return sl, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

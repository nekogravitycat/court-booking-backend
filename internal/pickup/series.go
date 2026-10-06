package pickup

import (
	"context"
	"time"
)

// GroupOccurrence is one concrete session of a group series.
type GroupOccurrence struct {
	StartTime time.Time
	EndTime   time.Time
}

// CreateGroupSeriesRequest creates one independent group per occurrence. The
// caller (the frontend) expands any recurrence rule; the backend only validates
// and stores the explicit occurrences.
type CreateGroupSeriesRequest struct {
	HostID        string
	Title         string
	Description   *string
	Social        *string
	LocationID    string
	SportID       string
	Fee           int
	Capacity      int
	MinSkillLevel int
	MaxSkillLevel *int
	Enable        bool
	// RegistrationDeadlineMinutesBeforeStart is applied to every occurrence:
	// registration_deadline = start_time - this many minutes.
	RegistrationDeadlineMinutesBeforeStart int
	Occurrences                            []GroupOccurrence
}

func (s *service) CreateGroupSeries(ctx context.Context, req CreateGroupSeriesRequest) (*GroupSeries, error) {
	if len(req.Occurrences) == 0 {
		return nil, ErrSeriesNoOccurrences
	}
	if len(req.Occurrences) > MaxPickupGroupsPerSeries {
		return nil, ErrSeriesTooManyOccurrences
	}
	if req.RegistrationDeadlineMinutesBeforeStart < 0 {
		return nil, ErrInvalidRegistrationDeadline
	}
	social, err := normalizeSocial(req.Social)
	if err != nil {
		return nil, err
	}

	// Every occurrence is validated with the single-group rules before anything is stored.
	now := time.Now()
	horizon := now.Add(MaxPickupSeriesHorizon)
	offset := time.Duration(req.RegistrationDeadlineMinutesBeforeStart) * time.Minute
	groups := make([]*PickupGroup, len(req.Occurrences))
	for i, o := range req.Occurrences {
		if !o.EndTime.After(o.StartTime) {
			return nil, ErrInvalidTimeRange
		}
		if o.StartTime.Before(now) || o.StartTime.After(horizon) {
			return nil, ErrOccurrenceOutsideHorizon
		}
		deadline := o.StartTime.Add(-offset)
		if deadline.Before(now) || deadline.After(o.StartTime) {
			return nil, ErrInvalidRegistrationDeadline
		}
		groups[i] = &PickupGroup{
			HostID:               req.HostID,
			Title:                req.Title,
			Description:          req.Description,
			Social:               social,
			StartTime:            o.StartTime,
			RegistrationDeadline: deadline,
			EndTime:              o.EndTime,
			Fee:                  req.Fee,
			Capacity:             req.Capacity,
			LocationID:           req.LocationID,
			SportID:              req.SportID,
			MinSkillLevel:        req.MinSkillLevel,
			MaxSkillLevel:        req.MaxSkillLevel,
			Status:               GroupStatusActive,
			Enable:               req.Enable,
		}
	}

	if err := s.validateSportAndSkillRange(ctx, req.SportID, req.MinSkillLevel, req.MaxSkillLevel); err != nil {
		return nil, err
	}

	return s.repo.CreateGroupSeries(ctx, req.HostID, groups)
}

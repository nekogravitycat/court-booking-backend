package skillrating

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/notification"
	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	"github.com/nekogravitycat/court-booking-backend/internal/skilllevel"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type Service interface {
	// Rate records (or overwrites) the host's rating of one participant.
	Rate(ctx context.Context, groupID, userID string, level int, raterID string, raterIsAdmin bool) (*Rating, error)
	// Remove deletes the rating of a participant for a group.
	Remove(ctx context.Context, groupID, userID, requesterID string, requesterIsAdmin bool) error
	// ListByGroup returns the ratings given for a group (host / admin only).
	ListByGroup(ctx context.Context, groupID, requesterID string, requesterIsAdmin bool) ([]*Rating, error)
	// Summary returns the user's composite rating per sport.
	Summary(ctx context.Context, userID string) ([]*SportSummary, error)
}

type service struct {
	repo              Repository
	pickupService     pickup.Service
	skillLevelService skilllevel.Service
	userService       user.Service
	notifier          notification.Service
}

func NewService(repo Repository, pickupService pickup.Service, skillLevelService skilllevel.Service, userService user.Service, notifier notification.Service) Service {
	return &service{
		repo:              repo,
		pickupService:     pickupService,
		skillLevelService: skillLevelService,
		userService:       userService,
		notifier:          notifier,
	}
}

// authorizeHost loads the group and checks the requester may manage its ratings.
func (s *service) authorizeHost(ctx context.Context, groupID, requesterID string, isAdmin bool) (*pickup.PickupGroup, error) {
	group, err := s.pickupService.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if !isAdmin && group.HostID != requesterID {
		return nil, ErrPermissionDenied
	}
	return group, nil
}

func (s *service) Rate(ctx context.Context, groupID, userID string, level int, raterID string, raterIsAdmin bool) (*Rating, error) {
	group, err := s.authorizeHost(ctx, groupID, raterID, raterIsAdmin)
	if err != nil {
		return nil, err
	}

	if userID == raterID {
		return nil, ErrCannotRateSelf
	}

	// Ratings are only meaningful once the session has actually happened.
	ended := group.Status == pickup.GroupStatusCompleted ||
		(group.Status == pickup.GroupStatusActive && !group.EndTime.After(time.Now()))
	if !ended {
		return nil, ErrGroupNotEnded
	}

	orders, err := s.pickupService.GetOrdersByGroupID(ctx, groupID, raterID, raterIsAdmin)
	if err != nil {
		return nil, err
	}
	participant := false
	for _, o := range orders {
		if o.UserID == userID && o.Status == pickup.OrderStatusConfirmed {
			participant = true
			break
		}
	}
	if !participant {
		return nil, ErrNotParticipant
	}

	sl, err := s.skillLevelService.GetBySportAndLevel(ctx, group.SportID, level)
	if err != nil {
		if errors.Is(err, skilllevel.ErrNotFound) {
			return nil, ErrSkillLevelNotSet
		}
		return nil, err
	}
	if !sl.IsActive {
		return nil, ErrSkillLevelNotSet
	}

	rating := &Rating{
		PickupGroupID: groupID,
		UserID:        userID,
		SportID:       group.SportID,
		Level:         level,
		RatedBy:       &raterID,
	}
	if err := s.repo.Upsert(ctx, rating); err != nil {
		return nil, err
	}

	// Best effort: the rating is already stored.
	if s.notifier != nil {
		gid := groupID
		if err := s.notifier.Notify(ctx, &notification.Notification{
			UserID:        userID,
			Type:          notification.TypeSkillRated,
			Title:         "收到程度評分",
			Content:       fmt.Sprintf("團主在「%s」為你評定的程度為：%s。", group.Title, sl.Label),
			PickupGroupID: &gid,
		}); err != nil {
			log.Printf("warning: failed to deliver skill rating notification: %v", err)
		}
	}

	return rating, nil
}

func (s *service) Remove(ctx context.Context, groupID, userID, requesterID string, requesterIsAdmin bool) error {
	if _, err := s.authorizeHost(ctx, groupID, requesterID, requesterIsAdmin); err != nil {
		return err
	}
	return s.repo.Delete(ctx, groupID, userID)
}

func (s *service) ListByGroup(ctx context.Context, groupID, requesterID string, requesterIsAdmin bool) ([]*Rating, error) {
	if _, err := s.authorizeHost(ctx, groupID, requesterID, requesterIsAdmin); err != nil {
		return nil, err
	}
	return s.repo.ListByGroup(ctx, groupID)
}

func (s *service) Summary(ctx context.Context, userID string) ([]*SportSummary, error) {
	if _, err := s.userService.GetByID(ctx, userID); err != nil {
		return nil, err
	}

	summaries, err := s.repo.SummarizeByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	for _, sum := range summaries {
		level := int(math.Round(sum.Average))
		if level < 1 {
			level = 1
		}
		sum.Level = level

		labels, err := s.skillLevelService.LabelsBySport(ctx, sum.SportID)
		if err != nil {
			return nil, err
		}
		sum.Label = labels[level]
	}
	return summaries, nil
}

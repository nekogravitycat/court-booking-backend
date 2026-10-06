package pickup

import (
	"context"
	"time"
)

// MarkAbsence marks a confirmed order of an ended, non-cancelled group as absent.
// Only the group host or a system admin may do so.
func (s *service) MarkAbsence(ctx context.Context, groupID, orderID, actorID string, isSysAdmin bool) (*PickupOrder, error) {
	var result *PickupOrder
	err := s.repo.WithOrderLock(ctx, orderID, func(repo Repository) error {
		order, group, err := loadAttendanceTarget(ctx, repo, groupID, orderID, actorID, isSysAdmin)
		if err != nil {
			return err
		}
		if group.Status == GroupStatusCancelled {
			return ErrAttendanceGroupCanceled
		}
		if time.Now().Before(group.EndTime) {
			return ErrAttendanceNotYetAllowed
		}
		if order.Status != OrderStatusConfirmed {
			return ErrAttendanceNotConfirmed
		}
		absent := AttendanceAbsent
		if err := repo.SetOrderAttendance(ctx, orderID, &absent, actorID); err != nil {
			return err
		}
		result, err = repo.GetOrderByID(ctx, orderID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ClearAbsence revokes an absence mark. It is allowed whatever the current group or
// order state, so a mistaken mark can always be undone.
func (s *service) ClearAbsence(ctx context.Context, groupID, orderID, actorID string, isSysAdmin bool) error {
	return s.repo.WithOrderLock(ctx, orderID, func(repo Repository) error {
		if _, _, err := loadAttendanceTarget(ctx, repo, groupID, orderID, actorID, isSysAdmin); err != nil {
			return err
		}
		return repo.SetOrderAttendance(ctx, orderID, nil, actorID)
	})
}

// loadAttendanceTarget loads the order and its group, requires the order to belong to
// the group, and requires the actor to be the group host or a system admin.
func loadAttendanceTarget(ctx context.Context, repo Repository, groupID, orderID, actorID string, isSysAdmin bool) (*PickupOrder, *PickupGroup, error) {
	order, err := repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, nil, err
	}
	if order.PickupGroupID != groupID {
		return nil, nil, ErrOrderNotFound
	}
	group, err := repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	if group.HostID != actorID && !isSysAdmin {
		return nil, nil, ErrPermissionDenied
	}
	return order, group, nil
}

func (s *service) GetUserStats(ctx context.Context, userID string) (*UserPickupStats, error) {
	return s.repo.GetUserPickupStats(ctx, userID)
}

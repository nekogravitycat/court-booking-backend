package pickup

import (
	"context"
	"errors"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/skilllevel"
	"github.com/nekogravitycat/court-booking-backend/internal/sports"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type CreateGroupRequest struct {
	HostID       string
	Title        string
	StartTime    time.Time
	EndTime      time.Time
	Fee          int
	Capacity     int
	LocationID   string
	SportID      string
	SkillLevelID string
	Enable       bool
}

type CreateOrderRequest struct {
	PickupGroupID string
	UserID        string
	BookerName    string
	BookerPhone   string
}

type UpdateOrderRequest struct {
	Status        *string
	PaymentStatus *string
}

type UpdateGroupRequest struct {
	Title        *string
	StartTime    *time.Time
	EndTime      *time.Time
	Fee          *int
	Capacity     *int
	LocationID   *string
	SportID      *string
	SkillLevelID *string
	Status       *string
	Enable       *bool
}

type Service interface {
	CreateGroup(ctx context.Context, req CreateGroupRequest) (*PickupGroup, error)
	GetGroupByID(ctx context.Context, id string) (*PickupGroup, error)
	ListGroups(ctx context.Context, filter GroupFilter) ([]*PickupGroup, int, error)
	UpdateGroup(ctx context.Context, id string, req UpdateGroupRequest) (*PickupGroup, error)
	DeleteGroup(ctx context.Context, id string) error

	GetOrdersByGroupID(ctx context.Context, groupID string) ([]*PickupOrder, error)
	GetOrdersByUserID(ctx context.Context, userID string) ([]*PickupOrder, error)

	CreateOrder(ctx context.Context, req CreateOrderRequest) (*PickupOrder, error)
	UpdateOrder(ctx context.Context, id string, req UpdateOrderRequest, updaterUserID string, isSysAdmin bool) (*PickupOrder, error)
	DeleteOrder(ctx context.Context, id, requesterUserID string, isSysAdmin bool) error
}

type service struct {
	repo              Repository
	userService       user.Service
	sportsService     sports.Service
	skillLevelService skilllevel.Service
}

func NewService(repo Repository, userService user.Service, sportsService sports.Service, skillLevelService skilllevel.Service) Service {
	return &service{
		repo:              repo,
		userService:       userService,
		sportsService:     sportsService,
		skillLevelService: skillLevelService,
	}
}

// validateSportAndSkill verifies the sport exists and is active, and that the
// skill level exists, is active, and belongs to that sport.
func (s *service) validateSportAndSkill(ctx context.Context, sportID, skillLevelID string) error {
	sport, err := s.sportsService.GetByID(ctx, sportID)
	if err != nil {
		if errors.Is(err, sports.ErrNotFound) {
			return ErrSportNotFound
		}
		return err
	}
	if !sport.IsActive {
		return ErrSportInactive
	}

	sl, err := s.skillLevelService.GetByID(ctx, skillLevelID)
	if err != nil {
		if errors.Is(err, skilllevel.ErrNotFound) {
			return ErrSkillLevelNotFound
		}
		return err
	}
	if sl.SportID != sportID {
		return ErrSkillLevelMismatch
	}
	if !sl.IsActive {
		return ErrSkillLevelInactive
	}
	return nil
}

func (s *service) CreateGroup(ctx context.Context, req CreateGroupRequest) (*PickupGroup, error) {
	if !req.EndTime.After(req.StartTime) {
		return nil, ErrInvalidTimeRange
	}

	if err := s.validateSportAndSkill(ctx, req.SportID, req.SkillLevelID); err != nil {
		return nil, err
	}

	group := &PickupGroup{
		HostID:       req.HostID,
		Title:        req.Title,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		Fee:          req.Fee,
		Capacity:     req.Capacity,
		LocationID:   req.LocationID,
		SportID:      req.SportID,
		SkillLevelID: req.SkillLevelID,
		Status:       GroupStatusActive,
		Enable:       req.Enable,
	}

	if err := s.repo.CreateGroup(ctx, group); err != nil {
		return nil, err
	}

	return s.repo.GetGroupByID(ctx, group.ID)
}

func (s *service) GetGroupByID(ctx context.Context, id string) (*PickupGroup, error) {
	return s.repo.GetGroupByID(ctx, id)
}

func (s *service) ListGroups(ctx context.Context, filter GroupFilter) ([]*PickupGroup, int, error) {
	return s.repo.ListGroups(ctx, filter)
}

func (s *service) UpdateGroup(ctx context.Context, id string, req UpdateGroupRequest) (*PickupGroup, error) {
	group, err := s.repo.GetGroupByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Title != nil {
		group.Title = *req.Title
	}
	if req.StartTime != nil {
		group.StartTime = *req.StartTime
	}
	if req.EndTime != nil {
		group.EndTime = *req.EndTime
	}
	if req.Fee != nil {
		group.Fee = *req.Fee
	}
	if req.Capacity != nil {
		// Do not allow lowering capacity below the number of participants already
		// occupying a seat; otherwise the group would be silently over capacity.
		if *req.Capacity < group.CurrentEnrolled {
			return nil, ErrCapacityBelowEnrolled
		}
		group.Capacity = *req.Capacity
	}
	if req.LocationID != nil {
		group.LocationID = *req.LocationID
	}

	// Re-validate the sport / skill-level pair whenever either changes, so the
	// two stay consistent (the skill level must belong to the group's sport).
	sportOrSkillChanged := false
	if req.SportID != nil {
		group.SportID = *req.SportID
		sportOrSkillChanged = true
	}
	if req.SkillLevelID != nil {
		group.SkillLevelID = *req.SkillLevelID
		sportOrSkillChanged = true
	}
	if sportOrSkillChanged {
		if err := s.validateSportAndSkill(ctx, group.SportID, group.SkillLevelID); err != nil {
			return nil, err
		}
	}

	if req.Status != nil {
		gs := GroupStatus(*req.Status)
		if gs != GroupStatusActive && gs != GroupStatusCancelled && gs != GroupStatusCompleted {
			return nil, ErrInvalidStatus
		}
		group.Status = gs
	}
	if req.Enable != nil {
		group.Enable = *req.Enable
	}

	if !group.EndTime.After(group.StartTime) {
		return nil, ErrInvalidTimeRange
	}

	if err := s.repo.UpdateGroup(ctx, group); err != nil {
		return nil, err
	}

	return s.repo.GetGroupByID(ctx, id)
}

func (s *service) DeleteGroup(ctx context.Context, id string) error {
	return s.repo.DeleteGroup(ctx, id)
}

func (s *service) GetOrdersByGroupID(ctx context.Context, groupID string) ([]*PickupOrder, error) {
	if _, err := s.repo.GetGroupByID(ctx, groupID); err != nil {
		return nil, err
	}
	return s.repo.GetOrdersByGroupID(ctx, groupID)
}

func (s *service) GetOrdersByUserID(ctx context.Context, userID string) ([]*PickupOrder, error) {
	return s.repo.GetOrdersByUserID(ctx, userID)
}

func (s *service) CreateOrder(ctx context.Context, req CreateOrderRequest) (*PickupOrder, error) {
	order := &PickupOrder{
		PickupGroupID: req.PickupGroupID,
		UserID:        req.UserID,
		BookerName:    req.BookerName,
		BookerPhone:   req.BookerPhone,
		Status:        OrderStatusPending,
		PaymentStatus: PaymentStatusPending,
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		return nil, err
	}

	return order, nil
}

// UpdateOrder updates an enrollment's lifecycle status and/or payment status.
//
// Permissions:
//   - The pickup group host (or a system admin) may set any status and the
//     payment status (this covers reviewing enrollments).
//   - The enrolling user (booker) may only move their own order to 'cancelled'
//     or 'cancel_request', and may not touch the payment status.
func (s *service) UpdateOrder(ctx context.Context, id string, req UpdateOrderRequest, updaterUserID string, isSysAdmin bool) (*PickupOrder, error) {
	order, err := s.repo.GetOrderByID(ctx, id)
	if err != nil {
		return nil, err
	}

	group, err := s.repo.GetGroupByID(ctx, order.PickupGroupID)
	if err != nil {
		return nil, err
	}

	isOwner := order.UserID == updaterUserID
	isReviewer := isSysAdmin || group.HostID == updaterUserID

	if !isOwner && !isReviewer {
		return nil, ErrPermissionDenied
	}

	oldStatus := order.Status

	// Payment status is reviewer-only.
	if req.PaymentStatus != nil {
		if !isReviewer {
			return nil, ErrPermissionDenied
		}
		ps := PaymentStatus(*req.PaymentStatus)
		if !ps.IsValid() {
			return nil, ErrInvalidStatus
		}
		order.PaymentStatus = ps
	}

	if req.Status != nil {
		st := OrderStatus(*req.Status)
		if !st.IsValid() {
			return nil, ErrInvalidStatus
		}
		// A plain booker may only cancel or request cancellation of their order.
		if isOwner && !isReviewer {
			if st != OrderStatusCancelled && st != OrderStatusCancelRequest {
				return nil, ErrPermissionDenied
			}
		}
		order.Status = st
	}

	// If the order is moving from a non-occupying state (cancelled / cancel
	// request) into a seat-occupying state, re-validate capacity inside a
	// transaction so a reviewer cannot push the group over its limit.
	if isOccupyingStatus(order.Status) && !isOccupyingStatus(oldStatus) {
		if err := s.repo.UpdateOrderWithCapacityCheck(ctx, order); err != nil {
			return nil, err
		}
		return order, nil
	}

	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return nil, err
	}

	return order, nil
}

// DeleteOrder hard-deletes an enrollment. Only a system admin may do this; a
// host removes a participant by rejecting the order (status=rejected) instead,
// which keeps the row and blocks the user from re-enrolling. The group's
// current_enrolled is derived from a live COUNT, so deleting the row decrements
// it automatically.
func (s *service) DeleteOrder(ctx context.Context, id, requesterUserID string, isSysAdmin bool) error {
	_ = requesterUserID // deletion is admin-only; the requester identity is not consulted.
	if !isSysAdmin {
		return ErrPermissionDenied
	}
	return s.repo.DeleteOrder(ctx, id)
}

// isOccupyingStatus reports whether an order in the given status counts against
// the group's capacity (i.e. occupies a seat). A cancel_request still holds the
// seat: it is only released once the order is actually cancelled (or rejected).
func isOccupyingStatus(s OrderStatus) bool {
	return s == OrderStatusPending || s == OrderStatusConfirmed || s == OrderStatusCancelRequest
}

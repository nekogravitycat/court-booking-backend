package pickup

import (
	"context"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type CreateGroupRequest struct {
	HostID     string
	Title      string
	HostName   string
	HostPhone  string
	StartTime  time.Time
	EndTime    time.Time
	Fee        int
	Capacity   int
	LocationID string
	SkillLevel string
	Enable     bool
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
	Title      *string
	HostName   *string
	HostPhone  *string
	StartTime  *time.Time
	EndTime    *time.Time
	Fee        *int
	Capacity   *int
	LocationID *string
	SkillLevel *string
	Status     *string
	Enable     *bool
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
}

type service struct {
	repo        Repository
	userService user.Service
}

func NewService(repo Repository, userService user.Service) Service {
	return &service{
		repo:        repo,
		userService: userService,
	}
}

func (s *service) CreateGroup(ctx context.Context, req CreateGroupRequest) (*PickupGroup, error) {
	if !req.EndTime.After(req.StartTime) {
		return nil, ErrInvalidTimeRange
	}

	sl := SkillLevel(req.SkillLevel)
	if sl != SkillLevelA && sl != SkillLevelB && sl != SkillLevelC && sl != SkillLevelD {
		return nil, ErrInvalidStatus
	}

	group := &PickupGroup{
		HostID:     req.HostID,
		Title:      req.Title,
		HostName:   req.HostName,
		HostPhone:  req.HostPhone,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Fee:        req.Fee,
		Capacity:   req.Capacity,
		LocationID: req.LocationID,
		SkillLevel: sl,
		Status:     GroupStatusActive,
		Enable:     req.Enable,
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
	if req.HostName != nil {
		group.HostName = *req.HostName
	}
	if req.HostPhone != nil {
		group.HostPhone = *req.HostPhone
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
		group.Capacity = *req.Capacity
	}
	if req.LocationID != nil {
		group.LocationID = *req.LocationID
	}
	if req.SkillLevel != nil {
		sl := SkillLevel(*req.SkillLevel)
		if sl != SkillLevelA && sl != SkillLevelB && sl != SkillLevelC && sl != SkillLevelD {
			return nil, ErrInvalidStatus
		}
		group.SkillLevel = sl
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

	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return nil, err
	}

	return order, nil
}

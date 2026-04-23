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
	Location   string
	SkillLevel string
}

type CreateOrderRequest struct {
	PickupGroupID string
	UserID        string
	BookerName    string
	BookerPhone   string
}

type UpdateOrderRequest struct {
	PaymentStatus string
}

type Service interface {
	CreateGroup(ctx context.Context, req CreateGroupRequest) (*PickupGroup, error)
	GetGroupByID(ctx context.Context, id string) (*PickupGroup, error)
	ListGroups(ctx context.Context, filter GroupFilter) ([]*PickupGroup, int, error)
	GetOrdersByGroupID(ctx context.Context, groupID string) ([]*PickupOrder, error)
	GetOrdersByUserID(ctx context.Context, userID string) ([]*PickupOrder, error)

	CreateOrder(ctx context.Context, req CreateOrderRequest) (*PickupOrder, error)
	UpdateOrder(ctx context.Context, id string, req UpdateOrderRequest, updaterUserID string) (*PickupOrder, error)
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
		Location:   req.Location,
		SkillLevel: sl,
		Status:     GroupStatusActive,
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
		PaymentStatus: PaymentStatusPending,
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		return nil, err
	}

	return order, nil
}

func (s *service) UpdateOrder(ctx context.Context, id string, req UpdateOrderRequest, updaterUserID string) (*PickupOrder, error) {
	order, err := s.repo.GetOrderByID(ctx, id)
	if err != nil {
		return nil, err
	}

	group, err := s.repo.GetGroupByID(ctx, order.PickupGroupID)
	if err != nil {
		return nil, err
	}

	if order.UserID != updaterUserID && group.HostID != updaterUserID {
		return nil, ErrPermissionDenied
	}

	ps := PaymentStatus(req.PaymentStatus)
	if ps != PaymentStatusPending && ps != PaymentStatusPaid &&
		ps != PaymentStatusFailed && ps != PaymentStatusCancelled {
		return nil, ErrInvalidStatus
	}

	order.PaymentStatus = ps
	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return nil, err
	}

	return order, nil
}

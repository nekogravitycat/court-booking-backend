package pickup

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nekogravitycat/court-booking-backend/internal/notification"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/skilllevel"
	"github.com/nekogravitycat/court-booking-backend/internal/sports"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// Party size bounds for a multi-person enrollment. A single seat uses the
// regular enrollment endpoint.
const (
	MinPartySize = 2
	MaxPartySize = 50
)

type CreateGroupRequest struct {
	HostID               string
	Title                string
	Description          *string
	Social               *string
	StartTime            time.Time
	RegistrationDeadline time.Time
	EndTime              time.Time
	Fee                  int
	Capacity             int
	LocationID           string
	SportID              string
	MinSkillLevel        int
	MaxSkillLevel        *int
	Enable               bool
}

// CreateOrderRequest enrolls a single user using their account skill level.
type CreateOrderRequest struct {
	PickupGroupID string
	UserID        string
	BookerName    string
	BookerPhone   string
}

// CreatePartyOrderRequest enrolls several people under one order. Members holds
// exactly PartySize entries; Members[0] is the organizer.
type CreatePartyOrderRequest struct {
	PickupGroupID string
	UserID        string
	OrganizerName string
	BookerPhone   string
	PartySize     int
	Members       []OrderMember
}

type UpdateOrderRequest struct {
	Status        *string
	PaymentStatus *string
}

type UpdateGroupRequest struct {
	Title       *string
	Description *string
	// Social distinguishes absent (unchanged), null (cleared) and a value.
	Social               request.Nullable[string]
	StartTime            *time.Time
	RegistrationDeadline *time.Time
	EndTime              *time.Time
	Fee                  *int
	Capacity             *int
	LocationID           *string
	SportID              *string
	MinSkillLevel        *int
	MaxSkillLevel        *int
	Status               *string
	Enable               *bool
}

type Service interface {
	CreateGroup(ctx context.Context, req CreateGroupRequest) (*PickupGroup, error)
	// CreateGroupSeries creates one independent group per occurrence, all-or-nothing.
	CreateGroupSeries(ctx context.Context, req CreateGroupSeriesRequest) (*GroupSeries, error)
	// MarkAbsence marks a confirmed order absent after its group has ended (host or system admin).
	MarkAbsence(ctx context.Context, groupID, orderID, actorID string, isSysAdmin bool) (*PickupOrder, error)
	// ClearAbsence revokes an absence mark (host or system admin).
	ClearAbsence(ctx context.Context, groupID, orderID, actorID string, isSysAdmin bool) error
	// GetUserStats returns the user's participation, absence count and absence rate.
	GetUserStats(ctx context.Context, userID string) (*UserPickupStats, error)
	GetGroupByID(ctx context.Context, id string) (*PickupGroup, error)
	ListGroups(ctx context.Context, filter GroupFilter) ([]*PickupGroup, int, error)
	UpdateGroup(ctx context.Context, id string, req UpdateGroupRequest) (*PickupGroup, error)
	DeleteGroup(ctx context.Context, id string) error

	GetOrdersByGroupID(ctx context.Context, groupID, requesterID string, isSysAdmin bool) ([]*PickupOrder, error)
	GetOrdersByUserID(ctx context.Context, userID string) ([]*PickupOrder, error)

	CreateOrder(ctx context.Context, req CreateOrderRequest) (*PickupOrder, error)
	// CreatePartyOrder enrolls several people (one seat each) under one order.
	// The members are anonymous and are never rated.
	CreatePartyOrder(ctx context.Context, req CreatePartyOrderRequest) (*PickupOrder, error)
	UpdateOrder(ctx context.Context, id string, req UpdateOrderRequest, updaterUserID string, isSysAdmin bool) (*PickupOrder, error)
	DeleteOrder(ctx context.Context, id string, isSysAdmin bool) error

	// GetParticipantStats returns the anonymous gender / age / skill-level
	// breakdown of the group's enrolled seats (pending and confirmed orders).
	GetParticipantStats(ctx context.Context, groupID string) (*ParticipantStats, error)
	// CanViewHostPhone reports whether the viewer may see the group host's phone:
	// the host, a system admin, or a participant whose enrollment is confirmed.
	CanViewHostPhone(ctx context.Context, group *PickupGroup, viewerID string, isSysAdmin bool) (bool, error)
	// OnUserDeactivated cancels a deactivated user's upcoming enrollments and hosted groups.
	OnUserDeactivated(ctx context.Context, userID string) error
}

type service struct {
	repo              Repository
	userService       user.Service
	sportsService     sports.Service
	skillLevelService skilllevel.Service
	notifier          notification.Service
}

func NewService(repo Repository, userService user.Service, sportsService sports.Service, skillLevelService skilllevel.Service, notifier notification.Service) Service {
	return &service{
		repo:              repo,
		userService:       userService,
		sportsService:     sportsService,
		skillLevelService: skillLevelService,
		notifier:          notifier,
	}
}

// validateSport verifies the sport exists and is active.
func (s *service) validateSport(ctx context.Context, sportID string) error {
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
	return nil
}

// validateSkillLevel verifies that the integer level is defined (and active) on
// the sport's scale.
func (s *service) validateSkillLevel(ctx context.Context, sportID string, level int) error {
	sl, err := s.skillLevelService.GetBySportAndLevel(ctx, sportID, level)
	if err != nil {
		if errors.Is(err, skilllevel.ErrNotFound) {
			return ErrSkillLevelNotFound
		}
		return err
	}
	if !sl.IsActive {
		return ErrSkillLevelInactive
	}
	return nil
}

// validateSkillLevelRange verifies that minLevel and maxLevel (when set) are
// each defined and active on the sport's scale, and that maxLevel is not below
// minLevel.
func (s *service) validateSkillLevelRange(ctx context.Context, sportID string, minLevel int, maxLevel *int) error {
	if err := s.validateSkillLevel(ctx, sportID, minLevel); err != nil {
		return err
	}
	if maxLevel == nil {
		return nil
	}
	if *maxLevel < minLevel {
		return ErrInvalidSkillLevelRange
	}
	return s.validateSkillLevel(ctx, sportID, *maxLevel)
}

// validateSportAndSkillRange verifies the sport is usable and the min/max
// skill-level range belongs to its scale.
func (s *service) validateSportAndSkillRange(ctx context.Context, sportID string, minLevel int, maxLevel *int) error {
	if err := s.validateSport(ctx, sportID); err != nil {
		return err
	}
	return s.validateSkillLevelRange(ctx, sportID, minLevel, maxLevel)
}

// notify delivers notifications on a best-effort basis: the operation that
// triggered them has already succeeded, so a delivery failure is only logged.
func (s *service) notify(ctx context.Context, ns ...*notification.Notification) {
	if s.notifier == nil || len(ns) == 0 {
		return
	}
	if err := s.notifier.NotifyMany(ctx, ns); err != nil {
		log.Printf("warning: failed to deliver %d notification(s): %v", len(ns), err)
	}
}

// normalizeSocial trims the social text; blank becomes nil. It rejects text longer than MaxSocialLength.
func normalizeSocial(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(t) > MaxSocialLength {
		return nil, ErrSocialTooLong
	}
	return &t, nil
}

// validateGroupTimes enforces the time rules every new group must meet: the
// registration deadline lies between now and the start, and the end follows the start.
func validateGroupTimes(now, start, deadline, end time.Time) error {
	if deadline.Before(now) || deadline.After(start) {
		return ErrInvalidRegistrationDeadline
	}
	if !end.After(start) {
		return ErrInvalidTimeRange
	}
	return nil
}

func (s *service) CreateGroup(ctx context.Context, req CreateGroupRequest) (*PickupGroup, error) {
	if err := validateGroupTimes(time.Now(), req.StartTime, req.RegistrationDeadline, req.EndTime); err != nil {
		return nil, err
	}
	social, err := normalizeSocial(req.Social)
	if err != nil {
		return nil, err
	}

	if err := s.validateSportAndSkillRange(ctx, req.SportID, req.MinSkillLevel, req.MaxSkillLevel); err != nil {
		return nil, err
	}

	group := &PickupGroup{
		HostID:               req.HostID,
		Title:                req.Title,
		Description:          req.Description,
		Social:               social,
		StartTime:            req.StartTime,
		RegistrationDeadline: req.RegistrationDeadline,
		EndTime:              req.EndTime,
		Fee:                  req.Fee,
		Capacity:             req.Capacity,
		LocationID:           req.LocationID,
		SportID:              req.SportID,
		MinSkillLevel:        req.MinSkillLevel,
		MaxSkillLevel:        req.MaxSkillLevel,
		Status:               GroupStatusActive,
		Enable:               req.Enable,
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
	if filter.SortBy == "distance" && (filter.Latitude == nil || filter.Longitude == nil) {
		return nil, 0, ErrDistanceNeedsOrigin
	}
	if filter.FollowedOnly && filter.ViewerUserID == "" {
		return nil, 0, ErrFollowedNeedsAuth
	}
	return s.repo.ListGroups(ctx, filter)
}

// validateGroupCatalogChange checks the sport / skill-level range a group would
// have after req is applied. It runs before the group lock is taken: the
// catalog services use other pool connections, and calling them while holding
// the global schedule lock could starve the pool and stall every enrollment.
func (s *service) validateGroupCatalogChange(ctx context.Context, id string, req UpdateGroupRequest) error {
	if req.SportID == nil && req.MinSkillLevel == nil && req.MaxSkillLevel == nil {
		return nil
	}
	group, err := s.repo.GetGroupByID(ctx, id)
	if err != nil {
		return err
	}
	sportID, minLevel, maxLevel := group.SportID, group.MinSkillLevel, group.MaxSkillLevel
	if req.SportID != nil {
		sportID = *req.SportID
	}
	if req.MinSkillLevel != nil {
		minLevel = *req.MinSkillLevel
	}
	if req.MaxSkillLevel != nil {
		maxLevel = req.MaxSkillLevel
	}
	return s.validateSportAndSkillRange(ctx, sportID, minLevel, maxLevel)
}

func (s *service) UpdateGroup(ctx context.Context, id string, req UpdateGroupRequest) (*PickupGroup, error) {
	if err := s.validateGroupCatalogChange(ctx, id, req); err != nil {
		return nil, err
	}
	var result, previous *PickupGroup
	err := s.repo.WithGroupLock(ctx, id, func(repo Repository) error {
		var err error
		previous, err = repo.GetGroupByID(ctx, id)
		if err != nil {
			return err
		}
		result, err = updateGroup(ctx, repo, previous, req)
		return err
	})
	if err != nil {
		return nil, err
	}
	if result.Status == GroupStatusCancelled && previous.Status != GroupStatusCancelled {
		s.notifyEnrolled(ctx, result, notification.TypePickupGroupCancelled, "臨打團已取消", fmt.Sprintf("「%s」已被團主取消。", result.Title))
	} else if result.Status == GroupStatusActive && (!result.StartTime.Equal(previous.StartTime) || !result.EndTime.Equal(previous.EndTime) || result.LocationID != previous.LocationID || previous.Status == GroupStatusCancelled) {
		s.notifyEnrolled(ctx, result, notification.TypePickupGroupUpdated, "臨打團資訊已更新", fmt.Sprintf("「%s」的時間或地點已變更，請重新確認活動資訊。", result.Title))
	}
	return result, nil
}

// updateGroup applies req to a copy of previous (the group as read under the lock) and persists it
// through repo, which must be the lock-scoped repository.
func updateGroup(ctx context.Context, repo Repository, previous *PickupGroup, req UpdateGroupRequest) (*PickupGroup, error) {
	updated := *previous
	group := &updated

	if req.Title != nil {
		group.Title = *req.Title
	}
	if req.Description != nil {
		group.Description = req.Description
	}
	if req.Social.Set {
		social, err := normalizeSocial(req.Social.Value)
		if err != nil {
			return nil, err
		}
		group.Social = social
	}
	if req.StartTime != nil {
		group.StartTime = *req.StartTime
	}
	if req.RegistrationDeadline != nil {
		group.RegistrationDeadline = *req.RegistrationDeadline
	}
	if group.RegistrationDeadline.After(group.StartTime) || ((req.RegistrationDeadline != nil || req.StartTime != nil) && group.RegistrationDeadline.Before(time.Now())) {
		return nil, ErrInvalidRegistrationDeadline
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

	// The sport / skill-level range was already validated by
	// validateGroupCatalogChange before the lock was taken.
	if req.SportID != nil {
		group.SportID = *req.SportID
	}
	if req.MinSkillLevel != nil {
		group.MinSkillLevel = *req.MinSkillLevel
	}
	if req.MaxSkillLevel != nil {
		group.MaxSkillLevel = req.MaxSkillLevel
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

	if err := repo.UpdateGroup(ctx, group); err != nil {
		return nil, err
	}

	return repo.GetGroupByID(ctx, group.ID)
}

// notifyEnrolled sends one notification to every user currently holding a seat
// in the group (the host, who is notified of nothing about their own edit, is
// excluded).
func (s *service) notifyEnrolled(ctx context.Context, group *PickupGroup, ntype, title, content string) {
	userIDs, err := s.repo.ListOccupyingUserIDs(ctx, group.ID)
	if err != nil {
		log.Printf("warning: failed to list enrolled users of group %s: %v", group.ID, err)
		return
	}

	groupID := group.ID
	var ns []*notification.Notification
	for _, uid := range userIDs {
		if uid == group.HostID {
			continue
		}
		ns = append(ns, &notification.Notification{
			UserID:        uid,
			Type:          ntype,
			Title:         title,
			Content:       content,
			PickupGroupID: &groupID,
		})
	}
	s.notify(ctx, ns...)
}

func (s *service) DeleteGroup(ctx context.Context, id string) error {
	return s.repo.DeleteGroup(ctx, id)
}

func (s *service) GetOrdersByGroupID(ctx context.Context, groupID, requesterID string, isSysAdmin bool) ([]*PickupOrder, error) {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group.HostID != requesterID && !isSysAdmin {
		return nil, ErrOrdersForbidden
	}
	return s.repo.GetOrdersByGroupID(ctx, groupID)
}

func (s *service) GetOrdersByUserID(ctx context.Context, userID string) ([]*PickupOrder, error) {
	return s.repo.GetOrdersByUserID(ctx, userID)
}

func (s *service) CreateOrder(ctx context.Context, req CreateOrderRequest) (*PickupOrder, error) {
	group, err := s.repo.GetGroupByID(ctx, req.PickupGroupID)
	if err != nil {
		return nil, err
	}
	level, err := s.enrollmentSkillLevel(ctx, req.UserID, group.SportID)
	if err != nil {
		return nil, err
	}

	order := &PickupOrder{
		EnrollmentSportID: group.SportID,
		PickupGroupID:     req.PickupGroupID,
		UserID:            req.UserID,
		BookerName:        req.BookerName,
		BookerPhone:       req.BookerPhone,
		Status:            OrderStatusPending,
		PaymentStatus:     PaymentStatusPending,
		SkillLevel:        level,
		PartySize:         1,
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		return nil, err
	}

	s.notifyHostOfEnrollment(ctx, group, order)
	return order, nil
}

func (s *service) CreatePartyOrder(ctx context.Context, req CreatePartyOrderRequest) (*PickupOrder, error) {
	if req.PartySize < MinPartySize || req.PartySize > MaxPartySize {
		return nil, ErrInvalidPartySize
	}
	if len(req.Members) != req.PartySize {
		return nil, ErrPartyMembersMismatch
	}

	group, err := s.repo.GetGroupByID(ctx, req.PickupGroupID)
	if err != nil {
		return nil, err
	}

	// Validate every member; each distinct level is looked up only once.
	level, err := s.enrollmentSkillLevel(ctx, req.UserID, group.SportID)
	if err != nil {
		return nil, err
	}
	// Copy before overriding the organizer so the caller's slice is untouched.
	req.Members = append([]OrderMember(nil), req.Members...)
	req.Members[0].SkillLevel = level
	checkedLevels := make(map[int]struct{})
	for _, m := range req.Members {
		if !user.IsValidGender(m.Gender) {
			return nil, user.ErrInvalidGender
		}
		if _, ok := checkedLevels[m.SkillLevel]; ok {
			continue
		}
		if err := s.validateSkillLevel(ctx, group.SportID, m.SkillLevel); err != nil {
			return nil, err
		}
		checkedLevels[m.SkillLevel] = struct{}{}
	}

	order := &PickupOrder{
		EnrollmentSportID: group.SportID,
		PickupGroupID:     req.PickupGroupID,
		UserID:            req.UserID,
		BookerName:        strings.TrimSpace(req.OrganizerName),
		BookerPhone:       req.BookerPhone,
		Status:            OrderStatusPending,
		PaymentStatus:     PaymentStatusPending,
		SkillLevel:        req.Members[0].SkillLevel, // Members[0] is the organizer
		PartySize:         req.PartySize,
		Members:           req.Members,
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		return nil, err
	}

	s.notifyHostOfEnrollment(ctx, group, order)
	return order, nil
}

// enrollmentSkillLevel captures the account declaration at enrollment time.
// Inactive scales cannot be used even when the account was configured earlier.
func (s *service) enrollmentSkillLevel(ctx context.Context, userID, sportID string) (int, error) {
	if err := s.validateSport(ctx, sportID); err != nil {
		return 0, err
	}
	level, err := s.userService.GetSkillLevel(ctx, userID, sportID)
	if err != nil {
		return 0, err
	}
	if err := s.validateSkillLevel(ctx, sportID, level.SkillLevel); err != nil {
		return 0, err
	}
	return level.SkillLevel, nil
}

// notifyHostOfEnrollment tells the host that someone enrolled.
func (s *service) notifyHostOfEnrollment(ctx context.Context, group *PickupGroup, order *PickupOrder) {
	if group.HostID == order.UserID {
		return
	}
	content := fmt.Sprintf("%s 報名了「%s」。", order.BookerName, group.Title)
	if order.PartySize > 1 {
		content = fmt.Sprintf("%s 報名了「%s」（共 %d 人）。", order.BookerName, group.Title, order.PartySize)
	}
	groupID, orderID := group.ID, order.ID
	s.notify(ctx, &notification.Notification{
		UserID:        group.HostID,
		Type:          notification.TypePickupOrderCreated,
		Title:         "有新的報名",
		Content:       content,
		PickupGroupID: &groupID,
		PickupOrderID: &orderID,
	})
}

// UpdateOrder updates an enrollment's lifecycle status and/or payment status.
//
// Permissions:
//   - The pickup group host (or a system admin) may set any status and the
//     payment status (this covers reviewing enrollments).
//   - The enrolling user (booker) may only move their own order to 'cancelled'
//     or 'cancel_request', and may not touch the payment status. Paid or
//     confirmed orders require reviewer approval before cancellation.
func (s *service) UpdateOrder(ctx context.Context, id string, req UpdateOrderRequest, updaterUserID string, isSysAdmin bool) (*PickupOrder, error) {
	var result, previous *PickupOrder
	var group *PickupGroup
	err := s.repo.WithOrderLock(ctx, id, func(repo Repository) error {
		var err error
		previous, err = repo.GetOrderByID(ctx, id)
		if err != nil {
			return err
		}
		group, err = repo.GetGroupByID(ctx, previous.PickupGroupID)
		if err != nil {
			return err
		}
		result, err = updateOrder(ctx, repo, previous, group, req, updaterUserID, isSysAdmin)
		return err
	})
	if err != nil {
		return nil, err
	}
	reviewer := isSysAdmin || group.HostID == updaterUserID
	s.notifyOrderChange(ctx, group, result, previous.Status, previous.PaymentStatus, updaterUserID, reviewer && result.UserID != updaterUserID)
	return result, nil
}

// updateOrder applies req to a copy of previous (the order as read under the lock) and persists
// it through repo, which must be the lock-scoped repository.
func updateOrder(ctx context.Context, repo Repository, previous *PickupOrder, group *PickupGroup, req UpdateOrderRequest, updaterUserID string, isSysAdmin bool) (*PickupOrder, error) {
	updated := *previous
	order := &updated

	isOwner := order.UserID == updaterUserID
	isReviewer := isSysAdmin || group.HostID == updaterUserID

	if !isOwner && !isReviewer {
		return nil, ErrPermissionDenied
	}

	oldStatus := order.Status
	oldPaymentStatus := order.PaymentStatus

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
			// A rejected or cancelled order is terminal for the booker: leaving it
			// would undo the host's rejection or re-occupy seats. Re-enrolling after
			// a cancellation goes through the enrollment endpoint instead.
			if oldStatus == OrderStatusRejected || oldStatus == OrderStatusCancelled {
				return nil, ErrPermissionDenied
			}
			if st == OrderStatusCancelled && (oldStatus == OrderStatusConfirmed || oldStatus == OrderStatusCancelRequest || oldPaymentStatus == PaymentStatusDone) {
				return nil, ErrCancellationRequiresReview
			}
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
		if err := repo.UpdateOrderWithCapacityCheck(ctx, order); err != nil {
			return nil, err
		}
	} else if err := repo.UpdateOrder(ctx, order); err != nil {
		return nil, err
	}

	return order, nil
}

// notifyOrderChange notifies the other party of a status / payment change: the
// booker when a reviewer acted, the host when the booker acted.
func (s *service) notifyOrderChange(ctx context.Context, group *PickupGroup, order *PickupOrder, oldStatus OrderStatus, oldPayment PaymentStatus, actorID string, byReviewer bool) {
	groupID, orderID := group.ID, order.ID
	base := func(userID, ntype, title, content string) *notification.Notification {
		return &notification.Notification{
			UserID:        userID,
			Type:          ntype,
			Title:         title,
			Content:       content,
			PickupGroupID: &groupID,
			PickupOrderID: &orderID,
		}
	}

	var ns []*notification.Notification

	if byReviewer {
		if order.UserID == actorID {
			return
		}
		if order.Status != oldStatus {
			switch order.Status {
			case OrderStatusConfirmed:
				ns = append(ns, base(order.UserID, notification.TypePickupOrderConfirmed,
					"報名已確認", fmt.Sprintf("你在「%s」的報名已被團主確認。", group.Title)))
			case OrderStatusRejected:
				ns = append(ns, base(order.UserID, notification.TypePickupOrderRejected,
					"報名未被接受", fmt.Sprintf("你在「%s」的報名未被團主接受。", group.Title)))
			case OrderStatusCancelled:
				ns = append(ns, base(order.UserID, notification.TypePickupOrderCancelledByHost,
					"報名已被取消", fmt.Sprintf("你在「%s」的報名已被團主取消。", group.Title)))
			}
		}
		if order.PaymentStatus != oldPayment {
			ns = append(ns, base(order.UserID, notification.TypePickupPaymentUpdated,
				"付款狀態已更新", fmt.Sprintf("你在「%s」的付款狀態已更新為%s。", group.Title, paymentStatusText(order.PaymentStatus))))
		}
	} else if order.Status != oldStatus && group.HostID != actorID {
		switch order.Status {
		case OrderStatusCancelled:
			ns = append(ns, base(group.HostID, notification.TypePickupOrderCancelled,
				"有人取消報名", fmt.Sprintf("%s 取消了「%s」的報名。", order.BookerName, group.Title)))
		case OrderStatusCancelRequest:
			ns = append(ns, base(group.HostID, notification.TypePickupOrderCancelRequested,
				"有人申請取消報名", fmt.Sprintf("%s 申請取消「%s」的報名，請儘速處理。", order.BookerName, group.Title)))
		}
	}

	s.notify(ctx, ns...)
}

func paymentStatusText(p PaymentStatus) string {
	switch p {
	case PaymentStatusDone:
		return "已付款"
	case PaymentStatusFailed:
		return "付款失敗"
	default:
		return "待付款"
	}
}

// DeleteOrder hard-deletes an enrollment. Only a system admin may do this; a
// host removes a participant by rejecting the order (status=rejected) instead,
// which keeps the row and blocks the user from re-enrolling. The group's
// current_enrolled is derived from a live SUM, so deleting the row releases its
// seats automatically.
func (s *service) DeleteOrder(ctx context.Context, id string, isSysAdmin bool) error {
	if !isSysAdmin {
		return ErrPermissionDenied
	}
	return s.repo.DeleteOrder(ctx, id)
}

func (s *service) GetParticipantStats(ctx context.Context, groupID string) (*ParticipantStats, error) {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	seats, tz, err := s.repo.ListParticipantSeats(ctx, groupID)
	if err != nil {
		return nil, err
	}

	labels, err := s.skillLevelService.LabelsBySport(ctx, group.SportID)
	if err != nil {
		return nil, err
	}

	// Ages are computed against the calendar date at the group's location.
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}

	stats := BuildParticipantStats(seats, time.Now().In(loc), labels)
	return &stats, nil
}

// isOccupyingStatus reports whether an order in the given status counts against
// the group's capacity (i.e. occupies a seat). A cancel_request still holds the
// seat: it is only released once the order is actually cancelled (or rejected).
func isOccupyingStatus(s OrderStatus) bool {
	return s == OrderStatusPending || s == OrderStatusConfirmed || s == OrderStatusCancelRequest
}

// OnUserDeactivated implements user.DeactivationHook: the deactivated user's
// upcoming enrollments are cancelled (the hosts are told) and the groups they
// host are cancelled (the enrolled users are told). It is idempotent.
func (s *service) OnUserDeactivated(ctx context.Context, userID string) error {
	orders, err := s.repo.CancelUpcomingOrdersByUser(ctx, userID)
	if err != nil {
		return err
	}
	groups, err := s.repo.CancelUpcomingGroupsByHost(ctx, userID)
	if err != nil {
		return err
	}

	var ns []*notification.Notification
	for _, o := range orders {
		if o.HostID == userID {
			continue
		}
		groupID, orderID := o.GroupID, o.OrderID
		ns = append(ns, &notification.Notification{
			UserID:        o.HostID,
			Type:          notification.TypePickupOrderCancelled,
			Title:         "有人取消報名",
			Content:       fmt.Sprintf("%s 的帳號已停用，其在「%s」的報名已自動取消。", o.BookerName, o.GroupTitle),
			PickupGroupID: &groupID,
			PickupOrderID: &orderID,
		})
	}
	s.notify(ctx, ns...)

	for _, g := range groups {
		s.notifyEnrolled(ctx, g, notification.TypePickupGroupCancelled, "臨打團已取消", fmt.Sprintf("「%s」已被取消，因為團主的帳號已停用。", g.Title))
	}
	return nil
}

func (s *service) CanViewHostPhone(ctx context.Context, group *PickupGroup, viewerID string, isSysAdmin bool) (bool, error) {
	if viewerID == "" {
		return false, nil
	}
	if group.HostID == viewerID || isSysAdmin {
		return true, nil
	}
	return s.repo.HasConfirmedOrder(ctx, group.ID, viewerID)
}

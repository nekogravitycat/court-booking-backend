package coach

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/notification"
)

// Actor identifies who performs a group action.
type Actor struct {
	UserID  string
	IsAdmin bool
}

// GroupService manages coach groups, membership and invitations.
type GroupService interface {
	Create(ctx context.Context, userID, name string, allowPrivate bool) (*Group, error)
	Get(ctx context.Context, id string) (*Group, error)
	GetMine(ctx context.Context, userID string) (*Group, error)
	List(ctx context.Context, filter GroupFilter) ([]*Group, int, error)
	Update(ctx context.Context, id string, actor Actor, name *string, allowPrivate *bool) (*Group, error)
	Disband(ctx context.Context, id string, actor Actor) error

	Invite(ctx context.Context, groupID string, actor Actor, inviteeID, role string) (*Invitation, error)
	ListGroupInvitations(ctx context.Context, groupID string, actor Actor, filter InvitationFilter) ([]*Invitation, int, error)
	CancelInvitation(ctx context.Context, groupID, invitationID string, actor Actor) error
	ListMyInvitations(ctx context.Context, userID string, filter InvitationFilter) ([]*Invitation, int, error)
	Accept(ctx context.Context, invitationID, userID string) (*Group, error)
	Decline(ctx context.Context, invitationID, userID string) error

	UpdateMember(ctx context.Context, groupID, userID string, actor Actor, u MemberUpdate) (*Member, error)
	RemoveMember(ctx context.Context, groupID, userID string, actor Actor) error
}

type groupService struct {
	repo     GroupRepository
	coaches  Service
	notifier notification.Service
}

func NewGroupService(repo GroupRepository, coaches Service, notifier notification.Service) GroupService {
	return &groupService{repo: repo, coaches: coaches, notifier: notifier}
}

func cleanGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || runes(name) > MaxGroupName {
		return "", ErrInvalidGroupName
	}
	return name, nil
}

func validRole(r string) bool { return r == RoleLeader || r == RoleCoach || r == RoleAssistant }

// requireLeader allows system admins and leaders of the group.
func (s *groupService) requireLeader(ctx context.Context, groupID string, actor Actor) error {
	if actor.IsAdmin {
		return nil
	}
	m, err := s.repo.GetMember(ctx, groupID, actor.UserID)
	if err != nil {
		if err == ErrNotInGroup {
			return ErrGroupForbidden
		}
		return err
	}
	if m.Role != RoleLeader {
		return ErrGroupForbidden
	}
	return nil
}

func (s *groupService) Create(ctx context.Context, userID, name string, allowPrivate bool) (*Group, error) {
	name, err := cleanGroupName(name)
	if err != nil {
		return nil, err
	}
	if err := s.coaches.RequireApproved(ctx, userID); err != nil {
		return nil, err
	}
	id, err := s.repo.CreateGroup(ctx, name, allowPrivate, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetGroup(ctx, id)
}

func (s *groupService) Get(ctx context.Context, id string) (*Group, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrGroupNotFound
	}
	return s.repo.GetGroup(ctx, id)
}

func (s *groupService) GetMine(ctx context.Context, userID string) (*Group, error) {
	return s.repo.GetGroupByMember(ctx, userID)
}

func (s *groupService) List(ctx context.Context, filter GroupFilter) ([]*Group, int, error) {
	return s.repo.ListGroups(ctx, filter)
}

func (s *groupService) Update(ctx context.Context, id string, actor Actor, name *string, allowPrivate *bool) (*Group, error) {
	if name == nil && allowPrivate == nil {
		return nil, ErrNothingToUpdate
	}
	if name != nil {
		n, err := cleanGroupName(*name)
		if err != nil {
			return nil, err
		}
		name = &n
	}
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	if err := s.requireLeader(ctx, id, actor); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateGroup(ctx, id, name, allowPrivate); err != nil {
		return nil, err
	}
	return s.repo.GetGroup(ctx, id)
}

func (s *groupService) Disband(ctx context.Context, id string, actor Actor) error {
	g, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.requireLeader(ctx, id, actor); err != nil {
		return err
	}
	members, err := s.repo.DisbandGroup(ctx, id)
	if err != nil {
		return err
	}
	for _, uid := range members {
		if uid == actor.UserID {
			continue
		}
		notify(ctx, s.notifier, newNotification(uid, NotifGroupDisbanded,
			"Coach group disbanded", "The coach group \""+g.Name+"\" was disbanded.", RefCoachGroup, id))
	}
	return nil
}

func (s *groupService) Invite(ctx context.Context, groupID string, actor Actor, inviteeID, role string) (*Invitation, error) {
	if role != RoleCoach && role != RoleAssistant {
		return nil, ErrInvalidInviteRole
	}
	if inviteeID == actor.UserID {
		return nil, ErrInviteSelf
	}
	if _, err := uuid.Parse(inviteeID); err != nil {
		return nil, ErrInviteeNotCoach
	}
	g, err := s.Get(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := s.requireLeader(ctx, groupID, actor); err != nil {
		return nil, err
	}
	if err := s.coaches.RequireApproved(ctx, inviteeID); err != nil {
		return nil, ErrInviteeNotCoach
	}
	inv, err := s.repo.CreateInvitation(ctx, groupID, inviteeID, role, actor.UserID)
	if err != nil {
		return nil, err
	}
	notify(ctx, s.notifier, newNotification(inviteeID, NotifGroupInvited,
		"Coach group invitation", "You were invited to join the coach group \""+g.Name+"\" as "+role+".",
		RefGroupInvitation, inv.ID))
	return inv, nil
}

func (s *groupService) ListGroupInvitations(ctx context.Context, groupID string, actor Actor, filter InvitationFilter) ([]*Invitation, int, error) {
	if _, err := s.Get(ctx, groupID); err != nil {
		return nil, 0, err
	}
	if err := s.requireLeader(ctx, groupID, actor); err != nil {
		return nil, 0, err
	}
	filter.GroupID = groupID
	return s.repo.ListInvitations(ctx, filter)
}

func (s *groupService) CancelInvitation(ctx context.Context, groupID, invitationID string, actor Actor) error {
	if _, err := uuid.Parse(invitationID); err != nil {
		return ErrInvitationNotFound
	}
	if _, err := s.Get(ctx, groupID); err != nil {
		return err
	}
	if err := s.requireLeader(ctx, groupID, actor); err != nil {
		return err
	}
	return s.repo.CancelInvitation(ctx, groupID, invitationID)
}

func (s *groupService) ListMyInvitations(ctx context.Context, userID string, filter InvitationFilter) ([]*Invitation, int, error) {
	filter.InviteeID = userID
	return s.repo.ListInvitations(ctx, filter)
}

func (s *groupService) Accept(ctx context.Context, invitationID, userID string) (*Group, error) {
	if _, err := uuid.Parse(invitationID); err != nil {
		return nil, ErrInvitationNotFound
	}
	// A suspended coach cannot join a group.
	if err := s.coaches.RequireApproved(ctx, userID); err != nil {
		return nil, err
	}
	inv, err := s.repo.RespondInvitation(ctx, invitationID, userID, true)
	if err != nil {
		return nil, err
	}
	s.notifyInviter(ctx, inv, NotifGroupInviteAccepted, "accepted")
	return s.repo.GetGroup(ctx, inv.GroupID)
}

func (s *groupService) Decline(ctx context.Context, invitationID, userID string) error {
	if _, err := uuid.Parse(invitationID); err != nil {
		return ErrInvitationNotFound
	}
	inv, err := s.repo.RespondInvitation(ctx, invitationID, userID, false)
	if err != nil {
		return err
	}
	s.notifyInviter(ctx, inv, NotifGroupInviteDeclined, "declined")
	return nil
}

func (s *groupService) notifyInviter(ctx context.Context, inv *Invitation, typ, verb string) {
	if inv.InvitedBy == nil {
		return
	}
	notify(ctx, s.notifier, newNotification(*inv.InvitedBy, typ, "Coach group invitation "+verb,
		inv.InviteeName+" "+verb+" the invitation to \""+inv.GroupName+"\".", RefGroupInvitation, inv.ID))
}

func (s *groupService) UpdateMember(ctx context.Context, groupID, userID string, actor Actor, u MemberUpdate) (*Member, error) {
	if u.Role == nil && u.Status == nil {
		return nil, ErrNothingToUpdate
	}
	if u.Role != nil && !validRole(*u.Role) {
		return nil, ErrInvalidRole
	}
	if u.Status != nil && *u.Status != MemberActive && *u.Status != MemberInactive {
		return nil, ErrInvalidMemberState
	}
	g, err := s.Get(ctx, groupID)
	if err != nil {
		return nil, err
	}
	// Members may toggle their own status; every other change needs a leader.
	if actor.UserID == userID && u.Role == nil {
		if _, err := s.repo.GetMember(ctx, groupID, userID); err != nil {
			return nil, err
		}
	} else if err := s.requireLeader(ctx, groupID, actor); err != nil {
		if err == ErrGroupForbidden && u.Role != nil {
			return nil, ErrRoleChangeForbid
		}
		return nil, err
	}

	m, err := s.repo.UpdateMember(ctx, groupID, userID, u)
	if err != nil {
		return nil, err
	}
	if userID != actor.UserID {
		notify(ctx, s.notifier, newNotification(userID, NotifGroupMemberChanged,
			"Coach group membership updated",
			"Your role in \""+g.Name+"\" is now "+m.Role+" ("+m.Status+").", RefCoachGroup, groupID))
	}
	return m, nil
}

func (s *groupService) RemoveMember(ctx context.Context, groupID, userID string, actor Actor) error {
	g, err := s.Get(ctx, groupID)
	if err != nil {
		return err
	}
	self := actor.UserID == userID
	if !self {
		if err := s.requireLeader(ctx, groupID, actor); err != nil {
			return err
		}
	}
	leaders, _ := s.repo.LeaderIDs(ctx, groupID)

	disbanded, err := s.repo.RemoveMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	switch {
	case disbanded:
		// The last member left; nobody remains to notify.
	case self:
		for _, l := range leaders {
			if l != userID {
				notify(ctx, s.notifier, newNotification(l, NotifGroupMemberLeft,
					"Member left coach group", "A member left \""+g.Name+"\".", RefCoachGroup, groupID))
			}
		}
	default:
		notify(ctx, s.notifier, newNotification(userID, NotifGroupMemberRemoved,
			"Removed from coach group", "You were removed from \""+g.Name+"\".", RefCoachGroup, groupID))
	}
	return nil
}

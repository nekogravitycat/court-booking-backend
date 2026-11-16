package coach

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound            = apperror.New(http.StatusNotFound, "coach not found")
	ErrAlreadyApplied      = apperror.New(http.StatusConflict, "a coach application already exists for this account")
	ErrNotCoach            = apperror.New(http.StatusForbidden, "an approved coach profile is required")
	ErrCoachSuspended      = apperror.New(http.StatusForbidden, "coach account is suspended")
	ErrApplicationNotOpen  = apperror.New(http.StatusConflict, "application is not pending review")
	ErrInvalidDecision     = apperror.New(http.StatusBadRequest, "decision must be approve or reject")
	ErrRejectReasonNeeded  = apperror.New(http.StatusBadRequest, "a reason is required when rejecting an application")
	ErrInvalidProfile      = apperror.New(http.StatusBadRequest, "invalid coach profile")
	ErrSportsRequired      = apperror.New(http.StatusBadRequest, "at least one sport is required")
	ErrTooManySports       = apperror.New(http.StatusBadRequest, "too many sports")
	ErrTooManyCertificates = apperror.New(http.StatusBadRequest, "too many certificates")
	ErrSportNotFound       = apperror.New(http.StatusBadRequest, "sport not found or inactive")
	ErrInvalidFile         = apperror.New(http.StatusBadRequest, "file does not exist, is not yours, or has an unsupported type")

	ErrGroupNotFound      = apperror.New(http.StatusNotFound, "coach group not found")
	ErrInvalidGroupName   = apperror.New(http.StatusBadRequest, "group name must be 1-50 characters")
	ErrAlreadyInGroup     = apperror.New(http.StatusConflict, "coach already belongs to a coach group")
	ErrNotInGroup         = apperror.New(http.StatusNotFound, "coach is not a member of this group")
	ErrGroupForbidden     = apperror.New(http.StatusForbidden, "only a group leader or system admin can do this")
	ErrInvalidRole        = apperror.New(http.StatusBadRequest, "role must be leader, coach, or assistant")
	ErrInvalidInviteRole  = apperror.New(http.StatusBadRequest, "invitations can only offer the coach or assistant role")
	ErrInvalidMemberState = apperror.New(http.StatusBadRequest, "status must be active or inactive")
	ErrNothingToUpdate    = apperror.New(http.StatusBadRequest, "no fields to update")
	ErrLastLeader         = apperror.New(http.StatusConflict, "the group must keep at least one leader; promote another member first")
	ErrInviteSelf         = apperror.New(http.StatusBadRequest, "cannot invite yourself")
	ErrInviteeNotCoach    = apperror.New(http.StatusBadRequest, "invitee must be an approved coach")
	ErrInvitationPending  = apperror.New(http.StatusConflict, "an invitation for this coach is already pending")
	ErrInvitationNotFound = apperror.New(http.StatusNotFound, "invitation not found")
	ErrInvitationClosed   = apperror.New(http.StatusConflict, "invitation is no longer pending")
	ErrRoleChangeForbid   = apperror.New(http.StatusForbidden, "only a group leader can change roles")
)

// Coach status values. A coach row is also the application until approved.
const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusSuspended = "suspended"
)

// Group member roles and states.
const (
	RoleLeader    = "leader"
	RoleCoach     = "coach"
	RoleAssistant = "assistant"

	MemberActive   = "active"
	MemberInactive = "inactive"
)

// Invitation states.
const (
	InvitePending   = "pending"
	InviteAccepted  = "accepted"
	InviteDeclined  = "declined"
	InviteCancelled = "cancelled"
)

// Field limits (in characters).
const (
	MaxBio             = 1000
	MaxTeachingContent = 1000
	MaxTargetAudience  = 500
	MaxExperience      = 1000
	MaxFeeInfo         = 500
	MaxCertTitle       = 50
	MaxSports          = 10
	MaxCertificates    = 10
	MaxGroupName       = 50
)

// Notification types and reference kinds used by this module.
const (
	NotifApplicationSubmitted = "coach_application_submitted" // To admins
	NotifApplicationApproved  = "coach_application_approved"
	NotifApplicationRejected  = "coach_application_rejected"
	NotifGroupInvited         = "coach_group_invited"
	NotifGroupInviteAccepted  = "coach_group_invite_accepted"
	NotifGroupInviteDeclined  = "coach_group_invite_declined"
	NotifGroupMemberChanged   = "coach_group_member_changed"
	NotifGroupMemberRemoved   = "coach_group_member_removed"
	NotifGroupMemberLeft      = "coach_group_member_left"
	NotifGroupDisbanded       = "coach_group_disbanded"

	RefCoach           = "coach"
	RefCoachGroup      = "coach_group"
	RefGroupInvitation = "coach_group_invitation"
)

// SportBrief is the sport summary shown on a coach profile.
type SportBrief struct {
	ID   string
	Code string
	Name string
}

// Certificate is an uploaded credential attached to a coach profile.
type Certificate struct {
	ID     string
	FileID string
	Title  string
}

// GroupBrief identifies the group a coach belongs to.
type GroupBrief struct {
	ID   string
	Name string
	Role string
}

// Coach is a coach profile (and application).
type Coach struct {
	UserID          string
	DisplayName     string
	Avatar          *string // Avatar file ID of the user account
	Status          string
	Bio             string
	TeachingContent string
	TargetAudience  string
	Experience      string
	FeeInfo         string
	PhotoFileID     *string
	RejectReason    *string
	ReviewedAt      *time.Time
	AppliedAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Sports          []SportBrief
	Certificates    []Certificate
	Group           *GroupBrief
	// Public rating summary from visible reviews (nil average = no reviews yet).
	RatingAverage *float64
	RatingCount   int
}

// CertificateProvided reports whether any certificate is attached.
func (c *Coach) CertificateProvided() bool { return len(c.Certificates) > 0 }

// CertificateInput references an uploaded file as a certificate.
type CertificateInput struct {
	FileID string
	Title  string
}

// ProfileInput carries a full coach profile for an application.
type ProfileInput struct {
	Bio             string
	TeachingContent string
	TargetAudience  string
	Experience      string
	FeeInfo         string
	PhotoFileID     *string
	SportIDs        []string
	Certificates    []CertificateInput
}

// ProfileUpdate is a partial profile update. Nil fields are left unchanged;
// a non-nil SportIDs / Certificates replaces the whole list.
type ProfileUpdate struct {
	Bio             *string
	TeachingContent *string
	TargetAudience  *string
	Experience      *string
	FeeInfo         *string
	PhotoFileID     *string
	ClearPhoto      bool
	SportIDs        *[]string
	Certificates    *[]CertificateInput
}

// CoachFilter defines parameters for listing coaches or applications.
type CoachFilter struct {
	Status   string // Empty = approved only for public lists
	SportID  string
	Keyword  string // Matches display name
	Page     int
	PageSize int
}

// Group is a coach group with its members.
type Group struct {
	ID                  string
	Name                string
	AllowPrivateCourses bool
	IsActive            bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Members             []*Member
	MemberCount         int
}

// Member is one coach in a group.
type Member struct {
	GroupID     string
	UserID      string
	DisplayName string
	Avatar      *string
	Role        string
	Status      string
	JoinedAt    time.Time
}

// Invitation is an offer for a coach to join a group.
type Invitation struct {
	ID          string
	GroupID     string
	GroupName   string
	InviteeID   string
	InviteeName string
	InvitedBy   *string
	Role        string
	Status      string
	CreatedAt   time.Time
	RespondedAt *time.Time
}

// GroupFilter defines parameters for listing groups.
type GroupFilter struct {
	Keyword  string
	Page     int
	PageSize int
}

// MemberUpdate is a partial member update.
type MemberUpdate struct {
	Role   *string
	Status *string
}

package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/coach"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

// ---- Coach profiles ----

type CertificateRequest struct {
	FileID string `json:"file_id" binding:"required,uuid"`
	Title  string `json:"title" binding:"required"`
}

// ApplyRequest is the body of POST /coach-applications.
type ApplyRequest struct {
	Bio             string               `json:"bio" binding:"required"`
	TeachingContent string               `json:"teaching_content" binding:"required"`
	TargetAudience  string               `json:"target_audience" binding:"required"`
	Experience      string               `json:"experience"`
	FeeInfo         string               `json:"fee_info" binding:"required"`
	PhotoFileID     *string              `json:"photo_file_id" binding:"omitempty,uuid"`
	SportIDs        []string             `json:"sport_ids" binding:"required,min=1"`
	Certificates    []CertificateRequest `json:"certificates" binding:"omitempty,dive"`
}

func (r ApplyRequest) ToInput() coach.ProfileInput {
	return coach.ProfileInput{
		Bio: r.Bio, TeachingContent: r.TeachingContent, TargetAudience: r.TargetAudience,
		Experience: r.Experience, FeeInfo: r.FeeInfo, PhotoFileID: r.PhotoFileID,
		SportIDs: r.SportIDs, Certificates: toCertInputs(r.Certificates),
	}
}

func toCertInputs(in []CertificateRequest) []coach.CertificateInput {
	out := make([]coach.CertificateInput, len(in))
	for i, c := range in {
		out[i] = coach.CertificateInput{FileID: c.FileID, Title: c.Title}
	}
	return out
}

// UpdateCoachRequest is the body of PATCH /coaches/me. A null photo_file_id
// removes the photo; a provided sport_ids / certificates replaces the list.
type UpdateCoachRequest struct {
	Bio             *string                  `json:"bio"`
	TeachingContent *string                  `json:"teaching_content"`
	TargetAudience  *string                  `json:"target_audience"`
	Experience      *string                  `json:"experience"`
	FeeInfo         *string                  `json:"fee_info"`
	PhotoFileID     request.Nullable[string] `json:"photo_file_id"`
	SportIDs        *[]string                `json:"sport_ids"`
	Certificates    *[]CertificateRequest    `json:"certificates"`
}

func (r UpdateCoachRequest) ToUpdate() coach.ProfileUpdate {
	u := coach.ProfileUpdate{
		Bio: r.Bio, TeachingContent: r.TeachingContent, TargetAudience: r.TargetAudience,
		Experience: r.Experience, FeeInfo: r.FeeInfo, SportIDs: r.SportIDs,
	}
	if r.PhotoFileID.Set {
		if r.PhotoFileID.Value == nil {
			u.ClearPhoto = true
		} else {
			u.PhotoFileID = r.PhotoFileID.Value
		}
	}
	if r.Certificates != nil {
		certs := toCertInputs(*r.Certificates)
		u.Certificates = &certs
	}
	return u
}

type SportResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type CertificateResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	FileURL string `json:"file_url"`
}

// RatingResponse is the public rating summary from visible reviews.
type RatingResponse struct {
	Average *float64 `json:"average"`
	Count   int      `json:"count"`
}

type GroupBriefResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type CoachResponse struct {
	ID                  string                `json:"id"`
	DisplayName         string                `json:"display_name"`
	Avatar              *string               `json:"avatar"`
	AvatarThumbnail     *string               `json:"avatar_thumbnail"`
	Status              string                `json:"status"`
	Bio                 string                `json:"bio"`
	TeachingContent     string                `json:"teaching_content"`
	TargetAudience      string                `json:"target_audience"`
	Experience          string                `json:"experience"`
	FeeInfo             string                `json:"fee_info"`
	Photo               *string               `json:"photo"`
	PhotoThumbnail      *string               `json:"photo_thumbnail"`
	Sports              []SportResponse       `json:"sports"`
	Certificates        []CertificateResponse `json:"certificates"`
	CertificateProvided bool                  `json:"certificate_provided"`
	Group               *GroupBriefResponse   `json:"group"`
	Rating              RatingResponse        `json:"rating"`
	RejectReason        *string               `json:"reject_reason"`
	AppliedAt           time.Time             `json:"applied_at"`
	ReviewedAt          *time.Time            `json:"reviewed_at"`
}

func NewCoachResponse(c *coach.Coach) CoachResponse {
	avatar, avatarThumb := file.URLs(c.Avatar)
	photo, photoThumb := file.URLs(c.PhotoFileID)
	sports := make([]SportResponse, len(c.Sports))
	for i, s := range c.Sports {
		sports[i] = SportResponse{ID: s.ID, Code: s.Code, Name: s.Name}
	}
	certs := make([]CertificateResponse, len(c.Certificates))
	for i, ct := range c.Certificates {
		certs[i] = CertificateResponse{ID: ct.ID, Title: ct.Title, FileURL: file.FileURL(ct.FileID)}
	}
	var group *GroupBriefResponse
	if c.Group != nil {
		group = &GroupBriefResponse{ID: c.Group.ID, Name: c.Group.Name, Role: c.Group.Role}
	}
	return CoachResponse{
		ID: c.UserID, DisplayName: c.DisplayName, Avatar: avatar, AvatarThumbnail: avatarThumb,
		Status: c.Status, Bio: c.Bio, TeachingContent: c.TeachingContent, TargetAudience: c.TargetAudience,
		Experience: c.Experience, FeeInfo: c.FeeInfo, Photo: photo, PhotoThumbnail: photoThumb,
		Sports: sports, Certificates: certs, CertificateProvided: c.CertificateProvided(), Group: group,
		Rating:       RatingResponse{Average: c.RatingAverage, Count: c.RatingCount},
		RejectReason: c.RejectReason, AppliedAt: c.AppliedAt.UTC(), ReviewedAt: utcPtr(c.ReviewedAt),
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// ListCoachesRequest holds query parameters for GET /coaches.
type ListCoachesRequest struct {
	request.ListParams
	SportID string `form:"sport_id" binding:"omitempty,uuid"`
	Keyword string `form:"keyword" binding:"omitempty,max=50"`
}

// ListApplicationsRequest holds query parameters for GET /admin/coach-applications.
type ListApplicationsRequest struct {
	request.ListParams
	Status string `form:"status" binding:"omitempty,oneof=pending approved rejected suspended"`
}

// ReviewRequest is the body of POST /admin/coach-applications/{id}/review.
type ReviewRequest struct {
	Decision string `json:"decision" binding:"required,oneof=approve reject"`
	Reason   string `json:"reason" binding:"max=500"`
}

// ---- Groups ----

type CreateGroupRequest struct {
	Name                string `json:"name" binding:"required"`
	AllowPrivateCourses bool   `json:"allow_private_courses"`
}

type UpdateGroupRequest struct {
	Name                *string `json:"name"`
	AllowPrivateCourses *bool   `json:"allow_private_courses"`
}

type ListGroupsRequest struct {
	request.ListParams
	Keyword string `form:"keyword" binding:"omitempty,max=50"`
}

type MemberResponse struct {
	UserID          string    `json:"user_id"`
	DisplayName     string    `json:"display_name"`
	Avatar          *string   `json:"avatar"`
	AvatarThumbnail *string   `json:"avatar_thumbnail"`
	Role            string    `json:"role"`
	Status          string    `json:"status"`
	JoinedAt        time.Time `json:"joined_at"`
}

func NewMemberResponse(m *coach.Member) MemberResponse {
	avatar, thumb := file.URLs(m.Avatar)
	return MemberResponse{
		UserID: m.UserID, DisplayName: m.DisplayName, Avatar: avatar, AvatarThumbnail: thumb,
		Role: m.Role, Status: m.Status, JoinedAt: m.JoinedAt.UTC(),
	}
}

type GroupResponse struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	AllowPrivateCourses bool             `json:"allow_private_courses"`
	MemberCount         int              `json:"member_count"`
	Members             []MemberResponse `json:"members"`
	CreatedAt           time.Time        `json:"created_at"`
}

func NewGroupResponse(g *coach.Group) GroupResponse {
	members := make([]MemberResponse, len(g.Members))
	for i, m := range g.Members {
		members[i] = NewMemberResponse(m)
	}
	return GroupResponse{
		ID: g.ID, Name: g.Name, AllowPrivateCourses: g.AllowPrivateCourses, MemberCount: g.MemberCount,
		Members: members, CreatedAt: g.CreatedAt.UTC(),
	}
}

type InviteRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
	Role   string `json:"role" binding:"required,oneof=coach assistant"`
}

type InvitationResponse struct {
	ID          string     `json:"id"`
	GroupID     string     `json:"group_id"`
	GroupName   string     `json:"group_name"`
	InviteeID   string     `json:"invitee_id"`
	InviteeName string     `json:"invitee_name"`
	InvitedBy   *string    `json:"invited_by"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	RespondedAt *time.Time `json:"responded_at"`
}

func NewInvitationResponse(i *coach.Invitation) InvitationResponse {
	return InvitationResponse{
		ID: i.ID, GroupID: i.GroupID, GroupName: i.GroupName, InviteeID: i.InviteeID, InviteeName: i.InviteeName,
		InvitedBy: i.InvitedBy, Role: i.Role, Status: i.Status, CreatedAt: i.CreatedAt.UTC(), RespondedAt: utcPtr(i.RespondedAt),
	}
}

type ListInvitationsRequest struct {
	request.ListParams
	Status string `form:"status" binding:"omitempty,oneof=pending accepted declined cancelled"`
}

type UpdateMemberRequest struct {
	Role   *string `json:"role" binding:"omitempty,oneof=leader coach assistant"`
	Status *string `json:"status" binding:"omitempty,oneof=active inactive"`
}

// ---- URI params ----

type GroupMemberURI struct {
	GroupID string `uri:"id" binding:"required,uuid"`
	UserID  string `uri:"user_id" binding:"required,uuid"`
}

type GroupInvitationURI struct {
	GroupID      string `uri:"id" binding:"required,uuid"`
	InvitationID string `uri:"invitation_id" binding:"required,uuid"`
}

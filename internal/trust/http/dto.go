package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/trust"
)

// ---- Requests ----

// ReviewRequest is the body of POST /enrollments/{id}/review. rating is 0-5 in
// steps of 0.5; comment is optional and at most 50 characters.
type ReviewRequest struct {
	CoachID *string  `json:"coach_id" binding:"omitempty,uuid"`
	Rating  *float64 `json:"rating" binding:"required,min=0,max=5"`
	Comment string   `json:"comment" binding:"max=50"`
}

type CreateComplaintRequest struct {
	EnrollmentID string  `json:"enrollment_id" binding:"required,uuid"`
	TargetUserID *string `json:"target_user_id" binding:"omitempty,uuid"`
	Category     string  `json:"category" binding:"required"`
	Description  string  `json:"description" binding:"required,max=1000"`
}

type UpdateComplaintRequest struct {
	Status string `json:"status" binding:"required,oneof=open in_review resolved dismissed"`
	Note   string `json:"note" binding:"max=1000"`
}

type ComplaintNoteRequest struct {
	Note string `json:"note" binding:"required,max=1000"`
}

type ReviewVisibilityRequest struct {
	IsVisible *bool  `json:"is_visible" binding:"required"`
	Reason    string `json:"reason" binding:"max=200"`
}

type ReputationEventRequest struct {
	Delta       int     `json:"delta" binding:"required,min=-100,max=100"`
	Reason      string  `json:"reason" binding:"required,max=200"`
	ComplaintID *string `json:"complaint_id" binding:"omitempty,uuid"`
}

type SuspendRequest struct {
	Reason string `json:"reason" binding:"required,max=1000"`
}

type LiftSuspensionRequest struct {
	Note string `json:"note" binding:"max=1000"`
}

type ListReviewsRequest struct {
	request.ListParams
	CoachID string `form:"coach_id" binding:"omitempty,uuid"`
	Hidden  bool   `form:"hidden"`
}

type ListComplaintsRequest struct {
	request.ListParams
	Status   string `form:"status" binding:"omitempty,oneof=open in_review resolved dismissed"`
	Category string `form:"category"`
	TargetID string `form:"target_user_id" binding:"omitempty,uuid"`
}

// ---- Responses ----

type PersonRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ReviewResponse struct {
	ID           string     `json:"id"`
	EnrollmentID string     `json:"enrollment_id,omitempty"`
	CoachID      string     `json:"coach_id"`
	CourseID     string     `json:"course_id"`
	CourseTitle  string     `json:"course_title"`
	Reviewer     PersonRef  `json:"reviewer"`
	Rating       float64    `json:"rating"`
	Comment      string     `json:"comment"`
	IsVisible    *bool      `json:"is_visible,omitempty"`
	HiddenReason *string    `json:"hidden_reason,omitempty"`
	HiddenAt     *time.Time `json:"hidden_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// NewReviewResponse builds the public view; admin adds moderation fields.
func NewReviewResponse(r *trust.Review, admin bool) ReviewResponse {
	resp := ReviewResponse{
		ID: r.ID, CoachID: r.CoachID, CourseID: r.CourseID, CourseTitle: r.CourseTitle,
		Reviewer: PersonRef{ID: r.ReviewerID, Name: r.ReviewerName}, Rating: r.Rating, Comment: r.Comment, CreatedAt: r.CreatedAt.UTC(),
	}
	if admin {
		resp.EnrollmentID = r.EnrollmentID
		resp.IsVisible = &r.IsVisible
		resp.HiddenReason = r.HiddenReason
		if r.HiddenAt != nil {
			t := r.HiddenAt.UTC()
			resp.HiddenAt = &t
		}
	}
	return resp
}

type RatingSummaryResponse struct {
	Average *float64 `json:"average"`
	Count   int      `json:"count"`
}

type CoachReviewsResponse struct {
	Summary  RatingSummaryResponse `json:"summary"`
	Items    []ReviewResponse      `json:"items"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Total    int                   `json:"total"`
}

type ComplaintEventResponse struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	ActorName  *string   `json:"actor_name,omitempty"`
	FromStatus *string   `json:"from_status"`
	ToStatus   *string   `json:"to_status"`
	Note       string    `json:"note"`
	IsInternal bool      `json:"is_internal"`
	CreatedAt  time.Time `json:"created_at"`
}

func NewComplaintEventResponse(e *trust.ComplaintEvent, admin bool) ComplaintEventResponse {
	resp := ComplaintEventResponse{
		ID: e.ID, Type: e.Type, FromStatus: e.FromStatus, ToStatus: e.ToStatus, Note: e.Note, IsInternal: e.IsInternal, CreatedAt: e.CreatedAt.UTC(),
	}
	if admin {
		resp.ActorName = e.ActorName
	}
	return resp
}

type ComplaintResponse struct {
	ID             string                   `json:"id"`
	Reporter       PersonRef                `json:"reporter"`
	ReporterRole   string                   `json:"reporter_role"`
	Target         PersonRef                `json:"target"`
	EnrollmentID   string                   `json:"enrollment_id"`
	CourseID       string                   `json:"course_id"`
	CourseTitle    string                   `json:"course_title"`
	Category       string                   `json:"category"`
	Description    string                   `json:"description"`
	Status         string                   `json:"status"`
	ResolutionNote *string                  `json:"resolution_note"`
	ResolvedAt     *time.Time               `json:"resolved_at"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
	Events         []ComplaintEventResponse `json:"events,omitempty"`
}

func NewComplaintResponse(k *trust.Complaint, admin bool) ComplaintResponse {
	resp := ComplaintResponse{
		ID: k.ID, Reporter: PersonRef{ID: k.ReporterID, Name: k.ReporterName}, ReporterRole: k.ReporterRole,
		Target: PersonRef{ID: k.TargetUserID, Name: k.TargetName}, EnrollmentID: k.EnrollmentID, CourseID: k.CourseID,
		CourseTitle: k.CourseTitle, Category: k.Category, Description: k.Description, Status: k.Status,
		ResolutionNote: k.ResolutionNote, CreatedAt: k.CreatedAt.UTC(), UpdatedAt: k.UpdatedAt.UTC(),
	}
	if k.ResolvedAt != nil {
		t := k.ResolvedAt.UTC()
		resp.ResolvedAt = &t
	}
	for _, e := range k.Events {
		resp.Events = append(resp.Events, NewComplaintEventResponse(e, admin))
	}
	return resp
}

type ReputationEventResponse struct {
	ID          string    `json:"id"`
	Delta       int       `json:"delta"`
	Reason      string    `json:"reason"`
	ComplaintID *string   `json:"complaint_id"`
	CreatedBy   *string   `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewReputationEventResponse(e *trust.ReputationEvent) ReputationEventResponse {
	return ReputationEventResponse{ID: e.ID, Delta: e.Delta, Reason: e.Reason, ComplaintID: e.ComplaintID, CreatedBy: e.CreatedBy, CreatedAt: e.CreatedAt.UTC()}
}

type AffectedCourseResponse struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	Status              string `json:"status"`
	OtherActiveTeachers int    `json:"other_active_teachers"`
	UpcomingSessions    int    `json:"upcoming_sessions"`
	ActiveEnrollments   int    `json:"active_enrollments"`
}

type AffectedSessionResponse struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	CourseTitle string    `json:"course_title"`
	StartAt     time.Time `json:"start_at"`
	EndAt       time.Time `json:"end_at"`
}

type AffectedEnrollmentResponse struct {
	ID            string    `json:"id"`
	CourseID      string    `json:"course_id"`
	CourseTitle   string    `json:"course_title"`
	Student       PersonRef `json:"student"`
	PaymentStatus string    `json:"payment_status"`
	AmountDue     int       `json:"amount_due"`
}

type ImpactResponse struct {
	Courses     []AffectedCourseResponse     `json:"courses"`
	Sessions    []AffectedSessionResponse    `json:"sessions"`
	Enrollments []AffectedEnrollmentResponse `json:"enrollments"`
}

func NewImpactResponse(i *trust.SuspensionImpact) *ImpactResponse {
	if i == nil {
		return nil
	}
	out := &ImpactResponse{
		Courses: []AffectedCourseResponse{}, Sessions: []AffectedSessionResponse{}, Enrollments: []AffectedEnrollmentResponse{},
	}
	for _, c := range i.Courses {
		out.Courses = append(out.Courses, AffectedCourseResponse{ID: c.ID, Title: c.Title, Status: c.Status,
			OtherActiveTeachers: c.OtherActiveTeachers, UpcomingSessions: c.UpcomingSessions, ActiveEnrollments: c.ActiveEnrollments})
	}
	for _, s := range i.Sessions {
		out.Sessions = append(out.Sessions, AffectedSessionResponse{ID: s.ID, CourseID: s.CourseID, CourseTitle: s.CourseTitle, StartAt: s.StartAt.UTC(), EndAt: s.EndAt.UTC()})
	}
	for _, e := range i.Enrollments {
		out.Enrollments = append(out.Enrollments, AffectedEnrollmentResponse{ID: e.ID, CourseID: e.CourseID, CourseTitle: e.CourseTitle,
			Student: PersonRef{ID: e.StudentID, Name: e.StudentName}, PaymentStatus: e.PaymentStatus, AmountDue: e.AmountDue})
	}
	return out
}

type SuspensionResponse struct {
	ID        string          `json:"id"`
	CoachID   string          `json:"coach_id"`
	CoachName string          `json:"coach_name"`
	Reason    string          `json:"reason"`
	Status    string          `json:"status"`
	CreatedBy *string         `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
	LiftedAt  *time.Time      `json:"lifted_at"`
	LiftNote  *string         `json:"lift_note"`
	Impact    *ImpactResponse `json:"impact,omitempty"`
}

func NewSuspensionResponse(s *trust.Suspension) SuspensionResponse {
	resp := SuspensionResponse{
		ID: s.ID, CoachID: s.CoachID, CoachName: s.CoachName, Reason: s.Reason, Status: s.Status, CreatedBy: s.CreatedBy,
		CreatedAt: s.CreatedAt.UTC(), LiftNote: s.LiftNote, Impact: NewImpactResponse(s.Impact),
	}
	if s.LiftedAt != nil {
		t := s.LiftedAt.UTC()
		resp.LiftedAt = &t
	}
	return resp
}

type ReputationResponse struct {
	CoachID          string                    `json:"coach_id"`
	CoachName        string                    `json:"coach_name"`
	CoachStatus      string                    `json:"coach_status"`
	BaseScore        int                       `json:"base_score"`
	TotalAdjustment  int                       `json:"total_adjustment"`
	Score            int                       `json:"score"`
	Rating           RatingSummaryResponse     `json:"rating"`
	OpenComplaints   int                       `json:"open_complaints"`
	TotalComplaints  int                       `json:"total_complaints"`
	ActiveSuspension *SuspensionResponse       `json:"active_suspension"`
	Events           []ReputationEventResponse `json:"events"`
}

func NewReputationResponse(r *trust.Reputation) ReputationResponse {
	resp := ReputationResponse{
		CoachID: r.CoachID, CoachName: r.CoachName, CoachStatus: r.CoachStatus, BaseScore: r.BaseScore,
		TotalAdjustment: r.TotalAdjustment, Score: r.Score, Rating: RatingSummaryResponse{Average: r.Rating.Average, Count: r.Rating.Count},
		OpenComplaints: r.OpenComplaints, TotalComplaints: r.TotalComplaints, Events: make([]ReputationEventResponse, len(r.Events)),
	}
	for i, e := range r.Events {
		resp.Events[i] = NewReputationEventResponse(e)
	}
	if r.ActiveSuspension != nil {
		s := NewSuspensionResponse(r.ActiveSuspension)
		resp.ActiveSuspension = &s
	}
	return resp
}

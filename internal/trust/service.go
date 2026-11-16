package trust

import (
	"context"
	"errors"
	"log"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/course"
	"github.com/nekogravitycat/court-booking-backend/internal/notification"
)

// ReviewInput is a student's review of a coach.
type ReviewInput struct {
	CoachID *string // Required when the course has several teaching coaches
	Rating  float64
	Comment string
}

// ComplaintInput is a new complaint about the other party of an enrollment.
type ComplaintInput struct {
	EnrollmentID string
	TargetUserID *string // Required when a student complains about a course with several coaches
	Category     string
	Description  string
}

// Viewer identifies who is reading.
type Viewer struct {
	UserID  string
	IsAdmin bool
}

// Service handles reviews, complaints, reputation and suspensions. Admin-only
// methods rely on the router's admin middleware for authorization.
type Service interface {
	CreateReview(ctx context.Context, enrollmentID, userID string, in ReviewInput) (*Review, error)
	ListCoachReviews(ctx context.Context, coachID string, f ReviewFilter) ([]*Review, ReviewSummary, int, error)
	ListMyReviews(ctx context.Context, enrollmentID string, v Viewer) ([]*Review, error)
	AdminListReviews(ctx context.Context, f ReviewFilter) ([]*Review, int, error)
	SetReviewVisibility(ctx context.Context, id string, visible bool, reason, adminID string) (*Review, error)

	CreateComplaint(ctx context.Context, userID string, in ComplaintInput) (*Complaint, error)
	GetComplaint(ctx context.Context, id string, v Viewer) (*Complaint, error)
	ListComplaints(ctx context.Context, v Viewer, f ComplaintFilter) ([]*Complaint, int, error)
	UpdateComplaint(ctx context.Context, id, adminID, status, note string) (*Complaint, error)
	AddComplaintNote(ctx context.Context, id, adminID, note string) (*ComplaintEvent, error)

	GetReputation(ctx context.Context, coachID string) (*Reputation, error)
	AdjustReputation(ctx context.Context, coachID, adminID string, delta int, reason string, complaintID *string) (*ReputationEvent, error)
	Suspend(ctx context.Context, coachID, adminID, reason string) (*Suspension, error)
	LiftSuspension(ctx context.Context, id, adminID, note string) (*Suspension, error)
	ListSuspensions(ctx context.Context, coachID string) ([]*Suspension, error)
}

type service struct {
	repo       Repository
	courses    course.Repository
	attendance course.AttendanceService
	notifier   notification.Service
}

func NewService(repo Repository, courses course.Repository, attendance course.AttendanceService, notifier notification.Service) Service {
	return &service{repo: repo, courses: courses, attendance: attendance, notifier: notifier}
}

func (s *service) notify(ctx context.Context, userID, typ, title, content, refType, refID string) {
	rt, ri := refType, refID
	if err := s.notifier.Notify(ctx, &notification.Notification{
		UserID: userID, Type: typ, Title: title, Content: content, RefType: &rt, RefID: &ri,
	}); err != nil {
		log.Printf("warning: failed to send %s notification: %v", typ, err)
	}
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// ---- Reviews ----

func (s *service) CreateReview(ctx context.Context, enrollmentID, userID string, in ReviewInput) (*Review, error) {
	if _, err := uuid.Parse(enrollmentID); err != nil {
		return nil, ErrEnrollmentMissing
	}
	if math.IsNaN(in.Rating) || in.Rating < 0 || in.Rating > 5 || in.Rating*2 != math.Trunc(in.Rating*2) {
		return nil, ErrInvalidRating
	}
	comment := strings.TrimSpace(in.Comment)
	if runes(comment) > MaxComment {
		return nil, ErrCommentTooLong
	}

	e, err := s.courses.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return nil, enrollmentErr(err)
	}
	if e.UserID != userID {
		return nil, ErrEnrollmentMissing
	}
	c, err := s.courses.Get(ctx, e.CourseID)
	if err != nil {
		return nil, err
	}

	coachID, err := pickTeacher(c, in.CoachID)
	if err != nil {
		return nil, err
	}
	attended, err := s.attendance.HasAttended(ctx, enrollmentID)
	if err != nil {
		return nil, err
	}
	if !attended {
		return nil, ErrNotAttended
	}

	rv, err := s.repo.CreateReview(ctx, &Review{
		EnrollmentID: enrollmentID, CoachID: coachID, CourseID: c.ID, ReviewerID: userID, Rating: in.Rating, Comment: comment,
	})
	if err != nil {
		return nil, err
	}
	s.notify(ctx, coachID, NotifReviewReceived, "New review",
		"A student reviewed you for \""+c.Title+"\".", RefReview, rv.ID)
	return rv, nil
}

// enrollmentErr maps only "not found" to a 404; real failures stay internal errors.
func enrollmentErr(err error) error {
	if errors.Is(err, course.ErrEnrollmentNotFound) {
		return ErrEnrollmentMissing
	}
	return err
}

// pickTeacher resolves which coach a review or complaint is about.
func pickTeacher(c *course.Course, requested *string) (string, error) {
	ids := make([]string, len(c.Teachers))
	for i, t := range c.Teachers {
		ids[i] = t.UserID
	}
	if requested != nil {
		if !slices.Contains(ids, *requested) {
			return "", ErrNotCourseCoach
		}
		return *requested, nil
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	return "", ErrCoachRequired
}

func (s *service) ListCoachReviews(ctx context.Context, coachID string, f ReviewFilter) ([]*Review, ReviewSummary, int, error) {
	if _, err := uuid.Parse(coachID); err != nil {
		return nil, ReviewSummary{}, 0, ErrCoachNotFound
	}
	if _, _, err := s.repo.CoachInfo(ctx, coachID); err != nil {
		return nil, ReviewSummary{}, 0, err
	}
	f.CoachID, f.VisibleOnly = coachID, true
	items, total, err := s.repo.ListReviews(ctx, f)
	if err != nil {
		return nil, ReviewSummary{}, 0, err
	}
	sum, err := s.repo.ReviewSummary(ctx, coachID)
	return items, sum, total, err
}

func (s *service) ListMyReviews(ctx context.Context, enrollmentID string, v Viewer) ([]*Review, error) {
	if _, err := uuid.Parse(enrollmentID); err != nil {
		return nil, ErrEnrollmentMissing
	}
	e, err := s.courses.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return nil, enrollmentErr(err)
	}
	if e.UserID != v.UserID {
		return nil, ErrEnrollmentMissing
	}
	items, _, err := s.repo.ListReviews(ctx, ReviewFilter{EnrollmentID: enrollmentID, ReviewerID: v.UserID, PageSize: 100})
	return items, err
}

func (s *service) AdminListReviews(ctx context.Context, f ReviewFilter) ([]*Review, int, error) {
	return s.repo.ListReviews(ctx, f)
}

func (s *service) SetReviewVisibility(ctx context.Context, id string, visible bool, reason, adminID string) (*Review, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrReviewNotFound
	}
	reason = strings.TrimSpace(reason)
	if !visible && (reason == "" || runes(reason) > MaxReason) {
		return nil, ErrHideReason
	}
	return s.repo.SetReviewVisibility(ctx, id, visible, reason, adminID)
}

// ---- Complaints ----

func (s *service) CreateComplaint(ctx context.Context, userID string, in ComplaintInput) (*Complaint, error) {
	if _, err := uuid.Parse(in.EnrollmentID); err != nil {
		return nil, ErrEnrollmentMissing
	}
	if !slices.Contains(Categories, in.Category) {
		return nil, ErrInvalidCategory
	}
	desc := strings.TrimSpace(in.Description)
	if desc == "" || runes(desc) > MaxNote {
		return nil, ErrInvalidComplaint
	}

	e, err := s.courses.GetEnrollment(ctx, in.EnrollmentID)
	if err != nil {
		return nil, enrollmentErr(err)
	}
	c, err := s.courses.Get(ctx, e.CourseID)
	if err != nil {
		return nil, err
	}

	var target string
	if e.UserID == userID {
		// A student complains about one of the course's coaches.
		if target, err = pickTeacher(c, in.TargetUserID); err != nil {
			if err == ErrCoachRequired {
				return nil, ErrTargetRequired
			}
			return nil, ErrInvalidTarget
		}
	} else {
		// A course manager complains about the enrolled student.
		ok, err := s.courses.IsManager(ctx, e.CourseID, userID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrEnrollmentMissing
		}
		if in.TargetUserID != nil && *in.TargetUserID != e.UserID {
			return nil, ErrInvalidTarget
		}
		target = e.UserID
	}

	k, err := s.repo.CreateComplaint(ctx, &Complaint{
		ReporterID: userID, TargetUserID: target, EnrollmentID: in.EnrollmentID, Category: in.Category, Description: desc,
	})
	if err != nil {
		return nil, err
	}
	admins, err := s.repo.AdminIDs(ctx)
	if err != nil {
		log.Printf("warning: failed to list admins for complaint: %v", err)
	}
	for _, id := range admins {
		s.notify(ctx, id, NotifComplaintFiled, "New complaint",
			k.ReporterName+" filed a complaint about "+k.TargetName+" ("+k.Category+").", RefComplaint, k.ID)
	}
	return k, nil
}

func (s *service) GetComplaint(ctx context.Context, id string, v Viewer) (*Complaint, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrComplaintNotFound
	}
	k, err := s.repo.GetComplaint(ctx, id)
	if err != nil {
		return nil, err
	}
	if !v.IsAdmin && k.ReporterID != v.UserID {
		return nil, ErrComplaintNotFound
	}
	// Reporters never see internal staff notes.
	if k.Events, err = s.repo.ListEvents(ctx, id, v.IsAdmin); err != nil {
		return nil, err
	}
	return k, nil
}

func (s *service) ListComplaints(ctx context.Context, v Viewer, f ComplaintFilter) ([]*Complaint, int, error) {
	if !v.IsAdmin {
		f.ReporterID = v.UserID
		f.TargetID = ""
	}
	return s.repo.ListComplaints(ctx, f)
}

func (s *service) UpdateComplaint(ctx context.Context, id, adminID, status, note string) (*Complaint, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrComplaintNotFound
	}
	switch status {
	case StatusOpen, StatusInReview, StatusResolved, StatusDismissed:
	default:
		return nil, ErrInvalidStatus
	}
	note = strings.TrimSpace(note)
	if runes(note) > MaxNote || ((status == StatusResolved || status == StatusDismissed) && note == "") {
		return nil, ErrNoteRequired
	}
	k, err := s.repo.UpdateComplaint(ctx, id, adminID, status, note)
	if err != nil {
		return nil, err
	}
	msg := "Your complaint about " + k.TargetName + " is now " + status + "."
	if note != "" {
		msg += " " + note
	}
	s.notify(ctx, k.ReporterID, NotifComplaintUpdated, "Complaint updated", msg, RefComplaint, k.ID)
	if k.Events, err = s.repo.ListEvents(ctx, id, true); err != nil {
		return nil, err
	}
	return k, nil
}

func (s *service) AddComplaintNote(ctx context.Context, id, adminID, note string) (*ComplaintEvent, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrComplaintNotFound
	}
	note = strings.TrimSpace(note)
	if note == "" || runes(note) > MaxNote {
		return nil, ErrNoteRequired
	}
	return s.repo.AddNote(ctx, id, adminID, note)
}

// ---- Reputation and suspension ----

func (s *service) GetReputation(ctx context.Context, coachID string) (*Reputation, error) {
	if _, err := uuid.Parse(coachID); err != nil {
		return nil, ErrCoachNotFound
	}
	name, status, err := s.repo.CoachInfo(ctx, coachID)
	if err != nil {
		return nil, err
	}
	rep := &Reputation{CoachID: coachID, CoachName: name, CoachStatus: status, BaseScore: BaseScore}
	if rep.TotalAdjustment, err = s.repo.SumReputation(ctx, coachID); err != nil {
		return nil, err
	}
	rep.Score = rep.BaseScore + rep.TotalAdjustment
	if rep.Events, err = s.repo.ListReputationEvents(ctx, coachID, 100); err != nil {
		return nil, err
	}
	if rep.Rating, err = s.repo.ReviewSummary(ctx, coachID); err != nil {
		return nil, err
	}
	if rep.OpenComplaints, rep.TotalComplaints, err = s.repo.ComplaintCounts(ctx, coachID); err != nil {
		return nil, err
	}
	if rep.ActiveSuspension, err = s.repo.ActiveSuspension(ctx, coachID); err != nil {
		return nil, err
	}
	if rep.ActiveSuspension != nil {
		if rep.ActiveSuspension.Impact, err = s.repo.Impact(ctx, coachID); err != nil {
			return nil, err
		}
	}
	return rep, nil
}

func (s *service) AdjustReputation(ctx context.Context, coachID, adminID string, delta int, reason string, complaintID *string) (*ReputationEvent, error) {
	if _, err := uuid.Parse(coachID); err != nil {
		return nil, ErrCoachNotFound
	}
	if delta == 0 || delta < -100 || delta > 100 {
		return nil, ErrInvalidDelta
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || runes(reason) > MaxReason {
		return nil, ErrReasonRequired
	}
	if _, _, err := s.repo.CoachInfo(ctx, coachID); err != nil {
		return nil, err
	}
	if complaintID != nil {
		if _, err := uuid.Parse(*complaintID); err != nil {
			return nil, ErrComplaintNotFound
		}
		ok, err := s.repo.ComplaintExistsFor(ctx, *complaintID, coachID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrComplaintNotFound
		}
	}
	e := &ReputationEvent{CoachID: coachID, Delta: delta, Reason: reason, ComplaintID: complaintID}
	if adminID != "" {
		e.CreatedBy = &adminID
	}
	return s.repo.AddReputationEvent(ctx, e)
}

func (s *service) Suspend(ctx context.Context, coachID, adminID, reason string) (*Suspension, error) {
	if _, err := uuid.Parse(coachID); err != nil {
		return nil, ErrCoachNotFound
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || runes(reason) > MaxNote {
		return nil, ErrReasonRequired
	}
	sus, err := s.repo.Suspend(ctx, coachID, adminID, reason)
	if err != nil {
		return nil, err
	}
	// Nothing is cancelled automatically: staff receive the list of what the
	// suspension affects and handle each case by hand.
	if sus.Impact, err = s.repo.Impact(ctx, coachID); err != nil {
		return nil, err
	}
	s.notify(ctx, coachID, NotifCoachSuspended, "Account suspended",
		"Your coach account was suspended: "+reason, RefSuspension, sus.ID)
	return sus, nil
}

func (s *service) LiftSuspension(ctx context.Context, id, adminID, note string) (*Suspension, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrSuspensionMissing
	}
	note = strings.TrimSpace(note)
	if runes(note) > MaxNote {
		return nil, ErrNoteRequired
	}
	sus, err := s.repo.Lift(ctx, id, adminID, note)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, sus.CoachID, NotifSuspensionLifted, "Suspension lifted",
		"Your coach account is active again.", RefSuspension, sus.ID)
	return sus, nil
}

func (s *service) ListSuspensions(ctx context.Context, coachID string) ([]*Suspension, error) {
	if _, err := uuid.Parse(coachID); err != nil {
		return nil, ErrCoachNotFound
	}
	if _, _, err := s.repo.CoachInfo(ctx, coachID); err != nil {
		return nil, err
	}
	items, err := s.repo.ListSuspensions(ctx, coachID)
	if err != nil {
		return nil, err
	}
	for _, sus := range items {
		if sus.Status == "active" {
			if sus.Impact, err = s.repo.Impact(ctx, coachID); err != nil {
				return nil, err
			}
		}
	}
	return items, nil
}

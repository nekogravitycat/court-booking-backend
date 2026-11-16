package course

import (
	"context"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/notification"
)

// TransferInput is a student's bank transfer report.
type TransferInput struct {
	Last5         string
	Amount        *int // Defaults to the amount due
	TransferredAt *time.Time
	Note          string
}

// CancelInput is a cancellation request. SessionID scopes it to one session
// (for example a coach no-show) instead of the whole enrollment.
type CancelInput struct {
	SessionID *string
	Reason    string
}

// ResolveInput is the coach's decision on a cancellation request.
type ResolveInput struct {
	Resolution   string
	RefundAmount *int
	Note         string
}

// PaymentService handles payment tracking and cancellation records. The coach
// is the payee, so the course managers review reports and resolve requests;
// system admins can do the same as an oversight path.
type PaymentService interface {
	ReportTransfer(ctx context.Context, enrollmentID string, userID string, in TransferInput) (*TransferReport, error)
	ListEnrollmentReports(ctx context.Context, enrollmentID string, v Viewer, f TransferReportFilter) ([]*TransferReport, int, error)
	ListReports(ctx context.Context, v Viewer, f TransferReportFilter) ([]*TransferReport, int, error)
	ReviewReport(ctx context.Context, id string, v Viewer, approve bool, note string) (*TransferReport, error)
	SetPayment(ctx context.Context, enrollmentID string, v Viewer, to string) (*Enrollment, error)

	RequestCancellation(ctx context.Context, enrollmentID string, v Viewer, in CancelInput) (*CancellationRequest, error)
	ListEnrollmentCancellations(ctx context.Context, enrollmentID string, v Viewer, f CancellationFilter) ([]*CancellationRequest, int, error)
	ListCancellations(ctx context.Context, v Viewer, f CancellationFilter) ([]*CancellationRequest, int, error)
	ResolveCancellation(ctx context.Context, id string, v Viewer, in ResolveInput) (*CancellationRequest, error)
}

type paymentService struct {
	repo     Repository
	pay      PaymentRepository
	notifier notification.Service
}

func NewPaymentService(repo Repository, pay PaymentRepository, notifier notification.Service) PaymentService {
	return &paymentService{repo: repo, pay: pay, notifier: notifier}
}

var last5Pattern = regexp.MustCompile(`^[0-9]{5}$`)

func (s *paymentService) notify(ctx context.Context, userID, typ, title, content, refType, refID string) {
	rt, ri := refType, refID
	if err := s.notifier.Notify(ctx, &notification.Notification{
		UserID: userID, Type: typ, Title: title, Content: content, RefType: &rt, RefID: &ri,
	}); err != nil {
		log.Printf("warning: failed to send %s notification: %v", typ, err)
	}
}

func (s *paymentService) notifyManagers(ctx context.Context, courseID, skip, typ, title, content, refType, refID string) {
	ids, err := s.repo.ManagerIDs(ctx, courseID)
	if err != nil {
		log.Printf("warning: failed to list course managers: %v", err)
		return
	}
	for _, id := range ids {
		if id != skip {
			s.notify(ctx, id, typ, title, content, refType, refID)
		}
	}
}

func (s *paymentService) isManager(ctx context.Context, courseID string, v Viewer) (bool, error) {
	if v.IsAdmin {
		return true, nil
	}
	if v.UserID == "" {
		return false, nil
	}
	return s.repo.IsManager(ctx, courseID, v.UserID)
}

// loadEnrollment returns the enrollment and whether the viewer owns it or manages its course.
func (s *paymentService) loadEnrollment(ctx context.Context, id string, v Viewer) (e *Enrollment, owner, manager bool, err error) {
	if _, perr := uuid.Parse(id); perr != nil {
		return nil, false, false, ErrEnrollmentNotFound
	}
	e, err = s.repo.GetEnrollment(ctx, id)
	if err != nil {
		return nil, false, false, err
	}
	owner = v.UserID != "" && e.UserID == v.UserID
	if manager, err = s.isManager(ctx, e.CourseID, v); err != nil {
		return nil, false, false, err
	}
	if !owner && !manager {
		return nil, false, false, ErrEnrollmentNotFound
	}
	return e, owner, manager, nil
}

func (s *paymentService) ReportTransfer(ctx context.Context, enrollmentID, userID string, in TransferInput) (*TransferReport, error) {
	if _, err := uuid.Parse(enrollmentID); err != nil {
		return nil, ErrEnrollmentNotFound
	}
	e, err := s.repo.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return nil, err
	}
	if e.UserID != userID {
		return nil, ErrEnrollmentNotFound
	}
	last5 := strings.TrimSpace(in.Last5)
	amount := e.AmountDue
	if in.Amount != nil {
		amount = *in.Amount
	}
	note := strings.TrimSpace(in.Note)
	if !last5Pattern.MatchString(last5) || amount < 1 || amount > 2000000 || runes(note) > MaxReason {
		return nil, ErrInvalidReport
	}
	if in.TransferredAt != nil && in.TransferredAt.After(time.Now().Add(5*time.Minute)) {
		return nil, ErrInvalidReport
	}

	t, err := s.pay.CreateTransferReport(ctx, NewTransferReport{
		EnrollmentID: enrollmentID, UserID: userID, Last5: last5, Amount: amount, TransferredAt: in.TransferredAt, Note: note,
	})
	if err != nil {
		return nil, err
	}
	s.notifyManagers(ctx, e.CourseID, "", NotifTransferReported, "Transfer reported",
		t.StudentName+" reported a transfer of NT$"+strconv.Itoa(t.Amount)+" (last 5 digits "+t.Last5+") for \""+t.CourseTitle+"\".",
		RefTransferReport, t.ID)
	return t, nil
}

func (s *paymentService) ListEnrollmentReports(ctx context.Context, enrollmentID string, v Viewer, f TransferReportFilter) ([]*TransferReport, int, error) {
	if _, _, _, err := s.loadEnrollment(ctx, enrollmentID, v); err != nil {
		return nil, 0, err
	}
	f.EnrollmentID = enrollmentID
	return s.pay.ListTransferReports(ctx, f)
}

func (s *paymentService) ListReports(ctx context.Context, v Viewer, f TransferReportFilter) ([]*TransferReport, int, error) {
	if !v.IsAdmin {
		f.ManagedBy = v.UserID
	}
	return s.pay.ListTransferReports(ctx, f)
}

func (s *paymentService) ReviewReport(ctx context.Context, id string, v Viewer, approve bool, note string) (*TransferReport, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrReportNotFound
	}
	existing, err := s.pay.GetTransferReport(ctx, id)
	if err != nil {
		return nil, err
	}
	ok, err := s.isManager(ctx, existing.CourseID, v)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrReportNotFound
	}
	note = strings.TrimSpace(note)
	if !approve && note == "" {
		return nil, ErrNoteRequired
	}
	if runes(note) > MaxRuleText {
		return nil, ErrInvalidReport
	}
	t, err := s.pay.ReviewTransferReport(ctx, id, v.UserID, approve, note)
	if err != nil {
		return nil, err
	}
	if approve {
		s.notify(ctx, t.StudentID, NotifTransferApproved, "Payment confirmed",
			"Your transfer for \""+t.CourseTitle+"\" was confirmed.", RefTransferReport, t.ID)
	} else {
		s.notify(ctx, t.StudentID, NotifTransferRejected, "Transfer not confirmed",
			"Your transfer report for \""+t.CourseTitle+"\" was not confirmed: "+note+". You can submit a new report.", RefTransferReport, t.ID)
	}
	return t, nil
}

func (s *paymentService) SetPayment(ctx context.Context, enrollmentID string, v Viewer, to string) (*Enrollment, error) {
	if to != PayPaid && to != PayUnpaid {
		return nil, ErrInvalidPaymentTarget
	}
	e, _, manager, err := s.loadEnrollment(ctx, enrollmentID, v)
	if err != nil {
		return nil, err
	}
	if !manager {
		return nil, ErrForbidden
	}
	out, err := s.pay.SetPaymentStatus(ctx, enrollmentID, to, v.UserID)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, e.UserID, NotifPaymentUpdated, "Payment status updated",
		"Your payment for \""+e.Course.Title+"\" is now marked "+to+".", RefEnrollment, e.ID)
	return out, nil
}

// autoCancelAllowed reports whether a student may cancel on their own: the
// course allows it, no money is involved, and the deadline has not passed.
func (s *paymentService) autoCancelAllowed(ctx context.Context, e *Enrollment) (bool, error) {
	if e.PaymentStatus != PayNotRequired && e.PaymentStatus != PayUnpaid {
		return false, nil
	}
	c, err := s.repo.Get(ctx, e.CourseID)
	if err != nil {
		return false, err
	}
	if !c.AllowCancel {
		return false, nil
	}
	first, err := s.pay.FirstSessionStart(ctx, e.CourseID)
	if err != nil || first == nil {
		return false, err
	}
	cutoff := *first
	if c.CancelDeadline != nil {
		cutoff = cutoff.Add(-time.Duration(*c.CancelDeadline) * time.Hour)
	}
	return time.Now().Before(cutoff), nil
}

func (s *paymentService) RequestCancellation(ctx context.Context, enrollmentID string, v Viewer, in CancelInput) (*CancellationRequest, error) {
	e, owner, _, err := s.loadEnrollment(ctx, enrollmentID, v)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || runes(reason) > MaxReason {
		return nil, ErrReasonRequired
	}
	if in.SessionID != nil {
		if _, err := uuid.Parse(*in.SessionID); err != nil {
			return nil, ErrInvalidSession
		}
		if ok, err := s.pay.SessionInCourse(ctx, *in.SessionID, e.CourseID); err != nil {
			return nil, err
		} else if !ok {
			return nil, ErrInvalidSession
		}
	}

	// The enrollment's owner files as a student; anyone else reaching here is a
	// course manager filing on the student's behalf.
	role := RoleStudent
	if !owner {
		role = RoleCoach
	}

	auto := false
	if role == RoleStudent && in.SessionID == nil {
		if auto, err = s.autoCancelAllowed(ctx, e); err != nil {
			return nil, err
		}
	}
	req, err := s.pay.CreateCancellation(ctx, NewCancellation{
		EnrollmentID: enrollmentID, SessionID: in.SessionID, RequestedBy: v.UserID, Role: role, Reason: reason, Auto: auto,
	})
	if err != nil {
		return nil, err
	}

	switch {
	case auto:
		s.notifyManagers(ctx, e.CourseID, v.UserID, NotifEnrollmentCancelled, "Student cancelled",
			req.StudentName+" cancelled their enrollment in \""+req.CourseTitle+"\".", RefCancellation, req.ID)
	case role == RoleStudent:
		s.notifyManagers(ctx, e.CourseID, v.UserID, NotifCancellationRequest, "Cancellation requested",
			req.StudentName+" asked to cancel \""+req.CourseTitle+"\": "+reason, RefCancellation, req.ID)
	default:
		s.notify(ctx, e.UserID, NotifCancellationRequest, "Cancellation requested by the coach",
			"The coach filed a cancellation for \""+req.CourseTitle+"\": "+reason, RefCancellation, req.ID)
	}
	return req, nil
}

func (s *paymentService) ListEnrollmentCancellations(ctx context.Context, enrollmentID string, v Viewer, f CancellationFilter) ([]*CancellationRequest, int, error) {
	if _, _, _, err := s.loadEnrollment(ctx, enrollmentID, v); err != nil {
		return nil, 0, err
	}
	f.EnrollmentID = enrollmentID
	return s.pay.ListCancellations(ctx, f)
}

func (s *paymentService) ListCancellations(ctx context.Context, v Viewer, f CancellationFilter) ([]*CancellationRequest, int, error) {
	if !v.IsAdmin {
		f.ViewerID = v.UserID
	}
	return s.pay.ListCancellations(ctx, f)
}

func (s *paymentService) ResolveCancellation(ctx context.Context, id string, v Viewer, in ResolveInput) (*CancellationRequest, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrCancellationNotFound
	}
	existing, err := s.pay.GetCancellation(ctx, id)
	if err != nil {
		return nil, err
	}
	ok, err := s.isManager(ctx, existing.CourseID, v)
	if err != nil {
		return nil, err
	}
	if !ok {
		if existing.StudentID == v.UserID {
			return nil, ErrForbidden
		}
		return nil, ErrCancellationNotFound
	}
	switch in.Resolution {
	case ResolutionCancelled, ResolutionRefunded, ResolutionMakeup, ResolutionRejected, ResolutionOther:
	default:
		return nil, ErrInvalidResolution
	}
	note := strings.TrimSpace(in.Note)
	if runes(note) > MaxRuleText {
		return nil, ErrInvalidResolution
	}

	req, err := s.pay.ResolveCancellation(ctx, id, CancellationResolution{
		Resolution: in.Resolution, RefundAmount: in.RefundAmount, Note: note, ResolvedBy: v.UserID,
	})
	if err != nil {
		return nil, err
	}

	msg := "Your request for \"" + req.CourseTitle + "\" was resolved: " + in.Resolution
	if in.Resolution == ResolutionRefunded && req.RefundAmount != nil {
		msg += " (NT$" + strconv.Itoa(*req.RefundAmount) + ")"
	}
	if note != "" {
		msg += ". " + note
	}
	s.notify(ctx, req.StudentID, NotifCancellationResolved, "Cancellation request resolved", msg, RefCancellation, req.ID)
	if req.RequestedBy != nil && *req.RequestedBy != req.StudentID && *req.RequestedBy != v.UserID {
		s.notify(ctx, *req.RequestedBy, NotifCancellationResolved, "Cancellation request resolved", msg, RefCancellation, req.ID)
	}
	return req, nil
}

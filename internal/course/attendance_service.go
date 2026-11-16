package course

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/notification"
)

// MaxAttendanceRecords bounds one attendance request.
const MaxAttendanceRecords = 200

// AttendanceService handles leave, make-up sessions and attendance. The
// course rules (allow_leave, leave deadline, make-up limit and validity) are
// the structured fields on the course; the service re-checks them whenever a
// student acts, so a stale client cannot bypass them.
type AttendanceService interface {
	RequestLeave(ctx context.Context, sessionID, userID, reason string) (*LeaveRequest, error)
	CancelLeave(ctx context.Context, leaveID, userID string) error
	MakeupOptions(ctx context.Context, leaveID string, v Viewer) (MakeupEligibility, []*MakeupOption, error)
	SelectMakeup(ctx context.Context, leaveID, userID, sessionID string) (*LeaveRequest, error)
	CancelMakeup(ctx context.Context, leaveID, userID string) (*LeaveRequest, error)

	Roster(ctx context.Context, sessionID string, v Viewer) (*SessionInfo, []*RosterEntry, error)
	MarkAttendance(ctx context.Context, sessionID string, v Viewer, recs []AttendanceRecord) (*SessionInfo, []*RosterEntry, error)
	EnrollmentAttendance(ctx context.Context, enrollmentID string, v Viewer) ([]*AttendanceItem, AttendanceSummary, error)
	// HasAttended reports whether the enrollment was marked present at least once.
	HasAttended(ctx context.Context, enrollmentID string) (bool, error)
}

type attendanceService struct {
	repo     Repository
	att      AttendanceRepository
	notifier notification.Service
}

func NewAttendanceService(repo Repository, att AttendanceRepository, notifier notification.Service) AttendanceService {
	return &attendanceService{repo: repo, att: att, notifier: notifier}
}

func (s *attendanceService) notify(ctx context.Context, userID, typ, title, content, refType, refID string) {
	rt, ri := refType, refID
	if err := s.notifier.Notify(ctx, &notification.Notification{
		UserID: userID, Type: typ, Title: title, Content: content, RefType: &rt, RefID: &ri,
	}); err != nil {
		log.Printf("warning: failed to send %s notification: %v", typ, err)
	}
}

func (s *attendanceService) notifyManagers(ctx context.Context, courseID, skip, typ, title, content, refType, refID string) {
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

func (s *attendanceService) isManager(ctx context.Context, courseID string, v Viewer) (bool, error) {
	if v.IsAdmin {
		return true, nil
	}
	if v.UserID == "" {
		return false, nil
	}
	return s.repo.IsManager(ctx, courseID, v.UserID)
}

func localTime(t time.Time, tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02 15:04")
}

func (s *attendanceService) RequestLeave(ctx context.Context, sessionID, userID, reason string) (*LeaveRequest, error) {
	if _, err := uuid.Parse(sessionID); err != nil {
		return nil, ErrSessionNotFound
	}
	sess, err := s.att.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	enrollmentID, err := s.att.ExpectedEnrollment(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if sess.Status != SessionScheduled || sess.CourseStatus == StatusCancelled {
		return nil, ErrSessionCancelled
	}
	now := time.Now()
	if !sess.StartAt.After(now) {
		return nil, ErrSessionStarted
	}
	if !sess.AllowLeave {
		return nil, ErrLeaveNotAllowed
	}
	if sess.LeaveDeadlineHrs != nil && now.After(sess.StartAt.Add(-time.Duration(*sess.LeaveDeadlineHrs)*time.Hour)) {
		return nil, ErrLeaveDeadline
	}
	reason = strings.TrimSpace(reason)
	if runes(reason) > MaxReason {
		return nil, ErrInvalidCourse
	}

	l, err := s.att.CreateLeave(ctx, sessionID, enrollmentID, reason)
	if err != nil {
		return nil, err
	}
	s.notifyManagers(ctx, sess.CourseID, userID, NotifLeaveRequested, "Leave requested",
		l.StudentName+" asked for leave on "+localTime(sess.StartAt, sess.Timezone)+" in \""+sess.CourseTitle+"\".", RefLeave, l.ID)
	return l, nil
}

func (s *attendanceService) loadOwnLeave(ctx context.Context, leaveID, userID string) (*LeaveContext, error) {
	if _, err := uuid.Parse(leaveID); err != nil {
		return nil, ErrLeaveNotFound
	}
	lc, err := s.att.GetLeaveContext(ctx, leaveID)
	if err != nil {
		return nil, err
	}
	if lc.Leave.StudentID != userID {
		return nil, ErrLeaveNotFound
	}
	return lc, nil
}

func (s *attendanceService) CancelLeave(ctx context.Context, leaveID, userID string) error {
	lc, err := s.loadOwnLeave(ctx, leaveID, userID)
	if err != nil {
		return err
	}
	if !lc.Leave.SessionStartAt.After(time.Now()) {
		return ErrSessionStarted
	}
	return s.att.DeleteLeave(ctx, leaveID)
}

// validUntil is the latest start time of a make-up session for the leave.
func validUntil(lc *LeaveContext) *time.Time {
	if lc.MakeupValidDays == nil {
		return nil
	}
	t := lc.Leave.SessionStartAt.Add(time.Duration(*lc.MakeupValidDays) * 24 * time.Hour)
	return &t
}

func (s *attendanceService) MakeupOptions(ctx context.Context, leaveID string, v Viewer) (MakeupEligibility, []*MakeupOption, error) {
	if _, err := uuid.Parse(leaveID); err != nil {
		return MakeupEligibility{}, nil, ErrLeaveNotFound
	}
	lc, err := s.att.GetLeaveContext(ctx, leaveID)
	if err != nil {
		return MakeupEligibility{}, nil, err
	}
	if lc.Leave.StudentID != v.UserID {
		ok, err := s.isManager(ctx, lc.Leave.CourseID, v)
		if err != nil {
			return MakeupEligibility{}, nil, err
		}
		if !ok {
			return MakeupEligibility{}, nil, ErrLeaveNotFound
		}
	}

	el := MakeupEligibility{Until: validUntil(lc)}
	if lc.MakeupLimit != nil {
		left := max(*lc.MakeupLimit-lc.UsedMakeups, 0)
		el.Remaining = &left
	}
	switch {
	case lc.Leave.Status == LeaveUsed:
		el.Reason = ErrLeaveUsed.Message
	case lc.EnrollmentStatus != EnrollActive:
		el.Reason = ErrEnrollmentInactive.Message
	default:
		if err := checkMakeupRules(lc, time.Now()); err != nil {
			el.Reason = err.Error()
		} else {
			el.Eligible = true
		}
	}
	if !el.Eligible {
		return el, []*MakeupOption{}, nil
	}
	opts, err := s.att.ListMakeupOptions(ctx, lc, el.Until)
	if err != nil {
		return MakeupEligibility{}, nil, err
	}
	return el, opts, nil
}

func (s *attendanceService) SelectMakeup(ctx context.Context, leaveID, userID, sessionID string) (*LeaveRequest, error) {
	lc, err := s.loadOwnLeave(ctx, leaveID, userID)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(sessionID); err != nil {
		return nil, ErrMakeupUnavailable
	}
	l, opt, err := s.att.SelectMakeup(ctx, leaveID, sessionID, validUntil(lc))
	if err != nil {
		return nil, err
	}
	s.notifyManagers(ctx, opt.CourseID, userID, NotifMakeupBooked, "Make-up student booked",
		l.StudentName+" will join \""+opt.CourseTitle+"\" on "+localTime(opt.StartAt, opt.Timezone)+" as a make-up student.", RefLeave, l.ID)
	return l, nil
}

func (s *attendanceService) CancelMakeup(ctx context.Context, leaveID, userID string) (*LeaveRequest, error) {
	if _, err := s.loadOwnLeave(ctx, leaveID, userID); err != nil {
		return nil, err
	}
	return s.att.CancelMakeup(ctx, leaveID)
}

func (s *attendanceService) requireManagerOfSession(ctx context.Context, sessionID string, v Viewer) (*SessionInfo, error) {
	if _, err := uuid.Parse(sessionID); err != nil {
		return nil, ErrSessionNotFound
	}
	sess, err := s.att.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	ok, err := s.isManager(ctx, sess.CourseID, v)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	return sess, nil
}

func (s *attendanceService) Roster(ctx context.Context, sessionID string, v Viewer) (*SessionInfo, []*RosterEntry, error) {
	sess, err := s.requireManagerOfSession(ctx, sessionID, v)
	if err != nil {
		return nil, nil, err
	}
	roster, err := s.att.Roster(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	return sess, roster, nil
}

func (s *attendanceService) MarkAttendance(ctx context.Context, sessionID string, v Viewer, recs []AttendanceRecord) (*SessionInfo, []*RosterEntry, error) {
	sess, err := s.requireManagerOfSession(ctx, sessionID, v)
	if err != nil {
		return nil, nil, err
	}
	if len(recs) == 0 || len(recs) > MaxAttendanceRecords {
		return nil, nil, ErrNoRecords
	}
	if sess.Status == SessionCancelled || sess.CourseStatus == StatusCancelled {
		return nil, nil, ErrSessionCancelled
	}
	if sess.StartAt.After(time.Now()) {
		return nil, nil, ErrSessionNotStarted
	}
	seen := make(map[string]bool, len(recs))
	for _, r := range recs {
		if _, err := uuid.Parse(r.EnrollmentID); err != nil {
			return nil, nil, ErrRecordNotInRoster
		}
		if seen[r.EnrollmentID] {
			return nil, nil, ErrRecordNotInRoster
		}
		seen[r.EnrollmentID] = true
		if r.Status != AttPresent && r.Status != AttAbsent && r.Status != AttPending {
			return nil, nil, ErrInvalidStatus
		}
	}
	if err := s.att.MarkAttendance(ctx, sessionID, v.UserID, recs); err != nil {
		return nil, nil, err
	}
	roster, err := s.att.Roster(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	return sess, roster, nil
}

func (s *attendanceService) EnrollmentAttendance(ctx context.Context, enrollmentID string, v Viewer) ([]*AttendanceItem, AttendanceSummary, error) {
	if _, err := uuid.Parse(enrollmentID); err != nil {
		return nil, AttendanceSummary{}, ErrEnrollmentNotFound
	}
	e, err := s.repo.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		return nil, AttendanceSummary{}, err
	}
	if e.UserID != v.UserID {
		ok, err := s.isManager(ctx, e.CourseID, v)
		if err != nil {
			return nil, AttendanceSummary{}, err
		}
		if !ok {
			return nil, AttendanceSummary{}, ErrEnrollmentNotFound
		}
	}
	return s.att.EnrollmentAttendance(ctx, enrollmentID)
}

func (s *attendanceService) HasAttended(ctx context.Context, enrollmentID string) (bool, error) {
	return s.att.HasAttended(ctx, enrollmentID)
}

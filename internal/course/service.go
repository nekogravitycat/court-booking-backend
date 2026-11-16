package course

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/coach"
	"github.com/nekogravitycat/court-booking-backend/internal/notification"
	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	"github.com/nekogravitycat/court-booking-backend/internal/skilllevel"
)

// Viewer identifies who is reading or acting. UserID is empty for guests.
type Viewer struct {
	UserID  string
	IsAdmin bool
}

// OptInt is a PATCH field for a nullable integer: absent, null (clear), or a value.
type OptInt struct {
	Set   bool
	Value *int
}

// OptTime is a PATCH field for a nullable timestamp.
type OptTime struct {
	Set   bool
	Value *time.Time
}

// CreateInput carries a new course. Exactly one of Schedule and Sessions is used.
type CreateInput struct {
	Course   Course // Scalar fields; ownership and derived fields are ignored
	GroupID  *string
	CoachIDs []string // Teaching coaches of a group course (default: the creator)
	Schedule *Schedule
	Sessions []TimeRange
}

// UpdateInput is a partial course update. Nil / unset fields are unchanged.
type UpdateInput struct {
	Title            *string
	Description      *string
	TeachingContent  *string
	Capacity         *int
	VenueName        *string
	LocationID       *string
	ClearLocation    bool
	Price            *int
	VenueFee         *int
	PaymentMethods   *[]string
	BankCode         *string // Empty string clears
	BankAccount      *string
	BankAccountName  *string
	AllowLeave       *bool
	LeaveDeadlineHrs OptInt
	AllowMakeup      *bool
	MakeupLimit      OptInt
	MakeupValidDays  OptInt
	AllowCancel      *bool
	CancelDeadline   OptInt
	RulesLeave       *string
	RulesMakeup      *string
	RulesCancel      *string
	RulesRefund      *string
	RulesNoShow      *string
	EnrollDeadline   OptTime
	Status           *string
	CoachIDs         *[]string
}

// SessionInput creates one extra session of a recurring course.
type SessionInput struct {
	Start     time.Time
	End       time.Time
	VenueNote string
}

// SessionUpdate reschedules or cancels one session.
type SessionUpdate struct {
	Start     *time.Time
	End       *time.Time
	VenueNote *string
	Cancel    bool
	Reason    string
}

// Service manages courses, sessions and enrollments.
type Service interface {
	Create(ctx context.Context, userID string, in CreateInput) (*Course, error)
	Get(ctx context.Context, id string, v Viewer) (*Course, error)
	List(ctx context.Context, f Filter) ([]*Course, int, error)
	Update(ctx context.Context, id string, v Viewer, in UpdateInput) (*Course, error)
	AddSession(ctx context.Context, courseID string, v Viewer, in SessionInput) (*Session, error)
	UpdateSession(ctx context.Context, courseID, sessionID string, v Viewer, in SessionUpdate) (*Session, error)

	Enroll(ctx context.Context, courseID, userID string, method *string) (*Enrollment, error)
	ListEnrollments(ctx context.Context, courseID string, v Viewer, f EnrollmentFilter) ([]*Enrollment, int, error)
	ListMyEnrollments(ctx context.Context, userID string, f EnrollmentFilter) ([]*Enrollment, int, error)
	GetEnrollment(ctx context.Context, id string, v Viewer) (*Enrollment, error)

	// IsManager reports whether the viewer may manage the course.
	IsManager(ctx context.Context, courseID string, v Viewer) (bool, error)
	// OnUserDeactivated implements user.DeactivationHook.
	OnUserDeactivated(ctx context.Context, userID string) error
}

type service struct {
	repo     Repository
	coaches  coach.Service
	groups   coach.GroupService
	levels   skilllevel.Service
	notifier notification.Service
}

func NewService(repo Repository, coaches coach.Service, groups coach.GroupService, levels skilllevel.Service, notifier notification.Service) Service {
	return &service{repo: repo, coaches: coaches, groups: groups, levels: levels, notifier: notifier}
}

// notify delivers best-effort: a failed notification never fails the action.
func (s *service) notify(ctx context.Context, userID, typ, title, content, refType, refID string) {
	rt, ri := refType, refID
	if err := s.notifier.Notify(ctx, &notification.Notification{
		UserID: userID, Type: typ, Title: title, Content: content, RefType: &rt, RefID: &ri,
	}); err != nil {
		log.Printf("warning: failed to send %s notification: %v", typ, err)
	}
}

func (s *service) notifyAll(ctx context.Context, userIDs []string, skip, typ, title, content, refType, refID string) {
	for _, id := range userIDs {
		if id != skip {
			s.notify(ctx, id, typ, title, content, refType, refID)
		}
	}
}

var (
	bankCodePattern    = regexp.MustCompile(`^\d{3,7}$`)
	bankAccountPattern = regexp.MustCompile(`^\d{6,20}$`)
)

func runes(s string) int { return utf8.RuneCountInString(s) }

// validateCourse checks the scalar fields of a course after defaults and merges.
func validateCourse(c *Course) error {
	c.Title = strings.TrimSpace(c.Title)
	if c.Title == "" || runes(c.Title) > MaxTitle {
		return ErrInvalidCourse
	}
	if runes(c.Description) > MaxDescription || runes(c.TeachingContent) > MaxTeaching || runes(c.VenueName) > MaxVenueName {
		return ErrInvalidCourse
	}
	if c.Capacity < 1 || c.Capacity > MaxCapacity {
		return ErrInvalidCourse
	}
	if c.Price < 0 || c.Price > MaxPrice || c.VenueFee < 0 || c.VenueFee > MaxPrice {
		return ErrInvalidCourse
	}
	for _, r := range []string{c.RulesLeave, c.RulesMakeup, c.RulesCancel, c.RulesRefund, c.RulesNoShow} {
		if runes(r) > MaxRuleText {
			return ErrInvalidRule
		}
	}
	if !validOptInt(c.LeaveDeadlineHrs, 0, 8760) || !validOptInt(c.CancelDeadline, 0, 8760) ||
		!validOptInt(c.MakeupLimit, 0, 100) || !validOptInt(c.MakeupValidDays, 1, 365) {
		return ErrInvalidRule
	}

	// Payment methods: unique, known, and required for a paid course.
	seen := map[string]bool{}
	methods := make([]string, 0, 2)
	for _, m := range c.PaymentMethods {
		if m != MethodOnsite && m != MethodTransfer {
			return ErrInvalidPayment
		}
		if !seen[m] {
			seen[m] = true
			methods = append(methods, m)
		}
	}
	c.PaymentMethods = methods
	if c.Price+c.VenueFee > 0 && len(methods) == 0 {
		return ErrInvalidPayment
	}

	trim := func(p **string) {
		if *p != nil {
			v := strings.TrimSpace(**p)
			if v == "" {
				*p = nil
			} else {
				*p = &v
			}
		}
	}
	trim(&c.BankCode)
	trim(&c.BankAccount)
	trim(&c.BankAccountName)
	if seen[MethodTransfer] {
		if c.BankCode == nil || c.BankAccount == nil {
			return ErrBankInfoRequired
		}
		if !bankCodePattern.MatchString(*c.BankCode) || !bankAccountPattern.MatchString(*c.BankAccount) {
			return ErrInvalidBankInfo
		}
	}
	if c.BankAccountName != nil && runes(*c.BankAccountName) > 50 {
		return ErrInvalidBankInfo
	}
	return nil
}

func validOptInt(v *int, min, max int) bool { return v == nil || (*v >= min && *v <= max) }

func uniqueUUIDs(ids []string) ([]string, error) {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		p, err := uuid.Parse(id)
		if err != nil {
			return nil, ErrInvalidTeacher
		}
		if k := p.String(); !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	if len(out) > MaxCoaches {
		return nil, ErrInvalidTeacher
	}
	return out, nil
}

func (s *service) Create(ctx context.Context, userID string, in CreateInput) (*Course, error) {
	c := in.Course
	switch c.Type {
	case TypeTrial, TypeSingle, TypeRecurring:
	default:
		return nil, ErrInvalidType
	}
	if c.Timezone == "" {
		c.Timezone = DefaultTimezone
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, ErrInvalidTimezone
	}
	if err := validateCourse(&c); err != nil {
		return nil, err
	}
	now := time.Now()
	if c.EnrollDeadline != nil && !c.EnrollDeadline.After(now) {
		return nil, ErrInvalidDeadline
	}
	if ok, err := s.repo.SportActive(ctx, c.SportID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrSportNotFound
	}
	if c.LocationID != nil {
		if ok, err := s.repo.LocationExists(ctx, *c.LocationID); err != nil {
			return nil, err
		} else if !ok {
			return nil, ErrLocationNotFound
		}
	}

	// Schedule: a weekly pattern or an explicit list, never both.
	if (in.Schedule == nil) == (len(in.Sessions) == 0) {
		return nil, ErrScheduleRequired
	}
	ranges := in.Sessions
	if in.Schedule != nil {
		if ranges, err = ExpandSchedule(*in.Schedule, loc); err != nil {
			return nil, err
		}
	}
	if ranges, err = validateRanges(ranges, now); err != nil {
		return nil, err
	}
	if c.Type != TypeRecurring && len(ranges) != 1 {
		return nil, ErrSingleSession
	}
	if c.EnrollDeadline != nil && c.EnrollDeadline.After(ranges[len(ranges)-1].End) {
		return nil, ErrInvalidDeadline
	}

	// Ownership.
	if err := s.coaches.RequireApproved(ctx, userID); err != nil {
		return nil, err
	}
	mine, err := s.groups.GetMine(ctx, userID)
	if err != nil && !errors.Is(err, coach.ErrNotInGroup) {
		return nil, err
	}
	var teachers []string
	c.CreatedBy = &userID
	if in.GroupID != nil {
		if mine == nil || mine.ID != *in.GroupID {
			return nil, ErrNotCoachOfGroup
		}
		var me *coach.Member
		for _, m := range mine.Members {
			if m.UserID == userID {
				me = m
			}
		}
		if me == nil || me.Status != coach.MemberActive || (me.Role != coach.RoleLeader && me.Role != coach.RoleCoach) {
			return nil, ErrNotCoachOfGroup
		}
		c.GroupID = in.GroupID
		teachers = in.CoachIDs
		if len(teachers) == 0 {
			teachers = []string{userID}
		}
		if teachers, err = uniqueUUIDs(teachers); err != nil {
			return nil, err
		}
		if n, err := s.repo.ActiveGroupMembers(ctx, *in.GroupID, teachers); err != nil {
			return nil, err
		} else if n != len(teachers) {
			return nil, ErrInvalidTeacher
		}
	} else {
		if len(in.CoachIDs) > 0 {
			return nil, ErrInvalidTeacher
		}
		if mine != nil && !mine.AllowPrivateCourses {
			return nil, ErrPrivateNotAllowed
		}
		c.CoachID = &userID
		teachers = []string{userID}
	}

	id, err := s.repo.Create(ctx, &c, teachers, ranges)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id, Viewer{UserID: userID})
}

func (s *service) IsManager(ctx context.Context, courseID string, v Viewer) (bool, error) {
	if v.IsAdmin {
		return true, nil
	}
	if v.UserID == "" {
		return false, nil
	}
	return s.repo.IsManager(ctx, courseID, v.UserID)
}

func (s *service) requireManager(ctx context.Context, courseID string, v Viewer) error {
	ok, err := s.IsManager(ctx, courseID, v)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

func (s *service) Get(ctx context.Context, id string, v Viewer) (*Course, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Sessions, err = s.repo.ListSessions(ctx, id); err != nil {
		return nil, err
	}
	if c.CanManage, err = s.IsManager(ctx, id, v); err != nil {
		return nil, err
	}

	// Bank details are shown to managers and enrolled students only.
	enrolled := false
	if v.UserID != "" {
		mine, _, err := s.repo.ListEnrollments(ctx, EnrollmentFilter{CourseID: id, UserID: v.UserID, Status: EnrollActive, PageSize: 1})
		if err != nil {
			return nil, err
		}
		if len(mine) > 0 {
			enrolled = true
			c.MyEnrollmentID = &mine[0].ID
		}
	}
	c.BankInfoAvailable = c.BankCode != nil && c.BankAccount != nil
	if !c.CanManage && !enrolled {
		c.BankCode, c.BankAccount, c.BankAccountName = nil, nil, nil
	}

	stats, err := s.participantStats(ctx, c)
	if err != nil {
		return nil, err
	}
	c.ParticipantStats = stats
	return c, nil
}

// participantStats builds the anonymous roster summary, reusing the pickup
// module's aggregation so both products show the same shape.
func (s *service) participantStats(ctx context.Context, c *Course) (*ParticipantStatsView, error) {
	seats, tz, err := s.repo.Seats(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	labels, err := s.levels.LabelsBySport(ctx, c.SportID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	ps := make([]pickup.ParticipantSeat, len(seats))
	for i, st := range seats {
		ps[i] = pickup.ParticipantSeat{Gender: st.Gender, BirthDate: st.BirthDate, SkillLevel: st.SkillLevel}
	}
	built := pickup.BuildParticipantStats(ps, time.Now().In(loc), labels)

	out := &ParticipantStatsView{Total: built.Total}
	for _, g := range built.Genders {
		out.Genders = append(out.Genders, CountItem{Key: g.Gender, Count: g.Count})
	}
	for _, a := range built.AgeGroups {
		out.AgeGroups = append(out.AgeGroups, CountItem{Key: a.Group, Count: a.Count})
	}
	for _, l := range built.SkillLevels {
		out.SkillLevels = append(out.SkillLevels, LevelCount{Level: l.Level, Label: l.Label, Count: l.Count})
	}
	return out, nil
}

func (s *service) List(ctx context.Context, f Filter) ([]*Course, int, error) {
	if f.CourseType != "" && f.CourseType != TypeTrial && f.CourseType != TypeSingle && f.CourseType != TypeRecurring {
		return nil, 0, ErrInvalidType
	}
	return s.repo.List(ctx, f)
}

func (s *service) Update(ctx context.Context, id string, v Viewer, in UpdateInput) (*Course, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}
	if err := s.requireManager(ctx, id, v); err != nil {
		return nil, err
	}
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status == StatusCancelled {
		return nil, ErrCourseCancelled
	}
	before := *c

	if in.Status != nil && *in.Status == StatusCancelled {
		return s.cancel(ctx, c, v)
	}
	if in.Status != nil && *in.Status != StatusOpen && *in.Status != StatusClosed {
		return nil, ErrInvalidCourse
	}

	apply := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	apply(&c.Title, in.Title)
	apply(&c.Description, in.Description)
	apply(&c.TeachingContent, in.TeachingContent)
	apply(&c.VenueName, in.VenueName)
	apply(&c.RulesLeave, in.RulesLeave)
	apply(&c.RulesMakeup, in.RulesMakeup)
	apply(&c.RulesCancel, in.RulesCancel)
	apply(&c.RulesRefund, in.RulesRefund)
	apply(&c.RulesNoShow, in.RulesNoShow)
	if in.Status != nil {
		c.Status = *in.Status
	}
	if in.Capacity != nil {
		c.Capacity = *in.Capacity
	}
	if in.Price != nil {
		c.Price = *in.Price
	}
	if in.VenueFee != nil {
		c.VenueFee = *in.VenueFee
	}
	if in.PaymentMethods != nil {
		c.PaymentMethods = *in.PaymentMethods
	}
	if in.BankCode != nil {
		c.BankCode = in.BankCode
	}
	if in.BankAccount != nil {
		c.BankAccount = in.BankAccount
	}
	if in.BankAccountName != nil {
		c.BankAccountName = in.BankAccountName
	}
	if in.AllowLeave != nil {
		c.AllowLeave = *in.AllowLeave
	}
	if in.AllowMakeup != nil {
		c.AllowMakeup = *in.AllowMakeup
	}
	if in.AllowCancel != nil {
		c.AllowCancel = *in.AllowCancel
	}
	if in.LeaveDeadlineHrs.Set {
		c.LeaveDeadlineHrs = in.LeaveDeadlineHrs.Value
	}
	if in.MakeupLimit.Set {
		c.MakeupLimit = in.MakeupLimit.Value
	}
	if in.MakeupValidDays.Set {
		c.MakeupValidDays = in.MakeupValidDays.Value
	}
	if in.CancelDeadline.Set {
		c.CancelDeadline = in.CancelDeadline.Value
	}
	if in.EnrollDeadline.Set {
		c.EnrollDeadline = in.EnrollDeadline.Value
	}
	if in.ClearLocation {
		c.LocationID = nil
	} else if in.LocationID != nil {
		if ok, err := s.repo.LocationExists(ctx, *in.LocationID); err != nil {
			return nil, err
		} else if !ok {
			return nil, ErrLocationNotFound
		}
		c.LocationID = in.LocationID
	}
	if err := validateCourse(c); err != nil {
		return nil, err
	}

	var teachers *[]string
	if in.CoachIDs != nil {
		if c.GroupID == nil {
			return nil, ErrInvalidTeacher
		}
		ids, err := uniqueUUIDs(*in.CoachIDs)
		if err != nil || len(ids) == 0 {
			return nil, ErrInvalidTeacher
		}
		if n, err := s.repo.ActiveGroupMembers(ctx, *c.GroupID, ids); err != nil {
			return nil, err
		} else if n != len(ids) {
			return nil, ErrInvalidTeacher
		}
		teachers = &ids
	}

	if err := s.repo.Update(ctx, c, teachers); err != nil {
		return nil, err
	}
	s.notifyCourseChange(ctx, &before, c)
	return s.Get(ctx, id, v)
}

// notifyCourseChange tells enrolled students when venue, price or payment
// details changed after they enrolled.
func (s *service) notifyCourseChange(ctx context.Context, before, after *Course) {
	changed := before.VenueName != after.VenueName || !equalPtr(before.LocationID, after.LocationID) ||
		before.Price != after.Price || before.VenueFee != after.VenueFee ||
		!equalPtr(before.BankCode, after.BankCode) || !equalPtr(before.BankAccount, after.BankAccount)
	if !changed {
		return
	}
	students, err := s.repo.StudentIDs(ctx, after.ID)
	if err != nil {
		log.Printf("warning: failed to list students for course update: %v", err)
		return
	}
	s.notifyAll(ctx, students, "", NotifCourseUpdated, "Course details changed",
		"The venue, fee, or payment details of \""+after.Title+"\" were updated. Please review the course page.", RefCourse, after.ID)
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *service) cancel(ctx context.Context, c *Course, v Viewer) (*Course, error) {
	cancelled, err := s.repo.Cancel(ctx, c.ID, v.UserID)
	if err != nil {
		return nil, err
	}
	for _, e := range cancelled {
		s.notify(ctx, e.UserID, NotifCourseCancelled, "Course cancelled",
			"The course \""+c.Title+"\" was cancelled by the coach. Contact the coach about any refund.", RefCourse, c.ID)
	}
	return s.Get(ctx, c.ID, v)
}

func (s *service) AddSession(ctx context.Context, courseID string, v Viewer, in SessionInput) (*Session, error) {
	if _, err := uuid.Parse(courseID); err != nil {
		return nil, ErrNotFound
	}
	if err := s.requireManager(ctx, courseID, v); err != nil {
		return nil, err
	}
	if runes(in.VenueNote) > MaxVenueNote {
		return nil, ErrInvalidCourse
	}
	rs, err := validateRanges([]TimeRange{{Start: in.Start, End: in.End}}, time.Now())
	if err != nil {
		return nil, err
	}
	return s.repo.AddSession(ctx, courseID, rs[0], strings.TrimSpace(in.VenueNote))
}

func (s *service) UpdateSession(ctx context.Context, courseID, sessionID string, v Viewer, in SessionUpdate) (*Session, error) {
	if _, err := uuid.Parse(courseID); err != nil {
		return nil, ErrNotFound
	}
	if _, err := uuid.Parse(sessionID); err != nil {
		return nil, ErrSessionNotFound
	}
	if err := s.requireManager(ctx, courseID, v); err != nil {
		return nil, err
	}
	if in.Start == nil && in.End == nil && in.VenueNote == nil && !in.Cancel {
		return nil, ErrNothingToUpdate
	}
	if (in.Start == nil) != (in.End == nil) {
		return nil, ErrInvalidSchedule
	}
	if in.Cancel && in.Start != nil {
		return nil, ErrInvalidSchedule
	}
	reason := strings.TrimSpace(in.Reason)
	if runes(reason) > MaxReason {
		return nil, ErrInvalidCourse
	}
	var rg *TimeRange
	if in.Start != nil {
		rs, err := validateRanges([]TimeRange{{Start: *in.Start, End: *in.End}}, time.Now())
		if err != nil {
			return nil, err
		}
		rg = &rs[0]
	}
	var note *string
	if in.VenueNote != nil {
		n := strings.TrimSpace(*in.VenueNote)
		if runes(n) > MaxVenueNote {
			return nil, ErrInvalidCourse
		}
		note = &n
	}
	var cancelReason *string
	if in.Cancel {
		cancelReason = &reason
	}

	// Make-up guests are released by the cancellation; remember who they are first.
	var guests []string
	if in.Cancel {
		guests, _ = s.repo.MakeupGuestIDs(ctx, sessionID)
	}
	sess, err := s.repo.UpdateSession(ctx, courseID, sessionID, rg, note, cancelReason)
	if err != nil {
		return nil, err
	}
	if len(guests) > 0 {
		s.notifyAll(ctx, guests, "", NotifMakeupReleased, "Make-up session cancelled",
			"A make-up session you booked was cancelled. Your leave is open again; pick another make-up session.", RefSession, sess.ID)
	}
	if rg != nil || in.Cancel {
		s.notifySessionChange(ctx, courseID, sess, in.Cancel, reason)
	}
	return sess, nil
}

func (s *service) notifySessionChange(ctx context.Context, courseID string, sess *Session, cancelled bool, reason string) {
	c, err := s.repo.Get(ctx, courseID)
	if err != nil {
		return
	}
	students, err := s.repo.StudentIDs(ctx, courseID)
	if err != nil {
		log.Printf("warning: failed to list students for session change: %v", err)
		return
	}
	loc, lerr := time.LoadLocation(c.Timezone)
	if lerr != nil {
		loc = time.UTC
	}
	when := sess.StartAt.In(loc).Format("2006-01-02 15:04")
	if cancelled {
		msg := "The session on " + when + " of \"" + c.Title + "\" was cancelled."
		if reason != "" {
			msg += " Reason: " + reason
		}
		s.notifyAll(ctx, students, "", NotifSessionCancelled, "Session cancelled", msg, RefSession, sess.ID)
		return
	}
	s.notifyAll(ctx, students, "", NotifSessionChanged, "Session rescheduled",
		"A session of \""+c.Title+"\" moved to "+when+".", RefSession, sess.ID)
}

func (s *service) Enroll(ctx context.Context, courseID, userID string, method *string) (*Enrollment, error) {
	if _, err := uuid.Parse(courseID); err != nil {
		return nil, ErrNotFound
	}
	e, err := s.repo.Enroll(ctx, courseID, userID, method)
	if err != nil {
		return nil, err
	}
	full, err := s.repo.GetEnrollment(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	managers, err := s.repo.ManagerIDs(ctx, courseID)
	if err != nil {
		log.Printf("warning: failed to list course managers: %v", err)
	} else {
		s.notifyAll(ctx, managers, userID, NotifEnrolled, "New enrollment",
			full.Student.DisplayName+" enrolled in \""+full.Course.Title+"\".", RefEnrollment, e.ID)
	}
	return full, nil
}

func (s *service) ListEnrollments(ctx context.Context, courseID string, v Viewer, f EnrollmentFilter) ([]*Enrollment, int, error) {
	if _, err := uuid.Parse(courseID); err != nil {
		return nil, 0, ErrNotFound
	}
	if err := s.requireManager(ctx, courseID, v); err != nil {
		return nil, 0, err
	}
	f.CourseID = courseID
	return s.repo.ListEnrollments(ctx, f)
}

func (s *service) ListMyEnrollments(ctx context.Context, userID string, f EnrollmentFilter) ([]*Enrollment, int, error) {
	f.UserID = userID
	return s.repo.ListEnrollments(ctx, f)
}

func (s *service) GetEnrollment(ctx context.Context, id string, v Viewer) (*Enrollment, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrEnrollmentNotFound
	}
	e, err := s.repo.GetEnrollment(ctx, id)
	if err != nil {
		return nil, err
	}
	if e.UserID == v.UserID {
		return e, nil
	}
	ok, err := s.IsManager(ctx, e.CourseID, v)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrEnrollmentNotFound
	}
	return e, nil
}

// OnUserDeactivated releases the account's seats when it is deactivated.
func (s *service) OnUserDeactivated(ctx context.Context, userID string) error {
	return s.repo.CancelUserEnrollments(ctx, userID)
}

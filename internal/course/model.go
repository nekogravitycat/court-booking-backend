package course

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrNotFound            = apperror.New(http.StatusNotFound, "course not found")
	ErrSessionNotFound     = apperror.New(http.StatusNotFound, "session not found")
	ErrEnrollmentNotFound  = apperror.New(http.StatusNotFound, "enrollment not found")
	ErrForbidden           = apperror.New(http.StatusForbidden, "you cannot manage this course")
	ErrInvalidCourse       = apperror.New(http.StatusBadRequest, "invalid course")
	ErrInvalidType         = apperror.New(http.StatusBadRequest, "course_type must be trial, single, or recurring")
	ErrInvalidSchedule     = apperror.New(http.StatusBadRequest, "invalid schedule")
	ErrScheduleRequired    = apperror.New(http.StatusBadRequest, "provide exactly one of schedule or sessions")
	ErrTooManySessions     = apperror.New(http.StatusBadRequest, "too many sessions (maximum 100)")
	ErrSessionsOverlap     = apperror.New(http.StatusBadRequest, "sessions of one course must not overlap")
	ErrSessionInPast       = apperror.New(http.StatusBadRequest, "sessions must start in the future")
	ErrSingleSession       = apperror.New(http.StatusBadRequest, "trial and single courses have exactly one session")
	ErrInvalidTimezone     = apperror.New(http.StatusBadRequest, "timezone must be a valid IANA time zone")
	ErrInvalidPayment      = apperror.New(http.StatusBadRequest, "payment_methods must be a subset of onsite and transfer, and a paid course needs at least one")
	ErrBankInfoRequired    = apperror.New(http.StatusBadRequest, "bank_code and bank_account are required when transfer is accepted")
	ErrInvalidBankInfo     = apperror.New(http.StatusBadRequest, "bank_code must be 3-7 digits and bank_account 6-20 digits")
	ErrInvalidRule         = apperror.New(http.StatusBadRequest, "invalid course rule value")
	ErrSportNotFound       = apperror.New(http.StatusBadRequest, "sport not found or inactive")
	ErrLocationNotFound    = apperror.New(http.StatusBadRequest, "location not found")
	ErrNotCoachOfGroup     = apperror.New(http.StatusForbidden, "only active leaders and coaches of the group can create group courses")
	ErrPrivateNotAllowed   = apperror.New(http.StatusForbidden, "your coach group does not allow private courses")
	ErrInvalidTeacher      = apperror.New(http.StatusBadRequest, "teaching coaches must be active members of the group")
	ErrCapacityBelowEnroll = apperror.New(http.StatusConflict, "capacity cannot be below the number of enrolled students")
	ErrNothingToUpdate     = apperror.New(http.StatusBadRequest, "no fields to update")
	ErrCourseCancelled     = apperror.New(http.StatusConflict, "course is cancelled")
	ErrSessionPast         = apperror.New(http.StatusConflict, "past sessions cannot be changed")
	ErrSessionCancelled    = apperror.New(http.StatusConflict, "session is cancelled")
	ErrSessionConflict     = apperror.New(http.StatusConflict, "session overlaps another session of this course")

	ErrInvalidDeadline     = apperror.New(http.StatusBadRequest, "enrollment_deadline must be in the future and before the last session ends")
	ErrEnrollClosed        = apperror.New(http.StatusConflict, "enrollment is closed for this course")
	ErrEnrollDeadline      = apperror.New(http.StatusConflict, "the enrollment deadline has passed")
	ErrCourseFull          = apperror.New(http.StatusConflict, "course is full")
	ErrAlreadyEnrolled     = apperror.New(http.StatusConflict, "already enrolled in this course")
	ErrNoUpcomingSessions  = apperror.New(http.StatusConflict, "course has no upcoming sessions")
	ErrOwnCourse           = apperror.New(http.StatusForbidden, "teachers cannot enroll in their own course")
	ErrPaymentMethod       = apperror.New(http.StatusBadRequest, "choose a payment method accepted by this course")
	ErrCoachUnavailable    = apperror.New(http.StatusConflict, "the coach of this course is not available")
	ErrEnrollmentForbidden = apperror.New(http.StatusForbidden, "you cannot view this enrollment")
)

// Course types.
const (
	TypeTrial     = "trial"
	TypeSingle    = "single"
	TypeRecurring = "recurring"
)

// Course statuses. "closed" stops new enrollments; "cancelled" ends the course.
const (
	StatusOpen      = "open"
	StatusClosed    = "closed"
	StatusCancelled = "cancelled"
)

// Session statuses.
const (
	SessionScheduled = "scheduled"
	SessionCancelled = "cancelled"
)

// Enrollment statuses.
const (
	EnrollActive    = "active"
	EnrollCancelled = "cancelled"
)

// Payment statuses of an enrollment, tracked independently of its status.
const (
	PayNotRequired = "not_required"
	PayUnpaid      = "unpaid"
	PayReported    = "reported"
	PayPaid        = "paid"
)

// Payment methods.
const (
	MethodOnsite   = "onsite"
	MethodTransfer = "transfer"
)

// Cancel sources recorded on a cancelled enrollment.
const (
	CancelByStudent = "student"
	CancelByCoach   = "coach"
	CancelByCourse  = "course_cancelled"
	CancelBySystem  = "system"
)

// Field limits (characters unless noted).
const (
	MaxTitle        = 100
	MaxDescription  = 2000
	MaxTeaching     = 2000
	MaxVenueName    = 100
	MaxRuleText     = 500
	MaxSessions     = 100
	MaxCoaches      = 10
	MaxScheduleDays = 366
	MaxVenueNote    = 200
	MaxReason       = 200
	MaxPrice        = 1000000
	MaxCapacity     = 500
	DefaultTimezone = "Asia/Taipei"
)

// Notification types and reference kinds.
const (
	NotifEnrolled         = "course_enrolled"          // To teachers: a student enrolled
	NotifSessionChanged   = "course_session_changed"   // To students: a session was rescheduled
	NotifSessionCancelled = "course_session_cancelled" // To students: a session was cancelled
	NotifCourseCancelled  = "course_cancelled"         // To students: the course was cancelled
	NotifCourseUpdated    = "course_updated"           // To students: key course details changed

	RefCourse     = "course"
	RefEnrollment = "enrollment"
	RefSession    = "course_session"
)

// Teacher is a teaching coach shown on a course.
type Teacher struct {
	UserID      string
	DisplayName string
	Avatar      *string
}

// Course is a coaching course.
type Course struct {
	ID               string
	CoachID          *string
	GroupID          *string
	CreatedBy        *string
	OwnerName        string // Coach display name or group name
	Title            string
	Description      string
	TeachingContent  string
	SportID          string
	SportName        string
	Type             string
	Capacity         int
	VenueName        string
	LocationID       *string
	Timezone         string
	Price            int
	VenueFee         int
	PaymentMethods   []string
	BankCode         *string
	BankAccount      *string
	BankAccountName  *string
	AllowLeave       bool
	LeaveDeadlineHrs *int
	AllowMakeup      bool
	MakeupLimit      *int
	MakeupValidDays  *int
	AllowCancel      bool
	CancelDeadline   *int
	RulesLeave       string
	RulesMakeup      string
	RulesCancel      string
	RulesRefund      string
	RulesNoShow      string
	EnrollDeadline   *time.Time
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time

	Teachers       []Teacher
	EnrolledCount  int
	SessionCount   int
	FirstSessionAt *time.Time
	LastSessionAt  *time.Time
	NextSessionAt  *time.Time
	Sessions       []*Session // Populated on detail only

	// Viewer-specific fields, filled by the service.
	CanManage         bool
	MyEnrollmentID    *string
	ParticipantStats  *ParticipantStatsView
	BankInfoAvailable bool
}

// IsFree reports whether the course costs nothing (fee and venue fee both zero).
func (c *Course) IsFree() bool { return c.Price+c.VenueFee == 0 }

// Session is one class meeting.
type Session struct {
	ID           string
	CourseID     string
	Seq          int
	StartAt      time.Time
	EndAt        time.Time
	Status       string
	VenueNote    string
	CancelReason *string
}

// Enrollment is a student's seat in a course.
type Enrollment struct {
	ID            string
	CourseID      string
	UserID        string
	Status        string
	PaymentStatus string
	PaymentMethod *string
	Price         int
	VenueFee      int
	AmountDue     int
	CancelSource  *string
	CancelledAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time

	// Joined data.
	Course     *CourseBrief
	Student    *Student
	Attendance AttendanceSummary
}

// CourseBrief is the course summary shown next to an enrollment.
type CourseBrief struct {
	ID            string
	Title         string
	Type          string
	Status        string
	SportName     string
	OwnerName     string
	NextSessionAt *time.Time
}

// Student is the student's identity as shown to a course manager.
type Student struct {
	UserID      string
	DisplayName string
	Avatar      *string
	Phone       *string
	LineID      *string
	SkillLevel  *int
	SkillLabel  *string
}

// ParticipantStatsView is the anonymous roster summary shown publicly.
type ParticipantStatsView struct {
	Total       int
	Genders     []CountItem
	AgeGroups   []CountItem
	SkillLevels []LevelCount
}

// CountItem is a labelled count.
type CountItem struct {
	Key   string
	Count int
}

// LevelCount is the number of students at one skill level (level 0 = not declared).
type LevelCount struct {
	Level int
	Label string
	Count int
}

// Filter defines parameters for searching courses.
type Filter struct {
	SportID      string
	CourseType   string
	CoachID      string // Courses taught by this coach (individually owned or via group)
	GroupID      string
	LocationID   string
	Keyword      string // Matches title and owner name
	Status       string // Default open
	FreeOnly     bool
	HasSeats     bool
	IncludeEnded bool // Include courses without upcoming sessions
	StartFrom    *time.Time
	StartTo      *time.Time
	ManagedBy    string // Only courses this user manages (set by service from the viewer)
	EnrolledBy   string // Only courses this user is actively enrolled in
	SortBy       string // next_session (default), created_at, price
	SortOrder    string
	Page         int
	PageSize     int
}

// EnrollmentFilter defines parameters for listing enrollments.
type EnrollmentFilter struct {
	CourseID      string
	UserID        string
	Status        string
	PaymentStatus string
	Page          int
	PageSize      int
}

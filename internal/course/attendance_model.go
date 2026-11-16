package course

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrLeaveNotAllowed   = apperror.New(http.StatusConflict, "this course does not allow leave requests")
	ErrLeaveDeadline     = apperror.New(http.StatusConflict, "the leave deadline for this session has passed")
	ErrLeaveExists       = apperror.New(http.StatusConflict, "you already requested leave for this session")
	ErrLeaveNotFound     = apperror.New(http.StatusNotFound, "leave request not found")
	ErrLeaveUsed         = apperror.New(http.StatusConflict, "a make-up session is already booked for this leave")
	ErrLeaveOpen         = apperror.New(http.StatusConflict, "no make-up session is booked for this leave")
	ErrSessionStarted    = apperror.New(http.StatusConflict, "the session has already started")
	ErrSessionNotStarted = apperror.New(http.StatusConflict, "attendance can only be recorded once the session has started")
	ErrNotInRoster       = apperror.New(http.StatusNotFound, "you are not enrolled in this session")
	ErrAttendanceMarked  = apperror.New(http.StatusConflict, "attendance was already recorded for this session")
	ErrOnLeave           = apperror.New(http.StatusConflict, "the student is on leave for this session; cancel the leave first")
	ErrRecordNotInRoster = apperror.New(http.StatusBadRequest, "every record must belong to a student attending this session")
	ErrNoRecords         = apperror.New(http.StatusBadRequest, "provide 1-200 attendance records")
	ErrMakeupNotAllowed  = apperror.New(http.StatusConflict, "this course does not allow make-up sessions")
	ErrMakeupLimit       = apperror.New(http.StatusConflict, "the make-up limit for this enrollment is used up")
	ErrMakeupExpired     = apperror.New(http.StatusConflict, "the make-up period for this leave has expired")
	ErrMakeupUnavailable = apperror.New(http.StatusConflict, "that session is not available for make-up (full, past, or not eligible)")
	ErrLeaveForbidden    = apperror.New(http.StatusForbidden, "only the student can do this")
	ErrInvalidStatus     = apperror.New(http.StatusBadRequest, "status must be present, absent, or pending")
)

// Attendance kinds and statuses.
const (
	KindRegular = "regular"
	KindMakeup  = "makeup"

	AttPending = "pending"
	AttPresent = "present"
	AttAbsent  = "absent"
	AttLeave   = "leave"

	LeaveOpen = "open"
	LeaveUsed = "used"
)

// Notification types and reference kinds.
const (
	NotifLeaveRequested = "leave_requested" // To teachers: a student asked for leave
	NotifMakeupBooked   = "makeup_booked"   // To teachers of the target class: a student joins as a make-up guest
	NotifMakeupReleased = "makeup_released" // To a student: their make-up session was cancelled and the leave is open again

	RefLeave = "leave_request"
)

// SessionInfo is a session with the rules of its course.
type SessionInfo struct {
	ID               string
	CourseID         string
	CourseTitle      string
	CourseStatus     string
	Timezone         string
	StartAt          time.Time
	EndAt            time.Time
	Status           string
	AllowLeave       bool
	LeaveDeadlineHrs *int
	AllowMakeup      bool
}

// LeaveRequest is a student's leave for one session.
type LeaveRequest struct {
	ID              string
	SessionID       string
	EnrollmentID    string
	StudentID       string
	StudentName     string
	CourseID        string
	CourseTitle     string
	SessionStartAt  time.Time
	SessionEndAt    time.Time
	Reason          string
	Status          string
	MakeupSessionID *string
	MakeupStartAt   *time.Time
	MakeupCourse    *string
	CreatedAt       time.Time
}

// MakeupEligibility summarizes whether a leave can be turned into a make-up.
type MakeupEligibility struct {
	Eligible bool
	Reason   string // Why not, when Eligible is false
	// Remaining is how many make-ups the enrollment may still book (nil = unlimited).
	Remaining *int
	// Until is the latest start time of a make-up session (nil = no limit).
	Until *time.Time
}

// MakeupOption is a session a student may join as a make-up guest.
type MakeupOption struct {
	SessionID      string
	CourseID       string
	CourseTitle    string
	OwnerName      string
	StartAt        time.Time
	EndAt          time.Time
	Timezone       string
	VenueName      string
	RemainingSeats int
}

// RosterEntry is one student expected at (or visiting) a session.
type RosterEntry struct {
	EnrollmentID string
	UserID       string
	DisplayName  string
	Avatar       *string
	Phone        *string
	LineID       *string
	SkillLevel   *int
	SkillLabel   *string
	Kind         string
	Status       string
	MarkedAt     *time.Time
	FromCourse   *string // For make-up guests: the course they normally attend
}

// AttendanceRecord marks one student's attendance.
type AttendanceRecord struct {
	EnrollmentID string
	Status       string
}

// AttendanceItem is one session in a student's attendance history.
type AttendanceItem struct {
	SessionID     string
	CourseID      string
	CourseTitle   string
	StartAt       time.Time
	EndAt         time.Time
	SessionStatus string
	Kind          string
	Status        string
}

// AttendanceSummary counts a student's recorded outcomes.
type AttendanceSummary struct {
	Present int
	Absent  int
	Leave   int
}

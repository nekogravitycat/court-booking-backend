// Package trust holds the platform's trust signals around coaches: public
// reviews, complaints, the internal reputation ledger and suspensions.
package trust

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrReviewNotFound    = apperror.New(http.StatusNotFound, "review not found")
	ErrReviewExists      = apperror.New(http.StatusConflict, "you already reviewed this coach for this enrollment")
	ErrNotAttended       = apperror.New(http.StatusForbidden, "only students who attended a session can review the coach")
	ErrInvalidRating     = apperror.New(http.StatusBadRequest, "rating must be between 0 and 5 in steps of 0.5")
	ErrCommentTooLong    = apperror.New(http.StatusBadRequest, "comment must be at most 50 characters")
	ErrCoachRequired     = apperror.New(http.StatusBadRequest, "coach_id is required because this course has several teaching coaches")
	ErrNotCourseCoach    = apperror.New(http.StatusBadRequest, "that coach does not teach this course")
	ErrHideReason        = apperror.New(http.StatusBadRequest, "a reason of 1-200 characters is required to hide a review")
	ErrEnrollmentMissing = apperror.New(http.StatusNotFound, "enrollment not found")

	ErrComplaintNotFound = apperror.New(http.StatusNotFound, "complaint not found")
	ErrInvalidCategory   = apperror.New(http.StatusBadRequest, "unknown complaint category")
	ErrInvalidComplaint  = apperror.New(http.StatusBadRequest, "description must be 1-1000 characters")
	ErrTargetRequired    = apperror.New(http.StatusBadRequest, "target_user_id is required because this course has several teaching coaches")
	ErrInvalidTarget     = apperror.New(http.StatusBadRequest, "the complaint target must be the other party of this enrollment")
	ErrComplaintOpen     = apperror.New(http.StatusConflict, "you already have an open complaint about this person for this enrollment")
	ErrInvalidStatus     = apperror.New(http.StatusBadRequest, "status must be open, in_review, resolved, or dismissed")
	ErrStatusUnchanged   = apperror.New(http.StatusConflict, "the complaint already has that status")
	ErrNoteRequired      = apperror.New(http.StatusBadRequest, "a note of 1-1000 characters is required")

	ErrCoachNotFound     = apperror.New(http.StatusNotFound, "coach not found")
	ErrInvalidDelta      = apperror.New(http.StatusBadRequest, "delta must be a non-zero integer between -100 and 100")
	ErrReasonRequired    = apperror.New(http.StatusBadRequest, "a reason of 1-200 characters is required")
	ErrAlreadySuspended  = apperror.New(http.StatusConflict, "the coach is already suspended")
	ErrCoachNotApproved  = apperror.New(http.StatusConflict, "only approved coaches can be suspended")
	ErrSuspensionMissing = apperror.New(http.StatusNotFound, "suspension not found")
	ErrSuspensionLifted  = apperror.New(http.StatusConflict, "the suspension was already lifted")
)

// BaseScore is the starting point of the internal reputation score. It is a
// placeholder for display only: the platform has not confirmed any deduction
// rule, threshold, or automatic suspension, so none is implemented.
const BaseScore = 100

// Review limits.
const (
	MaxComment = 50
	MaxReason  = 200
	MaxNote    = 1000
)

// Complaint statuses.
const (
	StatusOpen      = "open"
	StatusInReview  = "in_review"
	StatusResolved  = "resolved"
	StatusDismissed = "dismissed"
)

// Complaint categories. Extend this list as the platform's rules grow.
var Categories = []string{"no_show", "misconduct", "payment", "safety", "quality", "other"}

// Complaint event types.
const (
	EventCreated       = "created"
	EventStatusChanged = "status_changed"
	EventNote          = "note"
)

// Complaint reporter roles.
const (
	RoleStudent = "student"
	RoleCoach   = "coach"
)

// Notification types and reference kinds.
const (
	NotifReviewReceived   = "coach_review_received"
	NotifComplaintFiled   = "complaint_filed"         // To admins
	NotifComplaintUpdated = "complaint_updated"       // To the reporter
	NotifCoachSuspended   = "coach_suspended"         // To the coach
	NotifSuspensionLifted = "coach_suspension_lifted" // To the coach
	RefReview             = "coach_review"
	RefComplaint          = "complaint"
	RefSuspension         = "coach_suspension"
)

// Review is a public review of a coach.
type Review struct {
	ID           string
	EnrollmentID string
	CoachID      string
	CourseID     string
	CourseTitle  string
	ReviewerID   string
	ReviewerName string
	Rating       float64 // 0-5 in steps of 0.5
	Comment      string
	IsVisible    bool
	HiddenReason *string
	HiddenAt     *time.Time
	CreatedAt    time.Time
}

// ReviewSummary is the public rating of a coach.
type ReviewSummary struct {
	Average *float64 // nil when there are no visible reviews
	Count   int
}

// ReviewFilter selects reviews.
type ReviewFilter struct {
	CoachID      string
	ReviewerID   string
	EnrollmentID string
	VisibleOnly  bool
	HiddenOnly   bool
	Page         int
	PageSize     int
}

// Complaint is a ticket between the two parties of an enrollment.
type Complaint struct {
	ID             string
	ReporterID     string
	ReporterName   string
	ReporterRole   string
	TargetUserID   string
	TargetName     string
	EnrollmentID   string
	CourseID       string
	CourseTitle    string
	Category       string
	Description    string
	Status         string
	ResolutionNote *string
	ResolvedBy     *string
	ResolvedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Events         []*ComplaintEvent
}

// ComplaintEvent is one entry of a complaint's history.
type ComplaintEvent struct {
	ID         string
	ActorID    *string
	ActorName  *string
	Type       string
	FromStatus *string
	ToStatus   *string
	Note       string
	IsInternal bool
	CreatedAt  time.Time
}

// ComplaintFilter selects complaints.
type ComplaintFilter struct {
	ReporterID string
	TargetID   string
	Status     string
	Category   string
	Page       int
	PageSize   int
}

// ReputationEvent is one entry of a coach's internal reputation ledger.
type ReputationEvent struct {
	ID          string
	CoachID     string
	Delta       int
	Reason      string
	ComplaintID *string
	CreatedBy   *string
	CreatedAt   time.Time
}

// Reputation is the internal view of a coach for platform staff.
type Reputation struct {
	CoachID          string
	CoachName        string
	CoachStatus      string
	BaseScore        int
	TotalAdjustment  int
	Score            int
	Events           []*ReputationEvent
	Rating           ReviewSummary
	OpenComplaints   int
	TotalComplaints  int
	ActiveSuspension *Suspension
}

// Suspension is a record of a coach being suspended.
type Suspension struct {
	ID        string
	CoachID   string
	CoachName string
	Reason    string
	Status    string
	CreatedBy *string
	CreatedAt time.Time
	LiftedBy  *string
	LiftedAt  *time.Time
	LiftNote  *string
	Impact    *SuspensionImpact // Populated on suspend and on request
}

// SuspensionImpact lists what a suspension leaves behind so staff can handle
// it by hand. Nothing is cancelled automatically.
type SuspensionImpact struct {
	Courses     []AffectedCourse
	Sessions    []AffectedSession
	Enrollments []AffectedEnrollment
}

// AffectedCourse is a live course the coach teaches.
type AffectedCourse struct {
	ID                  string
	Title               string
	Status              string
	OtherActiveTeachers int // Teachers who can still run the course
	UpcomingSessions    int
	ActiveEnrollments   int
}

// AffectedSession is an upcoming session of an affected course.
type AffectedSession struct {
	ID          string
	CourseID    string
	CourseTitle string
	StartAt     time.Time
	EndAt       time.Time
}

// AffectedEnrollment is an active enrollment of an affected course.
type AffectedEnrollment struct {
	ID            string
	CourseID      string
	CourseTitle   string
	StudentID     string
	StudentName   string
	PaymentStatus string
	AmountDue     int
}

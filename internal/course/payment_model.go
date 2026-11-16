package course

import (
	"net/http"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/pkg/apperror"
)

var (
	ErrReportNotFound       = apperror.New(http.StatusNotFound, "transfer report not found")
	ErrReportPending        = apperror.New(http.StatusConflict, "a transfer report is already waiting for review")
	ErrReportClosed         = apperror.New(http.StatusConflict, "transfer report was already reviewed")
	ErrInvalidReport        = apperror.New(http.StatusBadRequest, "last5 must be exactly 5 digits and amount between 1 and 2000000")
	ErrInvalidDecision      = apperror.New(http.StatusBadRequest, "decision must be approve or reject")
	ErrNoteRequired         = apperror.New(http.StatusBadRequest, "a note is required when rejecting")
	ErrNotTransfer          = apperror.New(http.StatusConflict, "this enrollment does not pay by bank transfer")
	ErrAlreadyPaid          = apperror.New(http.StatusConflict, "this enrollment is already paid or refunded")
	ErrEnrollmentInactive   = apperror.New(http.StatusConflict, "enrollment is not active")
	ErrNoPaymentNeeded      = apperror.New(http.StatusConflict, "this enrollment does not require payment")
	ErrPaymentUnchanged     = apperror.New(http.StatusConflict, "payment status cannot be changed that way")
	ErrInvalidPaymentTarget = apperror.New(http.StatusBadRequest, "payment_status must be paid or unpaid")

	ErrCancellationNotFound = apperror.New(http.StatusNotFound, "cancellation request not found")
	ErrCancellationPending  = apperror.New(http.StatusConflict, "a cancellation request is already open for this enrollment")
	ErrCancellationClosed   = apperror.New(http.StatusConflict, "cancellation request was already resolved")
	ErrInvalidResolution    = apperror.New(http.StatusBadRequest, "resolution must be cancelled, refunded, makeup, rejected, or other")
	ErrInvalidRefund        = apperror.New(http.StatusBadRequest, "refund_amount is required for a refund and must be between 0 and the amount paid; it is not allowed otherwise")
	ErrNothingToRefund      = apperror.New(http.StatusConflict, "only paid enrollments can be refunded")
	ErrReasonRequired       = apperror.New(http.StatusBadRequest, "a reason of 1-200 characters is required")
	ErrInvalidSession       = apperror.New(http.StatusBadRequest, "session does not belong to this course")
	ErrCancelForbidden      = apperror.New(http.StatusForbidden, "you cannot cancel this enrollment")
)

// Payment status of a refunded enrollment.
const PayRefunded = "refunded"

// Transfer report states.
const (
	ReportPending  = "pending"
	ReportApproved = "approved"
	ReportRejected = "rejected"
)

// Cancellation request states, requesters, and resolutions.
const (
	CancelPending  = "pending"
	CancelResolved = "resolved"

	RoleStudent = "student"
	RoleCoach   = "coach"
	RoleSystem  = "system"

	ResolutionCancelled = "cancelled"
	ResolutionRefunded  = "refunded"
	ResolutionMakeup    = "makeup"
	ResolutionRejected  = "rejected"
	ResolutionOther     = "other"
)

// Notification types.
const (
	NotifTransferReported     = "transfer_reported"      // To teachers: a student reported a transfer
	NotifTransferApproved     = "transfer_approved"      // To the student
	NotifTransferRejected     = "transfer_rejected"      // To the student
	NotifPaymentUpdated       = "payment_updated"        // To the student: marked paid / unpaid by hand
	NotifCancellationRequest  = "cancellation_requested" // To the other party
	NotifCancellationResolved = "cancellation_resolved"  // To the student and the requester
	NotifEnrollmentCancelled  = "enrollment_cancelled"   // To teachers: a student cancelled on their own

	RefTransferReport = "transfer_report"
	RefCancellation   = "cancellation_request"
)

// TransferReport is a student's report of a bank transfer.
type TransferReport struct {
	ID             string
	EnrollmentID   string
	CourseID       string
	CourseTitle    string
	StudentID      string
	StudentName    string
	Last5          string
	Amount         int
	ExpectedAmount int // The enrollment's amount due: course fee plus venue fee
	TransferredAt  *time.Time
	Note           string
	Status         string
	ReviewNote     *string
	ReviewedBy     *string
	ReviewedAt     *time.Time
	CreatedAt      time.Time
}

// AmountMatches reports whether the reported amount equals the amount due.
func (t *TransferReport) AmountMatches() bool { return t.Amount == t.ExpectedAmount }

// CancellationRequest records a cancellation, no-show, refund or make-up request.
type CancellationRequest struct {
	ID               string
	EnrollmentID     string
	CourseID         string
	CourseTitle      string
	StudentID        string
	StudentName      string
	SessionID        *string
	RequestedBy      *string
	RequesterRole    string
	Reason           string
	Status           string
	Resolution       *string
	RefundAmount     *int
	ResolutionNote   *string
	ResolvedBy       *string
	ResolvedAt       *time.Time
	CreatedAt        time.Time
	AmountDue        int
	PaymentStatus    string
	EnrollmentStatus string
}

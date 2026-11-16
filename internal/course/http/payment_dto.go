package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/course"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

type TransferReportRequest struct {
	Last5         string     `json:"last5" binding:"required,len=5,numeric"`
	Amount        *int       `json:"amount" binding:"omitempty,min=1,max=2000000"`
	TransferredAt *time.Time `json:"transferred_at"`
	Note          string     `json:"note" binding:"max=200"`
}

type ReviewTransferRequest struct {
	Decision string `json:"decision" binding:"required,oneof=approve reject"`
	Note     string `json:"note" binding:"max=500"`
}

type UpdatePaymentRequest struct {
	PaymentStatus string `json:"payment_status" binding:"required,oneof=paid unpaid"`
}

type CancellationBody struct {
	Reason    string  `json:"reason" binding:"required,max=200"`
	SessionID *string `json:"session_id" binding:"omitempty,uuid"`
}

type ResolveCancellationRequest struct {
	Resolution   string `json:"resolution" binding:"required,oneof=cancelled refunded makeup rejected other"`
	RefundAmount *int   `json:"refund_amount" binding:"omitempty,min=0,max=2000000"`
	Note         string `json:"note" binding:"max=500"`
}

type ListReportsRequest struct {
	request.ListParams
	Status   string `form:"status" binding:"omitempty,oneof=pending approved rejected"`
	CourseID string `form:"course_id" binding:"omitempty,uuid"`
}

type ListCancellationsRequest struct {
	request.ListParams
	Status   string `form:"status" binding:"omitempty,oneof=pending resolved"`
	CourseID string `form:"course_id" binding:"omitempty,uuid"`
}

type PersonRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TransferReportResponse struct {
	ID             string     `json:"id"`
	EnrollmentID   string     `json:"enrollment_id"`
	CourseID       string     `json:"course_id"`
	CourseTitle    string     `json:"course_title"`
	Student        PersonRef  `json:"student"`
	Last5          string     `json:"last5"`
	Amount         int        `json:"amount"`
	ExpectedAmount int        `json:"expected_amount"`
	AmountMatches  bool       `json:"amount_matches"`
	TransferredAt  *time.Time `json:"transferred_at"`
	Note           string     `json:"note"`
	Status         string     `json:"status"`
	ReviewNote     *string    `json:"review_note"`
	ReviewedAt     *time.Time `json:"reviewed_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

func NewTransferReportResponse(t *course.TransferReport) TransferReportResponse {
	return TransferReportResponse{
		ID: t.ID, EnrollmentID: t.EnrollmentID, CourseID: t.CourseID, CourseTitle: t.CourseTitle,
		Student: PersonRef{ID: t.StudentID, Name: t.StudentName}, Last5: t.Last5, Amount: t.Amount,
		ExpectedAmount: t.ExpectedAmount, AmountMatches: t.AmountMatches(), TransferredAt: utcPtr(t.TransferredAt),
		Note: t.Note, Status: t.Status, ReviewNote: t.ReviewNote, ReviewedAt: utcPtr(t.ReviewedAt), CreatedAt: t.CreatedAt.UTC(),
	}
}

type CancellationResponse struct {
	ID               string     `json:"id"`
	EnrollmentID     string     `json:"enrollment_id"`
	CourseID         string     `json:"course_id"`
	CourseTitle      string     `json:"course_title"`
	Student          PersonRef  `json:"student"`
	SessionID        *string    `json:"session_id"`
	RequestedBy      *string    `json:"requested_by"`
	RequesterRole    string     `json:"requester_role"`
	Reason           string     `json:"reason"`
	Status           string     `json:"status"`
	Resolution       *string    `json:"resolution"`
	RefundAmount     *int       `json:"refund_amount"`
	ResolutionNote   *string    `json:"resolution_note"`
	ResolvedAt       *time.Time `json:"resolved_at"`
	AmountDue        int        `json:"amount_due"`
	PaymentStatus    string     `json:"payment_status"`
	EnrollmentStatus string     `json:"enrollment_status"`
	CreatedAt        time.Time  `json:"created_at"`
}

func NewCancellationResponse(q *course.CancellationRequest) CancellationResponse {
	return CancellationResponse{
		ID: q.ID, EnrollmentID: q.EnrollmentID, CourseID: q.CourseID, CourseTitle: q.CourseTitle,
		Student: PersonRef{ID: q.StudentID, Name: q.StudentName}, SessionID: q.SessionID, RequestedBy: q.RequestedBy,
		RequesterRole: q.RequesterRole, Reason: q.Reason, Status: q.Status, Resolution: q.Resolution,
		RefundAmount: q.RefundAmount, ResolutionNote: q.ResolutionNote, ResolvedAt: utcPtr(q.ResolvedAt),
		AmountDue: q.AmountDue, PaymentStatus: q.PaymentStatus, EnrollmentStatus: q.EnrollmentStatus, CreatedAt: q.CreatedAt.UTC(),
	}
}

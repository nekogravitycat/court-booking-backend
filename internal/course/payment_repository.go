package course

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
)

// TransferReportFilter selects transfer reports. ManagedBy limits the result
// to courses the user manages; leave it empty for an unrestricted (admin) list.
type TransferReportFilter struct {
	CourseID     string
	EnrollmentID string
	Status       string
	ManagedBy    string
	Page         int
	PageSize     int
}

// CancellationFilter selects cancellation requests.
type CancellationFilter struct {
	CourseID     string
	EnrollmentID string
	RequestedBy  string // Requests this user filed
	ForStudent   string // Requests about this student's enrollments
	ViewerID     string // Requests the user may see: about their own enrollments or on courses they manage
	Status       string
	ManagedBy    string
	Page         int
	PageSize     int
}

// NewTransferReport carries a student's transfer report.
type NewTransferReport struct {
	EnrollmentID  string
	UserID        string
	Last5         string
	Amount        int
	TransferredAt *time.Time
	Note          string
}

// NewCancellation carries a cancellation request. Auto cancels the enrollment
// immediately (used when the course rules allow a no-refund self-service cancel).
type NewCancellation struct {
	EnrollmentID string
	SessionID    *string
	RequestedBy  string
	Role         string
	Reason       string
	Auto         bool
}

// CancellationResolution is a coach's decision on a request.
type CancellationResolution struct {
	Resolution   string
	RefundAmount *int
	Note         string
	ResolvedBy   string
}

// PaymentRepository persists transfer reports, payment marks and cancellations.
type PaymentRepository interface {
	CreateTransferReport(ctx context.Context, n NewTransferReport) (*TransferReport, error)
	GetTransferReport(ctx context.Context, id string) (*TransferReport, error)
	ListTransferReports(ctx context.Context, f TransferReportFilter) ([]*TransferReport, int, error)
	// ReviewTransferReport approves (enrollment becomes paid) or rejects
	// (enrollment returns to unpaid) a pending report.
	ReviewTransferReport(ctx context.Context, id, reviewerID string, approve bool, note string) (*TransferReport, error)
	// SetPaymentStatus marks an enrollment paid or unpaid by hand.
	SetPaymentStatus(ctx context.Context, enrollmentID, to, byUser string) (*Enrollment, error)

	CreateCancellation(ctx context.Context, n NewCancellation) (*CancellationRequest, error)
	GetCancellation(ctx context.Context, id string) (*CancellationRequest, error)
	ListCancellations(ctx context.Context, f CancellationFilter) ([]*CancellationRequest, int, error)
	ResolveCancellation(ctx context.Context, id string, r CancellationResolution) (*CancellationRequest, error)
	// FirstSessionStart returns the earliest scheduled session start of the
	// enrollment's course (nil when none).
	FirstSessionStart(ctx context.Context, courseID string) (*time.Time, error)
	// SessionInCourse reports whether the session belongs to the course.
	SessionInCourse(ctx context.Context, sessionID, courseID string) (bool, error)
}

type pgxPaymentRepository struct {
	pool *pgxpool.Pool
}

func NewPgxPaymentRepository(pool *pgxpool.Pool) PaymentRepository {
	return &pgxPaymentRepository{pool: pool}
}

const reportColumns = `r.id, r.enrollment_id, e.course_id, c.title, e.user_id, COALESCE(u.display_name, u.username),
	r.last5, r.amount, e.amount_due, r.transferred_at, r.note, r.status, r.review_note, r.reviewed_by, r.reviewed_at, r.created_at`

func reportSelect(withTotal bool) squirrel.SelectBuilder {
	cols := reportColumns
	if withTotal {
		cols += ", count(*) OVER() AS total_count"
	}
	return squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select(cols).
		From("public.transfer_reports r").
		Join("public.enrollments e ON e.id = r.enrollment_id").
		Join("public.courses c ON c.id = e.course_id").
		Join("public.users u ON u.id = e.user_id")
}

func scanReport(row pgx.Row, extra ...any) (*TransferReport, error) {
	var t TransferReport
	dest := append([]any{&t.ID, &t.EnrollmentID, &t.CourseID, &t.CourseTitle, &t.StudentID, &t.StudentName,
		&t.Last5, &t.Amount, &t.ExpectedAmount, &t.TransferredAt, &t.Note, &t.Status, &t.ReviewNote, &t.ReviewedBy, &t.ReviewedAt, &t.CreatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *pgxPaymentRepository) getReport(ctx context.Context, q querier, id string) (*TransferReport, error) {
	sql, args, err := reportSelect(false).Where(squirrel.Eq{"r.id": id}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build report query failed: %w", err)
	}
	t, err := scanReport(q.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReportNotFound
		}
		return nil, fmt.Errorf("get transfer report failed: %w", err)
	}
	return t, nil
}

func (r *pgxPaymentRepository) GetTransferReport(ctx context.Context, id string) (*TransferReport, error) {
	return r.getReport(ctx, r.pool, id)
}

func (r *pgxPaymentRepository) CreateTransferReport(ctx context.Context, n NewTransferReport) (*TransferReport, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var owner, status, payStatus string
	var method *string
	err = tx.QueryRow(ctx, "SELECT user_id, status, payment_status, payment_method FROM public.enrollments WHERE id = $1 FOR UPDATE",
		n.EnrollmentID).Scan(&owner, &status, &payStatus, &method)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != n.UserID) {
		return nil, ErrEnrollmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock enrollment failed: %w", err)
	}
	if status != EnrollActive {
		return nil, ErrEnrollmentInactive
	}
	if method == nil || *method != MethodTransfer {
		return nil, ErrNotTransfer
	}
	if payStatus == PayPaid || payStatus == PayRefunded {
		return nil, ErrAlreadyPaid
	}

	var id string
	err = tx.QueryRow(ctx, `INSERT INTO public.transfer_reports (enrollment_id, last5, amount, transferred_at, note)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, n.EnrollmentID, n.Last5, n.Amount, n.TransferredAt, n.Note).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrReportPending
		}
		return nil, fmt.Errorf("create transfer report failed: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE public.enrollments SET payment_status = 'reported', updated_at = now() WHERE id = $1", n.EnrollmentID); err != nil {
		return nil, fmt.Errorf("mark enrollment reported failed: %w", err)
	}
	t, err := r.getReport(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *pgxPaymentRepository) ListTransferReports(ctx context.Context, f TransferReportFilter) ([]*TransferReport, int, error) {
	q := reportSelect(true).OrderBy("(r.status = 'pending') DESC", "r.created_at DESC", "r.id")
	if f.CourseID != "" {
		q = q.Where(squirrel.Eq{"e.course_id": f.CourseID})
	}
	if f.EnrollmentID != "" {
		q = q.Where(squirrel.Eq{"r.enrollment_id": f.EnrollmentID})
	}
	if f.Status != "" {
		q = q.Where(squirrel.Eq{"r.status": f.Status})
	}
	if f.ManagedBy != "" {
		q = q.Where(fmt.Sprintf(managerSQL, "?"), f.ManagedBy, f.ManagedBy, f.ManagedBy, f.ManagedBy)
	}
	return pagination.Collect(ctx, r.pool, q, f.Page, f.PageSize, "transfer report", func(rows pgx.Rows, total *int) (*TransferReport, error) {
		t, err := scanReport(rows, total)
		if err != nil {
			return nil, fmt.Errorf("scan transfer report failed: %w", err)
		}
		return t, nil
	})
}

func (r *pgxPaymentRepository) ReviewTransferReport(ctx context.Context, id, reviewerID string, approve bool, note string) (*TransferReport, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var enrollmentID, status string
	err = tx.QueryRow(ctx, "SELECT enrollment_id, status FROM public.transfer_reports WHERE id = $1 FOR UPDATE", id).Scan(&enrollmentID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReportNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock transfer report failed: %w", err)
	}
	if status != ReportPending {
		return nil, ErrReportClosed
	}
	var enrollStatus string
	if err := tx.QueryRow(ctx, "SELECT status FROM public.enrollments WHERE id = $1 FOR UPDATE", enrollmentID).Scan(&enrollStatus); err != nil {
		return nil, fmt.Errorf("lock enrollment failed: %w", err)
	}
	if enrollStatus != EnrollActive {
		return nil, ErrEnrollmentInactive
	}

	next, pay := ReportRejected, PayUnpaid
	if approve {
		next, pay = ReportApproved, PayPaid
	}
	var noteArg *string
	if note != "" {
		noteArg = &note
	}
	if _, err := tx.Exec(ctx, `UPDATE public.transfer_reports SET status = $2, review_note = $3, reviewed_by = $4, reviewed_at = now() WHERE id = $1`,
		id, next, noteArg, reviewerID); err != nil {
		return nil, fmt.Errorf("review transfer report failed: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE public.enrollments SET payment_status = $2, updated_at = now() WHERE id = $1", enrollmentID, pay); err != nil {
		return nil, fmt.Errorf("update payment status failed: %w", err)
	}
	t, err := r.getReport(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *pgxPaymentRepository) SetPaymentStatus(ctx context.Context, enrollmentID, to, byUser string) (*Enrollment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status, payStatus string
	err = tx.QueryRow(ctx, "SELECT status, payment_status FROM public.enrollments WHERE id = $1 FOR UPDATE", enrollmentID).Scan(&status, &payStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEnrollmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock enrollment failed: %w", err)
	}
	if status != EnrollActive {
		return nil, ErrEnrollmentInactive
	}
	switch {
	case payStatus == PayNotRequired:
		return nil, ErrNoPaymentNeeded
	case payStatus == PayRefunded:
		return nil, ErrAlreadyPaid
	case to == PayPaid && payStatus == PayPaid, to == PayUnpaid && payStatus == PayUnpaid:
		return nil, ErrPaymentUnchanged
	case to == PayUnpaid && payStatus == PayReported:
		return nil, ErrPaymentUnchanged
	}

	if to == PayPaid {
		// Marking paid settles any report still waiting for review.
		if _, err := tx.Exec(ctx, `UPDATE public.transfer_reports SET status = 'approved', review_note = 'marked paid by hand',
			reviewed_by = $2, reviewed_at = now() WHERE enrollment_id = $1 AND status = 'pending'`, enrollmentID, nullableID(byUser)); err != nil {
			return nil, fmt.Errorf("settle pending report failed: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE public.enrollments SET payment_status = $2, updated_at = now() WHERE id = $1", enrollmentID, to); err != nil {
		return nil, fmt.Errorf("update payment status failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return (&pgxRepository{pool: r.pool}).GetEnrollment(ctx, enrollmentID)
}

func (r *pgxPaymentRepository) FirstSessionStart(ctx context.Context, courseID string) (*time.Time, error) {
	var t *time.Time
	if err := r.pool.QueryRow(ctx, "SELECT min(start_at) FROM public.course_sessions WHERE course_id = $1 AND status = 'scheduled'", courseID).Scan(&t); err != nil {
		return nil, fmt.Errorf("get first session failed: %w", err)
	}
	return t, nil
}

func (r *pgxPaymentRepository) SessionInCourse(ctx context.Context, sessionID, courseID string) (bool, error) {
	var ok bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.course_sessions WHERE id = $1 AND course_id = $2)", sessionID, courseID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check session failed: %w", err)
	}
	return ok, nil
}

const cancelColumns = `q.id, q.enrollment_id, e.course_id, c.title, e.user_id, COALESCE(u.display_name, u.username),
	q.session_id, q.requested_by, q.requester_role, q.reason, q.status, q.resolution, q.refund_amount, q.resolution_note,
	q.resolved_by, q.resolved_at, q.created_at, e.amount_due, e.payment_status, e.status`

func cancelSelect(withTotal bool) squirrel.SelectBuilder {
	cols := cancelColumns
	if withTotal {
		cols += ", count(*) OVER() AS total_count"
	}
	return squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select(cols).
		From("public.cancellation_requests q").
		Join("public.enrollments e ON e.id = q.enrollment_id").
		Join("public.courses c ON c.id = e.course_id").
		Join("public.users u ON u.id = e.user_id")
}

func scanCancellation(row pgx.Row, extra ...any) (*CancellationRequest, error) {
	var q CancellationRequest
	dest := append([]any{&q.ID, &q.EnrollmentID, &q.CourseID, &q.CourseTitle, &q.StudentID, &q.StudentName,
		&q.SessionID, &q.RequestedBy, &q.RequesterRole, &q.Reason, &q.Status, &q.Resolution, &q.RefundAmount, &q.ResolutionNote,
		&q.ResolvedBy, &q.ResolvedAt, &q.CreatedAt, &q.AmountDue, &q.PaymentStatus, &q.EnrollmentStatus}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *pgxPaymentRepository) getCancellation(ctx context.Context, q querier, id string) (*CancellationRequest, error) {
	sql, args, err := cancelSelect(false).Where(squirrel.Eq{"q.id": id}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build cancellation query failed: %w", err)
	}
	c, err := scanCancellation(q.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCancellationNotFound
		}
		return nil, fmt.Errorf("get cancellation failed: %w", err)
	}
	return c, nil
}

func (r *pgxPaymentRepository) GetCancellation(ctx context.Context, id string) (*CancellationRequest, error) {
	return r.getCancellation(ctx, r.pool, id)
}

func (r *pgxPaymentRepository) CreateCancellation(ctx context.Context, n NewCancellation) (*CancellationRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, "SELECT status FROM public.enrollments WHERE id = $1 FOR UPDATE", n.EnrollmentID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEnrollmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock enrollment failed: %w", err)
	}
	if status != EnrollActive {
		return nil, ErrEnrollmentInactive
	}

	var id string
	err = tx.QueryRow(ctx, `INSERT INTO public.cancellation_requests (enrollment_id, session_id, requested_by, requester_role, reason)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, n.EnrollmentID, n.SessionID, nullableID(n.RequestedBy), n.Role, n.Reason).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCancellationPending
		}
		return nil, fmt.Errorf("create cancellation failed: %w", err)
	}
	if n.Auto {
		if err := applyCancellation(ctx, tx, n.EnrollmentID, CancelByStudent); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE public.cancellation_requests SET status = 'resolved', resolution = 'cancelled',
			resolution_note = 'cancelled by the student within the course rules', resolved_at = now() WHERE id = $1`, id); err != nil {
			return nil, fmt.Errorf("resolve automatic cancellation failed: %w", err)
		}
	}
	c, err := r.getCancellation(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// applyCancellation cancels the enrollment and rejects its pending transfer reports.
func applyCancellation(ctx context.Context, q querier, enrollmentID, source string) error {
	if _, err := q.Exec(ctx, `UPDATE public.transfer_reports SET status = 'rejected', review_note = 'enrollment cancelled', reviewed_at = now()
		WHERE enrollment_id = $1 AND status = 'pending'`, enrollmentID); err != nil {
		return fmt.Errorf("reject transfer reports failed: %w", err)
	}
	if _, err := q.Exec(ctx, `UPDATE public.enrollments SET status = 'cancelled', cancel_source = COALESCE(cancel_source, $2),
		cancelled_at = COALESCE(cancelled_at, now()), updated_at = now() WHERE id = $1`, enrollmentID, source); err != nil {
		return fmt.Errorf("cancel enrollment failed: %w", err)
	}
	return nil
}

func (r *pgxPaymentRepository) ResolveCancellation(ctx context.Context, id string, res CancellationResolution) (*CancellationRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var enrollmentID, status, role string
	var sessionID *string
	err = tx.QueryRow(ctx, "SELECT enrollment_id, status, requester_role, session_id FROM public.cancellation_requests WHERE id = $1 FOR UPDATE", id).
		Scan(&enrollmentID, &status, &role, &sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCancellationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock cancellation failed: %w", err)
	}
	if status != CancelPending {
		return nil, ErrCancellationClosed
	}

	var amountDue int
	var payStatus string
	if err := tx.QueryRow(ctx, "SELECT amount_due, payment_status FROM public.enrollments WHERE id = $1 FOR UPDATE", enrollmentID).Scan(&amountDue, &payStatus); err != nil {
		return nil, fmt.Errorf("lock enrollment failed: %w", err)
	}
	if res.Resolution == ResolutionRefunded {
		if res.RefundAmount == nil || *res.RefundAmount < 0 || *res.RefundAmount > amountDue {
			return nil, ErrInvalidRefund
		}
		if payStatus != PayPaid && payStatus != PayRefunded {
			return nil, ErrNothingToRefund
		}
	} else if res.RefundAmount != nil {
		return nil, ErrInvalidRefund
	}

	var noteArg *string
	if res.Note != "" {
		noteArg = &res.Note
	}
	if _, err := tx.Exec(ctx, `UPDATE public.cancellation_requests SET status = 'resolved', resolution = $2, refund_amount = $3,
		resolution_note = $4, resolved_by = $5, resolved_at = now() WHERE id = $1`,
		id, res.Resolution, res.RefundAmount, noteArg, nullableID(res.ResolvedBy)); err != nil {
		return nil, fmt.Errorf("resolve cancellation failed: %w", err)
	}

	// A request about the whole enrollment ends it; a session-scoped request
	// (for example a coach no-show) only records the outcome.
	if sessionID == nil && (res.Resolution == ResolutionCancelled || res.Resolution == ResolutionRefunded) {
		source := CancelByStudent
		if role != RoleStudent {
			source = CancelByCoach
		}
		if err := applyCancellation(ctx, tx, enrollmentID, source); err != nil {
			return nil, err
		}
	}
	if res.Resolution == ResolutionRefunded && res.RefundAmount != nil && *res.RefundAmount > 0 {
		if _, err := tx.Exec(ctx, "UPDATE public.enrollments SET payment_status = 'refunded', updated_at = now() WHERE id = $1", enrollmentID); err != nil {
			return nil, fmt.Errorf("record refund failed: %w", err)
		}
	}
	c, err := r.getCancellation(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *pgxPaymentRepository) ListCancellations(ctx context.Context, f CancellationFilter) ([]*CancellationRequest, int, error) {
	q := cancelSelect(true).OrderBy("(q.status = 'pending') DESC", "q.created_at DESC", "q.id")
	if f.CourseID != "" {
		q = q.Where(squirrel.Eq{"e.course_id": f.CourseID})
	}
	if f.EnrollmentID != "" {
		q = q.Where(squirrel.Eq{"q.enrollment_id": f.EnrollmentID})
	}
	if f.RequestedBy != "" {
		q = q.Where(squirrel.Eq{"q.requested_by": f.RequestedBy})
	}
	if f.ForStudent != "" {
		q = q.Where(squirrel.Eq{"e.user_id": f.ForStudent})
	}
	if f.Status != "" {
		q = q.Where(squirrel.Eq{"q.status": f.Status})
	}
	if f.ManagedBy != "" {
		q = q.Where(fmt.Sprintf(managerSQL, "?"), f.ManagedBy, f.ManagedBy, f.ManagedBy, f.ManagedBy)
	}
	if f.ViewerID != "" {
		q = q.Where("(e.user_id = ? OR "+fmt.Sprintf(managerSQL, "?")+")", f.ViewerID, f.ViewerID, f.ViewerID, f.ViewerID, f.ViewerID)
	}
	return pagination.Collect(ctx, r.pool, q, f.Page, f.PageSize, "cancellation request", func(rows pgx.Rows, total *int) (*CancellationRequest, error) {
		c, err := scanCancellation(rows, total)
		if err != nil {
			return nil, fmt.Errorf("scan cancellation request failed: %w", err)
		}
		return c, nil
	})
}

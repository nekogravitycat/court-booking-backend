package trust

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
)

// Repository persists reviews, complaints, reputation events and suspensions.
type Repository interface {
	CreateReview(ctx context.Context, r *Review) (*Review, error)
	GetReview(ctx context.Context, id string) (*Review, error)
	ListReviews(ctx context.Context, f ReviewFilter) ([]*Review, int, error)
	SetReviewVisibility(ctx context.Context, id string, visible bool, reason, adminID string) (*Review, error)
	ReviewSummary(ctx context.Context, coachID string) (ReviewSummary, error)

	CreateComplaint(ctx context.Context, c *Complaint) (*Complaint, error)
	GetComplaint(ctx context.Context, id string) (*Complaint, error)
	ListComplaints(ctx context.Context, f ComplaintFilter) ([]*Complaint, int, error)
	ListEvents(ctx context.Context, complaintID string, includeInternal bool) ([]*ComplaintEvent, error)
	// UpdateComplaint changes the status; note is the message shown to the reporter.
	UpdateComplaint(ctx context.Context, id, adminID, status, note string) (*Complaint, error)
	AddNote(ctx context.Context, id, adminID, note string) (*ComplaintEvent, error)
	AdminIDs(ctx context.Context) ([]string, error)

	CoachInfo(ctx context.Context, coachID string) (name, status string, err error)
	AddReputationEvent(ctx context.Context, e *ReputationEvent) (*ReputationEvent, error)
	ListReputationEvents(ctx context.Context, coachID string, limit int) ([]*ReputationEvent, error)
	SumReputation(ctx context.Context, coachID string) (int, error)
	ComplaintCounts(ctx context.Context, coachID string) (open, total int, err error)
	ComplaintExistsFor(ctx context.Context, complaintID, coachID string) (bool, error)

	Suspend(ctx context.Context, coachID, adminID, reason string) (*Suspension, error)
	Lift(ctx context.Context, id, adminID, note string) (*Suspension, error)
	ListSuspensions(ctx context.Context, coachID string) ([]*Suspension, error)
	ActiveSuspension(ctx context.Context, coachID string) (*Suspension, error)
	Impact(ctx context.Context, coachID string) (*SuspensionImpact, error)
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func isUniqueViolation(err error) bool {
	var e *pgconn.PgError
	return errors.As(err, &e) && e.Code == pgerrcode.UniqueViolation
}

func nullable(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// ---- Reviews ----

const reviewColumns = `r.id, r.enrollment_id, r.coach_id, r.course_id, c.title, r.reviewer_id, COALESCE(u.display_name, u.username),
	r.rating_x2, r.comment, r.is_visible, r.hidden_reason, r.hidden_at, r.created_at`

func reviewSelect(withTotal bool) squirrel.SelectBuilder {
	cols := reviewColumns
	if withTotal {
		cols += ", count(*) OVER() AS total_count"
	}
	return squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select(cols).
		From("public.coach_reviews r").
		Join("public.courses c ON c.id = r.course_id").
		Join("public.users u ON u.id = r.reviewer_id")
}

func scanReview(row pgx.Row, extra ...any) (*Review, error) {
	var r Review
	var x2 int16
	dest := append([]any{&r.ID, &r.EnrollmentID, &r.CoachID, &r.CourseID, &r.CourseTitle, &r.ReviewerID, &r.ReviewerName,
		&x2, &r.Comment, &r.IsVisible, &r.HiddenReason, &r.HiddenAt, &r.CreatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	r.Rating = float64(x2) / 2
	return &r, nil
}

func (r *pgxRepository) getReview(ctx context.Context, q querier, id string) (*Review, error) {
	sql, args, err := reviewSelect(false).Where(squirrel.Eq{"r.id": id}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build review query failed: %w", err)
	}
	rv, err := scanReview(q.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReviewNotFound
		}
		return nil, fmt.Errorf("get review failed: %w", err)
	}
	return rv, nil
}

func (r *pgxRepository) GetReview(ctx context.Context, id string) (*Review, error) {
	return r.getReview(ctx, r.pool, id)
}

func (r *pgxRepository) CreateReview(ctx context.Context, rv *Review) (*Review, error) {
	var id string
	err := r.pool.QueryRow(ctx, `INSERT INTO public.coach_reviews (enrollment_id, coach_id, course_id, reviewer_id, rating_x2, comment)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		rv.EnrollmentID, rv.CoachID, rv.CourseID, rv.ReviewerID, int16(rv.Rating*2), rv.Comment).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrReviewExists
		}
		return nil, fmt.Errorf("create review failed: %w", err)
	}
	return r.getReview(ctx, r.pool, id)
}

func (r *pgxRepository) ListReviews(ctx context.Context, f ReviewFilter) ([]*Review, int, error) {
	q := reviewSelect(true).OrderBy("r.created_at DESC", "r.id")
	if f.CoachID != "" {
		q = q.Where(squirrel.Eq{"r.coach_id": f.CoachID})
	}
	if f.ReviewerID != "" {
		q = q.Where(squirrel.Eq{"r.reviewer_id": f.ReviewerID})
	}
	if f.EnrollmentID != "" {
		q = q.Where(squirrel.Eq{"r.enrollment_id": f.EnrollmentID})
	}
	if f.VisibleOnly {
		q = q.Where("r.is_visible")
	}
	if f.HiddenOnly {
		q = q.Where("NOT r.is_visible")
	}
	return pagination.Collect(ctx, r.pool, q, f.Page, f.PageSize, "review", func(rows pgx.Rows, total *int) (*Review, error) {
		rv, err := scanReview(rows, total)
		if err != nil {
			return nil, fmt.Errorf("scan review failed: %w", err)
		}
		return rv, nil
	})
}

func (r *pgxRepository) SetReviewVisibility(ctx context.Context, id string, visible bool, reason, adminID string) (*Review, error) {
	var ct pgconn.CommandTag
	var err error
	if visible {
		ct, err = r.pool.Exec(ctx, "UPDATE public.coach_reviews SET is_visible = true, hidden_reason = NULL, hidden_by = NULL, hidden_at = NULL WHERE id = $1", id)
	} else {
		ct, err = r.pool.Exec(ctx, "UPDATE public.coach_reviews SET is_visible = false, hidden_reason = $2, hidden_by = $3, hidden_at = now() WHERE id = $1", id, reason, nullable(adminID))
	}
	if err != nil {
		return nil, fmt.Errorf("update review visibility failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrReviewNotFound
	}
	return r.getReview(ctx, r.pool, id)
}

func (r *pgxRepository) ReviewSummary(ctx context.Context, coachID string) (ReviewSummary, error) {
	var s ReviewSummary
	err := r.pool.QueryRow(ctx, `SELECT round(avg(rating_x2) / 2.0, 2)::float8, count(*) FROM public.coach_reviews
		WHERE coach_id = $1 AND is_visible`, coachID).Scan(&s.Average, &s.Count)
	if err != nil {
		return s, fmt.Errorf("review summary failed: %w", err)
	}
	return s, nil
}

// ---- Complaints ----

const complaintColumns = `k.id, k.reporter_id, COALESCE(ru.display_name, ru.username),
	CASE WHEN k.reporter_id = e.user_id THEN 'student' ELSE 'coach' END,
	k.target_user_id, COALESCE(tu.display_name, tu.username), k.enrollment_id, e.course_id, c.title,
	k.category, k.description, k.status, k.resolution_note, k.resolved_by, k.resolved_at, k.created_at, k.updated_at`

func complaintSelect(withTotal bool) squirrel.SelectBuilder {
	cols := complaintColumns
	if withTotal {
		cols += ", count(*) OVER() AS total_count"
	}
	return squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select(cols).
		From("public.complaints k").
		Join("public.enrollments e ON e.id = k.enrollment_id").
		Join("public.courses c ON c.id = e.course_id").
		Join("public.users ru ON ru.id = k.reporter_id").
		Join("public.users tu ON tu.id = k.target_user_id")
}

func scanComplaint(row pgx.Row, extra ...any) (*Complaint, error) {
	var k Complaint
	dest := append([]any{&k.ID, &k.ReporterID, &k.ReporterName, &k.ReporterRole, &k.TargetUserID, &k.TargetName, &k.EnrollmentID,
		&k.CourseID, &k.CourseTitle, &k.Category, &k.Description, &k.Status, &k.ResolutionNote, &k.ResolvedBy, &k.ResolvedAt,
		&k.CreatedAt, &k.UpdatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *pgxRepository) getComplaint(ctx context.Context, q querier, id string) (*Complaint, error) {
	sql, args, err := complaintSelect(false).Where(squirrel.Eq{"k.id": id}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build complaint query failed: %w", err)
	}
	k, err := scanComplaint(q.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrComplaintNotFound
		}
		return nil, fmt.Errorf("get complaint failed: %w", err)
	}
	return k, nil
}

func (r *pgxRepository) GetComplaint(ctx context.Context, id string) (*Complaint, error) {
	return r.getComplaint(ctx, r.pool, id)
}

func (r *pgxRepository) CreateComplaint(ctx context.Context, c *Complaint) (*Complaint, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `INSERT INTO public.complaints (reporter_id, target_user_id, enrollment_id, category, description)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, c.ReporterID, c.TargetUserID, c.EnrollmentID, c.Category, c.Description).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrComplaintOpen
		}
		return nil, fmt.Errorf("create complaint failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.complaint_events (complaint_id, actor_id, event_type, to_status)
		VALUES ($1, $2, 'created', 'open')`, id, c.ReporterID); err != nil {
		return nil, fmt.Errorf("record complaint event failed: %w", err)
	}
	out, err := r.getComplaint(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *pgxRepository) ListComplaints(ctx context.Context, f ComplaintFilter) ([]*Complaint, int, error) {
	q := complaintSelect(true).OrderBy("(k.status IN ('open', 'in_review')) DESC", "k.created_at DESC", "k.id")
	if f.ReporterID != "" {
		q = q.Where(squirrel.Eq{"k.reporter_id": f.ReporterID})
	}
	if f.TargetID != "" {
		q = q.Where(squirrel.Eq{"k.target_user_id": f.TargetID})
	}
	if f.Status != "" {
		q = q.Where(squirrel.Eq{"k.status": f.Status})
	}
	if f.Category != "" {
		q = q.Where(squirrel.Eq{"k.category": f.Category})
	}
	return pagination.Collect(ctx, r.pool, q, f.Page, f.PageSize, "complaint", func(rows pgx.Rows, total *int) (*Complaint, error) {
		k, err := scanComplaint(rows, total)
		if err != nil {
			return nil, fmt.Errorf("scan complaint failed: %w", err)
		}
		return k, nil
	})
}

func (r *pgxRepository) ListEvents(ctx context.Context, complaintID string, includeInternal bool) ([]*ComplaintEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT v.id, v.actor_id, COALESCE(u.display_name, u.username), v.event_type, v.from_status, v.to_status,
		v.note, v.is_internal, v.created_at
		FROM public.complaint_events v LEFT JOIN public.users u ON u.id = v.actor_id
		WHERE v.complaint_id = $1 AND ($2 OR NOT v.is_internal) ORDER BY v.created_at, v.id`, complaintID, includeInternal)
	if err != nil {
		return nil, fmt.Errorf("list complaint events failed: %w", err)
	}
	defer rows.Close()
	out := []*ComplaintEvent{}
	for rows.Next() {
		var e ComplaintEvent
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorName, &e.Type, &e.FromStatus, &e.ToStatus, &e.Note, &e.IsInternal, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan complaint event failed: %w", err)
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (r *pgxRepository) UpdateComplaint(ctx context.Context, id, adminID, status, note string) (*Complaint, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var current string
	err = tx.QueryRow(ctx, "SELECT status FROM public.complaints WHERE id = $1 FOR UPDATE", id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrComplaintNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock complaint failed: %w", err)
	}
	if current == status {
		return nil, ErrStatusUnchanged
	}

	closing := status == StatusResolved || status == StatusDismissed
	var noteArg any
	if closing {
		noteArg = note
	}
	if _, err := tx.Exec(ctx, `UPDATE public.complaints SET status = $2,
		resolution_note = CASE WHEN $3 THEN $4 ELSE NULL END,
		resolved_by = CASE WHEN $3 THEN $5::uuid ELSE NULL END,
		resolved_at = CASE WHEN $3 THEN now() ELSE NULL END,
		updated_at = now() WHERE id = $1`, id, status, closing, noteArg, nullable(adminID)); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrComplaintOpen
		}
		return nil, fmt.Errorf("update complaint failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.complaint_events (complaint_id, actor_id, event_type, from_status, to_status, note)
		VALUES ($1, $2, 'status_changed', $3, $4, $5)`, id, nullable(adminID), current, status, note); err != nil {
		return nil, fmt.Errorf("record complaint event failed: %w", err)
	}
	out, err := r.getComplaint(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *pgxRepository) AddNote(ctx context.Context, id, adminID, note string) (*ComplaintEvent, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.complaints WHERE id = $1)", id).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check complaint failed: %w", err)
	}
	if !exists {
		return nil, ErrComplaintNotFound
	}
	var e ComplaintEvent
	err := r.pool.QueryRow(ctx, `INSERT INTO public.complaint_events (complaint_id, actor_id, event_type, note, is_internal)
		VALUES ($1, $2, 'note', $3, true) RETURNING id, actor_id, event_type, from_status, to_status, note, is_internal, created_at`,
		id, nullable(adminID), note).Scan(&e.ID, &e.ActorID, &e.Type, &e.FromStatus, &e.ToStatus, &e.Note, &e.IsInternal, &e.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add complaint note failed: %w", err)
	}
	return &e, nil
}

func (r *pgxRepository) AdminIDs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, "SELECT id FROM public.users WHERE is_system_admin AND is_active")
	if err != nil {
		return nil, fmt.Errorf("list admins failed: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ---- Reputation ----

func (r *pgxRepository) CoachInfo(ctx context.Context, coachID string) (string, string, error) {
	var name, status string
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(u.display_name, u.username), c.status FROM public.coaches c
		JOIN public.users u ON u.id = c.user_id WHERE c.user_id = $1`, coachID).Scan(&name, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrCoachNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("get coach failed: %w", err)
	}
	return name, status, nil
}

func (r *pgxRepository) AddReputationEvent(ctx context.Context, e *ReputationEvent) (*ReputationEvent, error) {
	out := *e
	err := r.pool.QueryRow(ctx, `INSERT INTO public.coach_reputation_events (coach_id, delta, reason, complaint_id, created_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`, e.CoachID, e.Delta, e.Reason, e.ComplaintID, e.CreatedBy).Scan(&out.ID, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add reputation event failed: %w", err)
	}
	return &out, nil
}

func (r *pgxRepository) ListReputationEvents(ctx context.Context, coachID string, limit int) ([]*ReputationEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, coach_id, delta, reason, complaint_id, created_by, created_at
		FROM public.coach_reputation_events WHERE coach_id = $1 ORDER BY created_at DESC, id LIMIT $2`, coachID, limit)
	if err != nil {
		return nil, fmt.Errorf("list reputation events failed: %w", err)
	}
	defer rows.Close()
	out := []*ReputationEvent{}
	for rows.Next() {
		var e ReputationEvent
		if err := rows.Scan(&e.ID, &e.CoachID, &e.Delta, &e.Reason, &e.ComplaintID, &e.CreatedBy, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan reputation event failed: %w", err)
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (r *pgxRepository) SumReputation(ctx context.Context, coachID string) (int, error) {
	var sum int
	if err := r.pool.QueryRow(ctx, "SELECT COALESCE(sum(delta), 0)::int FROM public.coach_reputation_events WHERE coach_id = $1", coachID).Scan(&sum); err != nil {
		return 0, fmt.Errorf("sum reputation failed: %w", err)
	}
	return sum, nil
}

func (r *pgxRepository) ComplaintCounts(ctx context.Context, coachID string) (int, int, error) {
	var open, total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status IN ('open', 'in_review')), count(*)
		FROM public.complaints WHERE target_user_id = $1`, coachID).Scan(&open, &total); err != nil {
		return 0, 0, fmt.Errorf("count complaints failed: %w", err)
	}
	return open, total, nil
}

func (r *pgxRepository) ComplaintExistsFor(ctx context.Context, complaintID, coachID string) (bool, error) {
	var ok bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.complaints WHERE id = $1 AND target_user_id = $2)", complaintID, coachID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check complaint failed: %w", err)
	}
	return ok, nil
}

// ---- Suspensions ----

const suspensionColumns = `s.id, s.coach_id, COALESCE(u.display_name, u.username), s.reason, s.status, s.created_by, s.created_at,
	s.lifted_by, s.lifted_at, s.lift_note`

func scanSuspension(row pgx.Row) (*Suspension, error) {
	var s Suspension
	if err := row.Scan(&s.ID, &s.CoachID, &s.CoachName, &s.Reason, &s.Status, &s.CreatedBy, &s.CreatedAt, &s.LiftedBy, &s.LiftedAt, &s.LiftNote); err != nil {
		return nil, err
	}
	return &s, nil
}

func getSuspension(ctx context.Context, q querier, id string) (*Suspension, error) {
	s, err := scanSuspension(q.QueryRow(ctx, "SELECT "+suspensionColumns+` FROM public.coach_suspensions s
		JOIN public.users u ON u.id = s.coach_id WHERE s.id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSuspensionMissing
		}
		return nil, fmt.Errorf("get suspension failed: %w", err)
	}
	return s, nil
}

func (r *pgxRepository) Suspend(ctx context.Context, coachID, adminID, reason string) (*Suspension, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, "SELECT status FROM public.coaches WHERE user_id = $1 FOR UPDATE", coachID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCoachNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock coach failed: %w", err)
	}
	switch status {
	case "suspended":
		return nil, ErrAlreadySuspended
	case "approved":
	default:
		return nil, ErrCoachNotApproved
	}

	var id string
	if err := tx.QueryRow(ctx, "INSERT INTO public.coach_suspensions (coach_id, reason, created_by) VALUES ($1, $2, $3) RETURNING id",
		coachID, reason, nullable(adminID)).Scan(&id); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadySuspended
		}
		return nil, fmt.Errorf("create suspension failed: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE public.coaches SET status = 'suspended', updated_at = now() WHERE user_id = $1", coachID); err != nil {
		return nil, fmt.Errorf("suspend coach failed: %w", err)
	}
	s, err := getSuspension(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r *pgxRepository) Lift(ctx context.Context, id, adminID, note string) (*Suspension, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var coachID, status string
	err = tx.QueryRow(ctx, "SELECT coach_id, status FROM public.coach_suspensions WHERE id = $1 FOR UPDATE", id).Scan(&coachID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSuspensionMissing
	}
	if err != nil {
		return nil, fmt.Errorf("lock suspension failed: %w", err)
	}
	if status != "active" {
		return nil, ErrSuspensionLifted
	}
	var noteArg *string
	if note != "" {
		noteArg = &note
	}
	if _, err := tx.Exec(ctx, "UPDATE public.coach_suspensions SET status = 'lifted', lifted_by = $2, lifted_at = now(), lift_note = $3 WHERE id = $1",
		id, nullable(adminID), noteArg); err != nil {
		return nil, fmt.Errorf("lift suspension failed: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE public.coaches SET status = 'approved', updated_at = now() WHERE user_id = $1 AND status = 'suspended'", coachID); err != nil {
		return nil, fmt.Errorf("restore coach failed: %w", err)
	}
	s, err := getSuspension(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r *pgxRepository) ListSuspensions(ctx context.Context, coachID string) ([]*Suspension, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+suspensionColumns+` FROM public.coach_suspensions s
		JOIN public.users u ON u.id = s.coach_id WHERE s.coach_id = $1 ORDER BY s.created_at DESC, s.id`, coachID)
	if err != nil {
		return nil, fmt.Errorf("list suspensions failed: %w", err)
	}
	defer rows.Close()
	out := []*Suspension{}
	for rows.Next() {
		s, err := scanSuspension(rows)
		if err != nil {
			return nil, fmt.Errorf("scan suspension failed: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *pgxRepository) ActiveSuspension(ctx context.Context, coachID string) (*Suspension, error) {
	s, err := scanSuspension(r.pool.QueryRow(ctx, "SELECT "+suspensionColumns+` FROM public.coach_suspensions s
		JOIN public.users u ON u.id = s.coach_id WHERE s.coach_id = $1 AND s.status = 'active'`, coachID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active suspension failed: %w", err)
	}
	return s, nil
}

func (r *pgxRepository) Impact(ctx context.Context, coachID string) (*SuspensionImpact, error) {
	imp := &SuspensionImpact{Courses: []AffectedCourse{}, Sessions: []AffectedSession{}, Enrollments: []AffectedEnrollment{}}

	rows, err := r.pool.Query(ctx, `SELECT c.id, c.title, c.status,
		(SELECT count(*) FROM public.course_coaches o JOIN public.coaches k ON k.user_id = o.coach_id
			WHERE o.course_id = c.id AND o.coach_id <> $1 AND k.status = 'approved'),
		(SELECT count(*) FROM public.course_sessions s WHERE s.course_id = c.id AND s.status = 'scheduled' AND s.start_at > now()),
		(SELECT count(*) FROM public.enrollments e WHERE e.course_id = c.id AND e.status = 'active')
		FROM public.courses c JOIN public.course_coaches cc ON cc.course_id = c.id AND cc.coach_id = $1
		WHERE c.status <> 'cancelled' ORDER BY c.created_at, c.id`, coachID)
	if err != nil {
		return nil, fmt.Errorf("list affected courses failed: %w", err)
	}
	var ids []string
	for rows.Next() {
		var a AffectedCourse
		if err := rows.Scan(&a.ID, &a.Title, &a.Status, &a.OtherActiveTeachers, &a.UpcomingSessions, &a.ActiveEnrollments); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan affected course failed: %w", err)
		}
		imp.Courses = append(imp.Courses, a)
		ids = append(ids, a.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return imp, nil
	}

	rows, err = r.pool.Query(ctx, `SELECT s.id, c.id, c.title, s.start_at, s.end_at FROM public.course_sessions s
		JOIN public.courses c ON c.id = s.course_id
		WHERE s.course_id = ANY($1::uuid[]) AND s.status = 'scheduled' AND s.start_at > now() ORDER BY s.start_at, s.id`, ids)
	if err != nil {
		return nil, fmt.Errorf("list affected sessions failed: %w", err)
	}
	for rows.Next() {
		var a AffectedSession
		if err := rows.Scan(&a.ID, &a.CourseID, &a.CourseTitle, &a.StartAt, &a.EndAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan affected session failed: %w", err)
		}
		imp.Sessions = append(imp.Sessions, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = r.pool.Query(ctx, `SELECT e.id, c.id, c.title, u.id, COALESCE(u.display_name, u.username), e.payment_status, e.amount_due
		FROM public.enrollments e JOIN public.courses c ON c.id = e.course_id JOIN public.users u ON u.id = e.user_id
		WHERE e.course_id = ANY($1::uuid[]) AND e.status = 'active' ORDER BY c.title, e.created_at, e.id`, ids)
	if err != nil {
		return nil, fmt.Errorf("list affected enrollments failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var a AffectedEnrollment
		if err := rows.Scan(&a.ID, &a.CourseID, &a.CourseTitle, &a.StudentID, &a.StudentName, &a.PaymentStatus, &a.AmountDue); err != nil {
			return nil, fmt.Errorf("scan affected enrollment failed: %w", err)
		}
		imp.Enrollments = append(imp.Enrollments, a)
	}
	return imp, rows.Err()
}

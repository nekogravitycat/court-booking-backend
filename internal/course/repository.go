package course

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

// CancelledEnrollment identifies a seat released by cancelling a whole course.
type CancelledEnrollment struct {
	ID            string
	UserID        string
	PaymentStatus string
}

// Seat is one occupied seat reduced to the attributes aggregated into the
// anonymous roster summary.
type Seat struct {
	Gender     *string
	BirthDate  *time.Time
	SkillLevel int // 0 when the student has not declared a level for the sport
}

// Repository persists courses, sessions and enrollments.
type Repository interface {
	// Create inserts the course, its teaching coaches and its sessions atomically.
	Create(ctx context.Context, c *Course, teacherIDs []string, ranges []TimeRange) (string, error)
	Get(ctx context.Context, id string) (*Course, error)
	List(ctx context.Context, f Filter) ([]*Course, int, error)
	ListSessions(ctx context.Context, courseID string) ([]*Session, error)
	// Update writes every mutable course field. teacherIDs, when non-nil,
	// replaces the teaching coaches. It rejects a capacity below the number of
	// active enrollments.
	Update(ctx context.Context, c *Course, teacherIDs *[]string) error
	// Cancel cancels the course, its future sessions and all active enrollments.
	// Paid or reported enrollments get a pending cancellation request so the
	// coach has a refund to-do list; pending transfer reports are rejected.
	Cancel(ctx context.Context, id, actorID string) ([]CancelledEnrollment, error)

	AddSession(ctx context.Context, courseID string, r TimeRange, venueNote string) (*Session, error)
	// UpdateSession reschedules and/or cancels a future session.
	UpdateSession(ctx context.Context, courseID, sessionID string, r *TimeRange, venueNote *string, cancelReason *string) (*Session, error)

	Enroll(ctx context.Context, courseID, userID string, method *string) (*Enrollment, error)
	GetEnrollment(ctx context.Context, id string) (*Enrollment, error)
	ListEnrollments(ctx context.Context, f EnrollmentFilter) ([]*Enrollment, int, error)
	// CancelUserEnrollments cancels the user's active enrollments (account deactivation).
	CancelUserEnrollments(ctx context.Context, userID string) error

	// IsManager reports whether the user may manage the course (owner coach,
	// teaching coach, creator, or a leader of the owning group).
	IsManager(ctx context.Context, courseID, userID string) (bool, error)
	// ManagerIDs returns the users to notify about course activity.
	ManagerIDs(ctx context.Context, courseID string) ([]string, error)
	// MakeupGuestIDs returns the users booked into the session as make-up guests.
	MakeupGuestIDs(ctx context.Context, sessionID string) ([]string, error)
	// StudentIDs returns the users with an active enrollment.
	StudentIDs(ctx context.Context, courseID string) ([]string, error)
	// ActiveGroupMembers counts how many of the users are active members of the group.
	ActiveGroupMembers(ctx context.Context, groupID string, userIDs []string) (int, error)
	Seats(ctx context.Context, courseID string) ([]Seat, string, error)
	LocationExists(ctx context.Context, id string) (bool, error)
	SportActive(ctx context.Context, id string) (bool, error)
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

func isFKViolation(err error) bool {
	var e *pgconn.PgError
	return errors.As(err, &e) && e.Code == pgerrcode.ForeignKeyViolation
}

// SQL fragments shared by queries.
const (
	// ownerAvailableSQL is true when at least one responsible coach can still teach.
	ownerAvailableSQL = `((c.coach_id IS NOT NULL AND EXISTS (SELECT 1 FROM public.coaches k JOIN public.users ku ON ku.id = k.user_id
			WHERE k.user_id = c.coach_id AND k.status = 'approved' AND ku.is_active))
		OR (c.group_id IS NOT NULL AND EXISTS (SELECT 1 FROM public.course_coaches cc JOIN public.coaches k ON k.user_id = cc.coach_id
			JOIN public.users ku ON ku.id = k.user_id WHERE cc.course_id = c.id AND k.status = 'approved' AND ku.is_active)))`

	// managerSQL is a format string: %[1]s is the placeholder holding the user id.
	managerSQL = `(c.coach_id = %[1]s OR c.created_by = %[1]s
		OR EXISTS (SELECT 1 FROM public.course_coaches cc WHERE cc.course_id = c.id AND cc.coach_id = %[1]s)
		OR (c.group_id IS NOT NULL AND EXISTS (SELECT 1 FROM public.coach_group_members m
			WHERE m.group_id = c.group_id AND m.user_id = %[1]s AND m.role = 'leader')))`
)

const courseColumns = `c.id, c.coach_id, c.group_id, c.created_by, COALESCE(g.name, COALESCE(u.display_name, u.username)),
	c.title, c.description, c.teaching_content, c.sport_id, s.name, c.course_type, c.capacity, c.venue_name, c.location_id,
	c.timezone, c.price, c.venue_fee, c.payment_methods, c.bank_code, c.bank_account, c.bank_account_name,
	c.allow_leave, c.leave_deadline_hours, c.allow_makeup, c.makeup_limit, c.makeup_valid_days, c.allow_cancel, c.cancel_deadline_hours,
	c.rules_leave, c.rules_makeup, c.rules_cancellation, c.rules_refund, c.rules_noshow,
	c.enrollment_deadline, c.status, c.created_at, c.updated_at,
	en.n, ss.n, ss.first_at, ss.last_at, ss.next_at`

const sessionAggregate = `LATERAL (SELECT count(*) AS n, min(start_at) AS first_at, max(start_at) AS last_at,
	min(start_at) FILTER (WHERE start_at > now()) AS next_at
	FROM public.course_sessions cs WHERE cs.course_id = c.id AND cs.status = 'scheduled') ss ON true`

const enrolledAggregate = `LATERAL (SELECT count(*) AS n FROM public.enrollments e WHERE e.course_id = c.id AND e.status = 'active') en ON true`

func scanCourse(row pgx.Row, extra ...any) (*Course, error) {
	var c Course
	dest := append([]any{
		&c.ID, &c.CoachID, &c.GroupID, &c.CreatedBy, &c.OwnerName,
		&c.Title, &c.Description, &c.TeachingContent, &c.SportID, &c.SportName, &c.Type, &c.Capacity, &c.VenueName, &c.LocationID,
		&c.Timezone, &c.Price, &c.VenueFee, &c.PaymentMethods, &c.BankCode, &c.BankAccount, &c.BankAccountName,
		&c.AllowLeave, &c.LeaveDeadlineHrs, &c.AllowMakeup, &c.MakeupLimit, &c.MakeupValidDays, &c.AllowCancel, &c.CancelDeadline,
		&c.RulesLeave, &c.RulesMakeup, &c.RulesCancel, &c.RulesRefund, &c.RulesNoShow,
		&c.EnrollDeadline, &c.Status, &c.CreatedAt, &c.UpdatedAt,
		&c.EnrolledCount, &c.SessionCount, &c.FirstSessionAt, &c.LastSessionAt, &c.NextSessionAt,
	}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	if c.PaymentMethods == nil {
		c.PaymentMethods = []string{}
	}
	return &c, nil
}

func (r *pgxRepository) loadTeachers(ctx context.Context, courses []*Course) error {
	if len(courses) == 0 {
		return nil
	}
	ids := make([]string, len(courses))
	byID := make(map[string]*Course, len(courses))
	for i, c := range courses {
		ids[i] = c.ID
		byID[c.ID] = c
		c.Teachers = []Teacher{}
	}
	rows, err := r.pool.Query(ctx, `SELECT cc.course_id, u.id, COALESCE(u.display_name, u.username), u.avatar
		FROM public.course_coaches cc JOIN public.users u ON u.id = cc.coach_id
		WHERE cc.course_id = ANY($1::uuid[]) ORDER BY u.display_name, u.id`, ids)
	if err != nil {
		return fmt.Errorf("load course teachers failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid string
		var t Teacher
		if err := rows.Scan(&cid, &t.UserID, &t.DisplayName, &t.Avatar); err != nil {
			return fmt.Errorf("scan teacher failed: %w", err)
		}
		byID[cid].Teachers = append(byID[cid].Teachers, t)
	}
	return rows.Err()
}

func (r *pgxRepository) Get(ctx context.Context, id string) (*Course, error) {
	c, err := scanCourse(r.pool.QueryRow(ctx, `SELECT `+courseColumns+` FROM public.courses c
		JOIN public.sports s ON s.id = c.sport_id
		LEFT JOIN public.users u ON u.id = c.coach_id
		LEFT JOIN public.coach_groups g ON g.id = c.group_id
		LEFT JOIN `+enrolledAggregate+`
		LEFT JOIN `+sessionAggregate+` WHERE c.id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get course failed: %w", err)
	}
	if err := r.loadTeachers(ctx, []*Course{c}); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *pgxRepository) Create(ctx context.Context, c *Course, teacherIDs []string, ranges []TimeRange) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `INSERT INTO public.courses (coach_id, group_id, created_by, title, description, teaching_content,
		sport_id, course_type, capacity, venue_name, location_id, timezone, price, venue_fee, payment_methods,
		bank_code, bank_account, bank_account_name, allow_leave, leave_deadline_hours, allow_makeup, makeup_limit,
		makeup_valid_days, allow_cancel, cancel_deadline_hours, rules_leave, rules_makeup, rules_cancellation,
		rules_refund, rules_noshow, enrollment_deadline)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31)
		RETURNING id`,
		c.CoachID, c.GroupID, c.CreatedBy, c.Title, c.Description, c.TeachingContent,
		c.SportID, c.Type, c.Capacity, c.VenueName, c.LocationID, c.Timezone, c.Price, c.VenueFee, c.PaymentMethods,
		c.BankCode, c.BankAccount, c.BankAccountName, c.AllowLeave, c.LeaveDeadlineHrs, c.AllowMakeup, c.MakeupLimit,
		c.MakeupValidDays, c.AllowCancel, c.CancelDeadline, c.RulesLeave, c.RulesMakeup, c.RulesCancel,
		c.RulesRefund, c.RulesNoShow, c.EnrollDeadline).Scan(&id)
	if err != nil {
		if isFKViolation(err) {
			return "", ErrInvalidCourse
		}
		return "", fmt.Errorf("create course failed: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO public.course_coaches (course_id, coach_id) SELECT $1, unnest($2::uuid[])", id, teacherIDs); err != nil {
		if isFKViolation(err) {
			return "", ErrInvalidTeacher
		}
		return "", fmt.Errorf("add course coaches failed: %w", err)
	}
	for i, rg := range ranges {
		if _, err := tx.Exec(ctx, "INSERT INTO public.course_sessions (course_id, seq, start_at, end_at) VALUES ($1, $2, $3, $4)",
			id, i+1, rg.Start, rg.End); err != nil {
			return "", fmt.Errorf("add course session failed: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (r *pgxRepository) ListSessions(ctx context.Context, courseID string) ([]*Session, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, course_id, seq, start_at, end_at, status, venue_note, cancel_reason
		FROM public.course_sessions WHERE course_id = $1 ORDER BY start_at, seq`, courseID)
	if err != nil {
		return nil, fmt.Errorf("list sessions failed: %w", err)
	}
	defer rows.Close()
	out := []*Session{}
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.CourseID, &s.Seq, &s.StartAt, &s.EndAt, &s.Status, &s.VenueNote, &s.CancelReason); err != nil {
			return nil, fmt.Errorf("scan session failed: %w", err)
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

func (r *pgxRepository) List(ctx context.Context, f Filter) ([]*Course, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	q := psql.Select(courseColumns, "count(*) OVER() AS total_count").
		From("public.courses c").
		Join("public.sports s ON s.id = c.sport_id").
		LeftJoin("public.users u ON u.id = c.coach_id").
		LeftJoin("public.coach_groups g ON g.id = c.group_id").
		LeftJoin(enrolledAggregate).
		LeftJoin(sessionAggregate)

	personal := f.ManagedBy != "" || f.EnrolledBy != ""
	if f.Status != "" {
		q = q.Where(squirrel.Eq{"c.status": f.Status})
	} else if !personal {
		q = q.Where(squirrel.Eq{"c.status": StatusOpen})
	}
	if !personal {
		q = q.Where(ownerAvailableSQL)
		if !f.IncludeEnded {
			q = q.Where("ss.next_at IS NOT NULL")
		}
	}
	if f.ManagedBy != "" {
		q = q.Where(fmt.Sprintf(managerSQL, "?"), f.ManagedBy, f.ManagedBy, f.ManagedBy, f.ManagedBy)
	}
	if f.EnrolledBy != "" {
		q = q.Where("EXISTS (SELECT 1 FROM public.enrollments e WHERE e.course_id = c.id AND e.user_id = ? AND e.status = 'active')", f.EnrolledBy)
	}
	if f.SportID != "" {
		q = q.Where(squirrel.Eq{"c.sport_id": f.SportID})
	}
	if f.CourseType != "" {
		q = q.Where(squirrel.Eq{"c.course_type": f.CourseType})
	}
	if f.GroupID != "" {
		q = q.Where(squirrel.Eq{"c.group_id": f.GroupID})
	}
	if f.LocationID != "" {
		q = q.Where(squirrel.Eq{"c.location_id": f.LocationID})
	}
	if f.CoachID != "" {
		q = q.Where("(c.coach_id = ? OR EXISTS (SELECT 1 FROM public.course_coaches cc WHERE cc.course_id = c.id AND cc.coach_id = ?))", f.CoachID, f.CoachID)
	}
	if f.Keyword != "" {
		like := "%" + request.EscapeLike(f.Keyword) + "%"
		q = q.Where("(c.title ILIKE ? OR COALESCE(g.name, COALESCE(u.display_name, u.username)) ILIKE ?)", like, like)
	}
	if f.FreeOnly {
		q = q.Where("c.price + c.venue_fee = 0")
	}
	if f.HasSeats {
		q = q.Where("en.n < c.capacity")
	}
	if f.StartFrom != nil {
		q = q.Where("EXISTS (SELECT 1 FROM public.course_sessions x WHERE x.course_id = c.id AND x.status = 'scheduled' AND x.start_at >= ?)", *f.StartFrom)
	}
	if f.StartTo != nil {
		q = q.Where("EXISTS (SELECT 1 FROM public.course_sessions x WHERE x.course_id = c.id AND x.status = 'scheduled' AND x.start_at < ?)", *f.StartTo)
	}

	dir := "ASC"
	if f.SortOrder == "desc" || f.SortOrder == "DESC" {
		dir = "DESC"
	}
	switch f.SortBy {
	case "created_at":
		q = q.OrderBy("c.created_at "+dir, "c.id")
	case "price":
		q = q.OrderBy("(c.price + c.venue_fee) "+dir, "c.id")
	default:
		q = q.OrderBy("ss.next_at "+dir+" NULLS LAST", "c.id")
	}

	items, total, err := pagination.Collect(ctx, r.pool, q, f.Page, f.PageSize, "course", func(rows pgx.Rows, total *int) (*Course, error) {
		c, err := scanCourse(rows, total)
		if err != nil {
			return nil, fmt.Errorf("scan course failed: %w", err)
		}
		return c, nil
	})
	if err != nil {
		return nil, 0, err
	}
	if err := r.loadTeachers(ctx, items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, c *Course, teacherIDs *[]string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var enrolled int
	var status string
	err = tx.QueryRow(ctx, `SELECT status, (SELECT count(*) FROM public.enrollments e WHERE e.course_id = c.id AND e.status = 'active')
		FROM public.courses c WHERE c.id = $1 FOR UPDATE`, c.ID).Scan(&status, &enrolled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock course failed: %w", err)
	}
	if status == StatusCancelled {
		return ErrCourseCancelled
	}
	if c.Capacity < enrolled {
		return ErrCapacityBelowEnroll
	}

	if _, err := tx.Exec(ctx, `UPDATE public.courses SET title = $2, description = $3, teaching_content = $4, capacity = $5,
		venue_name = $6, location_id = $7, price = $8, venue_fee = $9, payment_methods = $10, bank_code = $11, bank_account = $12,
		bank_account_name = $13, allow_leave = $14, leave_deadline_hours = $15, allow_makeup = $16, makeup_limit = $17,
		makeup_valid_days = $18, allow_cancel = $19, cancel_deadline_hours = $20, rules_leave = $21, rules_makeup = $22,
		rules_cancellation = $23, rules_refund = $24, rules_noshow = $25, enrollment_deadline = $26, status = $27, updated_at = now()
		WHERE id = $1`,
		c.ID, c.Title, c.Description, c.TeachingContent, c.Capacity,
		c.VenueName, c.LocationID, c.Price, c.VenueFee, c.PaymentMethods, c.BankCode, c.BankAccount,
		c.BankAccountName, c.AllowLeave, c.LeaveDeadlineHrs, c.AllowMakeup, c.MakeupLimit,
		c.MakeupValidDays, c.AllowCancel, c.CancelDeadline, c.RulesLeave, c.RulesMakeup,
		c.RulesCancel, c.RulesRefund, c.RulesNoShow, c.EnrollDeadline, c.Status); err != nil {
		return fmt.Errorf("update course failed: %w", err)
	}
	if teacherIDs != nil {
		if _, err := tx.Exec(ctx, "DELETE FROM public.course_coaches WHERE course_id = $1", c.ID); err != nil {
			return fmt.Errorf("clear course coaches failed: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO public.course_coaches (course_id, coach_id) SELECT $1, unnest($2::uuid[])", c.ID, *teacherIDs); err != nil {
			if isFKViolation(err) {
				return ErrInvalidTeacher
			}
			return fmt.Errorf("add course coaches failed: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *pgxRepository) Cancel(ctx context.Context, id, actorID string) ([]CancelledEnrollment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, "SELECT status FROM public.courses WHERE id = $1 FOR UPDATE", id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock course failed: %w", err)
	}
	if status == StatusCancelled {
		return nil, ErrCourseCancelled
	}
	if _, err := tx.Exec(ctx, "UPDATE public.courses SET status = 'cancelled', updated_at = now() WHERE id = $1", id); err != nil {
		return nil, fmt.Errorf("cancel course failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE public.course_sessions SET status = 'cancelled', cancel_reason = 'course cancelled', updated_at = now()
		WHERE course_id = $1 AND status = 'scheduled' AND start_at > now()`, id); err != nil {
		return nil, fmt.Errorf("cancel course sessions failed: %w", err)
	}
	if err := releaseMakeups(ctx, tx, "SELECT id FROM public.course_sessions WHERE course_id = $1 AND start_at > now()", id); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `UPDATE public.enrollments SET status = 'cancelled', cancel_source = $2, cancelled_at = now(), updated_at = now()
		WHERE course_id = $1 AND status = 'active' RETURNING id, user_id, payment_status`, id, CancelByCourse)
	if err != nil {
		return nil, fmt.Errorf("cancel enrollments failed: %w", err)
	}
	var out []CancelledEnrollment
	for rows.Next() {
		var ce CancelledEnrollment
		if err := rows.Scan(&ce.ID, &ce.UserID, &ce.PaymentStatus); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, ce)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE public.transfer_reports SET status = 'rejected', review_note = 'course cancelled', reviewed_at = now()
		WHERE status = 'pending' AND enrollment_id IN (SELECT id FROM public.enrollments WHERE course_id = $1)`, id); err != nil {
		return nil, fmt.Errorf("reject transfer reports failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.cancellation_requests (enrollment_id, requested_by, requester_role, reason)
		SELECT id, $2, 'coach', 'course cancelled' FROM public.enrollments
		WHERE course_id = $1 AND status = 'cancelled' AND cancel_source = $3 AND payment_status IN ('paid', 'reported')`,
		id, nullableID(actorID), CancelByCourse); err != nil {
		return nil, fmt.Errorf("record refund to-do failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// nullableID turns an empty id into SQL NULL.
func nullableID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// releaseMakeups frees make-up guests booked into the given sessions: their
// bookings are removed and the leaves become open again, so they can pick
// another session.
func releaseMakeups(ctx context.Context, q querier, sessionFilter string, arg any) error {
	if _, err := q.Exec(ctx, `UPDATE public.leave_requests SET status = 'open', makeup_session_id = NULL, updated_at = now()
		WHERE makeup_session_id IN (`+sessionFilter+`)`, arg); err != nil {
		return fmt.Errorf("reopen make-up leaves failed: %w", err)
	}
	if _, err := q.Exec(ctx, `DELETE FROM public.session_attendances WHERE kind = 'makeup' AND session_id IN (`+sessionFilter+`)`, arg); err != nil {
		return fmt.Errorf("remove make-up bookings failed: %w", err)
	}
	return nil
}

func hasOverlap(ctx context.Context, q querier, courseID, excludeSessionID string, rg TimeRange) (bool, error) {
	var overlap bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.course_sessions
		WHERE course_id = $1 AND status = 'scheduled' AND ($2 = '' OR id::text <> $2)
		AND tstzrange(start_at, end_at) && tstzrange($3::timestamptz, $4::timestamptz))`, courseID, excludeSessionID, rg.Start, rg.End).Scan(&overlap)
	if err != nil {
		return false, fmt.Errorf("check session overlap failed: %w", err)
	}
	return overlap, nil
}

func (r *pgxRepository) AddSession(ctx context.Context, courseID string, rg TimeRange, venueNote string) (*Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status, ctype string
	err = tx.QueryRow(ctx, "SELECT status, course_type FROM public.courses WHERE id = $1 FOR UPDATE", courseID).Scan(&status, &ctype)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock course failed: %w", err)
	}
	if status == StatusCancelled {
		return nil, ErrCourseCancelled
	}
	if ctype != TypeRecurring {
		return nil, ErrSingleSession
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM public.course_sessions WHERE course_id = $1", courseID).Scan(&count); err != nil {
		return nil, fmt.Errorf("count sessions failed: %w", err)
	}
	if count >= MaxSessions {
		return nil, ErrTooManySessions
	}
	if overlap, err := hasOverlap(ctx, tx, courseID, "", rg); err != nil {
		return nil, err
	} else if overlap {
		return nil, ErrSessionConflict
	}
	var s Session
	err = tx.QueryRow(ctx, `INSERT INTO public.course_sessions (course_id, seq, start_at, end_at, venue_note)
		VALUES ($1, (SELECT COALESCE(max(seq), 0) + 1 FROM public.course_sessions WHERE course_id = $1), $2, $3, $4)
		RETURNING id, course_id, seq, start_at, end_at, status, venue_note, cancel_reason`,
		courseID, rg.Start, rg.End, venueNote).
		Scan(&s.ID, &s.CourseID, &s.Seq, &s.StartAt, &s.EndAt, &s.Status, &s.VenueNote, &s.CancelReason)
	if err != nil {
		return nil, fmt.Errorf("insert session failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *pgxRepository) UpdateSession(ctx context.Context, courseID, sessionID string, rg *TimeRange, venueNote *string, cancelReason *string) (*Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock the course so concurrent session edits cannot create an overlap.
	var courseStatus string
	err = tx.QueryRow(ctx, "SELECT status FROM public.courses WHERE id = $1 FOR UPDATE", courseID).Scan(&courseStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock course failed: %w", err)
	}
	if courseStatus == StatusCancelled {
		return nil, ErrCourseCancelled
	}

	var s Session
	err = tx.QueryRow(ctx, `SELECT id, course_id, seq, start_at, end_at, status, venue_note, cancel_reason
		FROM public.course_sessions WHERE id = $1 AND course_id = $2 FOR UPDATE`, sessionID, courseID).
		Scan(&s.ID, &s.CourseID, &s.Seq, &s.StartAt, &s.EndAt, &s.Status, &s.VenueNote, &s.CancelReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock session failed: %w", err)
	}
	if s.Status == SessionCancelled {
		return nil, ErrSessionCancelled
	}
	if !s.StartAt.After(time.Now()) {
		return nil, ErrSessionPast
	}
	if rg != nil {
		if overlap, err := hasOverlap(ctx, tx, courseID, sessionID, *rg); err != nil {
			return nil, err
		} else if overlap {
			return nil, ErrSessionConflict
		}
		s.StartAt, s.EndAt = rg.Start, rg.End
	}
	if venueNote != nil {
		s.VenueNote = *venueNote
	}
	if cancelReason != nil {
		s.Status = SessionCancelled
		s.CancelReason = cancelReason
	}
	if cancelReason != nil {
		if err := releaseMakeups(ctx, tx, "SELECT $1::uuid", sessionID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE public.course_sessions SET start_at = $2, end_at = $3, venue_note = $4, status = $5,
		cancel_reason = $6, updated_at = now() WHERE id = $1`,
		s.ID, s.StartAt, s.EndAt, s.VenueNote, s.Status, s.CancelReason); err != nil {
		return nil, fmt.Errorf("update session failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &s, nil
}

const enrollmentColumns = `e.id, e.course_id, e.user_id, e.status, e.payment_status, e.payment_method, e.price, e.venue_fee,
	e.amount_due, e.cancel_source, e.cancelled_at, e.created_at, e.updated_at`

func scanEnrollment(row pgx.Row, extra ...any) (*Enrollment, error) {
	var e Enrollment
	dest := append([]any{&e.ID, &e.CourseID, &e.UserID, &e.Status, &e.PaymentStatus, &e.PaymentMethod, &e.Price, &e.VenueFee,
		&e.AmountDue, &e.CancelSource, &e.CancelledAt, &e.CreatedAt, &e.UpdatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *pgxRepository) Enroll(ctx context.Context, courseID, userID string, method *string) (*Enrollment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Locking the course row serializes enrollments, so the capacity check
	// cannot be raced into overbooking.
	var status string
	var capacity, price, venueFee int
	var deadline *time.Time
	var methods []string
	var available bool
	err = tx.QueryRow(ctx, `SELECT c.status, c.capacity, c.price, c.venue_fee, c.payment_methods, c.enrollment_deadline,
		`+ownerAvailableSQL+` FROM public.courses c WHERE c.id = $1 FOR UPDATE`, courseID).
		Scan(&status, &capacity, &price, &venueFee, &methods, &deadline, &available)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock course failed: %w", err)
	}
	if status != StatusOpen {
		return nil, ErrEnrollClosed
	}
	if !available {
		return nil, ErrCoachUnavailable
	}
	if deadline != nil && time.Now().After(*deadline) {
		return nil, ErrEnrollDeadline
	}

	var upcoming bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.course_sessions
		WHERE course_id = $1 AND status = 'scheduled' AND start_at > now())`, courseID).Scan(&upcoming); err != nil {
		return nil, fmt.Errorf("check upcoming sessions failed: %w", err)
	}
	if !upcoming {
		return nil, ErrNoUpcomingSessions
	}

	var teaches bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.course_coaches WHERE course_id = $1 AND coach_id = $2)
		OR EXISTS (SELECT 1 FROM public.courses WHERE id = $1 AND (coach_id = $2 OR created_by = $2))`, courseID, userID).Scan(&teaches); err != nil {
		return nil, fmt.Errorf("check teacher failed: %w", err)
	}
	if teaches {
		return nil, ErrOwnCourse
	}

	var taken int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM public.enrollments WHERE course_id = $1 AND status = 'active'", courseID).Scan(&taken); err != nil {
		return nil, fmt.Errorf("count enrollments failed: %w", err)
	}
	if taken >= capacity {
		return nil, ErrCourseFull
	}

	amount := price + venueFee
	payStatus := PayNotRequired
	var chosen *string
	if amount > 0 {
		payStatus = PayUnpaid
		if method == nil || !slices.Contains(methods, *method) {
			return nil, ErrPaymentMethod
		}
		chosen = method
	}

	var id string
	err = tx.QueryRow(ctx, `INSERT INTO public.enrollments (course_id, user_id, payment_status, payment_method, price, venue_fee, amount_due)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		courseID, userID, payStatus, chosen, price, venueFee, amount).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyEnrolled
		}
		return nil, fmt.Errorf("create enrollment failed: %w", err)
	}
	e, err := scanEnrollment(tx.QueryRow(ctx, "SELECT "+enrollmentColumns+" FROM public.enrollments e WHERE e.id = $1", id))
	if err != nil {
		return nil, fmt.Errorf("reload enrollment failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

const enrollmentJoinCols = enrollmentColumns + `,
	c.id, c.title, c.course_type, c.status, sp.name, COALESCE(g.name, COALESCE(cu.display_name, cu.username)), ns.next_at,
	u.id, COALESCE(u.display_name, u.username), u.avatar, u.phone, u.line_id, usl.skill_level, sl.label,
	att.present, att.absent, att.on_leave`

const nextSessionAggregate = `LATERAL (SELECT min(start_at) AS next_at FROM public.course_sessions cs
	WHERE cs.course_id = c.id AND cs.status = 'scheduled' AND cs.start_at > now()) ns ON true`

const attendanceAggregate = `LATERAL (SELECT count(*) FILTER (WHERE sa.status = 'present') AS present,
	count(*) FILTER (WHERE sa.status = 'absent') AS absent, count(*) FILTER (WHERE sa.status = 'leave') AS on_leave
	FROM public.session_attendances sa WHERE sa.enrollment_id = e.id) att ON true`

func scanEnrollmentJoined(row pgx.Row, extra ...any) (*Enrollment, error) {
	var cb CourseBrief
	var st Student
	var sum AttendanceSummary
	e, err := scanEnrollment(row, append([]any{
		&cb.ID, &cb.Title, &cb.Type, &cb.Status, &cb.SportName, &cb.OwnerName, &cb.NextSessionAt,
		&st.UserID, &st.DisplayName, &st.Avatar, &st.Phone, &st.LineID, &st.SkillLevel, &st.SkillLabel,
		&sum.Present, &sum.Absent, &sum.Leave,
	}, extra...)...)
	if err != nil {
		return nil, err
	}
	e.Course, e.Student, e.Attendance = &cb, &st, sum
	return e, nil
}

func enrollmentSelect(psql squirrel.StatementBuilderType, withTotal bool) squirrel.SelectBuilder {
	cols := enrollmentJoinCols
	if withTotal {
		cols += ", count(*) OVER() AS total_count"
	}
	return psql.Select(cols).
		From("public.enrollments e").
		Join("public.courses c ON c.id = e.course_id").
		Join("public.sports sp ON sp.id = c.sport_id").
		LeftJoin("public.users cu ON cu.id = c.coach_id").
		LeftJoin("public.coach_groups g ON g.id = c.group_id").
		Join("public.users u ON u.id = e.user_id").
		LeftJoin("public.user_skill_levels usl ON usl.user_id = e.user_id AND usl.sport_id = c.sport_id").
		LeftJoin("public.skill_levels sl ON sl.sport_id = usl.sport_id AND sl.level = usl.skill_level").
		LeftJoin(nextSessionAggregate).
		LeftJoin(attendanceAggregate)
}

func (r *pgxRepository) GetEnrollment(ctx context.Context, id string) (*Enrollment, error) {
	sql, args, err := enrollmentSelect(squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar), false).Where(squirrel.Eq{"e.id": id}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build enrollment query failed: %w", err)
	}
	e, err := scanEnrollmentJoined(r.pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEnrollmentNotFound
		}
		return nil, fmt.Errorf("get enrollment failed: %w", err)
	}
	return e, nil
}

func (r *pgxRepository) ListEnrollments(ctx context.Context, f EnrollmentFilter) ([]*Enrollment, int, error) {
	q := enrollmentSelect(squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar), true).OrderBy("e.created_at DESC", "e.id")
	if f.CourseID != "" {
		q = q.Where(squirrel.Eq{"e.course_id": f.CourseID})
	}
	if f.UserID != "" {
		q = q.Where(squirrel.Eq{"e.user_id": f.UserID})
	}
	if f.Status != "" {
		q = q.Where(squirrel.Eq{"e.status": f.Status})
	}
	if f.PaymentStatus != "" {
		q = q.Where(squirrel.Eq{"e.payment_status": f.PaymentStatus})
	}
	return pagination.Collect(ctx, r.pool, q, f.Page, f.PageSize, "enrollment", func(rows pgx.Rows, total *int) (*Enrollment, error) {
		e, err := scanEnrollmentJoined(rows, total)
		if err != nil {
			return nil, fmt.Errorf("scan enrollment failed: %w", err)
		}
		return e, nil
	})
}

func (r *pgxRepository) CancelUserEnrollments(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE public.transfer_reports SET status = 'rejected', review_note = 'account deactivated', reviewed_at = now()
		WHERE status = 'pending' AND enrollment_id IN (SELECT id FROM public.enrollments WHERE user_id = $1 AND status = 'active')`, userID); err != nil {
		return fmt.Errorf("reject transfer reports failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE public.enrollments SET status = 'cancelled', cancel_source = $2, cancelled_at = now(), updated_at = now()
		WHERE user_id = $1 AND status = 'active'`, userID, CancelBySystem); err != nil {
		return fmt.Errorf("cancel user enrollments failed: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *pgxRepository) IsManager(ctx context.Context, courseID, userID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, "SELECT "+fmt.Sprintf(managerSQL, "$2")+" FROM public.courses c WHERE c.id = $1", courseID, userID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("check course manager failed: %w", err)
	}
	return ok, nil
}

func (r *pgxRepository) collectIDs(ctx context.Context, sql string, args ...any) ([]string, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query ids failed: %w", err)
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

func (r *pgxRepository) ManagerIDs(ctx context.Context, courseID string) ([]string, error) {
	return r.collectIDs(ctx, `SELECT DISTINCT id FROM (
		SELECT coach_id AS id FROM public.course_coaches WHERE course_id = $1
		UNION SELECT created_by FROM public.courses WHERE id = $1 AND created_by IS NOT NULL
		UNION SELECT coach_id FROM public.courses WHERE id = $1 AND coach_id IS NOT NULL) t`, courseID)
}

func (r *pgxRepository) MakeupGuestIDs(ctx context.Context, sessionID string) ([]string, error) {
	return r.collectIDs(ctx, `SELECT e.user_id FROM public.session_attendances sa JOIN public.enrollments e ON e.id = sa.enrollment_id
		WHERE sa.session_id = $1 AND sa.kind = 'makeup'`, sessionID)
}

func (r *pgxRepository) StudentIDs(ctx context.Context, courseID string) ([]string, error) {
	return r.collectIDs(ctx, "SELECT user_id FROM public.enrollments WHERE course_id = $1 AND status = 'active'", courseID)
}

func (r *pgxRepository) ActiveGroupMembers(ctx context.Context, groupID string, userIDs []string) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM public.coach_group_members
		WHERE group_id = $1 AND user_id = ANY($2::uuid[]) AND status = 'active'`, groupID, userIDs).Scan(&n); err != nil {
		return 0, fmt.Errorf("count group members failed: %w", err)
	}
	return n, nil
}

func (r *pgxRepository) Seats(ctx context.Context, courseID string) ([]Seat, string, error) {
	var tz string
	if err := r.pool.QueryRow(ctx, "SELECT timezone FROM public.courses WHERE id = $1", courseID).Scan(&tz); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("get course timezone failed: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT u.gender, u.birth_date, COALESCE(usl.skill_level, 0)
		FROM public.enrollments e JOIN public.courses c ON c.id = e.course_id JOIN public.users u ON u.id = e.user_id
		LEFT JOIN public.user_skill_levels usl ON usl.user_id = e.user_id AND usl.sport_id = c.sport_id
		WHERE e.course_id = $1 AND e.status = 'active'`, courseID)
	if err != nil {
		return nil, "", fmt.Errorf("list seats failed: %w", err)
	}
	defer rows.Close()
	var seats []Seat
	for rows.Next() {
		var s Seat
		if err := rows.Scan(&s.Gender, &s.BirthDate, &s.SkillLevel); err != nil {
			return nil, "", err
		}
		seats = append(seats, s)
	}
	return seats, tz, rows.Err()
}

func (r *pgxRepository) LocationExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.locations WHERE id = $1)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check location failed: %w", err)
	}
	return ok, nil
}

func (r *pgxRepository) SportActive(ctx context.Context, id string) (bool, error) {
	var ok bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.sports WHERE id = $1 AND is_active)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check sport failed: %w", err)
	}
	return ok, nil
}

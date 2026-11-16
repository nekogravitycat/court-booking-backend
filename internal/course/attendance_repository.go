package course

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LeaveContext is a leave together with the rules and counters needed to decide
// whether it can become a make-up.
type LeaveContext struct {
	Leave            *LeaveRequest
	AllowMakeup      bool
	MakeupLimit      *int
	MakeupValidDays  *int
	CoachID          *string
	GroupID          *string
	SportID          string
	EnrollmentStatus string
	UsedMakeups      int
}

// AttendanceRepository persists leave requests, make-up bookings and attendance.
//
// Seats of a session are counted as the active regular enrollments that did not
// take leave plus the make-up guests; selecting a make-up locks the target
// session row, so concurrent bookings cannot exceed the capacity.
type AttendanceRepository interface {
	GetSession(ctx context.Context, id string) (*SessionInfo, error)
	// ExpectedEnrollment returns the user's active enrollment that expects to
	// attend the session as a regular student (ErrNotInRoster otherwise).
	ExpectedEnrollment(ctx context.Context, sessionID, userID string) (string, error)

	CreateLeave(ctx context.Context, sessionID, enrollmentID, reason string) (*LeaveRequest, error)
	GetLeaveContext(ctx context.Context, id string) (*LeaveContext, error)
	DeleteLeave(ctx context.Context, id string) error
	ListMakeupOptions(ctx context.Context, lc *LeaveContext, validUntil *time.Time) ([]*MakeupOption, error)
	SelectMakeup(ctx context.Context, leaveID, targetSessionID string, validUntil *time.Time) (*LeaveRequest, *MakeupOption, error)
	CancelMakeup(ctx context.Context, leaveID string) (*LeaveRequest, error)

	Roster(ctx context.Context, sessionID string) ([]*RosterEntry, error)
	MarkAttendance(ctx context.Context, sessionID, markerID string, recs []AttendanceRecord) error
	EnrollmentAttendance(ctx context.Context, enrollmentID string) ([]*AttendanceItem, AttendanceSummary, error)
	// MakeupGuestIDs returns the users booked into the session as make-up guests.
	MakeupGuestIDs(ctx context.Context, sessionID string) ([]string, error)
	// HasAttended reports whether the enrollment was marked present at least once.
	HasAttended(ctx context.Context, enrollmentID string) (bool, error)
}

type pgxAttendanceRepository struct {
	pool *pgxpool.Pool
}

func NewPgxAttendanceRepository(pool *pgxpool.Pool) AttendanceRepository {
	return &pgxAttendanceRepository{pool: pool}
}

func (r *pgxAttendanceRepository) GetSession(ctx context.Context, id string) (*SessionInfo, error) {
	var s SessionInfo
	err := r.pool.QueryRow(ctx, `SELECT s.id, s.course_id, c.title, c.status, c.timezone, s.start_at, s.end_at, s.status,
		c.allow_leave, c.leave_deadline_hours, c.allow_makeup
		FROM public.course_sessions s JOIN public.courses c ON c.id = s.course_id WHERE s.id = $1`, id).
		Scan(&s.ID, &s.CourseID, &s.CourseTitle, &s.CourseStatus, &s.Timezone, &s.StartAt, &s.EndAt, &s.Status,
			&s.AllowLeave, &s.LeaveDeadlineHrs, &s.AllowMakeup)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session failed: %w", err)
	}
	return &s, nil
}

func (r *pgxAttendanceRepository) ExpectedEnrollment(ctx context.Context, sessionID, userID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT e.id FROM public.enrollments e JOIN public.course_sessions s ON s.course_id = e.course_id
		WHERE s.id = $1 AND e.user_id = $2 AND e.status = 'active' AND e.created_at < s.start_at`, sessionID, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotInRoster
	}
	if err != nil {
		return "", fmt.Errorf("find expected enrollment failed: %w", err)
	}
	return id, nil
}

const leaveColumns = `l.id, l.session_id, l.enrollment_id, e.user_id, COALESCE(u.display_name, u.username), c.id, c.title,
	s.start_at, s.end_at, l.reason, l.status, l.makeup_session_id, ms.start_at, mc.title, l.created_at`

const leaveFrom = ` FROM public.leave_requests l
	JOIN public.enrollments e ON e.id = l.enrollment_id
	JOIN public.users u ON u.id = e.user_id
	JOIN public.course_sessions s ON s.id = l.session_id
	JOIN public.courses c ON c.id = s.course_id
	LEFT JOIN public.course_sessions ms ON ms.id = l.makeup_session_id
	LEFT JOIN public.courses mc ON mc.id = ms.course_id`

func scanLeave(row pgx.Row, extra ...any) (*LeaveRequest, error) {
	var l LeaveRequest
	dest := append([]any{&l.ID, &l.SessionID, &l.EnrollmentID, &l.StudentID, &l.StudentName, &l.CourseID, &l.CourseTitle,
		&l.SessionStartAt, &l.SessionEndAt, &l.Reason, &l.Status, &l.MakeupSessionID, &l.MakeupStartAt, &l.MakeupCourse, &l.CreatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &l, nil
}

func getLeave(ctx context.Context, q querier, id string) (*LeaveRequest, error) {
	l, err := scanLeave(q.QueryRow(ctx, "SELECT "+leaveColumns+leaveFrom+" WHERE l.id = $1", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrLeaveNotFound
		}
		return nil, fmt.Errorf("get leave failed: %w", err)
	}
	return l, nil
}

func (r *pgxAttendanceRepository) CreateLeave(ctx context.Context, sessionID, enrollmentID, reason string) (*LeaveRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Serialize with attendance marking on this enrollment.
	if _, err := tx.Exec(ctx, "SELECT id FROM public.enrollments WHERE id = $1 FOR UPDATE", enrollmentID); err != nil {
		return nil, fmt.Errorf("lock enrollment failed: %w", err)
	}
	var marked string
	err = tx.QueryRow(ctx, "SELECT status FROM public.session_attendances WHERE session_id = $1 AND enrollment_id = $2", sessionID, enrollmentID).Scan(&marked)
	if err == nil && marked != AttPending {
		return nil, ErrAttendanceMarked
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check attendance failed: %w", err)
	}

	var id string
	err = tx.QueryRow(ctx, `INSERT INTO public.leave_requests (session_id, enrollment_id, reason) VALUES ($1, $2, $3) RETURNING id`,
		sessionID, enrollmentID, reason).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrLeaveExists
		}
		return nil, fmt.Errorf("create leave failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.session_attendances (session_id, enrollment_id, kind, status)
		VALUES ($1, $2, 'regular', 'leave') ON CONFLICT (session_id, enrollment_id) DO UPDATE SET status = 'leave'`, sessionID, enrollmentID); err != nil {
		return nil, fmt.Errorf("record leave attendance failed: %w", err)
	}
	l, err := getLeave(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return l, nil
}

func (r *pgxAttendanceRepository) GetLeaveContext(ctx context.Context, id string) (*LeaveContext, error) {
	l, err := getLeave(ctx, r.pool, id)
	if err != nil {
		return nil, err
	}
	return r.leaveContext(ctx, r.pool, l)
}

func (r *pgxAttendanceRepository) leaveContext(ctx context.Context, q querier, l *LeaveRequest) (*LeaveContext, error) {
	lc := &LeaveContext{Leave: l}
	err := q.QueryRow(ctx, `SELECT c.allow_makeup, c.makeup_limit, c.makeup_valid_days, c.coach_id, c.group_id, c.sport_id, e.status,
		(SELECT count(*) FROM public.leave_requests x WHERE x.enrollment_id = e.id AND x.status = 'used')
		FROM public.enrollments e JOIN public.courses c ON c.id = e.course_id WHERE e.id = $1`, l.EnrollmentID).
		Scan(&lc.AllowMakeup, &lc.MakeupLimit, &lc.MakeupValidDays, &lc.CoachID, &lc.GroupID, &lc.SportID, &lc.EnrollmentStatus, &lc.UsedMakeups)
	if err != nil {
		return nil, fmt.Errorf("load leave context failed: %w", err)
	}
	return lc, nil
}

func (r *pgxAttendanceRepository) DeleteLeave(ctx context.Context, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var sessionID, enrollmentID, status string
	err = tx.QueryRow(ctx, "SELECT session_id, enrollment_id, status FROM public.leave_requests WHERE id = $1 FOR UPDATE", id).Scan(&sessionID, &enrollmentID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaveNotFound
	}
	if err != nil {
		return fmt.Errorf("lock leave failed: %w", err)
	}
	if status == LeaveUsed {
		return ErrLeaveUsed
	}
	if _, err := tx.Exec(ctx, "DELETE FROM public.session_attendances WHERE session_id = $1 AND enrollment_id = $2 AND status = 'leave'", sessionID, enrollmentID); err != nil {
		return fmt.Errorf("remove leave attendance failed: %w", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM public.leave_requests WHERE id = $1", id); err != nil {
		return fmt.Errorf("delete leave failed: %w", err)
	}
	return tx.Commit(ctx)
}

// optionsSQL selects sessions a student may attend as a make-up guest: another
// class of the same owner and sport, upcoming, with a free seat, that the
// student does not already attend. $1 course to exclude, $2 sport, $3 owner
// coach, $4 owner group, $5 latest start (nullable), $6 enrollment, $7 user.
const optionsSQL = `SELECT s.id, c.id, c.title, COALESCE(g.name, COALESCE(u.display_name, u.username)), s.start_at, s.end_at,
	c.timezone, c.venue_name, (c.capacity - reg.n - mk.n)
	FROM public.course_sessions s
	JOIN public.courses c ON c.id = s.course_id
	LEFT JOIN public.users u ON u.id = c.coach_id
	LEFT JOIN public.coach_groups g ON g.id = c.group_id
	CROSS JOIN LATERAL (SELECT count(*) AS n FROM public.enrollments e WHERE e.course_id = c.id AND e.status = 'active'
		AND e.created_at < s.start_at
		AND NOT EXISTS (SELECT 1 FROM public.leave_requests lr WHERE lr.session_id = s.id AND lr.enrollment_id = e.id)) reg
	CROSS JOIN LATERAL (SELECT count(*) AS n FROM public.session_attendances sa WHERE sa.session_id = s.id AND sa.kind = 'makeup') mk
	WHERE c.id <> $1 AND c.sport_id = $2 AND c.status <> 'cancelled'
	AND (($3::uuid IS NOT NULL AND c.coach_id = $3::uuid) OR ($4::uuid IS NOT NULL AND c.group_id = $4::uuid))
	AND s.status = 'scheduled' AND s.start_at > now()
	AND ($5::timestamptz IS NULL OR s.start_at <= $5::timestamptz)
	AND NOT EXISTS (SELECT 1 FROM public.session_attendances sa WHERE sa.session_id = s.id AND sa.enrollment_id = $6::uuid)
	AND NOT EXISTS (SELECT 1 FROM public.enrollments e2 WHERE e2.course_id = c.id AND e2.user_id = $7::uuid
		AND e2.status = 'active' AND e2.created_at < s.start_at)
	AND (c.capacity - reg.n - mk.n) > 0
	AND ` + ownerAvailableSQL

func queryOptions(ctx context.Context, q querier, lc *LeaveContext, validUntil *time.Time, onlySession string) ([]*MakeupOption, error) {
	sql := optionsSQL
	args := []any{lc.Leave.CourseID, lc.SportID, lc.CoachID, lc.GroupID, validUntil, lc.Leave.EnrollmentID, lc.Leave.StudentID}
	if onlySession != "" {
		sql += " AND s.id = $8::uuid"
		args = append(args, onlySession)
	}
	sql += " ORDER BY s.start_at, s.id"
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list make-up options failed: %w", err)
	}
	defer rows.Close()
	out := []*MakeupOption{}
	for rows.Next() {
		var o MakeupOption
		if err := rows.Scan(&o.SessionID, &o.CourseID, &o.CourseTitle, &o.OwnerName, &o.StartAt, &o.EndAt, &o.Timezone, &o.VenueName, &o.RemainingSeats); err != nil {
			return nil, fmt.Errorf("scan make-up option failed: %w", err)
		}
		out = append(out, &o)
	}
	return out, rows.Err()
}

func (r *pgxAttendanceRepository) ListMakeupOptions(ctx context.Context, lc *LeaveContext, validUntil *time.Time) ([]*MakeupOption, error) {
	return queryOptions(ctx, r.pool, lc, validUntil, "")
}

func (r *pgxAttendanceRepository) SelectMakeup(ctx context.Context, leaveID, targetSessionID string, validUntil *time.Time) (*LeaveRequest, *MakeupOption, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	// Lock order is target session first, then the leave. Cancelling a session
	// releases its make-up guests in the same order, so the two cannot deadlock.
	var locked string
	err = tx.QueryRow(ctx, "SELECT id FROM public.course_sessions WHERE id = $1 FOR UPDATE", targetSessionID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrMakeupUnavailable
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lock target session failed: %w", err)
	}
	var status string
	err = tx.QueryRow(ctx, "SELECT status FROM public.leave_requests WHERE id = $1 FOR UPDATE", leaveID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrLeaveNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lock leave failed: %w", err)
	}
	if status != LeaveOpen {
		return nil, nil, ErrLeaveUsed
	}

	l, err := getLeave(ctx, tx, leaveID)
	if err != nil {
		return nil, nil, err
	}
	lc, err := r.leaveContext(ctx, tx, l)
	if err != nil {
		return nil, nil, err
	}
	if lc.EnrollmentStatus != EnrollActive {
		return nil, nil, ErrEnrollmentInactive
	}
	if err := checkMakeupRules(lc, time.Now()); err != nil {
		return nil, nil, err
	}

	opts, err := queryOptions(ctx, tx, lc, validUntil, targetSessionID)
	if err != nil {
		return nil, nil, err
	}
	if len(opts) == 0 {
		return nil, nil, ErrMakeupUnavailable
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.session_attendances (session_id, enrollment_id, kind, status)
		VALUES ($1, $2, 'makeup', 'pending')`, targetSessionID, l.EnrollmentID); err != nil {
		if isUniqueViolation(err) {
			return nil, nil, ErrMakeupUnavailable
		}
		return nil, nil, fmt.Errorf("book make-up failed: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE public.leave_requests SET status = 'used', makeup_session_id = $2, updated_at = now() WHERE id = $1", leaveID, targetSessionID); err != nil {
		return nil, nil, fmt.Errorf("mark leave used failed: %w", err)
	}
	updated, err := getLeave(ctx, tx, leaveID)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return updated, opts[0], nil
}

// checkMakeupRules applies the course's structured make-up rules to a leave.
func checkMakeupRules(lc *LeaveContext, now time.Time) error {
	if !lc.AllowMakeup {
		return ErrMakeupNotAllowed
	}
	if lc.MakeupLimit != nil && lc.UsedMakeups >= *lc.MakeupLimit {
		return ErrMakeupLimit
	}
	if lc.MakeupValidDays != nil && now.After(lc.Leave.SessionStartAt.Add(time.Duration(*lc.MakeupValidDays)*24*time.Hour)) {
		return ErrMakeupExpired
	}
	return nil
}

func (r *pgxAttendanceRepository) CancelMakeup(ctx context.Context, leaveID string) (*LeaveRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	var makeupID *string
	var enrollmentID string
	err = tx.QueryRow(ctx, "SELECT status, makeup_session_id, enrollment_id FROM public.leave_requests WHERE id = $1 FOR UPDATE", leaveID).Scan(&status, &makeupID, &enrollmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLeaveNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock leave failed: %w", err)
	}
	if status != LeaveUsed || makeupID == nil {
		return nil, ErrLeaveOpen
	}
	var start time.Time
	if err := tx.QueryRow(ctx, "SELECT start_at FROM public.course_sessions WHERE id = $1", *makeupID).Scan(&start); err != nil {
		return nil, fmt.Errorf("get make-up session failed: %w", err)
	}
	if !start.After(time.Now()) {
		return nil, ErrSessionStarted
	}
	if _, err := tx.Exec(ctx, "DELETE FROM public.session_attendances WHERE session_id = $1 AND enrollment_id = $2 AND kind = 'makeup'", *makeupID, enrollmentID); err != nil {
		return nil, fmt.Errorf("remove make-up booking failed: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE public.leave_requests SET status = 'open', makeup_session_id = NULL, updated_at = now() WHERE id = $1", leaveID); err != nil {
		return nil, fmt.Errorf("reopen leave failed: %w", err)
	}
	l, err := getLeave(ctx, tx, leaveID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return l, nil
}

const rosterSQL = `SELECT e.id, u.id, COALESCE(u.display_name, u.username), u.avatar, u.phone, u.line_id, usl.skill_level, sl.label,
	m.kind, sa.status, sa.marked_at, fc.title
	FROM (
		SELECT e.id AS enrollment_id, 'regular'::text AS kind FROM public.enrollments e JOIN public.course_sessions s ON s.course_id = e.course_id
		WHERE s.id = $1 AND e.status = 'active' AND e.created_at < s.start_at
		UNION
		SELECT sa.enrollment_id, sa.kind FROM public.session_attendances sa
		JOIN public.enrollments e ON e.id = sa.enrollment_id
		WHERE sa.session_id = $1 AND (sa.kind = 'regular' OR e.status = 'active' OR sa.status IN ('present', 'absent'))
	) m
	JOIN public.enrollments e ON e.id = m.enrollment_id
	JOIN public.users u ON u.id = e.user_id
	JOIN public.course_sessions s ON s.id = $1
	JOIN public.courses tc ON tc.id = s.course_id
	LEFT JOIN public.session_attendances sa ON sa.session_id = s.id AND sa.enrollment_id = e.id
	LEFT JOIN public.user_skill_levels usl ON usl.user_id = e.user_id AND usl.sport_id = tc.sport_id
	LEFT JOIN public.skill_levels sl ON sl.sport_id = usl.sport_id AND sl.level = usl.skill_level
	LEFT JOIN public.courses fc ON fc.id = e.course_id AND m.kind = 'makeup'
	ORDER BY (m.kind = 'makeup'), u.display_name, e.id`

func (r *pgxAttendanceRepository) Roster(ctx context.Context, sessionID string) ([]*RosterEntry, error) {
	rows, err := r.pool.Query(ctx, rosterSQL, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list roster failed: %w", err)
	}
	defer rows.Close()
	out := []*RosterEntry{}
	for rows.Next() {
		var e RosterEntry
		var kind, status *string
		if err := rows.Scan(&e.EnrollmentID, &e.UserID, &e.DisplayName, &e.Avatar, &e.Phone, &e.LineID, &e.SkillLevel, &e.SkillLabel,
			&kind, &status, &e.MarkedAt, &e.FromCourse); err != nil {
			return nil, fmt.Errorf("scan roster failed: %w", err)
		}
		e.Kind, e.Status = KindRegular, AttPending
		if kind != nil {
			e.Kind = *kind
		}
		if status != nil {
			e.Status = *status
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (r *pgxAttendanceRepository) MarkAttendance(ctx context.Context, sessionID, markerID string, recs []AttendanceRecord) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Lock the session so concurrent markings and leave requests serialize.
	var locked string
	if err := tx.QueryRow(ctx, "SELECT id FROM public.course_sessions WHERE id = $1 FOR UPDATE", sessionID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("lock session failed: %w", err)
	}

	rows, err := tx.Query(ctx, rosterSQL, sessionID)
	if err != nil {
		return fmt.Errorf("list roster failed: %w", err)
	}
	type member struct{ kind, status string }
	roster := map[string]member{}
	for rows.Next() {
		var enrollmentID, userID, name string
		var avatar, phone, line, label, from *string
		var level *int
		var markedAt *time.Time
		var kind, status *string
		if err := rows.Scan(&enrollmentID, &userID, &name, &avatar, &phone, &line, &level, &label, &kind, &status, &markedAt, &from); err != nil {
			rows.Close()
			return fmt.Errorf("scan roster failed: %w", err)
		}
		m := member{kind: KindRegular, status: AttPending}
		if kind != nil {
			m.kind = *kind
		}
		if status != nil {
			m.status = *status
		}
		roster[enrollmentID] = m
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, rec := range recs {
		m, ok := roster[rec.EnrollmentID]
		if !ok {
			return ErrRecordNotInRoster
		}
		if m.status == AttLeave {
			return ErrOnLeave
		}
		if _, err := tx.Exec(ctx, `INSERT INTO public.session_attendances (session_id, enrollment_id, kind, status, marked_by, marked_at)
			VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (session_id, enrollment_id) DO UPDATE SET status = EXCLUDED.status, marked_by = EXCLUDED.marked_by, marked_at = now()`,
			sessionID, rec.EnrollmentID, m.kind, rec.Status, nullableID(markerID)); err != nil {
			return fmt.Errorf("record attendance failed: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *pgxAttendanceRepository) EnrollmentAttendance(ctx context.Context, enrollmentID string) ([]*AttendanceItem, AttendanceSummary, error) {
	var sum AttendanceSummary
	rows, err := r.pool.Query(ctx, `SELECT s.id, c.id, c.title, s.start_at, s.end_at, s.status, COALESCE(sa.kind, 'regular'), COALESCE(sa.status, 'pending')
		FROM public.enrollments e
		JOIN public.course_sessions s ON (s.course_id = e.course_id AND s.start_at > e.created_at)
			OR s.id IN (SELECT session_id FROM public.session_attendances WHERE enrollment_id = e.id)
		JOIN public.courses c ON c.id = s.course_id
		LEFT JOIN public.session_attendances sa ON sa.session_id = s.id AND sa.enrollment_id = e.id
		WHERE e.id = $1 ORDER BY s.start_at, s.id`, enrollmentID)
	if err != nil {
		return nil, sum, fmt.Errorf("list attendance failed: %w", err)
	}
	defer rows.Close()
	out := []*AttendanceItem{}
	for rows.Next() {
		var it AttendanceItem
		if err := rows.Scan(&it.SessionID, &it.CourseID, &it.CourseTitle, &it.StartAt, &it.EndAt, &it.SessionStatus, &it.Kind, &it.Status); err != nil {
			return nil, sum, fmt.Errorf("scan attendance failed: %w", err)
		}
		switch it.Status {
		case AttPresent:
			sum.Present++
		case AttAbsent:
			sum.Absent++
		case AttLeave:
			sum.Leave++
		}
		out = append(out, &it)
	}
	return out, sum, rows.Err()
}

func (r *pgxAttendanceRepository) MakeupGuestIDs(ctx context.Context, sessionID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT e.user_id FROM public.session_attendances sa JOIN public.enrollments e ON e.id = sa.enrollment_id
		WHERE sa.session_id = $1 AND sa.kind = 'makeup'`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list make-up guests failed: %w", err)
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

func (r *pgxAttendanceRepository) HasAttended(ctx context.Context, enrollmentID string) (bool, error) {
	var ok bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.session_attendances WHERE enrollment_id = $1 AND status = 'present')", enrollmentID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check attendance failed: %w", err)
	}
	return ok, nil
}

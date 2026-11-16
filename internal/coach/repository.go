package coach

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
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

// Cleanup lists files that became unreferenced after a profile write. The
// caller deletes them best-effort once the write is durable.
type Cleanup struct {
	FileIDs []string
}

// Repository persists coach profiles.
type Repository interface {
	// Apply creates a pending application, or re-opens a rejected one. It
	// returns ErrAlreadyApplied for any other existing status.
	Apply(ctx context.Context, userID string, p ProfileInput) (Cleanup, error)
	Get(ctx context.Context, userID string) (*Coach, error)
	// Update applies a partial profile update. Pending and approved coaches can
	// edit; suspended and rejected ones cannot.
	Update(ctx context.Context, userID string, u ProfileUpdate) (Cleanup, error)
	List(ctx context.Context, filter CoachFilter) ([]*Coach, int, error)
	// Review moves a pending application to approved or rejected.
	Review(ctx context.Context, userID, reviewerID string, approve bool, reason string) error
	// CountActiveSports returns how many of the given sport IDs exist and are active.
	CountActiveSports(ctx context.Context, ids []string) (int, error)
	// AdminIDs returns the IDs of all active system admins.
	AdminIDs(ctx context.Context) ([]string, error)
	// GetStatus returns the coach status, or ErrNotFound.
	GetStatus(ctx context.Context, userID string) (string, error)
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

// querier is satisfied by both the pool and a transaction.
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

const coachSelect = `SELECT c.user_id, COALESCE(u.display_name, u.username), u.avatar, c.status,
	c.bio, c.teaching_content, c.target_audience, c.experience, c.fee_info, c.photo_file_id,
	c.reject_reason, c.reviewed_at, c.applied_at, c.created_at, c.updated_at`

func scanCoach(row pgx.Row, extra ...any) (*Coach, error) {
	var c Coach
	dest := append([]any{
		&c.UserID, &c.DisplayName, &c.Avatar, &c.Status,
		&c.Bio, &c.TeachingContent, &c.TargetAudience, &c.Experience, &c.FeeInfo, &c.PhotoFileID,
		&c.RejectReason, &c.ReviewedAt, &c.AppliedAt, &c.CreatedAt, &c.UpdatedAt,
	}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *pgxRepository) Get(ctx context.Context, userID string) (*Coach, error) {
	c, err := scanCoach(r.pool.QueryRow(ctx,
		coachSelect+` FROM public.coaches c JOIN public.users u ON u.id = c.user_id WHERE c.user_id = $1`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get coach failed: %w", err)
	}
	if err := r.loadDetails(ctx, []*Coach{c}, true); err != nil {
		return nil, err
	}
	return c, nil
}

// loadDetails fills sports, and optionally certificates and group, for coaches.
func (r *pgxRepository) loadDetails(ctx context.Context, coaches []*Coach, full bool) error {
	if len(coaches) == 0 {
		return nil
	}
	ids := make([]string, len(coaches))
	byID := make(map[string]*Coach, len(coaches))
	for i, c := range coaches {
		ids[i] = c.UserID
		byID[c.UserID] = c
		c.Sports = []SportBrief{}
		c.Certificates = []Certificate{}
	}

	rows, err := r.pool.Query(ctx, `SELECT cs.coach_id, s.id, s.code, s.name
		FROM public.coach_sports cs JOIN public.sports s ON s.id = cs.sport_id
		WHERE cs.coach_id = ANY($1::uuid[]) ORDER BY s.code`, ids)
	if err != nil {
		return fmt.Errorf("load coach sports failed: %w", err)
	}
	for rows.Next() {
		var coachID string
		var s SportBrief
		if err := rows.Scan(&coachID, &s.ID, &s.Code, &s.Name); err != nil {
			rows.Close()
			return fmt.Errorf("scan coach sport failed: %w", err)
		}
		byID[coachID].Sports = append(byID[coachID].Sports, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if !full {
		return nil
	}

	rows, err = r.pool.Query(ctx, `SELECT coach_id, id, file_id, title
		FROM public.coach_certificates WHERE coach_id = ANY($1::uuid[]) ORDER BY created_at, id`, ids)
	if err != nil {
		return fmt.Errorf("load coach certificates failed: %w", err)
	}
	for rows.Next() {
		var coachID string
		var cert Certificate
		if err := rows.Scan(&coachID, &cert.ID, &cert.FileID, &cert.Title); err != nil {
			rows.Close()
			return fmt.Errorf("scan certificate failed: %w", err)
		}
		byID[coachID].Certificates = append(byID[coachID].Certificates, cert)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = r.pool.Query(ctx, `SELECT coach_id, round(avg(rating_x2) / 2.0, 2)::float8, count(*)
		FROM public.coach_reviews WHERE coach_id = ANY($1::uuid[]) AND is_visible GROUP BY coach_id`, ids)
	if err != nil {
		return fmt.Errorf("load coach ratings failed: %w", err)
	}
	for rows.Next() {
		var userID string
		var avg float64
		var n int
		if err := rows.Scan(&userID, &avg, &n); err != nil {
			rows.Close()
			return fmt.Errorf("scan coach rating failed: %w", err)
		}
		byID[userID].RatingAverage, byID[userID].RatingCount = &avg, n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = r.pool.Query(ctx, `SELECT m.user_id, g.id, g.name, m.role
		FROM public.coach_group_members m JOIN public.coach_groups g ON g.id = m.group_id
		WHERE m.user_id = ANY($1::uuid[]) AND g.is_active`, ids)
	if err != nil {
		return fmt.Errorf("load coach group failed: %w", err)
	}
	for rows.Next() {
		var userID string
		var g GroupBrief
		if err := rows.Scan(&userID, &g.ID, &g.Name, &g.Role); err != nil {
			rows.Close()
			return fmt.Errorf("scan coach group failed: %w", err)
		}
		byID[userID].Group = &g
	}
	rows.Close()
	return rows.Err()
}

func (r *pgxRepository) GetStatus(ctx context.Context, userID string) (string, error) {
	var status string
	err := r.pool.QueryRow(ctx, "SELECT status FROM public.coaches WHERE user_id = $1", userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get coach status failed: %w", err)
	}
	return status, nil
}

func (r *pgxRepository) CountActiveSports(ctx context.Context, ids []string) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		"SELECT count(*) FROM public.sports WHERE id = ANY($1::uuid[]) AND is_active", ids).Scan(&n); err != nil {
		return 0, fmt.Errorf("count sports failed: %w", err)
	}
	return n, nil
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

// replaceSportsAndCertificates rewrites the coach's sports and certificates and
// returns the file IDs of certificates that were dropped.
func replaceSports(ctx context.Context, q querier, userID string, sportIDs []string) error {
	if _, err := q.Exec(ctx, "DELETE FROM public.coach_sports WHERE coach_id = $1", userID); err != nil {
		return fmt.Errorf("clear coach sports failed: %w", err)
	}
	if _, err := q.Exec(ctx,
		"INSERT INTO public.coach_sports (coach_id, sport_id) SELECT $1, unnest($2::uuid[])", userID, sportIDs); err != nil {
		if isFKViolation(err) {
			return ErrSportNotFound
		}
		return fmt.Errorf("insert coach sports failed: %w", err)
	}
	return nil
}

func replaceCertificates(ctx context.Context, q querier, userID string, certs []CertificateInput) ([]string, error) {
	rows, err := q.Query(ctx, "SELECT file_id FROM public.coach_certificates WHERE coach_id = $1", userID)
	if err != nil {
		return nil, fmt.Errorf("list certificates failed: %w", err)
	}
	old := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		old[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if _, err := q.Exec(ctx, "DELETE FROM public.coach_certificates WHERE coach_id = $1", userID); err != nil {
		return nil, fmt.Errorf("clear certificates failed: %w", err)
	}
	for _, c := range certs {
		if _, err := q.Exec(ctx,
			"INSERT INTO public.coach_certificates (coach_id, file_id, title) VALUES ($1, $2, $3)",
			userID, c.FileID, c.Title); err != nil {
			if isFKViolation(err) {
				return nil, ErrInvalidFile
			}
			return nil, fmt.Errorf("insert certificate failed: %w", err)
		}
		delete(old, c.FileID)
	}
	removed := make([]string, 0, len(old))
	for id := range old {
		removed = append(removed, id)
	}
	return removed, nil
}

func (r *pgxRepository) Apply(ctx context.Context, userID string, p ProfileInput) (Cleanup, error) {
	var cleanup Cleanup
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return cleanup, err
	}
	defer tx.Rollback(ctx)

	// Serialize concurrent applications of one user on the user row.
	if _, err := tx.Exec(ctx, "SELECT id FROM public.users WHERE id = $1 FOR UPDATE", userID); err != nil {
		return cleanup, fmt.Errorf("lock user failed: %w", err)
	}

	var status string
	var oldPhoto *string
	err = tx.QueryRow(ctx, "SELECT status, photo_file_id FROM public.coaches WHERE user_id = $1 FOR UPDATE", userID).Scan(&status, &oldPhoto)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := tx.Exec(ctx, `INSERT INTO public.coaches
			(user_id, bio, teaching_content, target_audience, experience, fee_info, photo_file_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			userID, p.Bio, p.TeachingContent, p.TargetAudience, p.Experience, p.FeeInfo, p.PhotoFileID); err != nil {
			if isUniqueViolation(err) {
				return cleanup, ErrAlreadyApplied
			}
			return cleanup, fmt.Errorf("insert coach failed: %w", err)
		}
	case err != nil:
		return cleanup, fmt.Errorf("lock coach failed: %w", err)
	case status != StatusRejected:
		return cleanup, ErrAlreadyApplied
	default:
		if _, err := tx.Exec(ctx, `UPDATE public.coaches SET status = 'pending', bio = $2, teaching_content = $3,
			target_audience = $4, experience = $5, fee_info = $6, photo_file_id = $7, reject_reason = NULL,
			reviewed_by = NULL, reviewed_at = NULL, applied_at = now(), updated_at = now()
			WHERE user_id = $1`,
			userID, p.Bio, p.TeachingContent, p.TargetAudience, p.Experience, p.FeeInfo, p.PhotoFileID); err != nil {
			return cleanup, fmt.Errorf("reopen application failed: %w", err)
		}
		if oldPhoto != nil && (p.PhotoFileID == nil || *p.PhotoFileID != *oldPhoto) {
			cleanup.FileIDs = append(cleanup.FileIDs, *oldPhoto)
		}
	}

	if err := replaceSports(ctx, tx, userID, p.SportIDs); err != nil {
		return cleanup, err
	}
	removed, err := replaceCertificates(ctx, tx, userID, p.Certificates)
	if err != nil {
		return cleanup, err
	}
	cleanup.FileIDs = append(cleanup.FileIDs, removed...)

	if err := tx.Commit(ctx); err != nil {
		return Cleanup{}, err
	}
	return cleanup, nil
}

func (r *pgxRepository) Update(ctx context.Context, userID string, u ProfileUpdate) (Cleanup, error) {
	var cleanup Cleanup
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return cleanup, err
	}
	defer tx.Rollback(ctx)

	var status string
	var oldPhoto *string
	err = tx.QueryRow(ctx, "SELECT status, photo_file_id FROM public.coaches WHERE user_id = $1 FOR UPDATE", userID).Scan(&status, &oldPhoto)
	if errors.Is(err, pgx.ErrNoRows) {
		return cleanup, ErrNotFound
	}
	if err != nil {
		return cleanup, fmt.Errorf("lock coach failed: %w", err)
	}
	switch status {
	case StatusSuspended:
		return cleanup, ErrCoachSuspended
	case StatusRejected:
		return cleanup, ErrNotCoach
	}

	set := squirrel.Update("public.coaches").PlaceholderFormat(squirrel.Dollar).Set("updated_at", squirrel.Expr("now()"))
	if u.Bio != nil {
		set = set.Set("bio", *u.Bio)
	}
	if u.TeachingContent != nil {
		set = set.Set("teaching_content", *u.TeachingContent)
	}
	if u.TargetAudience != nil {
		set = set.Set("target_audience", *u.TargetAudience)
	}
	if u.Experience != nil {
		set = set.Set("experience", *u.Experience)
	}
	if u.FeeInfo != nil {
		set = set.Set("fee_info", *u.FeeInfo)
	}
	switch {
	case u.ClearPhoto:
		set = set.Set("photo_file_id", nil)
	case u.PhotoFileID != nil:
		set = set.Set("photo_file_id", *u.PhotoFileID)
	}
	sql, args, err := set.Where(squirrel.Eq{"user_id": userID}).ToSql()
	if err != nil {
		return cleanup, fmt.Errorf("build coach update failed: %w", err)
	}
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return cleanup, fmt.Errorf("update coach failed: %w", err)
	}
	if oldPhoto != nil && (u.ClearPhoto || (u.PhotoFileID != nil && *u.PhotoFileID != *oldPhoto)) {
		cleanup.FileIDs = append(cleanup.FileIDs, *oldPhoto)
	}

	if u.SportIDs != nil {
		if err := replaceSports(ctx, tx, userID, *u.SportIDs); err != nil {
			return Cleanup{}, err
		}
	}
	if u.Certificates != nil {
		removed, err := replaceCertificates(ctx, tx, userID, *u.Certificates)
		if err != nil {
			return Cleanup{}, err
		}
		cleanup.FileIDs = append(cleanup.FileIDs, removed...)
	}

	if err := tx.Commit(ctx); err != nil {
		return Cleanup{}, err
	}
	return cleanup, nil
}

func (r *pgxRepository) List(ctx context.Context, f CoachFilter) ([]*Coach, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	status := f.Status
	if status == "" {
		status = StatusApproved
	}
	q := psql.Select(
		"c.user_id", "COALESCE(u.display_name, u.username)", "u.avatar", "c.status",
		"c.bio", "c.teaching_content", "c.target_audience", "c.experience", "c.fee_info", "c.photo_file_id",
		"c.reject_reason", "c.reviewed_at", "c.applied_at", "c.created_at", "c.updated_at",
		"count(*) OVER() AS total_count",
	).From("public.coaches c").Join("public.users u ON u.id = c.user_id").
		Where(squirrel.Eq{"c.status": status}).
		Where("u.is_active").
		OrderBy("c.applied_at DESC", "c.user_id")
	if f.SportID != "" {
		q = q.Where("EXISTS (SELECT 1 FROM public.coach_sports cs WHERE cs.coach_id = c.user_id AND cs.sport_id = ?)", f.SportID)
	}
	if f.Keyword != "" {
		q = q.Where("COALESCE(u.display_name, u.username) ILIKE ?", "%"+request.EscapeLike(f.Keyword)+"%")
	}

	items, total, err := pagination.Collect(ctx, r.pool, q, f.Page, f.PageSize, "coach", func(rows pgx.Rows, total *int) (*Coach, error) {
		c, err := scanCoach(rows, total)
		if err != nil {
			return nil, fmt.Errorf("scan coach failed: %w", err)
		}
		return c, nil
	})
	if err != nil {
		return nil, 0, err
	}
	if err := r.loadDetails(ctx, items, true); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *pgxRepository) Review(ctx context.Context, userID, reviewerID string, approve bool, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, "SELECT status FROM public.coaches WHERE user_id = $1 FOR UPDATE", userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock coach failed: %w", err)
	}
	if status != StatusPending {
		return ErrApplicationNotOpen
	}

	next := StatusRejected
	var rejectReason *string
	if approve {
		next = StatusApproved
	} else {
		rejectReason = &reason
	}
	if _, err := tx.Exec(ctx, `UPDATE public.coaches SET status = $2, reject_reason = $3, reviewed_by = $4,
		reviewed_at = now(), updated_at = now() WHERE user_id = $1`, userID, next, rejectReason, reviewerID); err != nil {
		return fmt.Errorf("review coach failed: %w", err)
	}
	return tx.Commit(ctx)
}

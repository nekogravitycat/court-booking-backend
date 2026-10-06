package pickup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/db"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
)

type Repository interface {
	WithGroupLock(ctx context.Context, id string, fn func(Repository) error) error
	WithOrderLock(ctx context.Context, id string, fn func(Repository) error) error
	CreateGroup(ctx context.Context, group *PickupGroup) error
	// CreateGroupSeries creates the series and all of its groups in one transaction.
	CreateGroupSeries(ctx context.Context, hostID string, groups []*PickupGroup) (*GroupSeries, error)
	GetGroupSeriesByID(ctx context.Context, id string) (*GroupSeries, error)
	// SetOrderAttendance sets (status non-nil) or clears (nil) the order's attendance mark.
	SetOrderAttendance(ctx context.Context, orderID string, status *string, markedBy string) error
	// GetUserPickupStats counts the user's finished, non-cancelled groups with a confirmed order,
	// and how many of those orders are marked absent.
	GetUserPickupStats(ctx context.Context, userID string) (*UserPickupStats, error)
	GetGroupByID(ctx context.Context, id string) (*PickupGroup, error)
	ListGroups(ctx context.Context, filter GroupFilter) ([]*PickupGroup, int, error)
	UpdateGroup(ctx context.Context, group *PickupGroup) error
	DeleteGroup(ctx context.Context, id string) error

	// CreateOrder uses a transaction with SELECT FOR UPDATE to prevent overbooking.
	CreateOrder(ctx context.Context, order *PickupOrder) error
	GetOrderByID(ctx context.Context, id string) (*PickupOrder, error)
	GetOrdersByGroupID(ctx context.Context, groupID string) ([]*PickupOrder, error)
	GetOrdersByUserID(ctx context.Context, userID string) ([]*PickupOrder, error)
	UpdateOrder(ctx context.Context, order *PickupOrder) error
	// DeleteOrder hard-deletes an order. The group's current_enrolled is derived
	// from a live COUNT, so removing the row decrements it automatically.
	DeleteOrder(ctx context.Context, id string) error

	// ListOccupyingUserIDs returns the distinct users holding a seat-occupying order
	// (pending, confirmed, cancel_request) in the group.
	ListOccupyingUserIDs(ctx context.Context, groupID string) ([]string, error)

	// ListParticipantSeats returns one entry per pending/confirmed seat of the
	// group (party members expanded), along with the IANA timezone of the group's
	// location. It returns ErrGroupNotFound when the group does not exist.
	ListParticipantSeats(ctx context.Context, groupID string) ([]ParticipantSeat, string, error)

	// UpdateOrderWithCapacityCheck re-validates the group capacity inside a
	// transaction (with SELECT FOR UPDATE) before applying the update. It is used
	// when an order moves back into a seat-occupying state to prevent overbooking.
	UpdateOrderWithCapacityCheck(ctx context.Context, order *PickupOrder) error

	// HasConfirmedOrder reports whether the user holds a confirmed (or
	// cancel-requested) order in the group.
	HasConfirmedOrder(ctx context.Context, groupID, userID string) (bool, error)

	// CancelUpcomingOrdersByUser cancels the user's seat-occupying orders in
	// active groups that have not ended, and returns what was cancelled.
	CancelUpcomingOrdersByUser(ctx context.Context, userID string) ([]CancelledOrder, error)
	// CancelUpcomingGroupsByHost cancels the host's active groups that have not
	// ended and returns them (only ID, HostID and Title are populated).
	CancelUpcomingGroupsByHost(ctx context.Context, hostID string) ([]*PickupGroup, error)
}

type pgxRepository struct {
	pool interface {
		Begin(context.Context) (pgx.Tx, error)
		Query(context.Context, string, ...any) (pgx.Rows, error)
		QueryRow(context.Context, string, ...any) pgx.Row
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	}
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

// Enrollment transactions take the shared schedule lock, then user, group,
// and order locks. Group edits take the exclusive schedule lock before reading
// participants, so time changes cannot race enrollment in another group.
const scheduleLockID int64 = 724916832

func (r *pgxRepository) WithGroupLock(ctx context.Context, id string, fn func(Repository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", scheduleLockID); err != nil {
		return err
	}
	var lockedID string
	if err := tx.QueryRow(ctx, "SELECT id FROM public.pickup_groups WHERE id = $1 FOR UPDATE", id).Scan(&lockedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGroupNotFound
		}
		return err
	}
	if err := fn(&pgxRepository{pool: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *pgxRepository) WithOrderLock(ctx context.Context, id string, fn func(Repository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", scheduleLockID); err != nil {
		return err
	}
	var userID, groupID string
	if err := tx.QueryRow(ctx, "SELECT user_id, pickup_group_id FROM public.pickup_orders WHERE id = $1", id).Scan(&userID, &groupID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrderNotFound
		}
		return err
	}
	// Lock user, group, then order in one round trip; a batch runs its statements in order,
	// so the lock order stays the same as separate statements.
	var locks pgx.Batch
	locks.Queue("SELECT id FROM public.users WHERE id = $1 FOR UPDATE", userID)
	locks.Queue("SELECT id FROM public.pickup_groups WHERE id = $1 FOR UPDATE", groupID)
	locks.Queue("SELECT id FROM public.pickup_orders WHERE id = $1 FOR UPDATE", id)
	results := tx.SendBatch(ctx, &locks)
	for i := 0; i < locks.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			results.Close()
			return err
		}
	}
	if err := results.Close(); err != nil {
		return err
	}
	if err := fn(&pgxRepository{pool: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// groupSelectColumns are the columns returned by the group read queries, in the
// order the scanners below expect. Host, sport, and skill-level display fields
// are resolved via JOIN rather than snapshotted on pickup_groups.
var groupSelectColumns = []string{
	"pg.id", "pg.host_id", "pg.title", "pg.description", "pg.social", "pg.pickup_group_series_id", "pg.start_time", "pg.registration_deadline", "pg.end_time", "pg.fee",
	"pg.capacity", "pg.location_id", "pg.sport_id", "s.code", "s.name",
	"pg.min_skill_level", "COALESCE(sl_min.label, '')", "pg.max_skill_level", "sl_max.label",
	"u.username", "u.display_name", "u.phone",
	"pg.status", "pg.enable", "pg.created_at", "pg.updated_at",
	"COALESCE(SUM(po.party_size) FILTER (WHERE po.status NOT IN ('cancelled', 'rejected')), 0) AS current_enrolled",
}

// groupJoins wires the sport, skill-level, host, and orders tables onto a base
// "public.pickup_groups pg" selection.
func groupJoins(b squirrel.SelectBuilder) squirrel.SelectBuilder {
	return b.
		From("public.pickup_groups pg").
		Join("public.sports s ON pg.sport_id = s.id").
		Join("public.locations l ON pg.location_id = l.id").
		LeftJoin("public.skill_levels sl_min ON sl_min.sport_id = pg.sport_id AND sl_min.level = pg.min_skill_level").
		LeftJoin("public.skill_levels sl_max ON sl_max.sport_id = pg.sport_id AND sl_max.level = pg.max_skill_level").
		Join("public.users u ON pg.host_id = u.id").
		LeftJoin("public.pickup_orders po ON pg.id = po.pickup_group_id").
		GroupBy("pg.id", "s.id", "sl_min.id", "sl_max.id", "u.id", "l.id")
}

// scanGroup scans a group row in the groupSelectColumns order. Extra trailing
// scan targets (e.g. enrolled_status, total_count) are appended by callers.
func scanGroupInto(g *PickupGroup, extra ...any) []any {
	targets := []any{
		&g.ID, &g.HostID, &g.Title, &g.Description, &g.Social, &g.PickupGroupSeriesID, &g.StartTime, &g.RegistrationDeadline, &g.EndTime, &g.Fee,
		&g.Capacity, &g.LocationID, &g.SportID, &g.SportCode, &g.SportName,
		&g.MinSkillLevel, &g.MinSkillLevelLabel, &g.MaxSkillLevel, &g.MaxSkillLevelLabel,
		&g.HostUsername, &g.HostDisplayName, &g.HostPhone,
		&g.Status, &g.Enable, &g.CreatedAt, &g.UpdatedAt, &g.CurrentEnrolled,
	}
	return append(targets, extra...)
}

// mapLocationFKError turns a pickup_groups.location_id foreign-key violation
// into ErrLocationNotFound; any other error is wrapped with msg.
func mapLocationFKError(err error, msg string) error {
	if db.IsViolation(err, pgerrcode.ForeignKeyViolation, "pickup_groups_location_id_fkey") {
		return ErrLocationNotFound
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func (r *pgxRepository) CreateGroup(ctx context.Context, g *PickupGroup) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.pickup_groups").
		Columns("host_id", "title", "description", "social", "pickup_group_series_id", "start_time", "registration_deadline", "end_time",
			"fee", "capacity", "location_id", "sport_id", "min_skill_level", "max_skill_level", "status", "enable").
		Values(g.HostID, g.Title, g.Description, g.Social, g.PickupGroupSeriesID, g.StartTime, g.RegistrationDeadline, g.EndTime,
			g.Fee, g.Capacity, g.LocationID, g.SportID, g.MinSkillLevel, g.MaxSkillLevel, g.Status, g.Enable).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create pickup group query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return mapLocationFKError(err, "create pickup group failed")
	}
	return nil
}

func (r *pgxRepository) CreateGroupSeries(ctx context.Context, hostID string, groups []*PickupGroup) (*GroupSeries, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var seriesID string
	if err := tx.QueryRow(ctx,
		"INSERT INTO public.pickup_group_series (host_id) VALUES ($1) RETURNING id", hostID,
	).Scan(&seriesID); err != nil {
		return nil, fmt.Errorf("create pickup group series failed: %w", err)
	}

	txRepo := &pgxRepository{pool: tx}
	for _, g := range groups {
		g.PickupGroupSeriesID = &seriesID
		if err := txRepo.CreateGroup(ctx, g); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit pickup group series failed: %w", err)
	}
	return r.GetGroupSeriesByID(ctx, seriesID)
}

func (r *pgxRepository) GetGroupSeriesByID(ctx context.Context, id string) (*GroupSeries, error) {
	var series GroupSeries
	if err := r.pool.QueryRow(ctx,
		"SELECT id, host_id, created_at FROM public.pickup_group_series WHERE id = $1", id,
	).Scan(&series.ID, &series.HostID, &series.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSeriesNotFound
		}
		return nil, fmt.Errorf("get pickup group series failed: %w", err)
	}

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := groupJoins(psql.Select(groupSelectColumns...)).
		Where(squirrel.Eq{"pg.pickup_group_series_id": id}).
		OrderBy("pg.start_time ASC", "pg.id ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list series groups query failed: %w", err)
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list series groups failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var g PickupGroup
		if err := rows.Scan(scanGroupInto(&g)...); err != nil {
			return nil, fmt.Errorf("scan series group failed: %w", err)
		}
		series.Groups = append(series.Groups, &g)
	}
	return &series, rows.Err()
}

func (r *pgxRepository) SetOrderAttendance(ctx context.Context, orderID string, status *string, markedBy string) error {
	var by *string
	var at any
	if status != nil {
		by = &markedBy
		at = squirrel.Expr("now()")
	}
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.pickup_orders").
		Set("attendance_status", status).
		Set("attendance_marked_by", by).
		Set("attendance_marked_at", at).
		Where(squirrel.Eq{"id": orderID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build set attendance query failed: %w", err)
	}
	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("set order attendance failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}

func (r *pgxRepository) GetUserPickupStats(ctx context.Context, userID string) (*UserPickupStats, error) {
	var stats UserPickupStats
	// DISTINCT group ids keep a user to one count per group.
	if err := r.pool.QueryRow(ctx,
		`SELECT count(DISTINCT pg.id), count(DISTINCT pg.id) FILTER (WHERE po.attendance_status = 'absent')
		 FROM public.pickup_orders po
		 JOIN public.pickup_groups pg ON pg.id = po.pickup_group_id
		 WHERE po.user_id = $1 AND po.status = 'confirmed' AND pg.status <> 'cancelled' AND pg.end_time <= now()`,
		userID,
	).Scan(&stats.ParticipationCount, &stats.AbsenceCount); err != nil {
		return nil, fmt.Errorf("get user pickup stats failed: %w", err)
	}
	if stats.ParticipationCount > 0 {
		rate := float64(stats.AbsenceCount) / float64(stats.ParticipationCount)
		stats.AbsenceRate = &rate
	}
	return &stats, nil
}

func (r *pgxRepository) GetGroupByID(ctx context.Context, id string) (*PickupGroup, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := groupJoins(psql.Select(groupSelectColumns...)).
		Where(squirrel.Eq{"pg.id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get pickup group query failed: %w", err)
	}

	var g PickupGroup
	if err := r.pool.QueryRow(ctx, query, args...).Scan(scanGroupInto(&g)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGroupNotFound
		}
		return nil, fmt.Errorf("get pickup group failed: %w", err)
	}
	return &g, nil
}

func (r *pgxRepository) ListGroups(ctx context.Context, filter GroupFilter) ([]*PickupGroup, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	query := groupJoins(psql.Select(groupSelectColumns...).
		// enrolled_status resolves the viewer's own order status for the group.
		// NULLIF guards against an empty (anonymous) viewer id, which cannot be
		// cast to uuid; the unique (group, user) constraint bounds it to one row.
		Column("(SELECT po2.status FROM public.pickup_orders po2 "+
			"WHERE po2.pickup_group_id = pg.id AND po2.user_id = NULLIF(?, '')::uuid "+
			"LIMIT 1) AS enrolled_status", filter.ViewerUserID).
		Column("COUNT(*) OVER() AS total_count"))

	// distance_km is the haversine distance to the requested origin, or NULL when
	// no origin was given. The column is always selected so the scan shape is fixed.
	if filter.Latitude != nil && filter.Longitude != nil {
		query = query.Column(squirrel.Expr(distanceKmExpr+" AS distance_km", *filter.Latitude, *filter.Latitude, *filter.Longitude))
	} else {
		query = query.Column("NULL::float8 AS distance_km")
	}

	if filter.Status != "" {
		query = query.Where(squirrel.Eq{"pg.status": filter.Status})
	}
	if filter.SportID != "" {
		query = query.Where(squirrel.Eq{"pg.sport_id": filter.SportID})
	}
	// A group matches when its [min_skill_level, max_skill_level] range overlaps
	// the requested [MinSkillLevel, MaxSkillLevel] one (an unset bound, on either
	// side, is unbounded).
	if filter.MinSkillLevel != nil {
		query = query.Where("(pg.max_skill_level IS NULL OR pg.max_skill_level >= ?)", *filter.MinSkillLevel)
	}
	if filter.MaxSkillLevel != nil {
		query = query.Where(squirrel.LtOrEq{"pg.min_skill_level": *filter.MaxSkillLevel})
	}
	if filter.FeeMin != nil {
		query = query.Where(squirrel.GtOrEq{"pg.fee": *filter.FeeMin})
	}
	if filter.FeeMax != nil {
		query = query.Where(squirrel.LtOrEq{"pg.fee": *filter.FeeMax})
	}
	if filter.FollowedOnly {
		query = query.Where("EXISTS (SELECT 1 FROM public.favorite_hosts fh WHERE fh.user_id = NULLIF(?, '')::uuid AND fh.host_id = pg.host_id)", filter.ViewerUserID)
	}
	if filter.HostID != "" {
		query = query.Where(squirrel.Eq{"pg.host_id": filter.HostID})
	}
	if filter.EnabledOnly {
		query = query.Where(squirrel.Eq{"pg.enable": true})
	}
	if filter.PubliclyVisibleOnly {
		// Publicly visible groups: active, enabled, and not yet ended. Fully
		// booked groups are still included.
		query = query.
			Where(squirrel.Eq{"pg.status": string(GroupStatusActive)}).
			Where(squirrel.Eq{"pg.enable": true}).
			Where("pg.end_time > now()")
	}

	orderBy := "pg.start_time"
	orderDir := "DESC"
	switch filter.SortBy {
	case "created_at":
		orderBy = "pg.created_at"
	case "min_skill_level":
		orderBy = "pg.min_skill_level"
	case "max_skill_level":
		orderBy = "pg.max_skill_level"
	case "distance":
		// Nearest first unless the caller asks otherwise; groups without a
		// computable distance (no origin) sort last.
		orderBy = "distance_km"
		orderDir = "ASC"
	}
	if filter.SortOrder != "" {
		orderDir = strings.ToUpper(filter.SortOrder)
	}
	if orderDir != "ASC" && orderDir != "DESC" {
		orderDir = "DESC"
	}
	// pg.id is a tiebreaker so pagination stays stable across equal sort keys.
	query = query.OrderBy(orderBy+" "+orderDir+" NULLS LAST", "pg.id")

	return pagination.Collect(ctx, r.pool, query, filter.Page, filter.PageSize, "pickup group", func(rows pgx.Rows, total *int) (*PickupGroup, error) {
		var g PickupGroup
		var enrolledStatus *string
		if err := rows.Scan(scanGroupInto(&g, &enrolledStatus, total, &g.DistanceKm)...); err != nil {
			return nil, fmt.Errorf("scan pickup group failed: %w", err)
		}
		if enrolledStatus != nil {
			g.EnrolledStatus = *enrolledStatus
		}
		return &g, nil
	})
}

func (r *pgxRepository) UpdateGroup(ctx context.Context, g *PickupGroup) error {
	var sportChanged, hasHistory bool
	if err := r.pool.QueryRow(ctx, `SELECT sport_id <> $2::uuid,
 EXISTS(SELECT 1 FROM public.pickup_orders WHERE pickup_group_id = $1) OR EXISTS(SELECT 1 FROM public.skill_ratings WHERE pickup_group_id = $1)
 FROM public.pickup_groups WHERE id = $1`, g.ID, g.SportID).Scan(&sportChanged, &hasHistory); err != nil {
		return err
	}
	if sportChanged && hasHistory {
		return ErrSportHasParticipants
	}
	if g.Status != GroupStatusCancelled {
		var conflict bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS (
   SELECT 1 FROM public.pickup_orders own_order
   JOIN public.pickup_orders other_order ON other_order.user_id = own_order.user_id AND other_order.pickup_group_id <> $1
   JOIN public.pickup_groups other_group ON other_group.id = other_order.pickup_group_id
   WHERE own_order.pickup_group_id = $1 AND own_order.status NOT IN ('cancelled', 'rejected')
   AND other_order.status NOT IN ('cancelled', 'rejected') AND other_group.status <> 'cancelled'
   AND other_group.start_time < $3 AND other_group.end_time > $2
  )`, g.ID, g.StartTime, g.EndTime).Scan(&conflict); err != nil {
			return err
		}
		if conflict {
			return ErrTimeConflict
		}
	}
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.pickup_groups").
		Set("title", g.Title).
		Set("description", g.Description).
		Set("social", g.Social).
		Set("start_time", g.StartTime).
		Set("registration_deadline", g.RegistrationDeadline).
		Set("end_time", g.EndTime).
		Set("fee", g.Fee).
		Set("capacity", g.Capacity).
		Set("location_id", g.LocationID).
		Set("sport_id", g.SportID).
		Set("min_skill_level", g.MinSkillLevel).
		Set("max_skill_level", g.MaxSkillLevel).
		Set("status", g.Status).
		Set("enable", g.Enable).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": g.ID}).
		Suffix("RETURNING updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build update pickup group query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&g.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGroupNotFound
		}
		return mapLocationFKError(err, "update pickup group failed")
	}
	return nil
}

func (r *pgxRepository) DeleteGroup(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.pickup_groups").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete pickup group query failed: %w", err)
	}

	result, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		if db.IsInUse(err) {
			return ErrGroupInUse
		}
		return fmt.Errorf("delete pickup group failed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrGroupNotFound
	}
	return nil
}

// CreateOrder enrolls a user in a pickup group.
// It uses a database transaction with SELECT FOR UPDATE on the pickup group row
// to prevent overbooking under concurrent requests.
func (r *pgxRepository) CreateOrder(ctx context.Context, order *PickupOrder) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", scheduleLockID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "SELECT id FROM public.users WHERE id = $1 FOR UPDATE", order.UserID); err != nil {
		return err
	}
	// Lock the pickup group row to serialize concurrent enrollment attempts.
	var capacity int
	var status, sportID string
	var enabled bool
	var startTime, endTime, deadline time.Time
	if err := tx.QueryRow(ctx,
		"SELECT capacity, status::TEXT, start_time, end_time, registration_deadline, enable, sport_id FROM public.pickup_groups WHERE id = $1 FOR UPDATE",
		order.PickupGroupID,
	).Scan(&capacity, &status, &startTime, &endTime, &deadline, &enabled, &sportID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGroupNotFound
		}
		return fmt.Errorf("lock pickup group failed: %w", err)
	}

	if status != string(GroupStatusActive) || !enabled || !endTime.After(time.Now()) {
		return ErrGroupNotActive
	}
	if !deadline.After(time.Now()) {
		return ErrRegistrationClosed
	}

	if order.EnrollmentSportID != "" && order.EnrollmentSportID != sportID {
		return ErrSportChanged
	}
	// Reject enrollment if the user already holds an occupying order (in any
	// other group) whose time range overlaps this group's.
	var timeConflict bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM public.pickup_orders po
			JOIN public.pickup_groups pg2 ON pg2.id = po.pickup_group_id
			WHERE po.user_id = $1
				AND po.pickup_group_id <> $2
				AND po.status NOT IN ('cancelled', 'rejected')
                AND pg2.status <> 'cancelled'
				AND pg2.start_time < $3
				AND pg2.end_time > $4
		)`,
		order.UserID, order.PickupGroupID, endTime, startTime,
	).Scan(&timeConflict); err != nil {
		return fmt.Errorf("check time conflict failed: %w", err)
	}
	if timeConflict {
		return ErrTimeConflict
	}

	// Look up any order this user already has for the group. A rejected user is
	// permanently blocked; a still-occupying enrollment (pending / confirmed /
	// cancel_request) is a duplicate; only a fully cancelled order is re-usable,
	// so the user may re-enroll and the existing row is reset in place. The group
	// row is locked FOR UPDATE above, so enrollment attempts for this group are
	// serialized and this read is stable within the transaction.
	var existingID, existingStatus string
	err = tx.QueryRow(ctx,
		"SELECT id, status::TEXT FROM public.pickup_orders WHERE pickup_group_id = $1 AND user_id = $2",
		order.PickupGroupID, order.UserID,
	).Scan(&existingID, &existingStatus)
	switch {
	case err == nil:
		switch OrderStatus(existingStatus) {
		case OrderStatusRejected:
			return ErrRejectedFromGroup
		case OrderStatusCancelled:
			// Re-enrollable: fall through to the capacity check and reset below.
		default:
			return ErrAlreadyEnrolled
		}
	case errors.Is(err, pgx.ErrNoRows):
		existingID = "" // no prior order; a fresh row will be inserted.
	default:
		return fmt.Errorf("check existing pickup order failed: %w", err)
	}

	// Count occupying enrollments within the same transaction (reads the locked
	// snapshot). A re-usable cancelled row is excluded here, so it never
	// double-counts against the capacity.
	var currentEnrolled int
	if err := tx.QueryRow(ctx,
		"SELECT COALESCE(SUM(party_size), 0) FROM public.pickup_orders WHERE pickup_group_id = $1 AND status NOT IN ('cancelled', 'rejected')",
		order.PickupGroupID,
	).Scan(&currentEnrolled); err != nil {
		return fmt.Errorf("count enrollments failed: %w", err)
	}

	// The order occupies PartySize seats, all of which must fit.
	if currentEnrolled+order.PartySize > capacity {
		return ErrGroupFullyBooked
	}

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	// Re-enroll: reset the existing cancelled order in place instead of inserting
	// a new row (the unique (group, user) constraint forbids a second row).
	if existingID != "" {
		q, args, err := psql.Update("public.pickup_orders").
			Set("status", order.Status).
			Set("payment_status", order.PaymentStatus).
			Set("booker_name", order.BookerName).
			Set("booker_phone", order.BookerPhone).
			Set("skill_level", order.SkillLevel).
			Set("party_size", order.PartySize).
			Set("updated_at", squirrel.Expr("now()")).
			Where(squirrel.Eq{"id": existingID}).
			Suffix("RETURNING id, created_at, updated_at").
			ToSql()
		if err != nil {
			return fmt.Errorf("build re-enroll pickup order query failed: %w", err)
		}
		if err := tx.QueryRow(ctx, q, args...).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt); err != nil {
			return fmt.Errorf("re-enroll pickup order failed: %w", err)
		}
		if err := replaceMembers(ctx, tx, order.ID, order.Members); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	q, args, err := psql.Insert("public.pickup_orders").
		Columns("pickup_group_id", "user_id", "booker_name", "booker_phone", "status", "payment_status", "skill_level", "party_size").
		Values(order.PickupGroupID, order.UserID, order.BookerName, order.BookerPhone, order.Status, order.PaymentStatus, order.SkillLevel, order.PartySize).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create pickup order query failed: %w", err)
	}

	if err := tx.QueryRow(ctx, q, args...).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt); err != nil {
		if db.IsUniqueViolation(err) {
			return ErrAlreadyEnrolled
		}
		return fmt.Errorf("create pickup order failed: %w", err)
	}

	if err := replaceMembers(ctx, tx, order.ID, order.Members); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// orderColumns are the pickup_orders columns returned by the order read queries,
// in the order scanOrderInto expects.
var orderColumns = []string{
	"id", "pickup_group_id", "user_id", "booker_name", "booker_phone",
	"status", "payment_status", "skill_level", "party_size",
	"attendance_status", "attendance_marked_by", "attendance_marked_at", "created_at", "updated_at",
}

func scanOrderInto(o *PickupOrder) []any {
	return []any{
		&o.ID, &o.PickupGroupID, &o.UserID, &o.BookerName, &o.BookerPhone,
		&o.Status, &o.PaymentStatus, &o.SkillLevel, &o.PartySize,
		&o.AttendanceStatus, &o.AttendanceMarkedBy, &o.AttendanceMarkedAt, &o.CreatedAt, &o.UpdatedAt,
	}
}

// attachMembers loads the party members of the given orders in one query and
// sets them on the matching orders. Single-enrollment orders have none.
func (r *pgxRepository) attachMembers(ctx context.Context, orders []*PickupOrder) error {
	if len(orders) == 0 {
		return nil
	}
	ids := make([]string, len(orders))
	byID := make(map[string]*PickupOrder, len(orders))
	for i, o := range orders {
		ids[i] = o.ID
		byID[o.ID] = o
	}

	rows, err := r.pool.Query(ctx,
		"SELECT order_id, gender, skill_level FROM public.pickup_order_members WHERE order_id = ANY($1::uuid[]) ORDER BY created_at, id",
		ids)
	if err != nil {
		return fmt.Errorf("list pickup order members failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var orderID string
		var m OrderMember
		if err := rows.Scan(&orderID, &m.Gender, &m.SkillLevel); err != nil {
			return fmt.Errorf("scan pickup order member failed: %w", err)
		}
		if o, ok := byID[orderID]; ok {
			o.Members = append(o.Members, m)
		}
	}
	return rows.Err()
}

func (r *pgxRepository) GetOrderByID(ctx context.Context, id string) (*PickupOrder, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(orderColumns...).
		From("public.pickup_orders").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get pickup order query failed: %w", err)
	}

	var o PickupOrder
	if err := r.pool.QueryRow(ctx, query, args...).Scan(scanOrderInto(&o)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get pickup order failed: %w", err)
	}
	if err := r.attachMembers(ctx, []*PickupOrder{&o}); err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *pgxRepository) GetOrdersByGroupID(ctx context.Context, groupID string) ([]*PickupOrder, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(orderColumns...).
		From("public.pickup_orders").
		Where(squirrel.Eq{"pickup_group_id": groupID}).
		OrderBy("created_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list pickup orders query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list pickup orders failed: %w", err)
	}
	defer rows.Close()

	var orders []*PickupOrder
	for rows.Next() {
		var o PickupOrder
		if err := rows.Scan(scanOrderInto(&o)...); err != nil {
			return nil, fmt.Errorf("scan pickup order failed: %w", err)
		}
		orders = append(orders, &o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pickup orders failed: %w", err)
	}
	if err := r.attachMembers(ctx, orders); err != nil {
		return nil, err
	}
	return orders, nil
}

func (r *pgxRepository) GetOrdersByUserID(ctx context.Context, userID string) ([]*PickupOrder, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(orderColumns...).
		From("public.pickup_orders").
		Where(squirrel.Eq{"user_id": userID}).
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list pickup orders by user query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list pickup orders by user failed: %w", err)
	}
	defer rows.Close()

	var orders []*PickupOrder
	for rows.Next() {
		var o PickupOrder
		if err := rows.Scan(scanOrderInto(&o)...); err != nil {
			return nil, fmt.Errorf("scan pickup order failed: %w", err)
		}
		orders = append(orders, &o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pickup orders failed: %w", err)
	}
	if err := r.attachMembers(ctx, orders); err != nil {
		return nil, err
	}
	return orders, nil
}

func (r *pgxRepository) UpdateOrder(ctx context.Context, o *PickupOrder) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.pickup_orders").
		Set("status", o.Status).
		Set("payment_status", o.PaymentStatus).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": o.ID}).
		Suffix("RETURNING updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build update pickup order query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&o.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrderNotFound
		}
		return fmt.Errorf("update pickup order failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) DeleteOrder(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.pickup_orders").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete pickup order query failed: %w", err)
	}

	result, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete pickup order failed: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// UpdateOrderWithCapacityCheck applies an order update only if the group still
// has room. It locks the group row and counts the other occupying orders within
// the same transaction, mirroring CreateOrder, so concurrent reactivations and
// enrollments cannot push the group over capacity.
func (r *pgxRepository) UpdateOrderWithCapacityCheck(ctx context.Context, o *PickupOrder) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var capacity int
	var enabled bool
	var status string
	var startTime, endTime, deadline time.Time
	if err := tx.QueryRow(ctx,
		"SELECT capacity, enable, status::text, start_time, end_time, registration_deadline FROM public.pickup_groups WHERE id = $1 FOR UPDATE",
		o.PickupGroupID,
	).Scan(&capacity, &enabled, &status, &startTime, &endTime, &deadline); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGroupNotFound
		}
		return fmt.Errorf("lock pickup group failed: %w", err)
	}

	if status != string(GroupStatusActive) || !enabled || !endTime.After(time.Now()) {
		return ErrGroupNotActive
	}
	if !deadline.After(time.Now()) {
		return ErrRegistrationClosed
	}
	var conflict bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1 FROM public.pickup_orders po JOIN public.pickup_groups pg ON pg.id = po.pickup_group_id
  WHERE po.user_id = $1 AND po.pickup_group_id <> $2 AND po.status NOT IN ('cancelled', 'rejected')
  AND pg.status <> 'cancelled' AND pg.start_time < $3 AND pg.end_time > $4
 )`, o.UserID, o.PickupGroupID, endTime, startTime).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return ErrTimeConflict
	}
	// Count occupying orders other than this one; this order is about to become
	// occupying, so it must fit within the remaining capacity.
	var currentEnrolled int
	if err := tx.QueryRow(ctx,
		"SELECT COALESCE(SUM(party_size), 0) FROM public.pickup_orders WHERE pickup_group_id = $1 AND id <> $2 AND status NOT IN ('cancelled', 'rejected')",
		o.PickupGroupID, o.ID,
	).Scan(&currentEnrolled); err != nil {
		return fmt.Errorf("count enrollments failed: %w", err)
	}

	if currentEnrolled+o.PartySize > capacity {
		return ErrGroupFullyBooked
	}

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.pickup_orders").
		Set("status", o.Status).
		Set("payment_status", o.PaymentStatus).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": o.ID}).
		Suffix("RETURNING updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build update pickup order query failed: %w", err)
	}

	if err := tx.QueryRow(ctx, query, args...).Scan(&o.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrderNotFound
		}
		return fmt.Errorf("update pickup order failed: %w", err)
	}

	return tx.Commit(ctx)
}

// distanceKmExpr is the haversine great-circle distance (km) between the origin
// and the joined location "l". Its placeholders are, in order: origin latitude
// (delta), origin latitude (cosine term), origin longitude. The argument of
// asin is clamped to 1 to guard against floating-point overshoot.
const distanceKmExpr = "(2 * 6371 * asin(sqrt(least(1::float8, " +
	"power(sin(radians(l.latitude::float8 - ?::float8) / 2), 2) + " +
	"cos(radians(?::float8)) * cos(radians(l.latitude::float8)) * " +
	"power(sin(radians(l.longitude::float8 - ?::float8) / 2), 2)))))"

// replaceMembers rewrites the party members of an order inside tx. A single
// enrollment has no members, so the call only clears any stale rows.
func replaceMembers(ctx context.Context, tx pgx.Tx, orderID string, members []OrderMember) error {
	if _, err := tx.Exec(ctx, "DELETE FROM public.pickup_order_members WHERE order_id = $1", orderID); err != nil {
		return fmt.Errorf("clear pickup order members failed: %w", err)
	}
	for _, m := range members {
		if _, err := tx.Exec(ctx,
			"INSERT INTO public.pickup_order_members (order_id, gender, skill_level) VALUES ($1, $2, $3)",
			orderID, m.Gender, m.SkillLevel,
		); err != nil {
			return fmt.Errorf("insert pickup order member failed: %w", err)
		}
	}
	return nil
}

func (r *pgxRepository) ListOccupyingUserIDs(ctx context.Context, groupID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT user_id::text FROM public.pickup_orders
		 WHERE pickup_group_id = $1 AND status IN ('pending', 'confirmed', 'cancel_request')`,
		groupID)
	if err != nil {
		return nil, fmt.Errorf("list occupying users failed: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan occupying user failed: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *pgxRepository) ListParticipantSeats(ctx context.Context, groupID string) ([]ParticipantSeat, string, error) {
	var tz string
	if err := r.pool.QueryRow(ctx,
		`SELECT l.timezone FROM public.pickup_groups pg
		 JOIN public.locations l ON l.id = pg.location_id WHERE pg.id = $1`,
		groupID,
	).Scan(&tz); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrGroupNotFound
		}
		return nil, "", fmt.Errorf("get group timezone failed: %w", err)
	}

	// Single enrollments take gender / birth date from the enrolling user; party
	// orders contribute one anonymous seat per member (no birth date).
	rows, err := r.pool.Query(ctx,
		`SELECT u.gender, u.birth_date, po.skill_level
		   FROM public.pickup_orders po
		   JOIN public.users u ON u.id = po.user_id
		  WHERE po.pickup_group_id = $1 AND po.status IN ('pending', 'confirmed') AND po.party_size = 1
		 UNION ALL
		 SELECT m.gender, NULL::date, m.skill_level
		   FROM public.pickup_order_members m
		   JOIN public.pickup_orders po ON po.id = m.order_id
		  WHERE po.pickup_group_id = $1 AND po.status IN ('pending', 'confirmed')`,
		groupID)
	if err != nil {
		return nil, "", fmt.Errorf("list participant seats failed: %w", err)
	}
	defer rows.Close()

	var seats []ParticipantSeat
	for rows.Next() {
		var s ParticipantSeat
		if err := rows.Scan(&s.Gender, &s.BirthDate, &s.SkillLevel); err != nil {
			return nil, "", fmt.Errorf("scan participant seat failed: %w", err)
		}
		seats = append(seats, s)
	}
	return seats, tz, rows.Err()
}

func (r *pgxRepository) CancelUpcomingOrdersByUser(ctx context.Context, userID string) ([]CancelledOrder, error) {
	rows, err := r.pool.Query(ctx,
		`UPDATE public.pickup_orders po
		 SET status = 'cancelled', updated_at = now()
		 FROM public.pickup_groups pg
		 WHERE po.pickup_group_id = pg.id
		   AND po.user_id = $1
		   AND po.status IN ('pending', 'confirmed', 'cancel_request')
		   AND pg.status = 'active'
		   AND pg.end_time > now()
		 RETURNING po.id::text, pg.id::text, pg.title, pg.host_id::text, po.booker_name`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("cancel upcoming orders failed: %w", err)
	}
	defer rows.Close()

	var out []CancelledOrder
	for rows.Next() {
		var c CancelledOrder
		if err := rows.Scan(&c.OrderID, &c.GroupID, &c.GroupTitle, &c.HostID, &c.BookerName); err != nil {
			return nil, fmt.Errorf("scan cancelled order failed: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *pgxRepository) CancelUpcomingGroupsByHost(ctx context.Context, hostID string) ([]*PickupGroup, error) {
	rows, err := r.pool.Query(ctx,
		`UPDATE public.pickup_groups
		 SET status = 'cancelled', updated_at = now()
		 WHERE host_id = $1 AND status = 'active' AND end_time > now()
		 RETURNING id::text, title`,
		hostID)
	if err != nil {
		return nil, fmt.Errorf("cancel upcoming groups failed: %w", err)
	}
	defer rows.Close()

	var out []*PickupGroup
	for rows.Next() {
		g := &PickupGroup{HostID: hostID}
		if err := rows.Scan(&g.ID, &g.Title); err != nil {
			return nil, fmt.Errorf("scan cancelled group failed: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *pgxRepository) HasConfirmedOrder(ctx context.Context, groupID, userID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM public.pickup_orders
		 WHERE pickup_group_id = $1 AND user_id = $2 AND status IN ('confirmed', 'cancel_request'))`,
		groupID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check confirmed order failed: %w", err)
	}
	return ok, nil
}

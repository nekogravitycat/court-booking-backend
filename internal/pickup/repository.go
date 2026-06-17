package pickup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	CreateGroup(ctx context.Context, group *PickupGroup) error
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

	// UpdateOrderWithCapacityCheck re-validates the group capacity inside a
	// transaction (with SELECT FOR UPDATE) before applying the update. It is used
	// when an order moves back into a seat-occupying state to prevent overbooking.
	UpdateOrderWithCapacityCheck(ctx context.Context, order *PickupOrder) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) CreateGroup(ctx context.Context, g *PickupGroup) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.pickup_groups").
		Columns("host_id", "title", "host_name", "host_phone", "start_time", "end_time",
			"fee", "capacity", "location_id", "skill_level", "status", "enable").
		Values(g.HostID, g.Title, g.HostName, g.HostPhone, g.StartTime, g.EndTime,
			g.Fee, g.Capacity, g.LocationID, g.SkillLevel, g.Status, g.Enable).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create pickup group query failed: %w", err)
	}

	return r.pool.QueryRow(ctx, query, args...).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
}

func (r *pgxRepository) GetGroupByID(ctx context.Context, id string) (*PickupGroup, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"pg.id", "pg.host_id", "pg.title", "pg.host_name", "pg.host_phone",
		"pg.start_time", "pg.end_time", "pg.fee", "pg.capacity", "pg.location_id",
		"pg.skill_level", "pg.status", "pg.enable", "pg.created_at", "pg.updated_at",
		"COALESCE(COUNT(po.id) FILTER (WHERE po.status NOT IN ('cancelled', 'cancel_request')), 0) AS current_enrolled",
	).
		From("public.pickup_groups pg").
		LeftJoin("public.pickup_orders po ON pg.id = po.pickup_group_id").
		Where(squirrel.Eq{"pg.id": id}).
		GroupBy("pg.id").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get pickup group query failed: %w", err)
	}

	var g PickupGroup
	if err := r.pool.QueryRow(ctx, query, args...).Scan(
		&g.ID, &g.HostID, &g.Title, &g.HostName, &g.HostPhone,
		&g.StartTime, &g.EndTime, &g.Fee, &g.Capacity, &g.LocationID,
		&g.SkillLevel, &g.Status, &g.Enable, &g.CreatedAt, &g.UpdatedAt,
		&g.CurrentEnrolled,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGroupNotFound
		}
		return nil, fmt.Errorf("get pickup group failed: %w", err)
	}
	return &g, nil
}

func (r *pgxRepository) ListGroups(ctx context.Context, filter GroupFilter) ([]*PickupGroup, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query := psql.Select(
		"pg.id", "pg.host_id", "pg.title", "pg.host_name", "pg.host_phone",
		"pg.start_time", "pg.end_time", "pg.fee", "pg.capacity", "pg.location_id",
		"pg.skill_level", "pg.status", "pg.enable", "pg.created_at", "pg.updated_at",
		"COALESCE(COUNT(po.id) FILTER (WHERE po.status NOT IN ('cancelled', 'cancel_request')), 0) AS current_enrolled",
		"COUNT(*) OVER() AS total_count",
	).
		From("public.pickup_groups pg").
		LeftJoin("public.pickup_orders po ON pg.id = po.pickup_group_id").
		GroupBy("pg.id")

	if filter.Status != "" {
		query = query.Where(squirrel.Eq{"pg.status": filter.Status})
	}
	if filter.SkillLevel != "" {
		query = query.Where(squirrel.Eq{"pg.skill_level": filter.SkillLevel})
	}
	if filter.HostID != "" {
		query = query.Where(squirrel.Eq{"pg.host_id": filter.HostID})
	}
	if filter.BookableOnly {
		// Only groups that can still be enrolled into: active, enabled, not yet
		// ended, and not fully booked.
		query = query.
			Where(squirrel.Eq{"pg.status": string(GroupStatusActive)}).
			Where(squirrel.Eq{"pg.enable": true}).
			Where("pg.end_time > now()").
			Having("COUNT(po.id) FILTER (WHERE po.status NOT IN ('cancelled', 'cancel_request')) < pg.capacity")
	}

	orderBy := "pg.start_time"
	if filter.SortBy != "" {
		orderBy = "pg." + filter.SortBy
	}
	orderDir := "DESC"
	if filter.SortOrder != "" {
		orderDir = strings.ToUpper(filter.SortOrder)
	}
	query = query.OrderBy(orderBy + " " + orderDir)

	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize
	query = query.Limit(uint64(filter.PageSize)).Offset(uint64(offset))

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build list pickup groups query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list pickup groups failed: %w", err)
	}
	defer rows.Close()

	var groups []*PickupGroup
	var total int

	for rows.Next() {
		var g PickupGroup
		if err := rows.Scan(
			&g.ID, &g.HostID, &g.Title, &g.HostName, &g.HostPhone,
			&g.StartTime, &g.EndTime, &g.Fee, &g.Capacity, &g.LocationID,
			&g.SkillLevel, &g.Status, &g.Enable, &g.CreatedAt, &g.UpdatedAt,
			&g.CurrentEnrolled, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan pickup group failed: %w", err)
		}
		groups = append(groups, &g)
	}

	return groups, total, nil
}

func (r *pgxRepository) UpdateGroup(ctx context.Context, g *PickupGroup) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.pickup_groups").
		Set("title", g.Title).
		Set("host_name", g.HostName).
		Set("host_phone", g.HostPhone).
		Set("start_time", g.StartTime).
		Set("end_time", g.EndTime).
		Set("fee", g.Fee).
		Set("capacity", g.Capacity).
		Set("location_id", g.LocationID).
		Set("skill_level", g.SkillLevel).
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
		return fmt.Errorf("update pickup group failed: %w", err)
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

	// Lock the pickup group row to serialize concurrent enrollment attempts.
	var capacity int
	var status string
	if err := tx.QueryRow(ctx,
		"SELECT capacity, status::TEXT FROM public.pickup_groups WHERE id = $1 FOR UPDATE",
		order.PickupGroupID,
	).Scan(&capacity, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGroupNotFound
		}
		return fmt.Errorf("lock pickup group failed: %w", err)
	}

	if status != string(GroupStatusActive) {
		return ErrGroupNotActive
	}

	// Count active enrollments within the same transaction (reads the locked snapshot).
	var currentEnrolled int
	if err := tx.QueryRow(ctx,
		"SELECT COUNT(*) FROM public.pickup_orders WHERE pickup_group_id = $1 AND status NOT IN ('cancelled', 'cancel_request')",
		order.PickupGroupID,
	).Scan(&currentEnrolled); err != nil {
		return fmt.Errorf("count enrollments failed: %w", err)
	}

	if currentEnrolled >= capacity {
		return ErrGroupFullyBooked
	}

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	q, args, err := psql.Insert("public.pickup_orders").
		Columns("pickup_group_id", "user_id", "booker_name", "booker_phone", "status", "payment_status").
		Values(order.PickupGroupID, order.UserID, order.BookerName, order.BookerPhone, order.Status, order.PaymentStatus).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create pickup order query failed: %w", err)
	}

	if err := tx.QueryRow(ctx, q, args...).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return ErrAlreadyEnrolled
		}
		return fmt.Errorf("create pickup order failed: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *pgxRepository) GetOrderByID(ctx context.Context, id string) (*PickupOrder, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"id", "pickup_group_id", "user_id", "booker_name", "booker_phone",
		"status", "payment_status", "created_at", "updated_at",
	).
		From("public.pickup_orders").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get pickup order query failed: %w", err)
	}

	var o PickupOrder
	if err := r.pool.QueryRow(ctx, query, args...).Scan(
		&o.ID, &o.PickupGroupID, &o.UserID, &o.BookerName, &o.BookerPhone,
		&o.Status, &o.PaymentStatus, &o.CreatedAt, &o.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get pickup order failed: %w", err)
	}
	return &o, nil
}

func (r *pgxRepository) GetOrdersByGroupID(ctx context.Context, groupID string) ([]*PickupOrder, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"id", "pickup_group_id", "user_id", "booker_name", "booker_phone",
		"status", "payment_status", "created_at", "updated_at",
	).
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
		if err := rows.Scan(
			&o.ID, &o.PickupGroupID, &o.UserID, &o.BookerName, &o.BookerPhone,
			&o.Status, &o.PaymentStatus, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan pickup order failed: %w", err)
		}
		orders = append(orders, &o)
	}
	return orders, nil
}

func (r *pgxRepository) GetOrdersByUserID(ctx context.Context, userID string) ([]*PickupOrder, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"id", "pickup_group_id", "user_id", "booker_name", "booker_phone",
		"status", "payment_status", "created_at", "updated_at",
	).
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
		if err := rows.Scan(
			&o.ID, &o.PickupGroupID, &o.UserID, &o.BookerName, &o.BookerPhone,
			&o.Status, &o.PaymentStatus, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan pickup order failed: %w", err)
		}
		orders = append(orders, &o)
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
	if err := tx.QueryRow(ctx,
		"SELECT capacity FROM public.pickup_groups WHERE id = $1 FOR UPDATE",
		o.PickupGroupID,
	).Scan(&capacity); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGroupNotFound
		}
		return fmt.Errorf("lock pickup group failed: %w", err)
	}

	// Count occupying orders other than this one; this order is about to become
	// occupying, so it must fit within the remaining capacity.
	var currentEnrolled int
	if err := tx.QueryRow(ctx,
		"SELECT COUNT(*) FROM public.pickup_orders WHERE pickup_group_id = $1 AND id <> $2 AND status NOT IN ('cancelled', 'cancel_request')",
		o.PickupGroupID, o.ID,
	).Scan(&currentEnrolled); err != nil {
		return fmt.Errorf("count enrollments failed: %w", err)
	}

	if currentEnrolled >= capacity {
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

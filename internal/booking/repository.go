package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, booking *Booking) error
	GetByID(ctx context.Context, id string) (*Booking, error)
	List(ctx context.Context, filter Filter) ([]*Booking, int, error)
	Update(ctx context.Context, booking *Booking) error
	Delete(ctx context.Context, id string) error

	// HasOverlap checks if there is any conflicting booking for the resource in the given time range.
	// excludeBookingID is used during updates to ignore the booking itself.
	HasOverlap(ctx context.Context, resourceID string, start, end time.Time, excludeBookingID string) (bool, error)
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, b *Booking) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.bookings").
		Columns("resource_id", "user_id", "start_time", "end_time", "status").
		Values(b.ResourceID, b.UserID, b.StartTime, b.EndTime, b.Status).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create booking query failed: %w", err)
	}

	return r.pool.QueryRow(ctx, query, args...).
		Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Booking, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"b.id", "b.resource_id", "r.name", "b.user_id", "u.display_name",
		"l.id", "l.name", "o.id", "o.name",
		"b.start_time", "b.end_time", "b.status", "b.created_at", "b.updated_at",
	).
		From("public.bookings b").
		Join("public.resources r ON b.resource_id = r.id").
		Join("public.users u ON b.user_id = u.id").
		Join("public.locations l ON r.location_id = l.id").
		Join("public.organizations o ON l.organization_id = o.id").
		Where(squirrel.Eq{"b.id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get booking query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var b Booking
	if err := row.Scan(
		&b.ID, &b.ResourceID, &b.ResourceName, &b.UserID, &b.UserName,
		&b.LocationID, &b.LocationName, &b.OrganizationID, &b.OrganizationName,
		&b.StartTime, &b.EndTime, &b.Status, &b.CreatedAt, &b.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get booking failed: %w", err)
	}
	return &b, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*Booking, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query := psql.Select(
		"b.id", "b.resource_id", "r.name", "b.user_id", "u.display_name",
		"l.id", "l.name", "o.id", "o.name",
		"b.start_time", "b.end_time", "b.status", "b.created_at", "b.updated_at",
		"count(*) OVER() as total_count",
	).
		From("public.bookings b").
		Join("public.resources r ON b.resource_id = r.id").
		Join("public.users u ON b.user_id = u.id").
		Join("public.locations l ON r.location_id = l.id").
		Join("public.organizations o ON l.organization_id = o.id")

	if filter.UserID != "" {
		query = query.Where(squirrel.Eq{"b.user_id": filter.UserID})
	}
	if filter.ResourceID != "" {
		query = query.Where(squirrel.Eq{"b.resource_id": filter.ResourceID})
	}
	if filter.OrganizationID != "" {
		query = query.Where(squirrel.Eq{"o.id": filter.OrganizationID})
	}
	if filter.Status != "" {
		query = query.Where(squirrel.Eq{"b.status": filter.Status})
	}
	// Date range filtering (intersection logic)
	if filter.StartTime != nil {
		query = query.Where(squirrel.GtOrEq{"b.end_time": filter.StartTime})
	}
	if filter.EndTime != nil {
		query = query.Where(squirrel.LtOrEq{"b.start_time": filter.EndTime})
	}

	// Sorting
	orderBy := "b.start_time"
	if filter.SortBy != "" {
		orderBy = "b." + filter.SortBy
	}

	orderDir := "DESC"
	if filter.SortOrder != "" {
		orderDir = filter.SortOrder
	}

	query = query.OrderBy(orderBy + " " + orderDir)

	// Pagination
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
		return nil, 0, fmt.Errorf("build list bookings query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list bookings failed: %w", err)
	}
	defer rows.Close()

	var bookings []*Booking
	var total int

	for rows.Next() {
		var b Booking
		if err := rows.Scan(
			&b.ID, &b.ResourceID, &b.ResourceName, &b.UserID, &b.UserName,
			&b.LocationID, &b.LocationName, &b.OrganizationID, &b.OrganizationName,
			&b.StartTime, &b.EndTime, &b.Status, &b.CreatedAt, &b.UpdatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan booking failed: %w", err)
		}
		bookings = append(bookings, &b)
	}

	return bookings, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, b *Booking) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.bookings").
		Set("start_time", b.StartTime).
		Set("end_time", b.EndTime).
		Set("status", b.Status).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": b.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update booking query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update booking failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.bookings").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete booking query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete booking failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) HasOverlap(ctx context.Context, resourceID string, start, end time.Time, excludeBookingID string) (bool, error) {
	// Logic:
	// 1. Resource matches
	// 2. Status is NOT cancelled
	// 3. Time overlaps: (NewStart < ExistingEnd) AND (NewEnd > ExistingStart)
	// 4. Exclude specific ID (for updates)

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	subQuery := psql.Select("1").
		From("public.bookings").
		Where(squirrel.Eq{"resource_id": resourceID}).
		Where(squirrel.NotEq{"status": "cancelled"}).
		Where(squirrel.Lt{"start_time": end}).
		Where(squirrel.Gt{"end_time": start})

	if excludeBookingID != "" {
		subQuery = subQuery.Where(squirrel.NotEq{"id": excludeBookingID})
	}

	sql, args, err := subQuery.ToSql()
	if err != nil {
		return false, fmt.Errorf("build check overlap query failed: %w", err)
	}

	query := "SELECT EXISTS (" + sql + ")"

	var exists bool
	err = r.pool.QueryRow(ctx, query, args...).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check overlap failed: %w", err)
	}
	return exists, nil
}

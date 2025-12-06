package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	const query = `
		INSERT INTO public.bookings (resource_id, user_id, start_time, end_time, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	return r.pool.QueryRow(ctx, query, b.ResourceID, b.UserID, b.StartTime, b.EndTime, b.Status).
		Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Booking, error) {
	const query = `
		SELECT
			b.id, b.resource_id, r.name, b.user_id, u.display_name,
			b.start_time, b.end_time, b.status, b.created_at, b.updated_at
		FROM public.bookings b
		JOIN public.resources r ON b.resource_id = r.id
		JOIN public.users u ON b.user_id = u.id
		WHERE b.id = $1
	`
	row := r.pool.QueryRow(ctx, query, id)

	var b Booking
	if err := row.Scan(
		&b.ID, &b.ResourceID, &b.ResourceName, &b.UserID, &b.UserName,
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
	var args []any
	queryBase := `
		SELECT
			b.id, b.resource_id, r.name, b.user_id, u.display_name,
			b.start_time, b.end_time, b.status, b.created_at, b.updated_at,
			count(*) OVER() as total_count
		FROM public.bookings b
		JOIN public.resources r ON b.resource_id = r.id
		JOIN public.users u ON b.user_id = u.id
		WHERE 1=1
	`
	paramIndex := 1

	if filter.UserID != "" {
		queryBase += fmt.Sprintf(" AND b.user_id = $%d", paramIndex)
		args = append(args, filter.UserID)
		paramIndex++
	}
	if filter.ResourceID != "" {
		queryBase += fmt.Sprintf(" AND b.resource_id = $%d", paramIndex)
		args = append(args, filter.ResourceID)
		paramIndex++
	}
	if filter.Status != "" {
		queryBase += fmt.Sprintf(" AND b.status = $%d", paramIndex)
		args = append(args, filter.Status)
		paramIndex++
	}
	// Date range filtering (intersection logic)
	if filter.StartTime != nil {
		queryBase += fmt.Sprintf(" AND b.end_time >= $%d", paramIndex)
		args = append(args, *filter.StartTime)
		paramIndex++
	}
	if filter.EndTime != nil {
		queryBase += fmt.Sprintf(" AND b.start_time <= $%d", paramIndex)
		args = append(args, *filter.EndTime)
		paramIndex++
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

	queryBase += " ORDER BY " + orderBy + " " + orderDir

	// Pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize

	queryBase += fmt.Sprintf(" LIMIT $%d OFFSET $%d", paramIndex, paramIndex+1)
	args = append(args, filter.PageSize, offset)

	rows, err := r.pool.Query(ctx, queryBase, args...)
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
			&b.StartTime, &b.EndTime, &b.Status, &b.CreatedAt, &b.UpdatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan booking failed: %w", err)
		}
		bookings = append(bookings, &b)
	}

	return bookings, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, b *Booking) error {
	const query = `
		UPDATE public.bookings
		SET start_time = $1, end_time = $2, status = $3, updated_at = now()
		WHERE id = $4
	`
	ct, err := r.pool.Exec(ctx, query, b.StartTime, b.EndTime, b.Status, b.ID)
	if err != nil {
		return fmt.Errorf("update booking failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM public.bookings WHERE id = $1`
	ct, err := r.pool.Exec(ctx, query, id)
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

	query := `
		SELECT EXISTS (
			SELECT 1 FROM public.bookings
			WHERE resource_id = $1
			  AND status != 'cancelled'
			  AND start_time < $3
			  AND end_time > $2
	`
	args := []any{resourceID, start, end}
	paramIndex := 4

	if excludeBookingID != "" {
		query += fmt.Sprintf(" AND id != $%d", paramIndex)
		args = append(args, excludeBookingID)
	}

	query += ")"

	var exists bool
	err := r.pool.QueryRow(ctx, query, args...).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check overlap failed: %w", err)
	}
	return exists, nil
}

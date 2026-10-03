package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/db"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
)

type Repository interface {
	Create(ctx context.Context, booking *Booking) error
	GetByID(ctx context.Context, id string) (*Booking, error)
	List(ctx context.Context, filter Filter) ([]*Booking, int, error)
	// ListOccupied returns the start/end/status of the resource's non-cancelled bookings that
	// overlap [from, to], without the joined display fields or pagination.
	ListOccupied(ctx context.Context, resourceID string, from, to time.Time) ([]*Booking, error)
	Update(ctx context.Context, booking *Booking) error
	Delete(ctx context.Context, id string) error
	// CountUpcomingActiveByUser counts the user's not-yet-ended, non-cancelled bookings.
	CountUpcomingActiveByUser(ctx context.Context, userID string) (int, error)
	// CancelUpcomingByUser cancels the user's bookings that have not started yet.
	CancelUpcomingByUser(ctx context.Context, userID string) error

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

	if err := r.pool.QueryRow(ctx, query, args...).
		Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return mapOverlapError(err)
	}
	return nil
}

// mapOverlapError translates the database-level overlap exclusion violation
// (raised by the bookings_no_overlap constraint) into ErrTimeConflict. This is
// the final guard against double-booking when concurrent requests both pass the
// application-level HasOverlap pre-check. Other errors are returned unchanged.
func mapOverlapError(err error) error {
	if db.IsViolation(err, pgerrcode.ExclusionViolation, "") {
		return ErrTimeConflict
	}
	return err
}

// selectBookings starts a booking query joined with its resource, user, location and organization.
// extra columns are appended after the columns scanBookingInto expects.
func selectBookings(extra ...string) squirrel.SelectBuilder {
	cols := append([]string{
		"b.id", "b.resource_id", "r.name", "r.sport_id", "b.user_id", "COALESCE(u.display_name, u.username)",
		"l.id", "l.name", "o.id", "o.name",
		"b.start_time", "b.end_time", "b.status", "b.payment_status", "b.created_at", "b.updated_at",
	}, extra...)
	return squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select(cols...).
		From("public.bookings b").
		Join("public.resources r ON b.resource_id = r.id").
		Join("public.users u ON b.user_id = u.id").
		Join("public.locations l ON r.location_id = l.id").
		Join("public.organizations o ON l.organization_id = o.id")
}

// scanBookingInto returns the scan destinations matching selectBookings, followed by extra.
func scanBookingInto(b *Booking, extra ...any) []any {
	return append([]any{
		&b.ID, &b.ResourceID, &b.ResourceName, &b.SportID, &b.UserID, &b.UserName,
		&b.LocationID, &b.LocationName, &b.OrganizationID, &b.OrganizationName,
		&b.StartTime, &b.EndTime, &b.Status, &b.PaymentStatus, &b.CreatedAt, &b.UpdatedAt,
	}, extra...)
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Booking, error) {
	query, args, err := selectBookings().
		Where(squirrel.Eq{"b.id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get booking query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var b Booking
	if err := row.Scan(scanBookingInto(&b)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get booking failed: %w", err)
	}
	return &b, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*Booking, int, error) {
	query := selectBookings("count(*) OVER() as total_count")

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

	query = query.OrderBy(orderBy+" "+orderDir, "b.id ASC")

	return pagination.Collect(ctx, r.pool, query, filter.Page, filter.PageSize, "booking", func(rows pgx.Rows, total *int) (*Booking, error) {
		var b Booking
		if err := rows.Scan(scanBookingInto(&b, total)...); err != nil {
			return nil, fmt.Errorf("scan booking failed: %w", err)
		}
		return &b, nil
	})
}

func (r *pgxRepository) ListOccupied(ctx context.Context, resourceID string, from, to time.Time) ([]*Booking, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT start_time, end_time, status FROM public.bookings
		 WHERE resource_id = $1 AND status <> $2 AND end_time >= $3 AND start_time <= $4
		 ORDER BY start_time`,
		resourceID, StatusCancelled, from, to)
	if err != nil {
		return nil, fmt.Errorf("list occupied bookings failed: %w", err)
	}
	defer rows.Close()

	var bookings []*Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.StartTime, &b.EndTime, &b.Status); err != nil {
			return nil, fmt.Errorf("scan occupied booking failed: %w", err)
		}
		bookings = append(bookings, &b)
	}
	return bookings, rows.Err()
}

func (r *pgxRepository) Update(ctx context.Context, b *Booking) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.bookings").
		Set("start_time", b.StartTime).
		Set("end_time", b.EndTime).
		Set("status", b.Status).
		Set("payment_status", b.PaymentStatus).
		Set("updated_at", squirrel.Expr("now()")).
		// Optimistic lock: the row must be unchanged since it was read, so a
		// concurrent edit is reported instead of silently overwritten.
		Where(squirrel.Eq{"id": b.ID, "updated_at": b.UpdatedAt}).
		Suffix("RETURNING updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build update booking query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&b.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.bookings WHERE id = $1)", b.ID).Scan(&exists); err != nil {
				return fmt.Errorf("check booking exists failed: %w", err)
			}
			if exists {
				return ErrConcurrentUpdate
			}
			return ErrNotFound
		}
		return mapOverlapError(fmt.Errorf("update booking failed: %w", err))
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

func (r *pgxRepository) CancelUpcomingByUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE public.bookings SET status = 'cancelled', updated_at = now()
		 WHERE user_id = $1 AND start_time > now() AND status <> 'cancelled'`,
		userID)
	if err != nil {
		return fmt.Errorf("cancel upcoming bookings failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) CountUpcomingActiveByUser(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		"SELECT count(*) FROM public.bookings WHERE user_id = $1 AND end_time > now() AND status <> 'cancelled'",
		userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active bookings failed: %w", err)
	}
	return n, nil
}

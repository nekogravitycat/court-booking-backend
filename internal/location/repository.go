package location

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines data access methods for locations.
type Repository interface {
	Create(ctx context.Context, loc *Location) error
	GetByID(ctx context.Context, id string) (*Location, error)
	List(ctx context.Context, filter LocationFilter) ([]*Location, int, error)
	Update(ctx context.Context, loc *Location) error
	Delete(ctx context.Context, id string) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, loc *Location) error {
	const query = `
		INSERT INTO public.locations (
			organization_id, name, capacity, opening_hours_start, opening_hours_end,
			location_info, opening, rule, facility, description, longitude, latitude
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at
	`

	// Note: Postgres handles casting string "HH:MM:SS" to TIME automatically in most cases.
	err := r.pool.QueryRow(
		ctx, query,
		loc.OrganizationID, loc.Name, loc.Capacity, loc.OpeningHoursStart, loc.OpeningHoursEnd,
		loc.LocationInfo, loc.Opening, loc.Rule, loc.Facility, loc.Description, loc.Longitude, loc.Latitude,
	).Scan(&loc.ID, &loc.CreatedAt)

	if err != nil {
		return fmt.Errorf("create location failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Location, error) {
	const query = `
		SELECT
			id, organization_id, name, created_at, capacity,
			opening_hours_start::text, opening_hours_end::text,
			location_info, opening, rule, facility, description, longitude, latitude
		FROM public.locations
		WHERE id = $1
	`
	// We cast TIME to ::text to scan into string easily.

	row := r.pool.QueryRow(ctx, query, id)

	var l Location
	err := row.Scan(
		&l.ID, &l.OrganizationID, &l.Name, &l.CreatedAt, &l.Capacity,
		&l.OpeningHoursStart, &l.OpeningHoursEnd,
		&l.LocationInfo, &l.Opening, &l.Rule, &l.Facility, &l.Description, &l.Longitude, &l.Latitude,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get location failed: %w", err)
	}
	return &l, nil
}

func (r *pgxRepository) List(ctx context.Context, filter LocationFilter) ([]*Location, int, error) {
	var args []interface{}
	queryBase := `
		SELECT
			id, organization_id, name, created_at, capacity,
			opening_hours_start::text, opening_hours_end::text,
			location_info, opening, rule, facility, description, longitude, latitude,
			count(*) OVER() as total_count
		FROM public.locations
		WHERE 1=1
	`

	// Dynamic Filtering
	paramIndex := 1
	if filter.OrganizationID != "" {
		queryBase += fmt.Sprintf(" AND organization_id = $%d", paramIndex)
		args = append(args, filter.OrganizationID)
		paramIndex++
	}
	if filter.Keyword != "" {
		queryBase += fmt.Sprintf(" AND (name ILIKE $%d OR location_info ILIKE $%d)", paramIndex, paramIndex)
		args = append(args, "%"+filter.Keyword+"%")
		paramIndex++
	}

	queryBase += " ORDER BY created_at DESC"

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
		return nil, 0, fmt.Errorf("list locations failed: %w", err)
	}
	defer rows.Close()

	var locations []*Location
	var total int

	for rows.Next() {
		var l Location
		if err := rows.Scan(
			&l.ID, &l.OrganizationID, &l.Name, &l.CreatedAt, &l.Capacity,
			&l.OpeningHoursStart, &l.OpeningHoursEnd,
			&l.LocationInfo, &l.Opening, &l.Rule, &l.Facility, &l.Description, &l.Longitude, &l.Latitude,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan location failed: %w", err)
		}
		locations = append(locations, &l)
	}

	return locations, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, loc *Location) error {
	const query = `
		UPDATE public.locations
		SET name=$1, capacity=$2, opening_hours_start=$3, opening_hours_end=$4,
			location_info=$5, opening=$6, rule=$7, facility=$8, description=$9,
			longitude=$10, latitude=$11
		WHERE id = $12
	`
	ct, err := r.pool.Exec(
		ctx, query,
		loc.Name, loc.Capacity, loc.OpeningHoursStart, loc.OpeningHoursEnd,
		loc.LocationInfo, loc.Opening, loc.Rule, loc.Facility, loc.Description,
		loc.Longitude, loc.Latitude, loc.ID,
	)
	if err != nil {
		return fmt.Errorf("update location failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM public.locations WHERE id = $1`
	ct, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete location failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

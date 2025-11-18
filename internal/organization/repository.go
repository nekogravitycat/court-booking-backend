package organization

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("organization not found")

// Repository defines methods for accessing organization data.
type Repository interface {
	Create(ctx context.Context, org *Organization) error
	GetByID(ctx context.Context, id int64) (*Organization, error)
	List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error)
	Update(ctx context.Context, org *Organization) error
	Delete(ctx context.Context, id int64) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

// NewPgxRepository creates a new organization repository.
func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, org *Organization) error {
	const query = `
		INSERT INTO public.organizations (name, is_active)
		VALUES ($1, $2)
		RETURNING id, created_at
	`
	// Default is_active to true if not handled by caller,
	// though DB default is also true.
	return r.pool.QueryRow(ctx, query, org.Name, org.IsActive).
		Scan(&org.ID, &org.CreatedAt)
}

func (r *pgxRepository) GetByID(ctx context.Context, id int64) (*Organization, error) {
	const query = `
		SELECT id, name, created_at, is_active
		FROM public.organizations
		WHERE id = $1 AND is_active = true
	`
	row := r.pool.QueryRow(ctx, query, id)

	var org Organization
	if err := row.Scan(&org.ID, &org.Name, &org.CreatedAt, &org.IsActive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetByID failed: %w", err)
	}
	return &org, nil
}

func (r *pgxRepository) List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error) {
	// Base query with window function for total count
	const queryBase = `
		SELECT id, name, created_at, is_active, count(*) OVER() AS total_count
		FROM public.organizations
		WHERE is_active = true
		ORDER BY id DESC
	`
	// Note: For now we only list active organizations.
	// If admins need to see inactive ones, we can add a filter field later.

	// Pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize

	query := queryBase + " LIMIT $1 OFFSET $2"

	rows, err := r.pool.Query(ctx, query, filter.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("List failed: %w", err)
	}
	defer rows.Close()

	var orgs []*Organization
	var total int

	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.CreatedAt, &o.IsActive, &total); err != nil {
			return nil, 0, fmt.Errorf("scan failed: %w", err)
		}
		orgs = append(orgs, &o)
	}

	return orgs, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, org *Organization) error {
	const query = `
		UPDATE public.organizations
		SET name = $1, is_active = $2
		WHERE id = $3
	`
	ct, err := r.pool.Exec(ctx, query, org.Name, org.IsActive, org.ID)
	if err != nil {
		return fmt.Errorf("Update failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id int64) error {
	// Soft delete implementation
	const query = `
		UPDATE public.organizations
		SET is_active = false
		WHERE id = $1
	`
	ct, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("Delete (soft) failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

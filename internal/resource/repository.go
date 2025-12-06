package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, res *Resource) error
	GetByID(ctx context.Context, id string) (*Resource, error)
	List(ctx context.Context, filter Filter) ([]*Resource, int, error)
	Update(ctx context.Context, res *Resource) error
	Delete(ctx context.Context, id string) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, res *Resource) error {
	const query = `
		INSERT INTO public.resources (resource_type_id, location_id, name)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`
	err := r.pool.QueryRow(ctx, query, res.ResourceTypeID, res.LocationID, res.Name).
		Scan(&res.ID, &res.CreatedAt)
	if err != nil {
		return fmt.Errorf("create resource failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Resource, error) {
	const query = `
		SELECT
			r.id, r.resource_type_id, rt.name, r.location_id, l.name, r.name, r.created_at
		FROM public.resources r
		JOIN public.resource_types rt ON r.resource_type_id = rt.id
		JOIN public.locations l ON r.location_id = l.id
		WHERE r.id = $1
	`
	row := r.pool.QueryRow(ctx, query, id)

	var res Resource
	if err := row.Scan(&res.ID, &res.ResourceTypeID, &res.ResourceTypeName, &res.LocationID, &res.LocationName, &res.Name, &res.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get resource failed: %w", err)
	}
	return &res, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*Resource, int, error) {
	var args []any
	queryBase := `
		SELECT
			r.id, r.resource_type_id, rt.name, r.location_id, l.name, r.name, r.created_at,
			count(*) OVER() as total_count
		FROM public.resources r
		JOIN public.resource_types rt ON r.resource_type_id = rt.id
		JOIN public.locations l ON r.location_id = l.id
		WHERE 1=1
	`
	paramIndex := 1

	if filter.LocationID != "" {
		queryBase += fmt.Sprintf(" AND r.location_id = $%d", paramIndex)
		args = append(args, filter.LocationID)
		paramIndex++
	}
	if filter.ResourceTypeID != "" {
		queryBase += fmt.Sprintf(" AND r.resource_type_id = $%d", paramIndex)
		args = append(args, filter.ResourceTypeID)
		paramIndex++
	}

	// Sorting
	orderBy := "r.created_at"
	if filter.SortBy != "" {
		orderBy = "r." + filter.SortBy
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
		return nil, 0, fmt.Errorf("list resources failed: %w", err)
	}
	defer rows.Close()

	var result []*Resource
	var total int

	for rows.Next() {
		var res Resource
		if err := rows.Scan(
			&res.ID, &res.ResourceTypeID, &res.ResourceTypeName, &res.LocationID, &res.LocationName,
			&res.Name, &res.CreatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan resource failed: %w", err)
		}
		result = append(result, &res)
	}

	return result, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, res *Resource) error {
	const query = `
		UPDATE public.resources
		SET name = $1
		WHERE id = $2
	`
	ct, err := r.pool.Exec(ctx, query, res.Name, res.ID)
	if err != nil {
		return fmt.Errorf("update resource failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM public.resources WHERE id = $1`
	ct, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete resource failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

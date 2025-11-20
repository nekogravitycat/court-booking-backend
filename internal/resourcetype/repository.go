package resourcetype

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, rt *ResourceType) error
	GetByID(ctx context.Context, id string) (*ResourceType, error)
	List(ctx context.Context, filter Filter) ([]*ResourceType, int, error)
	Update(ctx context.Context, rt *ResourceType) error
	Delete(ctx context.Context, id string) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, rt *ResourceType) error {
	const query = `
		INSERT INTO public.resource_types (organization_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`
	err := r.pool.QueryRow(ctx, query, rt.OrganizationID, rt.Name, rt.Description).
		Scan(&rt.ID, &rt.CreatedAt)
	if err != nil {
		return fmt.Errorf("create resource type failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*ResourceType, error) {
	const query = `
		SELECT id, organization_id, name, description, created_at
		FROM public.resource_types
		WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, query, id)

	var rt ResourceType
	if err := row.Scan(&rt.ID, &rt.OrganizationID, &rt.Name, &rt.Description, &rt.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get resource type failed: %w", err)
	}
	return &rt, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*ResourceType, int, error) {
	var args []interface{}
	queryBase := `
		SELECT id, organization_id, name, description, created_at, count(*) OVER() as total_count
		FROM public.resource_types
		WHERE 1=1
	`

	paramIndex := 1
	if filter.OrganizationID != "" {
		queryBase += fmt.Sprintf(" AND organization_id = $%d", paramIndex)
		args = append(args, filter.OrganizationID)
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
		return nil, 0, fmt.Errorf("list resource types failed: %w", err)
	}
	defer rows.Close()

	var result []*ResourceType
	var total int

	for rows.Next() {
		var rt ResourceType
		if err := rows.Scan(
			&rt.ID, &rt.OrganizationID, &rt.Name, &rt.Description, &rt.CreatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan resource type failed: %w", err)
		}
		result = append(result, &rt)
	}

	return result, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, rt *ResourceType) error {
	const query = `
		UPDATE public.resource_types
		SET name = $1, description = $2
		WHERE id = $3
	`
	ct, err := r.pool.Exec(ctx, query, rt.Name, rt.Description, rt.ID)
	if err != nil {
		return fmt.Errorf("update resource type failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM public.resource_types WHERE id = $1`
	ct, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete resource type failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

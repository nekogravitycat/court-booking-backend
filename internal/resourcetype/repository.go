package resourcetype

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
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
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.resource_types").
		Columns("organization_id", "name", "description").
		Values(rt.OrganizationID, rt.Name, rt.Description).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create resource type query failed: %w", err)
	}

	err = r.pool.QueryRow(ctx, query, args...).
		Scan(&rt.ID, &rt.CreatedAt)
	if err != nil {
		return fmt.Errorf("create resource type failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*ResourceType, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"rt.id", "rt.organization_id", "o.name", "rt.name", "rt.description", "rt.created_at",
	).
		From("public.resource_types rt").
		Join("public.organizations o ON rt.organization_id = o.id").
		Where(squirrel.Eq{"rt.id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get resource type query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var rt ResourceType
	if err := row.Scan(&rt.ID, &rt.OrganizationID, &rt.OrganizationName, &rt.Name, &rt.Description, &rt.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get resource type failed: %w", err)
	}
	return &rt, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*ResourceType, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select(
		"rt.id", "rt.organization_id", "o.name", "rt.name", "rt.description", "rt.created_at",
		"count(*) OVER() as total_count",
	).
		From("public.resource_types rt").
		Join("public.organizations o ON rt.organization_id = o.id")

	if filter.OrganizationID != "" {
		queryBuilder = queryBuilder.Where(squirrel.Eq{"rt.organization_id": filter.OrganizationID})
	}

	// Sorting
	orderBy := "created_at"
	if filter.SortBy != "" {
		orderBy = filter.SortBy
	}

	orderDir := "DESC"
	if filter.SortOrder != "" {
		orderDir = filter.SortOrder
	}

	queryBuilder = queryBuilder.OrderBy(orderBy + " " + orderDir)

	// Pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize

	queryBuilder = queryBuilder.Limit(uint64(filter.PageSize)).Offset(uint64(offset))

	sql, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build list resource types query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list resource types failed: %w", err)
	}
	defer rows.Close()

	var result []*ResourceType
	var total int

	for rows.Next() {
		var rt ResourceType
		if err := rows.Scan(
			&rt.ID, &rt.OrganizationID, &rt.OrganizationName, &rt.Name, &rt.Description, &rt.CreatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan resource type failed: %w", err)
		}
		result = append(result, &rt)
	}

	return result, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, rt *ResourceType) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.resource_types").
		Set("name", rt.Name).
		Set("description", rt.Description).
		Where(squirrel.Eq{"id": rt.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update resource type query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update resource type failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.resource_types").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete resource type query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete resource type failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

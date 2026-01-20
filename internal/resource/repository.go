package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
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
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.resources").
		Columns("resource_type", "location_id", "name").
		Values(res.ResourceType, res.LocationID, res.Name).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create resource query failed: %w", err)
	}

	err = r.pool.QueryRow(ctx, query, args...).
		Scan(&res.ID, &res.CreatedAt)
	if err != nil {
		return fmt.Errorf("create resource failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Resource, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"r.id", "r.resource_type", "r.location_id", "l.name", "r.name", "r.created_at",
	).
		From("public.resources r").
		Join("public.locations l ON r.location_id = l.id").
		Where(squirrel.Eq{"r.id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get resource query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var res Resource
	if err := row.Scan(&res.ID, &res.ResourceType, &res.LocationID, &res.LocationName, &res.Name, &res.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get resource failed: %w", err)
	}
	return &res, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*Resource, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query := psql.Select(
		"r.id", "r.resource_type", "r.location_id", "l.name", "r.name", "r.created_at",
		"count(*) OVER() as total_count",
	).
		From("public.resources r").
		Join("public.locations l ON r.location_id = l.id")

	if filter.OrganizationID != "" {
		query = query.Where(squirrel.Eq{"l.organization_id": filter.OrganizationID})
	}
	if filter.LocationID != "" {
		query = query.Where(squirrel.Eq{"r.location_id": filter.LocationID})
	}
	if filter.ResourceType != "" {
		query = query.Where(squirrel.Eq{"r.resource_type": filter.ResourceType})
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
		return nil, 0, fmt.Errorf("build list resources query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list resources failed: %w", err)
	}
	defer rows.Close()

	var result []*Resource
	var total int

	for rows.Next() {
		var res Resource
		if err := rows.Scan(
			&res.ID, &res.ResourceType, &res.LocationID, &res.LocationName,
			&res.Name, &res.CreatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan resource failed: %w", err)
		}
		result = append(result, &res)
	}

	return result, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, res *Resource) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.resources").
		Set("name", res.Name).
		Where(squirrel.Eq{"id": res.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update resource query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update resource failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.resources").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete resource query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete resource failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

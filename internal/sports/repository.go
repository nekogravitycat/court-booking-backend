package sports

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, sp *Sport) error
	GetByID(ctx context.Context, id string) (*Sport, error)
	List(ctx context.Context, filter Filter) ([]*Sport, int, error)
	Update(ctx context.Context, sp *Sport) error
	Delete(ctx context.Context, id string) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, sp *Sport) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.sports").
		Columns("code", "name", "is_active").
		Values(sp.Code, sp.Name, sp.IsActive).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create sport query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&sp.ID, &sp.CreatedAt, &sp.UpdatedAt); err != nil {
		var e *pgconn.PgError
		if errors.As(err, &e) && e.Code == pgerrcode.UniqueViolation {
			return ErrCodeAlreadyUsed
		}
		return fmt.Errorf("create sport failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Sport, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("id", "code", "name", "is_active", "created_at", "updated_at").
		From("public.sports").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get sport query failed: %w", err)
	}

	var sp Sport
	if err := r.pool.QueryRow(ctx, query, args...).Scan(
		&sp.ID, &sp.Code, &sp.Name, &sp.IsActive, &sp.CreatedAt, &sp.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get sport failed: %w", err)
	}
	return &sp, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*Sport, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query := psql.Select(
		"id", "code", "name", "is_active", "created_at", "updated_at",
		"count(*) OVER() AS total_count",
	).From("public.sports")

	if filter.ActiveOnly {
		query = query.Where(squirrel.Eq{"is_active": true})
	}

	orderBy := "created_at"
	if filter.SortBy != "" {
		orderBy = filter.SortBy
	}
	orderDir := "DESC"
	if filter.SortOrder != "" {
		orderDir = filter.SortOrder
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
		return nil, 0, fmt.Errorf("build list sports query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list sports failed: %w", err)
	}
	defer rows.Close()

	var result []*Sport
	var total int
	for rows.Next() {
		var sp Sport
		if err := rows.Scan(
			&sp.ID, &sp.Code, &sp.Name, &sp.IsActive, &sp.CreatedAt, &sp.UpdatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan sport failed: %w", err)
		}
		result = append(result, &sp)
	}
	return result, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, sp *Sport) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.sports").
		Set("code", sp.Code).
		Set("name", sp.Name).
		Set("is_active", sp.IsActive).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": sp.ID}).
		Suffix("RETURNING updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build update sport query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&sp.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		var e *pgconn.PgError
		if errors.As(err, &e) && e.Code == pgerrcode.UniqueViolation {
			return ErrCodeAlreadyUsed
		}
		return fmt.Errorf("update sport failed: %w", err)
	}
	return nil
}

// Delete soft-deletes the sport by clearing its is_active flag.
func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.sports").
		Set("is_active", false).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete sport query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete sport failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

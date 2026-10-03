package announcement

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

type Repository interface {
	Create(ctx context.Context, a *Announcement) error
	GetByID(ctx context.Context, id string) (*Announcement, error)
	List(ctx context.Context, filter Filter) ([]*Announcement, int, error)
	Update(ctx context.Context, a *Announcement) error
	Delete(ctx context.Context, id string) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, a *Announcement) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.announcements").
		Columns("title", "content").
		Values(a.Title, a.Content).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create announcement query failed: %w", err)
	}

	return r.pool.QueryRow(ctx, query, args...).
		Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Announcement, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("id", "title", "content", "created_at", "updated_at").
		From("public.announcements").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get announcement query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var a Announcement
	if err := row.Scan(&a.ID, &a.Title, &a.Content, &a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get announcement failed: %w", err)
	}
	return &a, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*Announcement, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query := psql.Select("id", "title", "content", "created_at", "updated_at", "count(*) OVER() as total_count").
		From("public.announcements")

	if filter.Keyword != "" {
		query = query.Where(squirrel.Or{
			squirrel.ILike{"title": "%" + request.EscapeLike(filter.Keyword) + "%"},
			squirrel.ILike{"content": "%" + request.EscapeLike(filter.Keyword) + "%"},
		})
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

	query = query.OrderBy(orderBy+" "+orderDir, "id ASC")

	return pagination.Collect(ctx, r.pool, query, filter.Page, filter.PageSize, "announcement", func(rows pgx.Rows, total *int) (*Announcement, error) {
		var a Announcement
		if err := rows.Scan(
			&a.ID, &a.Title, &a.Content, &a.CreatedAt, &a.UpdatedAt, total,
		); err != nil {
			return nil, fmt.Errorf("scan announcement failed: %w", err)
		}
		return &a, nil
	})
}

func (r *pgxRepository) Update(ctx context.Context, a *Announcement) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.announcements").
		Set("title", a.Title).
		Set("content", a.Content).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": a.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update announcement query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update announcement failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.announcements").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete announcement query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete announcement failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

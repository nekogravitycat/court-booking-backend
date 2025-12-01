package announcement

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	const query = `
		INSERT INTO public.announcements (title, content)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at
	`
	return r.pool.QueryRow(ctx, query, a.Title, a.Content).
		Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Announcement, error) {
	const query = `
		SELECT id, title, content, created_at, updated_at
		FROM public.announcements
		WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, query, id)

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
	var args []any
	queryBase := `
		SELECT id, title, content, created_at, updated_at, count(*) OVER() as total_count
		FROM public.announcements
		WHERE 1=1
	`
	paramIndex := 1

	if filter.Keyword != "" {
		queryBase += fmt.Sprintf(" AND (title ILIKE $%d OR content ILIKE $%d)", paramIndex, paramIndex)
		args = append(args, "%"+filter.Keyword+"%")
		paramIndex++
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
		return nil, 0, fmt.Errorf("list announcements failed: %w", err)
	}
	defer rows.Close()

	var result []*Announcement
	var total int

	for rows.Next() {
		var a Announcement
		if err := rows.Scan(
			&a.ID, &a.Title, &a.Content, &a.CreatedAt, &a.UpdatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan announcement failed: %w", err)
		}
		result = append(result, &a)
	}

	return result, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, a *Announcement) error {
	const query = `
		UPDATE public.announcements
		SET title = $1, content = $2, updated_at = now()
		WHERE id = $3
	`
	ct, err := r.pool.Exec(ctx, query, a.Title, a.Content, a.ID)
	if err != nil {
		return fmt.Errorf("update announcement failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM public.announcements WHERE id = $1`
	ct, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete announcement failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

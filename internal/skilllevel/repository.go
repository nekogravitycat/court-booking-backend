package skilllevel

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
	Create(ctx context.Context, sl *SkillLevel) error
	GetByID(ctx context.Context, id string) (*SkillLevel, error)
	List(ctx context.Context, filter Filter) ([]*SkillLevel, int, error)
	Update(ctx context.Context, sl *SkillLevel) error
	Delete(ctx context.Context, id string) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, sl *SkillLevel) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.skill_levels").
		Columns("sport_id", "name", "sort_order", "is_active").
		Values(sl.SportID, sl.Name, sl.SortOrder, sl.IsActive).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create skill level query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&sl.ID, &sl.CreatedAt, &sl.UpdatedAt); err != nil {
		var e *pgconn.PgError
		if errors.As(err, &e) {
			switch e.Code {
			case pgerrcode.UniqueViolation:
				return ErrNameAlreadyUsed
			case pgerrcode.ForeignKeyViolation:
				return ErrSportNotFound
			}
		}
		return fmt.Errorf("create skill level failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*SkillLevel, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("id", "sport_id", "name", "sort_order", "is_active", "created_at", "updated_at").
		From("public.skill_levels").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get skill level query failed: %w", err)
	}

	var sl SkillLevel
	if err := r.pool.QueryRow(ctx, query, args...).Scan(
		&sl.ID, &sl.SportID, &sl.Name, &sl.SortOrder, &sl.IsActive, &sl.CreatedAt, &sl.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get skill level failed: %w", err)
	}
	return &sl, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*SkillLevel, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query := psql.Select(
		"id", "sport_id", "name", "sort_order", "is_active", "created_at", "updated_at",
		"count(*) OVER() AS total_count",
	).From("public.skill_levels")

	if filter.SportID != "" {
		query = query.Where(squirrel.Eq{"sport_id": filter.SportID})
	}
	if filter.ActiveOnly {
		query = query.Where(squirrel.Eq{"is_active": true})
	}

	orderBy := "sort_order"
	if filter.SortBy != "" {
		orderBy = filter.SortBy
	}
	orderDir := "ASC"
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
		return nil, 0, fmt.Errorf("build list skill levels query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list skill levels failed: %w", err)
	}
	defer rows.Close()

	var result []*SkillLevel
	var total int
	for rows.Next() {
		var sl SkillLevel
		if err := rows.Scan(
			&sl.ID, &sl.SportID, &sl.Name, &sl.SortOrder, &sl.IsActive, &sl.CreatedAt, &sl.UpdatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan skill level failed: %w", err)
		}
		result = append(result, &sl)
	}
	return result, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, sl *SkillLevel) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.skill_levels").
		Set("name", sl.Name).
		Set("sort_order", sl.SortOrder).
		Set("is_active", sl.IsActive).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": sl.ID}).
		Suffix("RETURNING updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build update skill level query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&sl.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		var e *pgconn.PgError
		if errors.As(err, &e) && e.Code == pgerrcode.UniqueViolation {
			return ErrNameAlreadyUsed
		}
		return fmt.Errorf("update skill level failed: %w", err)
	}
	return nil
}

// Delete soft-deletes the skill level by clearing its is_active flag.
func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.skill_levels").
		Set("is_active", false).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete skill level query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete skill level failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

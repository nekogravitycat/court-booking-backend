package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/db"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
)

type Repository interface {
	Create(ctx context.Context, res *Resource) error
	GetByID(ctx context.Context, id string) (*Resource, error)
	List(ctx context.Context, filter Filter) ([]*Resource, int, error)
	Update(ctx context.Context, res *Resource) error
	SetCover(ctx context.Context, id string, cover *string) (*string, error)
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
		Columns("resource_type", "sport_id", "location_id", "name", "price", "cover").
		Values(res.ResourceType, res.SportID, res.LocationID, res.Name, res.Price, res.Cover).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create resource query failed: %w", err)
	}

	err = r.pool.QueryRow(ctx, query, args...).
		Scan(&res.ID, &res.CreatedAt)
	if err != nil {
		if isSportFKViolation(err) {
			return ErrInvalidSport
		}
		return fmt.Errorf("create resource failed: %w", err)
	}
	return nil
}

// isSportFKViolation reports whether err is a foreign-key violation on resources.sport_id.
func isSportFKViolation(err error) bool {
	return db.IsViolation(err, pgerrcode.ForeignKeyViolation, "resources_sport_id_fkey")
}

// selectResources starts a resource query joined with its location.
// extra columns are appended after the columns scanResourceInto expects.
func selectResources(extra ...string) squirrel.SelectBuilder {
	cols := append([]string{
		"r.id", "r.resource_type", "r.sport_id", "r.location_id", "l.name", "r.name", "r.price", "r.cover", "r.created_at",
	}, extra...)
	return squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select(cols...).
		From("public.resources r").
		Join("public.locations l ON r.location_id = l.id")
}

// scanResourceInto returns the scan destinations matching selectResources, followed by extra.
func scanResourceInto(res *Resource, extra ...any) []any {
	return append([]any{
		&res.ID, &res.ResourceType, &res.SportID, &res.LocationID, &res.LocationName,
		&res.Name, &res.Price, &res.Cover, &res.CreatedAt,
	}, extra...)
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Resource, error) {
	query, args, err := selectResources().
		Where(squirrel.Eq{"r.id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get resource query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var res Resource
	if err := row.Scan(scanResourceInto(&res)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get resource failed: %w", err)
	}
	return &res, nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*Resource, int, error) {
	query := selectResources("count(*) OVER() as total_count")

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

	query = query.OrderBy(orderBy+" "+orderDir, "r.id ASC")

	return pagination.Collect(ctx, r.pool, query, filter.Page, filter.PageSize, "resource", func(rows pgx.Rows, total *int) (*Resource, error) {
		var res Resource
		if err := rows.Scan(scanResourceInto(&res, total)...); err != nil {
			return nil, fmt.Errorf("scan resource failed: %w", err)
		}
		return &res, nil
	})
}

func (r *pgxRepository) Update(ctx context.Context, res *Resource) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.resources").
		Set("name", res.Name).
		Set("price", res.Price).
		Set("sport_id", res.SportID).
		Set("cover", res.Cover).
		Where(squirrel.Eq{"id": res.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update resource query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		if isSportFKViolation(err) {
			return ErrInvalidSport
		}
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
		// A booking referencing this resource (ON DELETE RESTRICT) surfaces as a
		// foreign-key violation; report it as a 409 conflict instead of a 500.
		if db.IsInUse(err) {
			return ErrResourceInUse
		}
		return fmt.Errorf("delete resource failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetCover replaces only the cover reference (so it cannot overwrite concurrent
// edits to other columns) and returns the previous cover.
func (r *pgxRepository) SetCover(ctx context.Context, id string, cover *string) (*string, error) {
	var old *string
	err := r.pool.QueryRow(ctx,
		`UPDATE public.resources t SET cover = $2
		 FROM (SELECT cover AS old_cover FROM public.resources WHERE id = $1 FOR UPDATE) o
		 WHERE t.id = $1
		 RETURNING o.old_cover`,
		id, cover).Scan(&old)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("set resource cover failed: %w", err)
	}
	return old, nil
}

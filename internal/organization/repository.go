package organization

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// Repository defines methods for accessing organization data.
type Repository interface {
	// Organization methods
	Create(ctx context.Context, org *Organization) error
	GetByID(ctx context.Context, id string) (*Organization, error)
	List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error)
	Update(ctx context.Context, org *Organization) error
	Delete(ctx context.Context, id string) error
	// Organization Manager methods
	AddOrganizationManager(ctx context.Context, orgID string, userID string) error
	RemoveOrganizationManager(ctx context.Context, orgID string, userID string) error
	IsOrganizationManager(ctx context.Context, orgID string, userID string) (bool, error)
	ListOrganizationManagers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error)
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

// NewPgxRepository creates a new organization repository.
func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

// ------------------------
//   Organization methods
// ------------------------

func (r *pgxRepository) Create(ctx context.Context, org *Organization) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.organizations").
		Columns("name", "owner_id", "is_active").
		Values(org.Name, org.OwnerID, org.IsActive).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create organization query failed: %w", err)
	}

	// Default is_active to true if not handled by caller,
	// though DB default is also true.
	return r.pool.QueryRow(ctx, query, args...).
		Scan(&org.ID, &org.CreatedAt)
}

func (r *pgxRepository) GetByID(ctx context.Context, id string) (*Organization, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("id", "name", "owner_id", "created_at", "is_active").
		From("public.organizations").
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"is_active": true}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get organization query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var org Organization
	if err := row.Scan(&org.ID, &org.Name, &org.OwnerID, &org.CreatedAt, &org.IsActive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrgNotFound
		}
		return nil, fmt.Errorf("GetByID failed: %w", err)
	}
	return &org, nil
}

func (r *pgxRepository) List(ctx context.Context, filter OrganizationFilter) ([]*Organization, int, error) {
	// Base query with window function for total count
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select("id", "name", "owner_id", "created_at", "is_active", "count(*) OVER() AS total_count").
		From("public.organizations").
		Where(squirrel.Eq{"is_active": true})

	orderBy := "id"
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
		return nil, 0, fmt.Errorf("build list organizations query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("List failed: %w", err)
	}
	defer rows.Close()

	var orgs []*Organization
	var total int

	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.OwnerID, &o.CreatedAt, &o.IsActive, &total); err != nil {
			return nil, 0, fmt.Errorf("scan failed: %w", err)
		}
		orgs = append(orgs, &o)
	}

	return orgs, total, nil
}

func (r *pgxRepository) Update(ctx context.Context, org *Organization) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.organizations").
		Set("name", org.Name).
		Set("is_active", org.IsActive).
		Where(squirrel.Eq{"id": org.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update organization query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("Update failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrOrgNotFound
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, id string) error {
	// Soft delete implementation
	// Soft delete implementation
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.organizations").
		Set("is_active", false).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete (soft) organization query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("Delete (soft) failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrOrgNotFound
	}
	return nil
}

// -----------------------------
//   Organization Manager methods
// -----------------------------

func (r *pgxRepository) AddOrganizationManager(ctx context.Context, orgID string, userID string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.organization_managers").
		Columns("organization_id", "user_id").
		Values(orgID, userID).
		ToSql()
	if err != nil {
		return fmt.Errorf("build add org manager query failed: %w", err)
	}

	_, err = r.pool.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return ErrUserAlreadyMember
			}
		}
		return fmt.Errorf("AddOrganizationManager failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) RemoveOrganizationManager(ctx context.Context, orgID string, userID string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Delete("public.organization_managers").
		Where(squirrel.Eq{"organization_id": orgID}).
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build remove org manager query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("RemoveOrganizationManager failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrUserNotMember
	}
	return nil
}

func (r *pgxRepository) IsOrganizationManager(ctx context.Context, orgID string, userID string) (bool, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("1").
		From("public.organization_managers").
		Where(squirrel.Eq{"organization_id": orgID}).
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build check org manager query failed: %w", err)
	}

	var one int
	err = r.pool.QueryRow(ctx, query, args...).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("IsOrganizationManager failed: %w", err)
	}
	return true, nil
}

func (r *pgxRepository) ListOrganizationManagers(ctx context.Context, orgID string, filter ManagerFilter) ([]*user.User, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select(
		"u.id", "u.email", "u.display_name", "u.created_at", "u.is_active", "count(*) OVER() AS total_count",
	).
		From("public.organization_managers om").
		Join("public.users u ON om.user_id = u.id").
		Where(squirrel.Eq{"om.organization_id": orgID})

	orderBy := "u.display_name"
	if filter.SortBy != "" {
		switch filter.SortBy {
		case "created_at":
			orderBy = "u.created_at"
		case "name":
			orderBy = "u.display_name"
		default:
			orderBy = "u." + filter.SortBy
		}
	}

	// Correcting SortBy logic. The user might send 'name' or 'email'.
	// The previous implementation used u.display_name ASC.

	switch filter.SortBy {
	case "name":
		orderBy = "u.display_name"
	case "email":
		orderBy = "u.email"
	case "created_at":
		orderBy = "u.created_at"
	default:
		orderBy = "u.display_name"
	}

	orderDir := "ASC"
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

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build list org managers query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListOrganizationManagers failed: %w", err)
	}
	defer rows.Close()

	var users []*user.User
	var total int

	for rows.Next() {
		var u user.User
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt, &u.IsActive, &total); err != nil {
			return nil, 0, fmt.Errorf("scan org manager failed: %w", err)
		}
		users = append(users, &u)
	}
	return users, total, nil
}

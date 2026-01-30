package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines methods for accessing user data from storage.
type Repository interface {
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
	Create(ctx context.Context, u *User) error
	UpdateLastLogin(ctx context.Context, id string, t time.Time) error
	List(ctx context.Context, filter UserFilter) ([]*User, int, error)
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id string) error
}

type pgxUserRepository struct {
	pool *pgxpool.Pool
}

// NewPgxRepository creates a new Repository implementation using pgxpool.
func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxUserRepository{
		pool: pool,
	}
}

func (r *pgxUserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"u.id", "u.email", "u.password_hash", "u.display_name", "u.avatar", "u.created_at",
		"u.last_login_at", "u.is_active", "u.is_system_admin",
		`COALESCE(
				(
					SELECT json_agg(json_build_object(
						'id', o.id,
						'name', o.name,
						'owner', (o.owner_id = u.id),
						'organization_manager', EXISTS(SELECT 1 FROM public.organization_managers om WHERE om.organization_id = o.id AND om.user_id = u.id),
						'location_manager', COALESCE((SELECT json_agg(lm.location_id) FROM public.location_managers lm WHERE lm.organization_id = o.id AND lm.user_id = u.id), '[]'::json)
					))
					FROM public.organizations o
					WHERE (o.owner_id = u.id OR o.id IN (
						SELECT organization_id FROM public.organization_members WHERE user_id = u.id
					)) AND o.is_active = true
				),
				'[]'::json
			) AS organizations`,
	).
		From("public.users u").
		Where(squirrel.Eq{"u.email": email}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get user by email query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var u User
	var orgsJSON []byte

	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.DisplayName,
		&u.Avatar,
		&u.CreatedAt,
		&u.LastLoginAt,
		&u.IsActive,
		&u.IsSystemAdmin,
		&orgsJSON, // Scan JSON for organizations
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetByEmail query failed: %w", err)
	}

	// Try parse the organizations JSON into the slice
	if len(orgsJSON) > 0 {
		if err := json.Unmarshal(orgsJSON, &u.Organizations); err != nil {
			log.Printf("warning: failed to unmarshal organizations for user %s: %v", u.ID, err)
		}
	}

	return &u, nil
}

func (r *pgxUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select(
		"u.id", "u.email", "u.password_hash", "u.display_name", "u.avatar", "u.created_at",
		"u.last_login_at", "u.is_active", "u.is_system_admin",
		`COALESCE(
				(
					SELECT json_agg(json_build_object(
						'id', o.id,
						'name', o.name,
						'owner', (o.owner_id = u.id),
						'organization_manager', EXISTS(SELECT 1 FROM public.organization_managers om WHERE om.organization_id = o.id AND om.user_id = u.id),
						'location_manager', COALESCE((SELECT json_agg(lm.location_id) FROM public.location_managers lm WHERE lm.organization_id = o.id AND lm.user_id = u.id), '[]'::json)
					))
					FROM public.organizations o
					WHERE (o.owner_id = u.id OR o.id IN (
						SELECT organization_id FROM public.organization_members WHERE user_id = u.id
					)) AND o.is_active = true
				),
				'[]'::json
			) AS organizations`,
	).
		From("public.users u").
		Where(squirrel.Eq{"u.id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get user by id query failed: %w", err)
	}

	row := r.pool.QueryRow(ctx, query, args...)

	var u User
	var orgsJSON []byte

	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.DisplayName,
		&u.Avatar,
		&u.CreatedAt,
		&u.LastLoginAt,
		&u.IsActive,
		&u.IsSystemAdmin,
		&orgsJSON, // Scan JSON for organizations
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetByID query failed: %w", err)
	}

	// Try parse the organizations JSON into the slice
	if len(orgsJSON) > 0 {
		if err := json.Unmarshal(orgsJSON, &u.Organizations); err != nil {
			log.Printf("warning: failed to unmarshal organizations for user %s: %v", u.ID, err)
		}
	}

	return &u, nil
}

func (r *pgxUserRepository) Create(ctx context.Context, u *User) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Insert("public.users").
		Columns("email", "password_hash", "display_name", "is_active", "is_system_admin").
		Values(u.Email, u.PasswordHash, u.DisplayName, u.IsActive, u.IsSystemAdmin).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build create user query failed: %w", err)
	}

	if err := r.pool.QueryRow(ctx, query, args...).Scan(&u.ID, &u.CreatedAt); err != nil {
		var e *pgconn.PgError
		if errors.As(err, &e) && e.Code == pgerrcode.UniqueViolation {
			return ErrEmailAlreadyUsed
		}
		return fmt.Errorf("Create user failed: %w", err)
	}

	return nil
}

func (r *pgxUserRepository) UpdateLastLogin(ctx context.Context, id string, t time.Time) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.users").
		Set("last_login_at", t).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update last login query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("UpdateLastLogin failed: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *pgxUserRepository) List(ctx context.Context, filter UserFilter) ([]*User, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select(
		"u.id", "u.email", "u.password_hash", "u.display_name", "u.avatar", "u.created_at",
		"u.last_login_at", "u.is_active", "u.is_system_admin",
		"count(*) OVER() AS total_count",
		`COALESCE(
				(
					SELECT json_agg(json_build_object(
						'id', o.id,
						'name', o.name,
						'owner', (o.owner_id = u.id),
						'organization_manager', EXISTS(SELECT 1 FROM public.organization_managers om WHERE om.organization_id = o.id AND om.user_id = u.id),
						'location_manager', COALESCE((SELECT json_agg(lm.location_id) FROM public.location_managers lm WHERE lm.organization_id = o.id AND lm.user_id = u.id), '[]'::json)
					))
					FROM public.organizations o
					WHERE (o.owner_id = u.id OR o.id IN (
						SELECT organization_id FROM public.organization_members WHERE user_id = u.id
					)) AND o.is_active = true
				),
				'[]'::json
			) AS organizations`,
	).From("public.users u")

	// Dynamic filtering
	if len(filter.IDs) > 0 {
		queryBuilder = queryBuilder.Where(squirrel.Eq{"u.id": filter.IDs})
	}
	if filter.Email != "" {
		queryBuilder = queryBuilder.Where(squirrel.ILike{"email": "%" + filter.Email + "%"})
	}
	if filter.DisplayName != "" {
		queryBuilder = queryBuilder.Where(squirrel.ILike{"display_name": "%" + filter.DisplayName + "%"})
	}
	if filter.IsActive != nil {
		queryBuilder = queryBuilder.Where(squirrel.Eq{"is_active": *filter.IsActive})
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
		return nil, 0, fmt.Errorf("build list users query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users failed: %w", err)
	}
	defer rows.Close()

	var users []*User
	var total int

	for rows.Next() {
		var u User
		var orgsJSON []byte

		if err := rows.Scan(
			&u.ID,
			&u.Email,
			&u.PasswordHash,
			&u.DisplayName,
			&u.Avatar,
			&u.CreatedAt,
			&u.LastLoginAt,
			&u.IsActive,
			&u.IsSystemAdmin,
			&total,    // Scan the window function result
			&orgsJSON, // Scan the JSON result for organizations
		); err != nil {
			return nil, 0, fmt.Errorf("scan user failed: %w", err)
		}

		// Parse the organizations JSON into the slice
		// pgx can actually scan directly into structs if setup correctly,
		// but using json.Unmarshal is safer and simpler for this specific case without extra config.
		if len(orgsJSON) > 0 {
			if err := json.Unmarshal(orgsJSON, &u.Organizations); err != nil {
				// Log the error but continue; we don't want one bad record to fail the whole list.
				log.Printf("warning: failed to unmarshal organizations for user %s: %v", u.ID, err)
			}
		}

		users = append(users, &u)
	}

	return users, total, nil
}

func (r *pgxUserRepository) Update(ctx context.Context, u *User) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.users").
		Set("display_name", u.DisplayName).
		Set("avatar", u.Avatar).
		Set("is_active", u.IsActive).
		Set("is_system_admin", u.IsSystemAdmin).
		Where(squirrel.Eq{"id": u.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update user query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update user failed: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *pgxUserRepository) Delete(ctx context.Context, id string) error {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Update("public.users").
		Set("is_active", false).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete user query failed: %w", err)
	}

	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete user failed: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

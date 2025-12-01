package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

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
	const query = `
		SELECT
			u.id,
			u.email,
			u.password_hash,
			u.display_name,
			u.created_at,
			u.last_login_at,
			u.is_active,
			u.is_system_admin,
			COALESCE(
				(
					SELECT json_agg(json_build_object('id', o.id, 'name', o.name))
					FROM public.organization_permissions op
					JOIN public.organizations o ON op.organization_id = o.id
					WHERE op.user_id = u.id AND o.is_active = true
				),
				'[]'::json
			) AS organizations
		FROM public.users u
		WHERE u.email = $1
	`

	row := r.pool.QueryRow(ctx, query, email)

	var u User
	var orgsJSON []byte

	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.DisplayName,
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
	const query = `
		SELECT
			u.id,
			u.email,
			u.password_hash,
			u.display_name,
			u.created_at,
			u.last_login_at,
			u.is_active,
			u.is_system_admin,
			COALESCE(
				(
					SELECT json_agg(json_build_object('id', o.id, 'name', o.name))
					FROM public.organization_permissions op
					JOIN public.organizations o ON op.organization_id = o.id
					WHERE op.user_id = u.id AND o.is_active = true
				),
				'[]'::json
			) AS organizations
		FROM public.users u
		WHERE u.id = $1
	`

	row := r.pool.QueryRow(ctx, query, id)

	var u User
	var orgsJSON []byte

	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.DisplayName,
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
	const query = `
		INSERT INTO public.users (email, password_hash, display_name, is_active, is_system_admin)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`

	if err := r.pool.QueryRow(
		ctx,
		query,
		u.Email,
		u.PasswordHash,
		u.DisplayName,
		u.IsActive,
		u.IsSystemAdmin,
	).Scan(&u.ID, &u.CreatedAt); err != nil {
		var e *pgconn.PgError
		if errors.As(err, &e) && e.Code == pgerrcode.UniqueViolation {
			return ErrEmailAlreadyUsed
		}
		return fmt.Errorf("Create user failed: %w", err)
	}

	return nil
}

func (r *pgxUserRepository) UpdateLastLogin(ctx context.Context, id string, t time.Time) error {
	const query = `
		UPDATE public.users
		SET last_login_at = $1
		WHERE id = $2
	`

	ct, err := r.pool.Exec(ctx, query, t, id)
	if err != nil {
		return fmt.Errorf("UpdateLastLogin failed: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *pgxUserRepository) List(ctx context.Context, filter UserFilter) ([]*User, int, error) {
	var args []any
	// We use a Correlated Subquery to fetch organizations as a JSON array.
	queryBuilder := bytes.NewBufferString(`
		SELECT
			u.id,
			u.email,
			u.password_hash,
			u.display_name,
			u.created_at,
			u.last_login_at,
			u.is_active,
			u.is_system_admin,
			count(*) OVER() AS total_count,
			COALESCE(
				(
					SELECT json_agg(json_build_object('id', o.id, 'name', o.name))
					FROM public.organization_permissions op
					JOIN public.organizations o ON op.organization_id = o.id
					WHERE op.user_id = u.id AND o.is_active = true
				),
				'[]'::json
			) AS organizations
		FROM public.users u
		WHERE 1=1
	`)

	// Dynamic filtering
	if filter.Email != "" {
		args = append(args, "%"+filter.Email+"%")
		queryBuilder.WriteString(" AND email ILIKE $" + strconv.Itoa(len(args)))
	}
	if filter.DisplayName != "" {
		args = append(args, "%"+filter.DisplayName+"%")
		queryBuilder.WriteString(" AND display_name ILIKE $" + strconv.Itoa(len(args)))
	}
	if filter.IsActive != nil {
		args = append(args, *filter.IsActive)
		queryBuilder.WriteString(" AND is_active = $" + strconv.Itoa(len(args)))
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

	queryBuilder.WriteString(" ORDER BY " + orderBy + " " + orderDir)

	// Pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize

	args = append(args, filter.PageSize, offset)
	queryBuilder.WriteString(" LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args)))

	rows, err := r.pool.Query(ctx, queryBuilder.String(), args...)
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
	const query = `
		UPDATE public.users
		SET display_name = $1, is_active = $2, is_system_admin = $3
		WHERE id = $4
	`

	ct, err := r.pool.Exec(ctx, query, u.DisplayName, u.IsActive, u.IsSystemAdmin, u.ID)
	if err != nil {
		return fmt.Errorf("update user failed: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *pgxUserRepository) Delete(ctx context.Context, id string) error {
	const query = `
		UPDATE public.users
		SET is_active = false
		WHERE id = $1
	`

	ct, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete user failed: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

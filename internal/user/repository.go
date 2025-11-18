package user

import (
	"context"
	"errors"
	"fmt"
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
}

// ErrNotFound is returned when a user is not found in the repository.
var ErrNotFound = errors.New("user not found")

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
		SELECT id, email, password_hash, display_name, created_at, last_login_at, is_active
		FROM public.users
		WHERE email = $1
	`

	row := r.pool.QueryRow(ctx, query, email)

	var u User
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.DisplayName,
		&u.CreatedAt,
		&u.LastLoginAt,
		&u.IsActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetByEmail query failed: %w", err)
	}

	return &u, nil
}

func (r *pgxUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	const query = `
		SELECT id, email, password_hash, display_name, created_at, last_login_at, is_active
		FROM public.users
		WHERE id = $1
	`

	row := r.pool.QueryRow(ctx, query, id)

	var u User
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.DisplayName,
		&u.CreatedAt,
		&u.LastLoginAt,
		&u.IsActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetByID query failed: %w", err)
	}

	return &u, nil
}

func (r *pgxUserRepository) Create(ctx context.Context, u *User) error {
	const query = `
		INSERT INTO public.users (email, password_hash, display_name, is_active)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`

	err := r.pool.QueryRow(
		ctx,
		query,
		u.Email,
		u.PasswordHash,
		u.DisplayName,
		u.IsActive,
	).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
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

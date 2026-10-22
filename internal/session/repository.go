package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// expiredRetention is how long an expired or revoked session row is kept before
// being purged.
const expiredRetention = 7 * 24 * time.Hour

// Repository persists sessions. Tokens are only ever handled as hashes.
type Repository interface {
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (*Session, error)
	// GetValidByTokenHash returns the session unless it is revoked or expired.
	GetValidByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	Revoke(ctx context.Context, id string) error
	// IsActive reports whether the session belongs to userID and is neither revoked nor expired.
	IsActive(ctx context.Context, id, userID string) (bool, error)
	// PurgeStale deletes sessions that expired or were revoked long ago.
	PurgeStale(ctx context.Context) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

// NewPgxRepository creates a Repository backed by pgxpool.
func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (*Session, error) {
	s := &Session{UserID: userID, ExpiresAt: expiresAt}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO public.sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		userID, tokenHash, expiresAt).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create session failed: %w", err)
	}
	return s, nil
}

func (r *pgxRepository) GetValidByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	var s Session
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, created_at, expires_at FROM public.sessions
		 WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
		tokenHash).Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("get session failed: %w", err)
	}
	return &s, nil
}

func (r *pgxRepository) Revoke(ctx context.Context, id string) error {
	if _, err := r.pool.Exec(ctx,
		"UPDATE public.sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL", id); err != nil {
		return fmt.Errorf("revoke session failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) IsActive(ctx context.Context, id, userID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM public.sessions
			WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL AND expires_at > now()
		)`, id, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check session failed: %w", err)
	}
	return ok, nil
}

func (r *pgxRepository) PurgeStale(ctx context.Context) error {
	cutoff := time.Now().Add(-expiredRetention)
	if _, err := r.pool.Exec(ctx,
		"DELETE FROM public.sessions WHERE expires_at < $1 OR revoked_at < $1", cutoff); err != nil {
		return fmt.Errorf("purge sessions failed: %w", err)
	}
	return nil
}

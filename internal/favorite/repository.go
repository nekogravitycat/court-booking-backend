package favorite

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines persistence for favorite hosts.
type Repository interface {
	AddFavorite(ctx context.Context, userID, hostID string) error
	RemoveFavorite(ctx context.Context, userID, hostID string) error
	ListFavorites(ctx context.Context, userID string) ([]*FavoriteHost, error)
	// DeleteFavoritesByHostID removes every favorite that points to the given
	// host. Used to keep favorites consistent when a host account is deleted.
	// It also satisfies user.HostFavoriteCleaner.
	DeleteFavoritesByHostID(ctx context.Context, hostID string) error
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) AddFavorite(ctx context.Context, userID, hostID string) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO public.favorite_hosts (user_id, host_id) VALUES ($1, $2)",
		userID, hostID,
	)
	if err != nil {
		var e *pgconn.PgError
		if errors.As(err, &e) && e.Code == pgerrcode.UniqueViolation {
			return ErrAlreadyFavorited
		}
		return fmt.Errorf("add favorite failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) RemoveFavorite(ctx context.Context, userID, hostID string) error {
	ct, err := r.pool.Exec(ctx,
		"DELETE FROM public.favorite_hosts WHERE user_id = $1 AND host_id = $2",
		userID, hostID,
	)
	if err != nil {
		return fmt.Errorf("remove favorite failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrFavoriteNotFound
	}
	return nil
}

func (r *pgxRepository) ListFavorites(ctx context.Context, userID string) ([]*FavoriteHost, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query, args, err := psql.Select("fh.host_id", "u.display_name", "u.avatar").
		From("public.favorite_hosts fh").
		Join("public.users u ON u.id = fh.host_id").
		Where(squirrel.Eq{"fh.user_id": userID}).
		OrderBy("fh.created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list favorites query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list favorites failed: %w", err)
	}
	defer rows.Close()

	var favorites []*FavoriteHost
	for rows.Next() {
		var f FavoriteHost
		if err := rows.Scan(&f.HostID, &f.Nickname, &f.Avatar); err != nil {
			return nil, fmt.Errorf("scan favorite failed: %w", err)
		}
		favorites = append(favorites, &f)
	}
	return favorites, nil
}

func (r *pgxRepository) DeleteFavoritesByHostID(ctx context.Context, hostID string) error {
	_, err := r.pool.Exec(ctx,
		"DELETE FROM public.favorite_hosts WHERE host_id = $1",
		hostID,
	)
	if err != nil {
		return fmt.Errorf("delete favorites by host failed: %w", err)
	}
	return nil
}

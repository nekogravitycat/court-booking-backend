package notification

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, n *Notification) error
	CreateMany(ctx context.Context, ns []*Notification) error
	List(ctx context.Context, filter Filter) ([]*Notification, int, error)
	CountUnread(ctx context.Context, userID string) (int, error)
	// MarkRead marks one notification of the user as read. It returns
	// ErrNotFound when the notification does not exist or is not the user's.
	MarkRead(ctx context.Context, userID, id string) error
	// MarkAllRead marks every unread notification of the user as read and returns
	// how many were changed.
	MarkAllRead(ctx context.Context, userID string) (int64, error)
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

const insertSQL = `INSERT INTO public.notifications (user_id, type, title, content, pickup_group_id, pickup_order_id)
	VALUES ($1, $2, $3, $4, $5, $6)`

func (r *pgxRepository) Create(ctx context.Context, n *Notification) error {
	err := r.pool.QueryRow(ctx, insertSQL+" RETURNING id, is_read, created_at",
		n.UserID, n.Type, n.Title, n.Content, n.PickupGroupID, n.PickupOrderID,
	).Scan(&n.ID, &n.IsRead, &n.CreatedAt)
	if err != nil {
		return fmt.Errorf("create notification failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) CreateMany(ctx context.Context, ns []*Notification) error {
	if len(ns) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, n := range ns {
		batch.Queue(insertSQL, n.UserID, n.Type, n.Title, n.Content, n.PickupGroupID, n.PickupOrderID)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range ns {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("create notifications failed: %w", err)
		}
	}
	return nil
}

func (r *pgxRepository) List(ctx context.Context, filter Filter) ([]*Notification, int, error) {
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	query := psql.Select(
		"id", "user_id", "type", "title", "content", "pickup_group_id", "pickup_order_id", "is_read", "created_at",
		"count(*) OVER() AS total_count",
	).
		From("public.notifications").
		Where(squirrel.Eq{"user_id": filter.UserID}).
		OrderBy("created_at DESC", "id DESC")

	if filter.UnreadOnly {
		query = query.Where(squirrel.Eq{"is_read": false})
	}

	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	query = query.Limit(uint64(filter.PageSize)).Offset(uint64((filter.Page - 1) * filter.PageSize))

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build list notifications query failed: %w", err)
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications failed: %w", err)
	}
	defer rows.Close()

	var result []*Notification
	var total int
	for rows.Next() {
		var n Notification
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.Type, &n.Title, &n.Content, &n.PickupGroupID, &n.PickupOrderID, &n.IsRead, &n.CreatedAt,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan notification failed: %w", err)
		}
		result = append(result, &n)
	}
	return result, total, rows.Err()
}

func (r *pgxRepository) CountUnread(ctx context.Context, userID string) (int, error) {
	var count int
	if err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM public.notifications WHERE user_id = $1 AND is_read = false", userID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count unread notifications failed: %w", err)
	}
	return count, nil
}

func (r *pgxRepository) MarkRead(ctx context.Context, userID, id string) error {
	// Matching on both id and owner keeps one user from touching another's inbox.
	// Re-marking an already-read row still affects one row, so it is idempotent.
	ct, err := r.pool.Exec(ctx,
		"UPDATE public.notifications SET is_read = true WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return fmt.Errorf("mark notification read failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgxRepository) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	ct, err := r.pool.Exec(ctx,
		"UPDATE public.notifications SET is_read = true WHERE user_id = $1 AND is_read = false", userID)
	if err != nil {
		return 0, fmt.Errorf("mark all notifications read failed: %w", err)
	}
	return ct.RowsAffected(), nil
}

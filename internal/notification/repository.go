package notification

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/pagination"
)

type Repository interface {
	CreateManual(ctx context.Context, senderID string, userIDs []string, title, content string) error
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

	return pagination.Collect(ctx, r.pool, query, filter.Page, filter.PageSize, "notification", func(rows pgx.Rows, total *int) (*Notification, error) {
		var n Notification
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.Type, &n.Title, &n.Content, &n.PickupGroupID, &n.PickupOrderID, &n.IsRead, &n.CreatedAt,
			total,
		); err != nil {
			return nil, fmt.Errorf("scan notification failed: %w", err)
		}
		return &n, nil
	})
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

// CreateManual validates and locks recipients, then delivers the entire batch atomically.
func (r *pgxRepository) CreateManual(ctx context.Context, senderID string, userIDs []string, title, content string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Serialize per-admin sends so concurrent requests cannot bypass the limit.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 9010))", senderID); err != nil {
		return err
	}
	var allowed bool
	if err := tx.QueryRow(ctx, "SELECT is_active AND is_system_admin FROM public.users WHERE id = $1 FOR SHARE", senderID).Scan(&allowed); err != nil {
		if err == pgx.ErrNoRows {
			return ErrManualSendForbidden
		}
		return err
	}
	if !allowed {
		return ErrManualSendForbidden
	}
	var recent int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM public.manual_notification_sends WHERE sender_id = $1 AND created_at > clock_timestamp() - interval '1 minute'", senderID).Scan(&recent); err != nil {
		return err
	}
	if recent >= 10 {
		return ErrSendRateLimited
	}
	rows, err := tx.Query(ctx, "SELECT id FROM public.users WHERE id = ANY($1::uuid[]) AND is_active = true ORDER BY id FOR SHARE", userIDs)
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if count != len(userIDs) {
		return ErrInvalidRecipients
	}
	if _, err := tx.Exec(ctx, "INSERT INTO public.notifications (user_id, type, title, content) SELECT unnest($1::uuid[]), $2, $3, $4", userIDs, TypeAdminMessage, title, content); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO public.manual_notification_sends (sender_id, recipient_count) VALUES ($1, $2)", senderID, count); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

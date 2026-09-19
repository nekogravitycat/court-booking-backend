package skillrating

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	// Upsert creates the rating, or overwrites the level when the participant was
	// already rated for this group. It fills ID and the timestamps.
	Upsert(ctx context.Context, r *Rating) error
	Delete(ctx context.Context, groupID, userID string) error
	ListByGroup(ctx context.Context, groupID string) ([]*Rating, error)
	// SummarizeByUser returns the per-sport average (rounding and labels are left
	// to the caller) of every rating the user received.
	SummarizeByUser(ctx context.Context, userID string) ([]*SportSummary, error)
}

type pgxRepository struct {
	pool *pgxpool.Pool
}

func NewPgxRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

func (r *pgxRepository) Upsert(ctx context.Context, rt *Rating) error {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO public.skill_ratings (pickup_group_id, user_id, sport_id, level, rated_by)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (pickup_group_id, user_id)
		 DO UPDATE SET level = EXCLUDED.level, rated_by = EXCLUDED.rated_by, updated_at = now()
		 RETURNING id, created_at, updated_at`,
		rt.PickupGroupID, rt.UserID, rt.SportID, rt.Level, rt.RatedBy,
	).Scan(&rt.ID, &rt.CreatedAt, &rt.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert skill rating failed: %w", err)
	}
	return nil
}

func (r *pgxRepository) Delete(ctx context.Context, groupID, userID string) error {
	ct, err := r.pool.Exec(ctx,
		"DELETE FROM public.skill_ratings WHERE pickup_group_id = $1 AND user_id = $2", groupID, userID)
	if err != nil {
		return fmt.Errorf("delete skill rating failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrRatingNotFound
	}
	return nil
}

func (r *pgxRepository) ListByGroup(ctx context.Context, groupID string) ([]*Rating, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, pickup_group_id, user_id, sport_id, level, rated_by, created_at, updated_at
		   FROM public.skill_ratings WHERE pickup_group_id = $1 ORDER BY created_at, id`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list skill ratings failed: %w", err)
	}
	defer rows.Close()

	var result []*Rating
	for rows.Next() {
		var rt Rating
		if err := rows.Scan(&rt.ID, &rt.PickupGroupID, &rt.UserID, &rt.SportID, &rt.Level, &rt.RatedBy, &rt.CreatedAt, &rt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan skill rating failed: %w", err)
		}
		result = append(result, &rt)
	}
	return result, rows.Err()
}

func (r *pgxRepository) SummarizeByUser(ctx context.Context, userID string) ([]*SportSummary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.id, s.code, s.name, AVG(sr.level)::float8, COUNT(*)
		   FROM public.skill_ratings sr
		   JOIN public.sports s ON s.id = sr.sport_id
		  WHERE sr.user_id = $1
		  GROUP BY s.id
		  ORDER BY s.name, s.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("summarize skill ratings failed: %w", err)
	}
	defer rows.Close()

	var result []*SportSummary
	for rows.Next() {
		var s SportSummary
		if err := rows.Scan(&s.SportID, &s.SportCode, &s.SportName, &s.Average, &s.RatingCount); err != nil {
			return nil, fmt.Errorf("scan skill rating summary failed: %w", err)
		}
		result = append(result, &s)
	}
	return result, rows.Err()
}

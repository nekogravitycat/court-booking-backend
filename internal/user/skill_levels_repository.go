package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/nekogravitycat/court-booking-backend/internal/db"
)

const skillLevelColumns = `us.sport_id, sp.name, us.skill_level, sl.label,
    (sp.is_active AND sl.is_active), us.updated_at`
const skillLevelTables = `public.user_skill_levels us
    JOIN public.sports sp ON sp.id = us.sport_id
    JOIN public.skill_levels sl ON sl.sport_id = us.sport_id AND sl.level = us.skill_level`

func (r *pgxUserRepository) GetSkillLevel(ctx context.Context, userID, sportID string) (*SportSkillLevel, error) {
	var level SportSkillLevel
	err := r.pool.QueryRow(ctx, "SELECT "+skillLevelColumns+" FROM "+skillLevelTables+
		" WHERE us.user_id = $1 AND us.sport_id = $2", userID, sportID).
		Scan(&level.SportID, &level.SportName, &level.SkillLevel, &level.Label, &level.IsActive, &level.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSkillLevelNotSet
	}
	if err != nil {
		return nil, fmt.Errorf("get user skill level: %w", err)
	}
	return &level, nil
}

func (r *pgxUserRepository) ListSkillLevels(ctx context.Context, userID string) ([]*SportSkillLevel, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+skillLevelColumns+" FROM "+skillLevelTables+
		" WHERE us.user_id = $1 ORDER BY sp.name, us.sport_id", userID)
	if err != nil {
		return nil, fmt.Errorf("list user skill levels: %w", err)
	}
	defer rows.Close()
	levels := make([]*SportSkillLevel, 0)
	for rows.Next() {
		var level SportSkillLevel
		if err := rows.Scan(&level.SportID, &level.SportName, &level.SkillLevel, &level.Label, &level.IsActive, &level.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user skill level: %w", err)
		}
		levels = append(levels, &level)
	}
	return levels, rows.Err()
}

func (r *pgxUserRepository) SetSkillLevel(ctx context.Context, userID, sportID string, level int) (*SportSkillLevel, error) {
	_, err := r.pool.Exec(ctx, `INSERT INTO public.user_skill_levels (user_id, sport_id, skill_level)
        VALUES ($1, $2, $3) ON CONFLICT (user_id, sport_id)
        DO UPDATE SET skill_level = EXCLUDED.skill_level, updated_at = NOW()`, userID, sportID, level)
	if err != nil {
		if db.IsViolation(err, pgerrcode.ForeignKeyViolation, "") {
			return nil, ErrInvalidSkillLevel
		}
		return nil, fmt.Errorf("set user skill level: %w", err)
	}
	return r.GetSkillLevel(ctx, userID, sportID)
}

// DeleteSkillLevel is idempotent and never changes existing order snapshots.
func (r *pgxUserRepository) DeleteSkillLevel(ctx context.Context, userID, sportID string) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM public.user_skill_levels WHERE user_id = $1 AND sport_id = $2", userID, sportID)
	return err
}

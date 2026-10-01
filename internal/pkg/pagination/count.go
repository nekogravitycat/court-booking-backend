package pagination

import (
	"context"
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Count preserves the complete filter and grouping while removing pagination.
// Lists use this fallback when the window count has no row to travel with.
func Count(ctx context.Context, db Querier, query squirrel.SelectBuilder) (int, error) {
	sql, args, err := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).
		Select("count(*)").FromSelect(query.RemoveLimit().RemoveOffset(), "matching").ToSql()
	if err != nil {
		return 0, fmt.Errorf("build list count failed: %w", err)
	}
	var total int
	if err := db.QueryRow(ctx, sql, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count list failed: %w", err)
	}
	return total, nil
}

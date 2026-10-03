package pagination

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

const defaultPageSize = 20

// Queryer is the subset of pgxpool.Pool used by Collect.
type Queryer interface {
	Querier
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// paginate applies LIMIT/OFFSET for the given page, defaulting to page 1 and 20 rows per page.
func paginate(q squirrel.SelectBuilder, page, pageSize int) squirrel.SelectBuilder {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	return q.Limit(uint64(pageSize)).Offset(uint64((page - 1) * pageSize))
}

// Collect runs a paginated list query (page defaults to 1, pageSize to 20) and returns the scanned
// rows with the total match count. scan must read the window-function total into its second
// argument. An empty first page means no matches; an empty later page recomputes the total with
// Count. what names the listed entity for error messages.
func Collect[T any](
	ctx context.Context,
	db Queryer,
	q squirrel.SelectBuilder,
	page, pageSize int,
	what string,
	scan func(rows pgx.Rows, total *int) (*T, error),
) ([]*T, int, error) {
	if page < 1 {
		page = 1
	}
	q = paginate(q, page, pageSize)

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build list %s query failed: %w", what, err)
	}

	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list %s failed: %w", what, err)
	}
	defer rows.Close()

	var items []*T
	var total int
	for rows.Next() {
		item, err := scan(rows, &total)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()

	if total == 0 && page > 1 {
		total, err = Count(ctx, db, q)
		if err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}

package pickup

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

type failingRows struct {
	pgx.Rows
	emitted bool
	failure error
}

func (r *failingRows) Next() bool {
	if !r.emitted {
		r.emitted = true
		return true
	}
	return false
}
func (r *failingRows) Scan(...any) error { return nil }
func (r *failingRows) Err() error        { return r.failure }
func (r *failingRows) Close()            {}

type failingQuery struct {
	pgx.Tx
	rows pgx.Rows
}

func (q failingQuery) Query(context.Context, string, ...any) (pgx.Rows, error) { return q.rows, nil }

func TestListGroupsRejectsLateRowError(t *testing.T) {
	failure := errors.New("row stream failed")
	repo := &pgxRepository{pool: failingQuery{rows: &failingRows{failure: failure}}}
	groups, total, err := repo.ListGroups(context.Background(), GroupFilter{})
	if !errors.Is(err, failure) || groups != nil || total != 0 {
		t.Fatalf("groups=%v total=%d err=%v", groups, total, err)
	}
}

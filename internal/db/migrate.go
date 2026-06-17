package db

import (
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/nekogravitycat/court-booking-backend/db/migrations"
)

// Migrate applies all pending up migrations embedded in the binary to the
// database at dsn.
//
// It is safe to call on every startup: golang-migrate records applied versions
// in a schema_migrations table and only runs what is missing, and it takes a
// session advisory lock so concurrent instances don't race each other.
func Migrate(dsn string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to load migration sources: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, toMigrateDSN(dsn))
	if err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// toMigrateDSN rewrites a standard postgres DSN to the URL scheme registered by
// golang-migrate's pgx/v5 database driver ("pgx5"). The query parameters
// (sslmode, etc.) are preserved unchanged.
func toMigrateDSN(dsn string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(dsn, prefix); ok {
			return "pgx5://" + rest
		}
	}
	return dsn
}

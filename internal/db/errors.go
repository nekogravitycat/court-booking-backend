package db

import (
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsViolation reports whether err is a PostgreSQL error with the given SQLSTATE code.
// When constraint is non-empty, the violated constraint must match it as well.
func IsViolation(err error, code, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// IsUniqueViolation reports whether err is a unique-constraint violation.
func IsUniqueViolation(err error) bool {
	return IsViolation(err, pgerrcode.UniqueViolation, "")
}

// IsInUse reports whether a delete was blocked by rows still referencing the target
// (a foreign-key or ON DELETE RESTRICT violation).
func IsInUse(err error) bool {
	return IsViolation(err, pgerrcode.ForeignKeyViolation, "") || IsViolation(err, pgerrcode.RestrictViolation, "")
}

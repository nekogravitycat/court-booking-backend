package user

import "time"

// User represents a user in the system.
// It maps (roughly) to the public.users table in PostgreSQL.
type User struct {
	ID           string // UUID
	Email        string
	PasswordHash string
	DisplayName  *string
	CreatedAt    time.Time
	LastLoginAt  *time.Time
	IsActive     bool
}

package user

import "time"

// User represents a user in the system.
type User struct {
	ID            string // UUID
	Email         string
	PasswordHash  string
	DisplayName   *string
	CreatedAt     time.Time
	LastLoginAt   *time.Time
	IsActive      bool
	IsSystemAdmin bool
}

// UserFilter defines filter options for listing users.
type UserFilter struct {
	Email       string
	DisplayName string
	IsActive    *bool // Use pointer to distinguish between false and nil (not set)

	Page     int
	PageSize int
	Sort     string // simple string for now, e.g., "created_at desc"
}

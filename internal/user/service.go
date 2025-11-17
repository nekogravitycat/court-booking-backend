package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/auth"
)

// Service defines business logic related to users.
type Service interface {
	Register(ctx context.Context, email, password, displayName string) (*User, error)
	Login(ctx context.Context, email, password string) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
}

// Service errors used to communicate business logic failures.
var (
	ErrEmailAlreadyUsed   = errors.New("email already used")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInactiveUser       = errors.New("user is inactive")
)

type service struct {
	repo   Repository
	hasher auth.PasswordHasher

	minPasswordLength int
}

// NewService creates a new user Service.
func NewService(repo Repository, hasher auth.PasswordHasher) Service {
	return &service{
		repo:              repo,
		hasher:            hasher,
		minPasswordLength: 8,
	}
}

func (s *service) Register(ctx context.Context, email, password, displayName string) (*User, error) {
	cleanEmail := normalizeEmail(email)
	if cleanEmail == "" {
		return nil, fmt.Errorf("email is required")
	}

	if len(password) < s.minPasswordLength {
		return nil, fmt.Errorf("password must be at least %d characters", s.minPasswordLength)
	}

	// Check if email is already used.
	_, err := s.repo.GetByEmail(ctx, cleanEmail)
	if err == nil {
		// Found an existing user.
		return nil, ErrEmailAlreadyUsed
	}
	// If the error is something other than "not found", propagate it.
	if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("failed to check existing email: %w", err)
	}

	// Hash the password.
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	var displayNamePtr *string
	if strings.TrimSpace(displayName) != "" {
		d := strings.TrimSpace(displayName)
		displayNamePtr = &d
	}

	u := &User{
		Email:        cleanEmail,
		PasswordHash: hash,
		DisplayName:  displayNamePtr,
		IsActive:     true,
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return u, nil
}

func (s *service) Login(ctx context.Context, email, password string) (*User, error) {
	cleanEmail := normalizeEmail(email)
	if cleanEmail == "" || strings.TrimSpace(password) == "" {
		return nil, ErrInvalidCredentials
	}

	u, err := s.repo.GetByEmail(ctx, cleanEmail)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to fetch user by email: %w", err)
	}

	if !u.IsActive {
		return nil, ErrInactiveUser
	}

	// Compare password hash.
	if err := s.hasher.Compare(u.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Update last_login_at (best effort; do not fail login if update fails).
	now := time.Now().UTC()
	if err := s.repo.UpdateLastLogin(ctx, u.ID, now); err != nil {
		// You might want to log this error, but do not expose it to the client.
	}

	return u, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*User, error) {
	return s.repo.GetByID(ctx, id)
}

// normalizeEmail trims spaces and lowercases the email.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

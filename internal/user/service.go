package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/auth"
)

type UpdateUserRequest struct {
	DisplayName   *string
	IsActive      *bool
	IsSystemAdmin *bool
}

// Service defines business logic related to users.
type Service interface {
	Register(ctx context.Context, email, password, displayName string) (*User, error)
	Login(ctx context.Context, email, password string) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)

	List(ctx context.Context, filter UserFilter) ([]*User, int, error)
	Update(ctx context.Context, id string, req UpdateUserRequest) (*User, error)
	Delete(ctx context.Context, id string) error
}

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
		return nil, ErrEmailRequired
	}

	if len(password) < s.minPasswordLength {
		return nil, ErrPasswordTooShort
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
		// Check for unique violation error from the repository.
		if errors.Is(err, ErrEmailAlreadyUsed) {
			return nil, ErrEmailAlreadyUsed
		}
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

func (s *service) GetByEmail(ctx context.Context, email string) (*User, error) {
	cleanEmail := normalizeEmail(email)
	return s.repo.GetByEmail(ctx, cleanEmail)
}

// normalizeEmail trims spaces and lowercases the email.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *service) List(ctx context.Context, filter UserFilter) ([]*User, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) Update(ctx context.Context, id string, req UpdateUserRequest) (*User, error) {
	// 1. Check if user exists
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. Apply updates if provided
	if req.DisplayName != nil {
		u.DisplayName = req.DisplayName
	}
	if req.IsActive != nil {
		u.IsActive = *req.IsActive
	}
	if req.IsSystemAdmin != nil {
		u.IsSystemAdmin = *req.IsSystemAdmin
	}

	// 3. Save changes
	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	// Check if user exists
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	return s.repo.Delete(ctx, id)
}

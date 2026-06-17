package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
)

type UpdateUserRequest struct {
	DisplayName   *string
	Phone         *string
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
	UpdateAvatar(ctx context.Context, id string, fileID string) error
	RemoveAvatar(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error

	// Pickup host role management
	IsPickupHost(ctx context.Context, userID string) (bool, error)
	AddPickupHost(ctx context.Context, userID string) error
	RemovePickupHost(ctx context.Context, userID string) error
	ListPickupHosts(ctx context.Context, filter UserFilter) ([]*User, int, error)
}

// HostFavoriteCleaner removes favorite entries that point to a given host.
// It is implemented by the favorite repository and injected to keep the user
// module decoupled from the favorite module (avoids an import cycle).
type HostFavoriteCleaner interface {
	DeleteFavoritesByHostID(ctx context.Context, hostID string) error
}

type service struct {
	repo            Repository
	hasher          auth.PasswordHasher
	fileService     file.Service
	favoriteCleaner HostFavoriteCleaner

	minPasswordLength int
	maxPasswordLength int

	// dummyPasswordHash is compared against on login attempts for non-existent
	// users, so the response time matches the existing-user path and does not
	// leak account existence via a timing side channel.
	dummyPasswordHash string
}

// NewService creates a new user Service.
// favoriteCleaner may be nil (e.g. in tests that don't exercise favorites);
// account deletion simply skips favorite cleanup in that case.
func NewService(repo Repository, hasher auth.PasswordHasher, fileService file.Service, favoriteCleaner HostFavoriteCleaner) Service {
	// Precompute a dummy hash at the configured cost so login timing for
	// unknown accounts matches the real bcrypt comparison cost.
	dummyHash, _ := hasher.Hash("dummy-password-for-constant-time-login")

	return &service{
		repo:              repo,
		hasher:            hasher,
		fileService:       fileService,
		favoriteCleaner:   favoriteCleaner,
		minPasswordLength: 8,
		// bcrypt only considers the first 72 bytes of a password and silently
		// ignores the rest; reject longer inputs so users are not misled.
		maxPasswordLength: 72,
		dummyPasswordHash: dummyHash,
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
	if len(password) > s.maxPasswordLength {
		return nil, ErrPasswordTooLong
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
			// Perform a dummy comparison so the response time matches the
			// existing-user path, preventing account enumeration via timing.
			if s.dummyPasswordHash != "" {
				_ = s.hasher.Compare(s.dummyPasswordHash, password)
			}
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
	if req.Phone != nil {
		u.Phone = req.Phone
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
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Clean up avatar file if exists
	if u.Avatar != nil && *u.Avatar != "" {
		_ = s.fileService.Delete(ctx, *u.Avatar)
	}

	// Remove this user from everyone else's favorites. Account deletion is a
	// soft delete (is_active=false), so the favorite_hosts FK cascade does not
	// fire; we clean up explicitly to satisfy the favorites requirement.
	if s.favoriteCleaner != nil {
		if err := s.favoriteCleaner.DeleteFavoritesByHostID(ctx, id); err != nil {
			return err
		}
	}

	return s.repo.Delete(ctx, id)
}

// ------------------------
//   Pickup host role
// ------------------------

func (s *service) IsPickupHost(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	return s.repo.IsPickupHost(ctx, userID)
}

func (s *service) AddPickupHost(ctx context.Context, userID string) error {
	// Verify the user exists first to return a clean 404.
	if _, err := s.repo.GetByID(ctx, userID); err != nil {
		return err
	}
	return s.repo.AddPickupHost(ctx, userID)
}

func (s *service) RemovePickupHost(ctx context.Context, userID string) error {
	return s.repo.RemovePickupHost(ctx, userID)
}

func (s *service) ListPickupHosts(ctx context.Context, filter UserFilter) ([]*User, int, error) {
	return s.repo.ListPickupHosts(ctx, filter)
}

func (s *service) UpdateAvatar(ctx context.Context, id string, fileID string) error {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	oldAvatar := u.Avatar

	// Persist the new reference first. Only after the new avatar is durably
	// stored do we delete the old file. Deleting first would leave an orphaned
	// file or a dangling reference if the update failed.
	u.Avatar = &fileID
	if err := s.repo.Update(ctx, u); err != nil {
		return err
	}

	// Best-effort cleanup of the previous avatar file.
	if oldAvatar != nil && *oldAvatar != "" && *oldAvatar != fileID {
		_ = s.fileService.Delete(ctx, *oldAvatar)
	}
	return nil
}

func (s *service) RemoveAvatar(ctx context.Context, id string) error {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	oldAvatar := u.Avatar

	// Clear the reference first, then delete the file. This keeps the database
	// consistent even if the storage delete fails (best effort).
	u.Avatar = nil
	if err := s.repo.Update(ctx, u); err != nil {
		return err
	}

	if oldAvatar != nil && *oldAvatar != "" {
		_ = s.fileService.Delete(ctx, *oldAvatar)
	}
	return nil
}

package user

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	"github.com/nekogravitycat/court-booking-backend/internal/skilllevel"
	"github.com/nekogravitycat/court-booking-backend/internal/sports"
)

// usernamePattern enforces a Twitter-style handle: 4-15 characters of lowercase
// letters, digits, or underscore. Input is lowercased before matching.
var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{4,15}$`)

// RegisterRequest carries the input for creating a new account. Gender and
// BirthDate are optional.
type RegisterRequest struct {
	Email       string
	Username    string
	Password    string
	DisplayName string
	Gender      *string
	BirthDate   *time.Time
	LineID      *string
}

type UpdateUserRequest struct {
	DisplayName   *string
	Phone         *string
	Gender        *string
	BirthDate     *time.Time
	LineID        *string
	IsActive      *bool
	IsSystemAdmin *bool
}

// Service defines business logic related to users.
type Service interface {
	GetSkillLevel(ctx context.Context, userID, sportID string) (*SportSkillLevel, error)
	ListSkillLevels(ctx context.Context, userID string) ([]*SportSkillLevel, error)
	SetSkillLevel(ctx context.Context, userID, sportID string, level int) (*SportSkillLevel, error)
	DeleteSkillLevel(ctx context.Context, userID, sportID string) error
	Register(ctx context.Context, req RegisterRequest) (*User, error)
	Login(ctx context.Context, email, password string) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
	// GetAccount returns only the account flags; prefer it over GetByID for authorization checks.
	GetAccount(ctx context.Context, id string) (*Account, error)
	GetByEmail(ctx context.Context, email string) (*User, error)

	List(ctx context.Context, filter UserFilter) ([]*User, int, error)
	Update(ctx context.Context, id string, req UpdateUserRequest, actingUserID string) (*User, error)
	UpdateAvatar(ctx context.Context, id string, fileID string) error
	RemoveAvatar(ctx context.Context, id string) error
	Delete(ctx context.Context, id string, actingUserID string) error
	// AddDeactivationHook registers a cleanup run after an account is deactivated.
	AddDeactivationHook(h DeactivationHook)

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

// DeactivationHook cleans up a user's future commitments (bookings, pickup
// enrollments, hosted groups) after the account is deactivated. Hooks are
// registered by the owning modules, which keeps the user module decoupled from
// them (avoids import cycles). Implementations must be idempotent: a failed
// deactivation request can be retried.
type DeactivationHook interface {
	OnUserDeactivated(ctx context.Context, userID string) error
}

type service struct {
	deactivationHooks []DeactivationHook
	sportsService     sports.Service
	skillLevelService skilllevel.Service
	repo              Repository
	hasher            auth.PasswordHasher
	fileService       file.Service
	favoriteCleaner   HostFavoriteCleaner

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
func NewService(repo Repository, hasher auth.PasswordHasher, fileService file.Service, favoriteCleaner HostFavoriteCleaner, sportsService sports.Service, skillLevelService skilllevel.Service) Service {
	// Precompute a dummy hash at the configured cost so login timing for
	// unknown accounts matches the real bcrypt comparison cost.
	dummyHash, _ := hasher.Hash("dummy-password-for-constant-time-login")

	return &service{
		sportsService:     sportsService,
		skillLevelService: skillLevelService,
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

func (s *service) Register(ctx context.Context, req RegisterRequest) (*User, error) {
	if req.LineID != nil && *req.LineID == "" {
		req.LineID = nil
	}
	email, username, password, displayName := req.Email, req.Username, req.Password, req.DisplayName
	cleanEmail := normalizeEmail(email)
	if cleanEmail == "" {
		return nil, ErrEmailRequired
	}

	cleanUsername := normalizeUsername(username)
	if cleanUsername == "" {
		return nil, ErrUsernameRequired
	}
	if !usernamePattern.MatchString(cleanUsername) {
		return nil, ErrInvalidUsername
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

	if req.LineID != nil && *req.LineID != "" && !IsValidLineID(*req.LineID) {
		return nil, ErrInvalidLineID
	}
	if req.Gender != nil && !IsValidGender(*req.Gender) {
		return nil, ErrInvalidGender
	}

	d := strings.TrimSpace(displayName)
	if d == "" {
		return nil, ErrDisplayNameRequired
	}
	displayNamePtr := &d

	u := &User{
		Email:        cleanEmail,
		Username:     cleanUsername,
		PasswordHash: hash,
		DisplayName:  displayNamePtr,
		Gender:       req.Gender,
		BirthDate:    req.BirthDate,
		LineID:       req.LineID,
		IsActive:     true,
	}

	if err := s.repo.Create(ctx, u); err != nil {
		// Pass through unique-violation errors mapped by the repository.
		if errors.Is(err, ErrEmailAlreadyUsed) || errors.Is(err, ErrUsernameAlreadyUsed) {
			return nil, err
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
		log.Printf("warning: failed to update last login for user %s: %v", u.ID, err)
	}

	return u, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) GetAccount(ctx context.Context, id string) (*Account, error) {
	return s.repo.GetAccount(ctx, id)
}

func (s *service) GetByEmail(ctx context.Context, email string) (*User, error) {
	cleanEmail := normalizeEmail(email)
	return s.repo.GetByEmail(ctx, cleanEmail)
}

// normalizeEmail trims spaces and lowercases the email.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// normalizeUsername trims spaces and lowercases the username so that a handle
// entered with uppercase letters resolves to its lowercase form.
func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func (s *service) List(ctx context.Context, filter UserFilter) ([]*User, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) AddDeactivationHook(h DeactivationHook) {
	s.deactivationHooks = append(s.deactivationHooks, h)
}

// checkCanDeactivate rejects deactivating oneself or the last active system admin.
func (s *service) checkCanDeactivate(ctx context.Context, target *User, actingUserID string) error {
	if !target.IsActive {
		return nil
	}
	if target.ID == actingUserID {
		return ErrCannotDeactivateSelf
	}
	if target.IsSystemAdmin {
		others, err := s.repo.CountActiveSystemAdminsExcept(ctx, target.ID)
		if err != nil {
			return err
		}
		if others == 0 {
			return ErrLastSystemAdmin
		}
	}
	return nil
}

func (s *service) runDeactivationHooks(ctx context.Context, userID string) error {
	for _, h := range s.deactivationHooks {
		if err := h.OnUserDeactivated(ctx, userID); err != nil {
			return fmt.Errorf("account deactivation cleanup failed: %w", err)
		}
	}
	return nil
}

func (s *service) Update(ctx context.Context, id string, req UpdateUserRequest, actingUserID string) (*User, error) {
	// 1. Check if user exists
	target, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	deactivating := req.IsActive != nil && !*req.IsActive && target.IsActive
	if deactivating {
		if err := s.checkCanDeactivate(ctx, target, actingUserID); err != nil {
			return nil, err
		}
	}

	// A system admin must not revoke their own system admin privilege, so the
	// system can never be left without an admin by accident.
	if req.IsSystemAdmin != nil && !*req.IsSystemAdmin && id == actingUserID {
		return nil, ErrCannotRevokeOwnAdmin
	}

	if req.LineID != nil && *req.LineID != "" && !IsValidLineID(*req.LineID) {
		return nil, ErrInvalidLineID
	}
	if req.Gender != nil && !IsValidGender(*req.Gender) {
		return nil, ErrInvalidGender
	}
	if req.DisplayName != nil {
		d := strings.TrimSpace(*req.DisplayName)
		if d == "" {
			return nil, ErrDisplayNameRequired
		}
		req.DisplayName = &d
	}

	// 3. Save changes
	if err := s.repo.Update(ctx, id, req); err != nil {
		return nil, err
	}
	if deactivating {
		if err := s.runDeactivationHooks(ctx, id); err != nil {
			return nil, err
		}
	}

	return s.repo.GetByID(ctx, id)
}

func (s *service) Delete(ctx context.Context, id string, actingUserID string) error {
	// Check if user exists
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.checkCanDeactivate(ctx, u, actingUserID); err != nil {
		return err
	}

	// Remove this user from everyone else's favorites. Account deletion is a
	// soft delete (is_active=false), so the favorite_hosts FK cascade does not
	// fire; we clean up explicitly to satisfy the favorites requirement.
	if s.favoriteCleaner != nil {
		if err := s.favoriteCleaner.DeleteFavoritesByHostID(ctx, id); err != nil {
			return err
		}
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if err := s.runDeactivationHooks(ctx, id); err != nil {
		return err
	}
	// Clean up avatar file if exists
	if u.Avatar != nil && *u.Avatar != "" {
		file.ReleaseReplaced(ctx, s.fileService, u.Avatar, "")
	}

	return nil
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
	// Persist the new reference first; only delete the old file once the new
	// reference is durably stored, to avoid orphaned files / dangling references.
	oldAvatar, err := s.repo.SetAvatar(ctx, id, &fileID)
	if err != nil {
		return err
	}
	file.ReleaseReplaced(ctx, s.fileService, oldAvatar, fileID)
	return nil
}

func (s *service) RemoveAvatar(ctx context.Context, id string) error {
	// Clear the reference first, then delete the file, keeping the database
	// consistent even if the storage delete fails (best effort).
	oldAvatar, err := s.repo.SetAvatar(ctx, id, nil)
	if err != nil {
		return err
	}
	file.ReleaseReplaced(ctx, s.fileService, oldAvatar, "")
	return nil
}

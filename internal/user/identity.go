package user

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/file"
)

const (
	// maxDisplayNameRunes matches the display_name limit enforced on registration.
	maxDisplayNameRunes = 50
	// maxEmailLength matches the email limit enforced on registration.
	maxEmailLength = 254
	// defaultExternalDisplayName is used when the provider shares no usable name.
	defaultExternalDisplayName = "LINE user"
	// createIdentityAttempts bounds retries on username/email collisions.
	createIdentityAttempts = 5
	// maxAvatarBytes caps an imported profile picture, same as a manual upload.
	maxAvatarBytes = 5 * 1024 * 1024
)

const usernameAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// generateUsername returns "l" plus 14 random characters (15 total, the maximum length).
func generateUsername() (string, error) {
	b := make([]byte, 14)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate username: %w", err)
	}
	for i := range b {
		b[i] = usernameAlphabet[int(b[i])%len(usernameAlphabet)]
	}
	return "l" + string(b), nil
}

func validProvider(p string) bool { return p == ProviderLine }

func cleanExternalDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > maxDisplayNameRunes {
		name = strings.TrimSpace(string(r[:maxDisplayNameRunes]))
	}
	if name == "" {
		return defaultExternalDisplayName
	}
	return name
}

// cleanExternalEmail returns the normalized email, or "" when it is missing or malformed.
func cleanExternalEmail(email string) string {
	email = normalizeEmail(email)
	if email == "" || len(email) > maxEmailLength {
		return ""
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return ""
	}
	return email
}

func (s *service) LoginWithIdentity(ctx context.Context, p IdentityProfile) (*User, bool, error) {
	if !validProvider(p.Provider) {
		return nil, false, ErrUnsupportedProvider
	}
	if p.Subject == "" {
		return nil, false, fmt.Errorf("identity subject is empty")
	}

	u, err := s.repo.GetByIdentity(ctx, p.Provider, p.Subject)
	if err == nil {
		return s.finishIdentityLogin(ctx, u, false)
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, fmt.Errorf("failed to look up identity: %w", err)
	}

	u, err = s.createFromIdentity(ctx, p)
	if err != nil {
		if errors.Is(err, ErrIdentityAlreadyLinked) {
			// A concurrent first login for the same person won the race; use its account.
			u, err = s.repo.GetByIdentity(ctx, p.Provider, p.Subject)
			if err != nil {
				return nil, false, fmt.Errorf("failed to look up identity after conflict: %w", err)
			}
			return s.finishIdentityLogin(ctx, u, false)
		}
		return nil, false, err
	}
	return s.finishIdentityLogin(ctx, u, true)
}

func (s *service) finishIdentityLogin(ctx context.Context, u *User, created bool) (*User, bool, error) {
	if !u.IsActive {
		return nil, false, ErrInactiveUser
	}
	now := time.Now().UTC()
	if err := s.repo.UpdateLastLogin(ctx, u.ID, now); err != nil {
		log.Printf("warning: failed to update last login for user %s: %v", u.ID, err)
	}
	return u, created, nil
}

// createFromIdentity registers a new account for an external identity, filling
// the profile from what the provider shared.
func (s *service) createFromIdentity(ctx context.Context, p IdentityProfile) (*User, error) {
	displayName := cleanExternalDisplayName(p.DisplayName)
	email := cleanExternalEmail(p.Email)

	for attempt := 0; attempt < createIdentityAttempts; attempt++ {
		username, err := generateUsername()
		if err != nil {
			return nil, err
		}
		u := &User{
			Email:       email,
			Username:    username,
			DisplayName: &displayName,
			IsActive:    true,
		}

		err = s.repo.CreateWithIdentity(ctx, u, p.Provider, p.Subject)
		switch {
		case err == nil:
			if p.FetchAvatar != nil {
				s.importAvatar(ctx, u.ID, p.FetchAvatar)
			}
			// Reload so the response carries avatar, linked providers and the other derived fields.
			created, err := s.repo.GetByID(ctx, u.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to reload created user: %w", err)
			}
			return created, nil
		case errors.Is(err, ErrUsernameAlreadyUsed):
			continue
		case errors.Is(err, ErrEmailAlreadyUsed):
			// The email belongs to another account; sign up without it rather than
			// failing or silently merging into that account.
			email = ""
			continue
		default:
			return nil, err
		}
	}
	return nil, fmt.Errorf("failed to create user for identity after %d attempts", createIdentityAttempts)
}

// importAvatar stores the provider's profile picture as the avatar. It is best
// effort: a failure is logged and never fails the sign-up.
func (s *service) importAvatar(ctx context.Context, userID string, fetch func(context.Context) ([]byte, error)) {
	data, err := fetch(ctx)
	if err != nil {
		log.Printf("warning: failed to fetch profile picture for user %s: %v", userID, err)
		return
	}
	f, err := s.fileService.UploadBytes(ctx, file.UploadBytesInput{
		UserID:       userID,
		Filename:     "avatar.jpg",
		Data:         data,
		MaxSizeBytes: maxAvatarBytes,
		AllowedTypes: []string{"image/jpeg", "image/png"},
		ResizeImage:  true,
	})
	if err != nil {
		log.Printf("warning: failed to store profile picture for user %s: %v", userID, err)
		return
	}
	if err := s.UpdateAvatar(ctx, userID, f.ID); err != nil {
		log.Printf("warning: failed to set avatar for user %s: %v", userID, err)
		if delErr := s.fileService.Delete(ctx, f.ID); delErr != nil {
			log.Printf("warning: failed to delete unused avatar file %s: %v", f.ID, delErr)
		}
	}
}

func (s *service) LinkIdentity(ctx context.Context, userID string, p IdentityProfile) error {
	if !validProvider(p.Provider) {
		return ErrUnsupportedProvider
	}
	if p.Subject == "" {
		return fmt.Errorf("identity subject is empty")
	}
	return s.repo.AddIdentity(ctx, userID, p.Provider, p.Subject)
}

func (s *service) UnlinkIdentity(ctx context.Context, userID, provider string) error {
	if !validProvider(provider) {
		return ErrUnsupportedProvider
	}
	return s.repo.RemoveIdentity(ctx, userID, provider)
}

func (s *service) SetPassword(ctx context.Context, userID, email, password string) error {
	if len(password) < s.minPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > s.maxPasswordLength {
		return ErrPasswordTooLong
	}

	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.HasPassword() {
		return ErrPasswordAlreadySet
	}

	// A password login needs an email to sign in with; take one now if the account has none.
	var emailPtr *string
	if u.Email == "" {
		clean := normalizeEmail(email)
		if clean == "" {
			return ErrEmailRequired
		}
		emailPtr = &clean
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	return s.repo.SetCredentials(ctx, userID, emailPtr, hash)
}

func (s *service) GetByUsername(ctx context.Context, username string) (*User, error) {
	return s.repo.GetByUsername(ctx, normalizeUsername(username))
}

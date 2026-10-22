package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"time"
)

// Service manages login sessions.
type Service interface {
	// Create starts a session for userID and returns it with the raw token to
	// hand to the browser; the raw token is never stored.
	Create(ctx context.Context, userID string) (*Session, string, error)
	// Resolve returns the live session for a raw token, or ErrInvalidSession.
	Resolve(ctx context.Context, rawToken string) (*Session, error)
	// Revoke ends a session immediately; revoking an unknown session is a no-op.
	Revoke(ctx context.Context, id string) error
	// IsActive reports whether the session belongs to userID and is still live.
	IsActive(ctx context.Context, id, userID string) (bool, error)
}

type service struct {
	repo Repository
	ttl  time.Duration
}

// NewService creates a Service whose sessions live for ttl.
func NewService(repo Repository, ttl time.Duration) Service {
	return &service{repo: repo, ttl: ttl}
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *service) Create(ctx context.Context, userID string) (*Session, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, "", fmt.Errorf("generate session token failed: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)

	sess, err := s.repo.Create(ctx, userID, hashToken(raw), time.Now().Add(s.ttl))
	if err != nil {
		return nil, "", err
	}

	// Housekeeping must never fail a login.
	if err := s.repo.PurgeStale(ctx); err != nil {
		log.Printf("warning: %v", err)
	}
	return sess, raw, nil
}

func (s *service) Resolve(ctx context.Context, rawToken string) (*Session, error) {
	return s.repo.GetValidByTokenHash(ctx, hashToken(rawToken))
}

func (s *service) Revoke(ctx context.Context, id string) error {
	return s.repo.Revoke(ctx, id)
}

func (s *service) IsActive(ctx context.Context, id, userID string) (bool, error) {
	return s.repo.IsActive(ctx, id, userID)
}

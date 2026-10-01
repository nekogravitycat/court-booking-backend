package notification

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

type Service interface {
	SendManual(ctx context.Context, senderID string, userIDs []string, title, content string) error
	// Notify delivers one notification to a user's inbox.
	Notify(ctx context.Context, n *Notification) error
	// NotifyMany delivers several notifications at once.
	NotifyMany(ctx context.Context, ns []*Notification) error
	List(ctx context.Context, filter Filter) ([]*Notification, int, error)
	CountUnread(ctx context.Context, userID string) (int, error)
	MarkRead(ctx context.Context, userID, id string) error
	MarkAllRead(ctx context.Context, userID string) (int64, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Notify(ctx context.Context, n *Notification) error {
	return s.repo.Create(ctx, n)
}

func (s *service) NotifyMany(ctx context.Context, ns []*Notification) error {
	return s.repo.CreateMany(ctx, ns)
}

func (s *service) List(ctx context.Context, filter Filter) ([]*Notification, int, error) {
	return s.repo.List(ctx, filter)
}

func (s *service) CountUnread(ctx context.Context, userID string) (int, error) {
	return s.repo.CountUnread(ctx, userID)
}

func (s *service) MarkRead(ctx context.Context, userID, id string) error {
	return s.repo.MarkRead(ctx, userID, id)
}

func (s *service) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	return s.repo.MarkAllRead(ctx, userID)
}

func (s *service) SendManual(ctx context.Context, senderID string, userIDs []string, title, content string) error {
	title, content = strings.TrimSpace(title), strings.TrimSpace(content)
	if len(userIDs) < 1 || len(userIDs) > 100 || utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 100 || utf8.RuneCountInString(content) < 1 || utf8.RuneCountInString(content) > 2000 {
		return ErrInvalidManualNotification
	}
	seen := make(map[string]bool, len(userIDs))
	ids := make([]string, len(userIDs))
	for i, id := range userIDs {
		parsed, err := uuid.Parse(id)
		if err != nil || seen[parsed.String()] {
			return ErrInvalidManualNotification
		}
		ids[i] = parsed.String()
		seen[ids[i]] = true
	}
	return s.repo.CreateManual(ctx, senderID, ids, title, content)
}

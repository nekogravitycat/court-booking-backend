package notification

import "context"

type Service interface {
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

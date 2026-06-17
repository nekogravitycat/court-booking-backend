package favorite

import (
	"context"
	"errors"

	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// Service defines business logic for favorite hosts.
type Service interface {
	AddFavorite(ctx context.Context, userID, hostID string) error
	RemoveFavorite(ctx context.Context, userID, hostID string) error
	ListFavorites(ctx context.Context, userID string) ([]*FavoriteHost, error)
}

type service struct {
	repo        Repository
	userService user.Service
}

func NewService(repo Repository, userService user.Service) Service {
	return &service{repo: repo, userService: userService}
}

func (s *service) AddFavorite(ctx context.Context, userID, hostID string) error {
	// The host must exist and currently hold the pickup host role.
	if _, err := s.userService.GetByID(ctx, hostID); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return ErrHostNotFound
		}
		return err
	}

	isHost, err := s.userService.IsPickupHost(ctx, hostID)
	if err != nil {
		return err
	}
	if !isHost {
		return ErrNotPickupHost
	}

	return s.repo.AddFavorite(ctx, userID, hostID)
}

func (s *service) RemoveFavorite(ctx context.Context, userID, hostID string) error {
	return s.repo.RemoveFavorite(ctx, userID, hostID)
}

func (s *service) ListFavorites(ctx context.Context, userID string) ([]*FavoriteHost, error) {
	return s.repo.ListFavorites(ctx, userID)
}

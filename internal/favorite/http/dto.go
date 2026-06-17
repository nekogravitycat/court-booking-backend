package http

import (
	"github.com/nekogravitycat/court-booking-backend/internal/favorite"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
)

// FavoriteHostRequest is the body shared by POST and DELETE /favorites/host.
type FavoriteHostRequest struct {
	HostID string `json:"host_id" binding:"required,uuid"`
}

// FavoriteHostResponse exposes only the host's public nickname and avatar.
type FavoriteHostResponse struct {
	HostID          string  `json:"host_id"`
	Nickname        *string `json:"nickname"`
	Avatar          *string `json:"avatar"`           // URL to avatar image
	AvatarThumbnail *string `json:"avatar_thumbnail"` // URL to avatar thumbnail
}

func NewFavoriteHostResponse(f *favorite.FavoriteHost) FavoriteHostResponse {
	resp := FavoriteHostResponse{
		HostID:   f.HostID,
		Nickname: f.Nickname,
	}
	if f.Avatar != nil {
		url := file.FileURL(*f.Avatar)
		resp.Avatar = &url
		thumb := file.ThumbnailURL(*f.Avatar)
		resp.AvatarThumbnail = &thumb
	}
	return resp
}

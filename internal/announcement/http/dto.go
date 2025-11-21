package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/announcement"
)

type AnnouncementResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewResponse(a *announcement.Announcement) AnnouncementResponse {
	return AnnouncementResponse{
		ID:        a.ID,
		Title:     a.Title,
		Content:   a.Content,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

type CreateRequest struct {
	Title   string `json:"title" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type UpdateRequest struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

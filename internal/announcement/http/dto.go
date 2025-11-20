package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/announcement"
)

type Response struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewResponse(a *announcement.Announcement) Response {
	return Response{
		ID:        a.ID,
		Title:     a.Title,
		Content:   a.Content,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

type CreateBody struct {
	Title   string `json:"title" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type UpdateBody struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

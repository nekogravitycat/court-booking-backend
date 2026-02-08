package http

import (
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/announcement"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
)

// ListAnnouncementsRequest defines query parameters for listing announcements.
type ListAnnouncementsRequest struct {
	request.ListParams
	Keyword string `form:"q"`
	SortBy  string `form:"sort_by" binding:"omitempty,oneof=title created_at"`
}

// Validate performs custom validation for ListAnnouncementsRequest.
func (r *ListAnnouncementsRequest) Validate() error {
	return nil
}

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
		CreatedAt: a.CreatedAt.UTC(),
		UpdatedAt: a.UpdatedAt.UTC(),
	}
}

type CreateRequest struct {
	Title   string `json:"title" binding:"required,min=1,max=200"`
	Content string `json:"content" binding:"required"`
}

// Validate performs custom validation for CreateRequest.
func (r *CreateRequest) Validate() error {
	return nil
}

type UpdateRequest struct {
	Title   *string `json:"title" binding:"omitempty,min=1,max=200"`
	Content *string `json:"content"`
}

// Validate performs custom validation for UpdateRequest.
func (r *UpdateRequest) Validate() error {
	return nil
}

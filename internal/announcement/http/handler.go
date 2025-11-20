package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/announcement"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type Handler struct {
	service announcement.Service
}

func NewHandler(service announcement.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("q")

	filter := announcement.Filter{
		Keyword:  keyword,
		Page:     page,
		PageSize: pageSize,
	}

	list, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list announcements"})
		return
	}

	items := make([]Response, len(list))
	for i, a := range list {
		items[i] = NewResponse(a)
	}

	resp := response.NewPageResponse(items, page, pageSize, total)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	a, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, announcement.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "announcement not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get announcement"})
		}
		return
	}

	c.JSON(http.StatusOK, NewResponse(a))
}

func (h *Handler) Create(c *gin.Context) {
	var body CreateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	req := announcement.CreateRequest{
		Title:   body.Title,
		Content: body.Content,
	}

	a, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, announcement.ErrTitleRequired),
			errors.Is(err, announcement.ErrContentRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create announcement"})
		}
		return
	}

	c.JSON(http.StatusCreated, NewResponse(a))
}

func (h *Handler) Update(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	var body UpdateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	req := announcement.UpdateRequest{
		Title:   body.Title,
		Content: body.Content,
	}

	a, err := h.service.Update(c.Request.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, announcement.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "announcement not found"})
		case errors.Is(err, announcement.ErrTitleRequired),
			errors.Is(err, announcement.ErrContentRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update announcement"})
		}
		return
	}

	c.JSON(http.StatusOK, NewResponse(a))
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		switch {
		case errors.Is(err, announcement.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "announcement not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete announcement"})
		}
		return
	}

	c.Status(http.StatusNoContent)
}

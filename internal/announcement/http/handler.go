package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/announcement"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type Handler struct {
	service announcement.Service
}

func NewHandler(service announcement.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c *gin.Context) {
	var req ListAnnouncementsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filter := announcement.Filter{
		Keyword:   req.Keyword,
		Page:      req.Page,
		PageSize:  req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	if filter.SortBy == "" {
		filter.SortBy = "created_at"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "DESC"
	} else {
		filter.SortOrder = strings.ToUpper(filter.SortOrder)
	}

	list, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list announcements"})
		return
	}

	items := make([]AnnouncementResponse, len(list))
	for i, a := range list {
		items[i] = NewResponse(a)
	}

	resp := response.NewPageResponse(items, req.Page, req.PageSize, total)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Get(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	a, err := h.service.GetByID(c.Request.Context(), req.ID)
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
	var body CreateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var body UpdateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req := announcement.UpdateRequest{
		Title:   body.Title,
		Content: body.Content,
	}

	a, err := h.service.Update(c.Request.Context(), uri.ID, req)
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
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	if err := h.service.Delete(c.Request.Context(), req.ID); err != nil {
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

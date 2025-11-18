package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
)

type OrganizationHandler struct {
	service organization.Service
}

func NewOrganizationHandler(service organization.Service) *OrganizationHandler {
	return &OrganizationHandler{service: service}
}

// GET /v1/organizations
func (h *OrganizationHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := organization.OrganizationFilter{
		Page:     page,
		PageSize: pageSize,
	}

	orgs, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list organizations"})
		return
	}

	items := make([]OrganizationResponse, len(orgs))
	for i, o := range orgs {
		items[i] = NewOrganizationResponse(o)
	}

	resp := response.NewPageResponse(items, page, pageSize, total)

	c.JSON(http.StatusOK, resp)
}

// POST /v1/organizations
// Auth: System Admin only
func (h *OrganizationHandler) Create(c *gin.Context) {
	var req CreateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	org, err := h.service.Create(c.Request.Context(), req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create organization"})
		return
	}

	c.JSON(http.StatusCreated, NewOrganizationResponse(org))
}

// GET /v1/organizations/:id
func (h *OrganizationHandler) Get(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	org, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == organization.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get organization"})
		return
	}

	c.JSON(http.StatusOK, NewOrganizationResponse(org))
}

// PATCH /v1/organizations/:id
// Auth: System Admin (initially)
func (h *OrganizationHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	var req UpdateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	org, err := h.service.Update(c.Request.Context(), id, req.Name)
	if err != nil {
		if err == organization.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update organization"})
		return
	}

	c.JSON(http.StatusOK, NewOrganizationResponse(org))
}

// DELETE /v1/organizations/:id
// Auth: System Admin
func (h *OrganizationHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization id"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		if err == organization.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete organization"})
		return
	}

	c.Status(http.StatusNoContent)
}

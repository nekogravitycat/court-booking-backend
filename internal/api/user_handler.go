package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type UserHandler struct {
	userService user.Service
}

func NewUserHandler(userService user.Service) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// GET /v1/users
// Strict Access: Only System Admin can use this.
func (h *UserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	sort := c.DefaultQuery("sort", "created_at DESC")
	email := c.Query("email")
	displayName := c.Query("display_name")

	// Parse is_active bool
	var isActivePtr *bool
	if v := c.Query("is_active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			isActivePtr = &b
		}
	}

	filter := user.UserFilter{
		Page:        page,
		PageSize:    pageSize,
		Sort:        sort,
		Email:       email,
		DisplayName: displayName,
		IsActive:    isActivePtr,
	}

	users, total, err := h.userService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	// Convert domain users to DTOs
	items := make([]UserResponse, len(users))
	for i, u := range users {
		items[i] = NewUserResponse(u)
	}

	c.JSON(http.StatusOK, PageResponse{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

// GET /v1/users/:id
// Strict Access: Only System Admin can use this. Normal users should use /me.
func (h *UserHandler) Get(c *gin.Context) {
	id := c.Param("id")

	targetUser, err := h.userService.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == user.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
		return
	}

	c.JSON(http.StatusOK, NewUserResponse(targetUser))
}

// PATCH /v1/users/:id
// Strict Access: Only System Admin can use this. Normal users should use /me.
func (h *UserHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var body UpdateUserBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	req := user.UpdateUserRequest{
		DisplayName:   body.DisplayName,
		IsActive:      body.IsActive,
		IsSystemAdmin: body.IsSystemAdmin,
	}

	updatedUser, err := h.userService.Update(c.Request.Context(), id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, NewUserResponse(updatedUser))
}

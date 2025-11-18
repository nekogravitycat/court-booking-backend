package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type UserHandler struct {
	userService user.Service
	jwtManager  *auth.JWTManager
}

func NewUserHandler(userService user.Service, jwtManager *auth.JWTManager) *UserHandler {
	return &UserHandler{
		userService: userService,
		jwtManager:  jwtManager,
	}
}

// Register handles POST /v1/auth/register.
func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.Register(ctx, req.Email, req.Password, req.DisplayName)
	if err != nil {
		if err == user.ErrEmailAlreadyUsed {
			c.JSON(http.StatusConflict, gin.H{"error": "email already used"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp := MeResponse{
		User: NewUserResponse(u),
	}

	c.JSON(http.StatusCreated, resp)
}

// Login handles POST /v1/auth/login.
func (h *UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.Login(ctx, req.Email, req.Password)
	if err != nil {
		// Generic error message for security
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid email or password",
		})
		return
	}

	// Generate JWT using the injected jwtManager
	token, err := h.jwtManager.GenerateAccessToken(u.ID, u.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to generate token",
		})
		return
	}

	resp := LoginResponse{
		AccessToken: token,
		User:        NewUserResponse(u),
	}

	c.JSON(http.StatusOK, resp)
}

// Me handles GET /v1/me.
func (h *UserHandler) Me(c *gin.Context) {
	// Assuming auth.GetUserID extracts the ID from context
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.GetByID(ctx, userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	resp := MeResponse{
		User: NewUserResponse(u),
	}

	c.JSON(http.StatusOK, resp)
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

	resp := response.NewPageResponse(items, page, pageSize, total)

	c.JSON(http.StatusOK, resp)
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

	resp := MeResponse{
		User: NewUserResponse(targetUser),
	}

	c.JSON(http.StatusOK, resp)
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
		if err == user.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	resp := MeResponse{
		User: NewUserResponse(updatedUser),
	}

	c.JSON(http.StatusOK, resp)
}

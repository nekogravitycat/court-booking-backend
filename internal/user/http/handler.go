package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type UserHandler struct {
	userService user.Service
	jwtManager  *auth.JWTManager
}

func NewHandler(userService user.Service, jwtManager *auth.JWTManager) *UserHandler {
	return &UserHandler{
		userService: userService,
		jwtManager:  jwtManager,
	}
}

// Register handles the user registration process.
// It validates the payload and creates a new user if the email is unique.
func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.Register(ctx, req.Email, req.Password, req.DisplayName)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrEmailAlreadyUsed):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case errors.Is(err, user.ErrEmailRequired), errors.Is(err, user.ErrPasswordTooShort):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		}
		return
	}

	resp := MeResponse{
		User: NewUserResponse(u),
	}

	c.JSON(http.StatusCreated, resp)
}

// Login authenticates a user using email and password.
// On success, it returns a JWT access token and the user profile.
func (h *UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.Login(ctx, req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrInvalidCredentials),
			errors.Is(err, user.ErrNotFound),
			errors.Is(err, user.ErrInactiveUser):
			// For security reasons, do not reveal which condition failed
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		}
		return
	}

	// Generate JWT using the injected jwtManager
	token, err := h.jwtManager.GenerateAccessToken(u.ID)
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

// Me retrieves the profile of the currently authenticated user.
// It relies on the user ID extracted from the JWT context.
func (h *UserHandler) Me(c *gin.Context) {
	// Assuming auth.GetUserID extracts the ID from context
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
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

// List retrieves a paginated list of users with optional filtering.
// Access Control: System Admin only.
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

// Get retrieves a specific user by their ID.
// Access Control: System Admin only.
func (h *UserHandler) Get(c *gin.Context) {
	id := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

	targetUser, err := h.userService.GetByID(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
			return
		}
	}

	resp := MeResponse{
		User: NewUserResponse(targetUser),
	}

	c.JSON(http.StatusOK, resp)
}

// Update modifies specific attributes of a user.
// Access Control: System Admin only.
func (h *UserHandler) Update(c *gin.Context) {
	id := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
		return
	}

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
		switch {
		case errors.Is(err, user.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		}
		return
	}

	resp := MeResponse{
		User: NewUserResponse(updatedUser),
	}

	c.JSON(http.StatusOK, resp)
}

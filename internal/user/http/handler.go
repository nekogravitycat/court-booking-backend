package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	filehttp "github.com/nekogravitycat/court-booking-backend/internal/file/http"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/request"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/response"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type UserHandler struct {
	userService user.Service
	jwtManager  *auth.JWTManager
	fileService file.Service
	fileHandler *filehttp.Handler
}

func NewHandler(userService user.Service, jwtManager *auth.JWTManager, fileService file.Service, fileHandler *filehttp.Handler) *UserHandler {
	return &UserHandler{
		userService: userService,
		jwtManager:  jwtManager,
		fileService: fileService,
		fileHandler: fileHandler,
	}
}

// Register handles the user registration process.
// It validates the payload and creates a new user if the email is unique.
func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.Register(ctx, req.Email, req.Password, req.DisplayName)
	if err != nil {
		response.Error(c, err)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.Login(ctx, req.Email, req.Password)
	if err != nil {
		// For security reasons, map specific errors to InvalidCredentials
		if errors.Is(err, user.ErrNotFound) || errors.Is(err, user.ErrInactiveUser) {
			response.Error(c, user.ErrInvalidCredentials)
			return
		}
		response.Error(c, err)
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
		response.Error(c, err)
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
	var req ListUsersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filter := user.UserFilter{
		Page:        req.Page,
		PageSize:    req.PageSize,
		SortBy:      req.SortBy,
		SortOrder:   req.SortOrder,
		Email:       req.Email,
		IDs:         req.IDs,
		DisplayName: req.DisplayName,
		IsActive:    req.IsActive,
	}

	if filter.SortBy == "" {
		filter.SortBy = "created_at"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "DESC"
	} else {
		filter.SortOrder = strings.ToUpper(filter.SortOrder)
	}

	users, total, err := h.userService.List(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Convert domain users to DTOs
	items := make([]UserResponse, len(users))
	for i, u := range users {
		items[i] = NewUserResponse(u)
	}

	resp := response.NewPageResponse(items, req.Page, req.PageSize, total)

	c.JSON(http.StatusOK, resp)
}

// Get retrieves a specific user by their ID.
// Access Control: System Admin only.
func (h *UserHandler) Get(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	targetUser, err := h.userService.GetByID(c.Request.Context(), req.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := MeResponse{
		User: NewUserResponse(targetUser),
	}

	c.JSON(http.StatusOK, resp)
}

// Update modifies specific attributes of a user.
// Access Control: System Admin only.
func (h *UserHandler) Update(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	var body UpdateUserRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "details": err.Error()})
		return
	}

	if err := body.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req := user.UpdateUserRequest{
		DisplayName:   body.DisplayName,
		IsActive:      body.IsActive,
		IsSystemAdmin: body.IsSystemAdmin,
	}

	updatedUser, err := h.userService.Update(c.Request.Context(), uri.ID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := MeResponse{
		User: NewUserResponse(updatedUser),
	}

	c.JSON(http.StatusOK, resp)
}

// isSelfOrSysAdmin verifies if the current user is the target user or a system admin.
func (h *UserHandler) isSelfOrSysAdmin(c *gin.Context, targetUserID string) bool {
	currentUserID := auth.GetUserID(c)
	if currentUserID == targetUserID {
		return true
	}

	// Check if system admin
	u, err := h.userService.GetByID(c.Request.Context(), currentUserID)
	return err == nil && u.IsSystemAdmin
}

// Delete performs a soft delete on a user.
// Access Control: System Admin only.
func (h *UserHandler) Delete(c *gin.Context) {
	var req request.ByIDRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	if err := h.userService.Delete(c.Request.Context(), req.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// UploadAvatar uploads an avatar image for a user.
func (h *UserHandler) UploadAvatar(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Permission check: Only the user themselves or system admin can upload their avatar
	if !h.isSelfOrSysAdmin(c, uri.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you can only upload your own avatar"})
		return
	}

	h.fileHandler.HandleFileUpload(c, filehttp.FileUploadConfig{
		MaxSizeBytes: 5 * 1024 * 1024, // 5MB
		AllowedTypes: []string{"image/jpeg", "image/png"},
		ResizeImage:  true,
		AfterUpload: func(ctx context.Context, fileID string) error {
			return h.userService.UpdateAvatar(ctx, uri.ID, fileID)
		},
	})
}

// RemoveAvatar removes the avatar image from a user.
func (h *UserHandler) RemoveAvatar(c *gin.Context) {
	var uri request.ByIDRequest
	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Permission check: Only the user themselves or system admin can remove their avatar
	if !h.isSelfOrSysAdmin(c, uri.ID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you can only remove your own avatar"})
		return
	}

	if err := h.userService.RemoveAvatar(c.Request.Context(), uri.ID); err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

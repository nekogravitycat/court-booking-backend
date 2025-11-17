package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

type AuthHandler struct {
	userService user.Service
	jwtManager  *auth.JWTManager
}

func NewAuthHandler(
	userService user.Service,
	jwtManager *auth.JWTManager,
) *AuthHandler {
	return &AuthHandler{
		userService: userService,
		jwtManager:  jwtManager,
	}
}

//
// POST /v1/auth/register
//

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.Register(ctx, req.Email, req.Password, req.DisplayName)
	if err != nil {
		switch err {
		case user.ErrEmailAlreadyUsed:
			c.JSON(http.StatusConflict, gin.H{"error": "email already used"})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	resp := RegisterResponse{
		User: NewUserResponse(u),
	}

	c.JSON(http.StatusCreated, resp)
}

//
// POST /v1/auth/login
//

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ctx := c.Request.Context()

	u, err := h.userService.Login(ctx, req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid email or password",
		})
		return
	}

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

//
// GET /v1/me
//

func (h *AuthHandler) Me(c *gin.Context) {
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

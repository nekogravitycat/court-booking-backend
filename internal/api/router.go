package api

import (
	"github.com/gin-gonic/gin"

	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

func NewRouter(
	userService user.Service,
	jwtManager *auth.JWTManager,
) *gin.Engine {

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	v1 := r.Group("/v1")

	// Auth routes
	authHandler := NewAuthHandler(userService, jwtManager)
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
	}

	// Protected routes
	protected := v1.Group("/")
	protected.Use(auth.AuthRequired(jwtManager))
	{
		protected.GET("/me", authHandler.Me)
	}

	return r
}

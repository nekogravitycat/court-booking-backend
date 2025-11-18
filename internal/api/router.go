package api

import (
	"github.com/gin-contrib/cors"
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

	config := cors.DefaultConfig()
	config.AllowOrigins = []string{
		"http://localhost:8081",
	}
	config.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	r.Use(cors.New(config))

	v1 := r.Group("/v1")

	// Handlers
	authHandler := NewAuthHandler(userService, jwtManager)
	userHandler := NewUserHandler(userService)

	// Public routes
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
	}

	// Protected routes (Logged in users)
	protected := v1.Group("/")
	protected.Use(auth.AuthRequired(jwtManager))
	{
		protected.GET("/me", authHandler.Me)

		usersGroup := protected.Group("/users")
		{
			// Admin only Get
			usersGroup.GET("/:id", RequireSystemAdmin(userService), userHandler.Get)

			// Admin Only List
			usersGroup.GET("", RequireSystemAdmin(userService), userHandler.List)

			// Admin Only Update
			usersGroup.PATCH("/:id", RequireSystemAdmin(userService), userHandler.Update)
		}
	}

	return r
}

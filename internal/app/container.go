package app

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/api"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// Config holds the dependencies and settings required to start the application.
type Config struct {
	DBPool       *pgxpool.Pool
	JWTSecret    string
	JWTTTL       time.Duration
	PasswordCost int
}

// Container holds the initialized components that are needed externally.
type Container struct {
	Router     *gin.Engine
	JWTManager *auth.JWTManager
}

// NewContainer initializes all modules and returns the container.
func NewContainer(cfg Config) *Container {
	// Init Components
	passwordHasher := auth.NewBcryptPasswordHasherWithCost(cfg.PasswordCost)
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTTTL)

	// User Module
	userRepo := user.NewPgxRepository(cfg.DBPool)
	userService := user.NewService(userRepo, passwordHasher)

	// Organization Module
	orgRepo := organization.NewPgxRepository(cfg.DBPool)
	orgService := organization.NewService(orgRepo)

	// Router
	router := api.NewRouter(userService, orgService, jwtManager)

	return &Container{
		Router:     router,
		JWTManager: jwtManager,
	}
}

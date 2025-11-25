package app

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/announcement"
	"github.com/nekogravitycat/court-booking-backend/internal/api"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/booking"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
	"github.com/nekogravitycat/court-booking-backend/internal/resourcetype"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

// Config holds the dependencies and settings required to start the application.
type Config struct {
	IsProduction bool
	ProdOrigins  string
	DBPool       *pgxpool.Pool
	JWTSecret    string
	JWTTTL       time.Duration
	BcryptCost   int
}

// Container holds the initialized components that are needed externally.
type Container struct {
	Router     *gin.Engine
	JWTManager *auth.JWTManager
}

// NewContainer initializes all modules and returns the container.
func NewContainer(cfg Config) *Container {
	// Init Components
	passwordHasher := auth.NewBcryptPasswordHasherWithCost(cfg.BcryptCost)
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTTTL)

	// User Module
	userRepo := user.NewPgxRepository(cfg.DBPool)
	userService := user.NewService(userRepo, passwordHasher)

	// Organization Module
	orgRepo := organization.NewPgxRepository(cfg.DBPool)
	orgService := organization.NewService(orgRepo, userService)

	// Location Module
	locRepo := location.NewPgxRepository(cfg.DBPool)
	locService := location.NewService(locRepo, orgService)

	// ResourceType Module
	rtRepo := resourcetype.NewPgxRepository(cfg.DBPool)
	rtService := resourcetype.NewService(rtRepo)

	// Resource Module
	resRepo := resource.NewPgxRepository(cfg.DBPool)
	resService := resource.NewService(resRepo, locService, rtService)

	// Booking Module
	bookingRepo := booking.NewPgxRepository(cfg.DBPool)
	bookingService := booking.NewService(bookingRepo, resService, locService, orgService)

	// Announcement Module
	annRepo := announcement.NewPgxRepository(cfg.DBPool)
	annService := announcement.NewService(annRepo)

	// API Router Config
	routerParams := api.Config{
		IsProduction:   cfg.IsProduction,
		ProdOrigins:    cfg.ProdOrigins,
		UserService:    userService,
		OrgService:     orgService,
		LocService:     locService,
		RTService:      rtService,
		ResService:     resService,
		BookingService: bookingService,
		AnnService:     annService,
		JWTManager:     jwtManager,
	}

	// Router
	router := api.NewRouter(routerParams)

	return &Container{
		Router:     router,
		JWTManager: jwtManager,
	}
}

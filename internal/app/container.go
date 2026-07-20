package app

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nekogravitycat/court-booking-backend/internal/announcement"
	"github.com/nekogravitycat/court-booking-backend/internal/api"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/booking"
	"github.com/nekogravitycat/court-booking-backend/internal/favorite"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/pickup"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/storage"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
	"github.com/nekogravitycat/court-booking-backend/internal/skilllevel"
	"github.com/nekogravitycat/court-booking-backend/internal/sports"
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

	// File Module
	store, err := storage.NewLocalStorage("storage")
	if err != nil {
		panic(err) // Critical failure if storage cannot be initialized
	}
	fileRepo := file.NewRepository(cfg.DBPool)
	fileService := file.NewService(fileRepo, store)

	// Favorite Module (repo created early so it can be injected into the user
	// service for favorite cleanup on account deletion).
	favoriteRepo := favorite.NewPgxRepository(cfg.DBPool)

	// User Module
	userRepo := user.NewPgxRepository(cfg.DBPool)
	userService := user.NewService(userRepo, passwordHasher, fileService, favoriteRepo)

	// Favorite Service (depends on user service to validate pickup hosts)
	favoriteService := favorite.NewService(favoriteRepo, userService)

	// Organization & Location Module
	orgRepo := organization.NewPgxRepository(cfg.DBPool)
	locRepo := location.NewPgxRepository(cfg.DBPool)
	orgService := organization.NewService(orgRepo, userService, locRepo, fileService)
	locService := location.NewService(locRepo, orgService, userService, fileService)

	// Resource Module
	resRepo := resource.NewPgxRepository(cfg.DBPool)
	resService := resource.NewService(resRepo, locService, fileService)

	// Booking Module
	bookingRepo := booking.NewPgxRepository(cfg.DBPool)
	bookingService := booking.NewService(bookingRepo, resService, locService, orgService)

	// Announcement Module
	annRepo := announcement.NewPgxRepository(cfg.DBPool)
	annService := announcement.NewService(annRepo)

	// Sports & Skill-Level lookup modules
	sportsRepo := sports.NewPgxRepository(cfg.DBPool)
	sportsService := sports.NewService(sportsRepo)
	skillLevelRepo := skilllevel.NewPgxRepository(cfg.DBPool)
	skillLevelService := skilllevel.NewService(skillLevelRepo)

	// Pickup Module
	pickupRepo := pickup.NewPgxRepository(cfg.DBPool)
	pickupService := pickup.NewService(pickupRepo, userService, sportsService, skillLevelService)

	// API Router Config
	routerParams := api.Config{
		IsProduction:      cfg.IsProduction,
		ProdOrigins:       cfg.ProdOrigins,
		UserService:       userService,
		OrgService:        orgService,
		LocService:        locService,
		ResService:        resService,
		BookingService:    bookingService,
		AnnService:        annService,
		SportsService:     sportsService,
		SkillLevelService: skillLevelService,
		PickupService:     pickupService,
		FavoriteService:   favoriteService,
		FileService:       fileService,
		JWTManager:        jwtManager,
	}

	// Router
	router := api.NewRouter(routerParams)

	return &Container{
		Router:     router,
		JWTManager: jwtManager,
	}
}

package api

import (
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/nekogravitycat/court-booking-backend/internal/announcement"
	annHttp "github.com/nekogravitycat/court-booking-backend/internal/announcement/http"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/booking"
	bookingHttp "github.com/nekogravitycat/court-booking-backend/internal/booking/http"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	fileHttp "github.com/nekogravitycat/court-booking-backend/internal/file/http"
	"github.com/nekogravitycat/court-booking-backend/internal/location"
	locHttp "github.com/nekogravitycat/court-booking-backend/internal/location/http"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	"github.com/nekogravitycat/court-booking-backend/internal/resource"
	resHttp "github.com/nekogravitycat/court-booking-backend/internal/resource/http"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
)

// Config holds all dependencies required to initialize the router.
type Config struct {
	IsProduction   bool
	ProdOrigins    string
	UserService    user.Service
	OrgService     organization.Service
	LocService     location.Service
	ResService     resource.Service
	BookingService booking.Service
	AnnService     announcement.Service
	FileService    file.Service
	JWTManager     *auth.JWTManager
}

// NewRouter initializes the HTTP router engine using the provided config.
func NewRouter(cfg Config) *gin.Engine {
	// Set Gin mode
	if cfg.IsProduction {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// Global Middleware
	r.Use(gin.Logger(), gin.Recovery())

	// CORS
	config := cors.DefaultConfig()

	// Parse allowed origins from config
	allowedOrigins := strings.Split(cfg.ProdOrigins, ",")
	for i, o := range allowedOrigins {
		allowedOrigins[i] = strings.TrimSpace(o)
	}

	if cfg.IsProduction {
		// Production: Strict mode, only allow exact domain match
		config.AllowOrigins = allowedOrigins
	} else {
		// Dev / Local: Allow all origins
		config.AllowOriginFunc = func(origin string) bool {
			return true
		}
	}

	config.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	r.Use(cors.New(config))

	// Auth Middleware
	authMiddleware := auth.AuthRequired(cfg.JWTManager)
	sysAdminMiddleware := RequireSystemAdmin(cfg.UserService)

	// Initialize Handlers (Injecting Services from cfg)
	fileHandler := fileHttp.NewHandler(cfg.FileService)
	userHandler := userHttp.NewHandler(cfg.UserService, cfg.JWTManager, cfg.FileService, fileHandler)
	orgHandler := orgHttp.NewHandler(cfg.OrgService, cfg.FileService, fileHandler)
	locHandler := locHttp.NewHandler(cfg.LocService, cfg.OrgService, cfg.FileService, fileHandler)
	resHandler := resHttp.NewHandler(cfg.ResService, cfg.LocService, cfg.OrgService, cfg.FileService, fileHandler)
	bookingHandler := bookingHttp.NewHandler(cfg.BookingService, cfg.UserService, cfg.ResService, cfg.LocService, cfg.OrgService)
	annHandler := annHttp.NewHandler(cfg.AnnService)

	// Register Routes
	v1 := r.Group("/v1")
	{
		fileHttp.RegisterRoutes(v1, fileHandler, authMiddleware)
		userHttp.RegisterRoutes(v1, userHandler, authMiddleware, sysAdminMiddleware)
		orgHttp.RegisterRoutes(v1, orgHandler, authMiddleware, sysAdminMiddleware)
		locHttp.RegisterRoutes(v1, locHandler, authMiddleware)
		resHttp.RegisterRoutes(v1, resHandler, authMiddleware)
		bookingHttp.RegisterRoutes(v1, bookingHandler, authMiddleware)
		annHttp.RegisterRoutes(v1, annHandler, authMiddleware, sysAdminMiddleware)
	}

	return r
}

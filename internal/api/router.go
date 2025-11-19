package api

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	orgHttp "github.com/nekogravitycat/court-booking-backend/internal/organization/http"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
	userHttp "github.com/nekogravitycat/court-booking-backend/internal/user/http"
)

// NewRouter initializes the HTTP router engine.
// It is responsible for assembling middleware (CORS, Logger, Auth) and registering routes for various modules.
func NewRouter(
	userService user.Service,
	orgService organization.Service,
	jwtManager *auth.JWTManager,
) *gin.Engine {

	r := gin.New()

	// Global Middleware:
	// - Logger: Logs request information to the console.
	// - Recovery: Captures panics to prevent server crashes and returns a 500 error.
	r.Use(gin.Logger(), gin.Recovery())

	// Configure CORS (Cross-Origin Resource Sharing).
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{
		"http://localhost:8081", // Swagger
	}
	config.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	r.Use(cors.New(config))

	// authMiddleware: Validates if the request contains a valid JWT.
	authMiddleware := auth.AuthRequired(jwtManager)
	// sysAdminMiddleware: Further checks if the authenticated user has System Admin privileges.
	sysAdminMiddleware := RequireSystemAdmin(userService)

	// Initialize HTTP Handlers for each module (injecting Service dependencies).
	userHandler := userHttp.NewUserHandler(userService, jwtManager)
	orgHandler := orgHttp.NewOrganizationHandler(orgService)

	// Register API routes under /v1
	v1 := r.Group("/v1")
	{
		userHttp.RegisterRoutes(v1, userHandler, authMiddleware, sysAdminMiddleware)
		orgHttp.RegisterRoutes(v1, orgHandler, authMiddleware, sysAdminMiddleware)
	}

	return r
}

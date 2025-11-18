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

func NewRouter(
	userService user.Service,
	orgService organization.Service,
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

	authMiddleware := auth.AuthRequired(jwtManager)
	sysAdminMiddleware := RequireSystemAdmin(userService)

	userHandler := userHttp.NewUserHandler(userService, jwtManager)
	orgHandler := orgHttp.NewOrganizationHandler(orgService)

	v1 := r.Group("/v1")
	{
		userHttp.RegisterRoutes(v1, userHandler, authMiddleware, sysAdminMiddleware)
		orgHttp.RegisterRoutes(v1, orgHandler, authMiddleware, sysAdminMiddleware)
	}

	return r
}

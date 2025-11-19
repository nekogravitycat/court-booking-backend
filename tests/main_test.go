package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require" // Use require for setup failures

	"github.com/nekogravitycat/court-booking-backend/internal/api"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

var (
	testRouter *gin.Engine
	testPool   *pgxpool.Pool
	jwtManager *auth.JWTManager
)

func TestMain(m *testing.M) {
	// 1. Setup Database Connection
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "postgres://gravity:yLJuh3kGh9j5@localhost:5432/mydb?sslmode=disable"
	}

	ctx := context.Background()
	var err error
	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}

	// 2. Init Components
	passwordHasher := auth.NewBcryptPasswordHasherWithCost(4)
	jwtManager = auth.NewJWTManager("test-secret", 15*time.Minute)

	userRepo := user.NewPgxRepository(testPool)
	userService := user.NewService(userRepo, passwordHasher)

	orgRepo := organization.NewPgxRepository(testPool)
	orgService := organization.NewService(orgRepo)

	// 3. Setup Router
	gin.SetMode(gin.TestMode)
	testRouter = api.NewRouter(userService, orgService, jwtManager)

	// 4. Run Tests
	exitCode := m.Run()

	// 5. Teardown
	testPool.Close()
	os.Exit(exitCode)
}

// clearTables helper
func clearTables() {
	ctx := context.Background()
	queries := []string{
		"TRUNCATE TABLE public.organization_permissions CASCADE",
		"TRUNCATE TABLE public.organizations CASCADE",
		"TRUNCATE TABLE public.users CASCADE",
	}
	for _, q := range queries {
		_, err := testPool.Exec(ctx, q)
		if err != nil {
			log.Printf("Failed to clean table: %v", err)
		}
	}
}

// executeRequest helper
func executeRequest(method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}

	req, _ := http.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	return w
}

// createTestUser creates a user and asserts success using require
func createTestUser(t *testing.T, email, password string, isAdmin bool) *user.User {
	hasher := auth.NewBcryptPasswordHasherWithCost(4)
	hash, err := hasher.Hash(password)
	require.NoError(t, err, "Failed to hash password")

	u := &user.User{
		Email:         email,
		PasswordHash:  hash,
		DisplayName:   &email,
		IsActive:      true,
		IsSystemAdmin: isAdmin,
	}

	repo := user.NewPgxRepository(testPool)
	err = repo.Create(context.Background(), u)
	require.NoError(t, err, "Failed to create test user in DB")

	savedUser, err := repo.GetByEmail(context.Background(), email)
	require.NoError(t, err, "Failed to fetch created user")

	return savedUser
}

// generateTokenHelper
func generateTokenHelper(userID, email string) string {
	token, _ := jwtManager.GenerateAccessToken(userID, email)
	return token
}

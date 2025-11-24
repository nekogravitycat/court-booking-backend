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
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"

	"github.com/nekogravitycat/court-booking-backend/internal/app"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

var (
	testRouter *gin.Engine
	testPool   *pgxpool.Pool
	jwtManager *auth.JWTManager
)

func TestMain(m *testing.M) {
	// Attempt to load .env from parent directory
	if err := godotenv.Load("../.env"); err != nil {
		log.Printf("No .env file found or failed to load: %v", err)
	}

	// Setup Database Connection
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		log.Fatalf("TEST_DB_DSN environment variable is not set")
	}

	ctx := context.Background()
	var err error
	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}

	// Get JWT Secret
	testSecret := os.Getenv("TEST_JWT_SECRET")
	if testSecret == "" {
		log.Fatalf("TEST_JWT_SECRET environment variable is not set")
	}

	// Initialize App Container using shared logic
	appContainer := app.NewContainer(app.Config{
		DBPool:       testPool,
		JWTSecret:    testSecret,
		JWTTTL:       30 * time.Minute,
		PasswordCost: 4, // Lower cost for testing purposes
	})

	// Assign global variables for tests to use
	testRouter = appContainer.Router
	jwtManager = appContainer.JWTManager

	// Setup Gin mode
	gin.SetMode(gin.TestMode)

	// Run Tests
	exitCode := m.Run()

	// Teardown
	testPool.Close()
	os.Exit(exitCode)
}

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

func executeRequest(method, path string, body any, token string) *httptest.ResponseRecorder {
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

func generateToken(userID, email string) string {
	token, _ := jwtManager.GenerateAccessToken(userID)
	return token
}

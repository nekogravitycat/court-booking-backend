package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const PROD_STRING = "prod"

// Config holds all application configuration loaded from environment.
type Config struct {
	IsProduction      bool
	ProdOrigins       string
	HTTPAddr          string
	DBDSN             string
	JWTSecret         string
	JWTAccessTokenTTL time.Duration
	BcryptCost        int
}

// Load loads configuration from .env (optional) and environment variables.
func Load() (*Config, error) {
	// Load .env file if it exists
	err := godotenv.Load()
	if err != nil {
		log.Printf("failed to load .env file: %v", err)
	}

	cfg := &Config{}

	// Production origin (default: empty)
	cfg.ProdOrigins = getEnv("PROD_ORIGINS", "")

	// Application environment (default: dev)
	appEnvStr := getEnv("APP_ENV", "dev")
	cfg.IsProduction = appEnvStr == PROD_STRING

	// HTTP listen address (default: :8080)
	cfg.HTTPAddr = getEnv("HTTP_ADDR", ":8080")

	// Database DSN is required
	cfg.DBDSN = os.Getenv("DB_DSN")
	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("DB_DSN is required")
	}

	// JWT secret is required for signing tokens
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	// JWT access token TTL, parse as time.Duration (e.g. "15m", "1h").
	ttlStr := getEnv("JWT_ACCESS_TOKEN_TTL", "15m")
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_ACCESS_TOKEN_TTL: %w", err)
	}
	cfg.JWTAccessTokenTTL = ttl

	// Bcrypt cost for password hashing (default: 12)
	cfg.BcryptCost = getEnvAsInt("BCRYPT_COST", 12)

	return cfg, nil
}

// getEnv returns the value of the environment variable if set,
// otherwise returns the provided default value.
func getEnv(key, defaultValue string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultValue
}

// getEnvAsInt retrieves an environment variable as an integer,
// or returns the default value if not set or invalid.
func getEnvAsInt(key string, defaultValue int) int {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		log.Printf("invalid integer for env %s: %v", key, err)
		log.Printf("using default value %d for env %s", defaultValue, key)
		return defaultValue
	}
	return val
}

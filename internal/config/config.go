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
	cfg.BcryptCost, err = getEnvAsInt("BCRYPT_COST", 12)
	if err != nil {
		return nil, fmt.Errorf("invalid BCRYPT_COST: %w", err)
	}

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

// getEnvAsInt retrieves an environment variable as an integer.
// It returns the default value if the variable is not set.
// It returns an error if the variable is set but is not a valid integer.
func getEnvAsInt(key string, defaultValue int) (int, error) {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultValue, nil
	}

	val, err := strconv.Atoi(valStr)
	if err != nil {
		// Return 0 and a wrapped error to provide context
		return 0, fmt.Errorf("env %s value %q is not a valid integer: %w", key, valStr, err)
	}

	return val, nil
}

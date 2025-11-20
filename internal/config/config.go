package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment.
type Config struct {
	ProdOrigins       string
	AppEnv            string
	HTTPAddr          string
	DBDSN             string
	JWTSecret         string
	JWTAccessTokenTTL time.Duration
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
	cfg.ProdOrigins = getEnvOrDefault("PROD_ORIGINS", "")

	// Application environment (default: local)
	cfg.AppEnv = getEnvOrDefault("APP_ENV", "local")

	// HTTP listen address (default: :8080)
	cfg.HTTPAddr = getEnvOrDefault("HTTP_ADDR", ":8080")

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
	ttlStr := getEnvOrDefault("JWT_ACCESS_TOKEN_TTL", "15m")
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_ACCESS_TOKEN_TTL: %w", err)
	}
	cfg.JWTAccessTokenTTL = ttl

	return cfg, nil
}

// getEnvOrDefault returns the value of the environment variable if set,
// otherwise returns the provided default value.
func getEnvOrDefault(key, defaultValue string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultValue
}

// getEnvAsIntOrDefault is a helper for parsing integer environment variables.
func getEnvAsIntOrDefault(key string, defaultValue int) (int, error) {
	if v, ok := os.LookupEnv(key); ok {
		i, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("invalid integer for %s: %w", key, err)
		}
		return i, nil
	}
	return defaultValue, nil
}

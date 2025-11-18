package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/api"
	"github.com/nekogravitycat/court-booking-backend/internal/auth"
	"github.com/nekogravitycat/court-booking-backend/internal/config"
	"github.com/nekogravitycat/court-booking-backend/internal/db"
	"github.com/nekogravitycat/court-booking-backend/internal/organization"
	"github.com/nekogravitycat/court-booking-backend/internal/user"
)

func main() {
	// For receiving Ctrl+C / SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Connect DB
	pool, err := db.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	// Init components
	passwordHasher := auth.NewBcryptPasswordHasher()
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTAccessTokenTTL)

	// User module
	userRepo := user.NewPgxRepository(pool)
	userService := user.NewService(userRepo, passwordHasher)

	// Organization module
	orgRepo := organization.NewPgxRepository(pool)
	orgService := organization.NewService(orgRepo)

	// Gin router
	router := api.NewRouter(userService, orgService, jwtManager)

	// Use http.Server for graceful shutdown
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}

	// Run server in separate goroutine
	go func() {
		log.Printf("server running on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for Ctrl+C
	<-ctx.Done()
	log.Println("shutdown signal received")

	// Create a shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server forced to shutdown: %v", err)
	}

	log.Println("server exited gracefully")
}

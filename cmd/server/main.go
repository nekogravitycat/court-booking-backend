package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nekogravitycat/court-booking-backend/internal/app"
	"github.com/nekogravitycat/court-booking-backend/internal/config"
	"github.com/nekogravitycat/court-booking-backend/internal/db"
)

const SERVER_SHUTDOWN_TIMEOUT = 5 * time.Second

func main() {
	// For receiving Ctrl+C / SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Log mode
	if cfg.IsProduction {
		log.Printf("starting server in production mode")
	} else {
		log.Printf("starting server in development mode")
	}

	// Connect DB
	pool, err := db.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	// Initialize App Container
	appContainer := app.NewContainer(app.Config{
		IsProduction: cfg.IsProduction,
		ProdOrigins:  cfg.ProdOrigins,
		DBPool:       pool,
		JWTSecret:    cfg.JWTSecret,
		JWTTTL:       cfg.JWTAccessTokenTTL,
		BcryptCost:   cfg.BcryptCost,
	})

	// Use http.Server for graceful shutdown
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: appContainer.Router,
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), SERVER_SHUTDOWN_TIMEOUT)
	defer cancel()

	// Shutdown HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server forced to shutdown: %v", err)
	} else {
		log.Println("server exited gracefully")
	}
}

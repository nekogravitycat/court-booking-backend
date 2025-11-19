package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/nekogravitycat/court-booking-backend/internal/app"
	"github.com/nekogravitycat/court-booking-backend/internal/config"
	"github.com/nekogravitycat/court-booking-backend/internal/db"
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

	// Initialize App Container
	appContainer := app.NewContainer(app.Config{
		DBPool:       pool,
		JWTSecret:    cfg.JWTSecret,
		JWTTTL:       cfg.JWTAccessTokenTTL,
		PasswordCost: bcrypt.DefaultCost,
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server forced to shutdown: %v", err)
	}

	log.Println("server exited gracefully")
}

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/nexconsult/cnpj-api/internal/api"
	"github.com/nexconsult/cnpj-api/internal/config"
	"github.com/nexconsult/cnpj-api/internal/logger"
	"github.com/nexconsult/cnpj-api/internal/services"
	"github.com/sirupsen/logrus"

	// Import docs for Swagger
	_ "github.com/nexconsult/cnpj-api/docs"
)

// @title CNPJ Consultation API
// @version 2.0
// @description High-performance CNPJ consultation API built with Go
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.nexconsult.com/support
// @contact.email support@nexconsult.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /api/v1
// @schemes http https

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	logger := logger.New(cfg.Log.Level, cfg.Log.Format)
	logger.Info("Starting CNPJ API Server...")

	// Set Gin mode
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize services
	serviceContainer, err := services.NewContainer(cfg, logger)
	if err != nil {
		logger.Fatalf("Failed to initialize services: %v", err)
	}
	defer serviceContainer.Close()

	// Initialize API server
	server := api.NewServer(cfg, logger, serviceContainer)

	// Setup HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      server.Router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.WithFields(logrus.Fields{
			"port":        cfg.Server.Port,
			"environment": cfg.Server.Environment,
		}).Info("Server starting...")

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Create a deadline for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Errorf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exited")
}

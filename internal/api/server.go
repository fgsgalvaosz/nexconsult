package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nexconsult/cnpj-api/internal/api/handlers"
	"github.com/nexconsult/cnpj-api/internal/api/middleware"
	"github.com/nexconsult/cnpj-api/internal/config"
	"github.com/nexconsult/cnpj-api/internal/services"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Server represents the HTTP server
type Server struct {
	Router   *gin.Engine
	config   *config.Config
	logger   *logrus.Logger
	services *services.Container
}

// NewServer creates a new HTTP server
func NewServer(cfg *config.Config, logger *logrus.Logger, services *services.Container) *Server {
	server := &Server{
		config:   cfg,
		logger:   logger,
		services: services,
	}

	server.setupRouter()
	return server
}

// setupRouter configures the router with all routes and middleware
func (s *Server) setupRouter() {
	// Create Gin router
	s.Router = gin.New()

	// Global middleware
	s.Router.Use(middleware.Logger(s.logger))
	s.Router.Use(middleware.Recovery(s.logger))
	s.Router.Use(middleware.CORS(s.config.Security.CORS))
	s.Router.Use(middleware.Security())
	s.Router.Use(middleware.RequestID())

	// Rate limiting middleware
	rateLimiter := middleware.NewRateLimiter(s.config.Security.RateLimit)
	s.Router.Use(rateLimiter.Middleware())

	// Health check endpoint (no rate limiting)
	s.Router.GET("/health", handlers.NewHealthHandler(s.services, s.logger).GetHealth)
	s.Router.GET("/health/ready", handlers.NewHealthHandler(s.services, s.logger).GetReadiness)
	s.Router.GET("/health/live", handlers.NewHealthHandler(s.services, s.logger).GetLiveness)

	// Metrics endpoint
	s.Router.GET("/metrics", handlers.NewMetricsHandler(s.services, s.logger).GetMetrics)

	// Swagger documentation
	if s.config.Server.Environment != "production" {
		s.Router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
		s.Router.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
		})
	}

	// API v1 routes
	v1 := s.Router.Group("/api/v1")
	{
		// CNPJ routes
		cnpjHandler := handlers.NewCNPJHandler(s.services.CNPJService, s.logger)
		cnpj := v1.Group("/cnpj")
		{
			cnpj.GET("/:cnpj", cnpjHandler.GetCNPJ)
			cnpj.POST("/batch", cnpjHandler.GetBatchCNPJ)
		}

		// Cache management routes (no auth for development)
		cache := v1.Group("/cache")
		// cache.Use(middleware.AdminAuth()) // Disabled for development
		{
			cacheHandler := handlers.NewCacheHandler(s.services.CacheService, s.logger)
			cache.GET("/stats", cacheHandler.GetStats)
			cache.DELETE("/clear", cacheHandler.Clear)
			cache.DELETE("/:cnpj", cacheHandler.Delete)
		}

		// Browser pool management routes (no auth for development)
		browser := v1.Group("/browser")
		// browser.Use(middleware.AdminAuth()) // Disabled for development
		{
			browserHandler := handlers.NewBrowserHandler(s.services.BrowserService, s.logger)
			browser.GET("/stats", browserHandler.GetStats)
			browser.POST("/restart", browserHandler.Restart)
			browser.GET("/health", browserHandler.GetHealth)
		}
	}

	// 404 handler
	s.Router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":     "Not Found",
			"message":   "The requested resource was not found",
			"timestamp": time.Now(),
			"path":      c.Request.URL.Path,
		})
	})

	// 405 handler
	s.Router.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"error":     "Method Not Allowed",
			"message":   "The requested method is not allowed for this resource",
			"timestamp": time.Now(),
			"path":      c.Request.URL.Path,
			"method":    c.Request.Method,
		})
	})
}

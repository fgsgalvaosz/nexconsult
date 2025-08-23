package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/swagger"

	"nexconsult/internal/types"
	"nexconsult/internal/worker"
)

// SetupRoutes configura as rotas da API
func SetupRoutes(app *fiber.App, workerPool *worker.WorkerPool, config *types.Config) {
	// Cria rate limiter com configurações
	rateLimiter := NewRateLimiter(config.RateLimit.RequestsPerMinute, config.RateLimit.BurstSize)

	// Inicia limpeza periódica dos limiters
	go func() {
		ticker := time.NewTicker(time.Duration(config.RateLimit.CleanupInterval) * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rateLimiter.cleanupOldLimiters()
		}
	}()

	// Middlewares globais
	app.Use(RecoveryMiddleware())
	app.Use(LoggingMiddleware())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-Correlation-ID",
	}))

	// Handlers
	handlers := NewHandlers(workerPool)

	// API v1 com rate limiting
	api := app.Group("/api/v1")
	api.Use(RateLimitMiddleware(rateLimiter))
	api.Use(QueueLimitMiddleware(config.RateLimit.MaxQueueSize))

	// CNPJ routes
	api.Get("/cnpj/:cnpj", handlers.GetCNPJ)

	// System routes (sem rate limiting)
	systemAPI := app.Group("/api/v1")
	systemAPI.Get("/status", handlers.GetStatus)
	systemAPI.Get("/health", handlers.HealthCheck)

	// Swagger documentation
	app.Get("/swagger/*", swagger.HandlerDefault)

	// Root redirect
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/")
	})
}

package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/swagger"
	
	"nexconsult/internal/worker"
)

// SetupRoutes configura as rotas da API
func SetupRoutes(app *fiber.App, workerPool *worker.WorkerPool) {
	// Middlewares
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))
	app.Use(logger.New(logger.Config{
		Format: "${time} ${status} - ${method} ${path} - ${latency}\n",
	}))

	// Handlers
	handlers := NewHandlers(workerPool)

	// API v1
	api := app.Group("/api/v1")
	
	// CNPJ routes
	api.Get("/cnpj/:cnpj", handlers.GetCNPJ)
	
	// System routes
	api.Get("/status", handlers.GetStatus)
	api.Get("/health", handlers.HealthCheck)

	// Swagger documentation
	app.Get("/swagger/*", swagger.HandlerDefault)
	
	// Root redirect
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/")
	})
}

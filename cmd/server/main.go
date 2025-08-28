package main

import (
	"log"
	"os"

	"nexconsult/internal/api"
	"nexconsult/internal/config"
	"nexconsult/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	// Carregar configuração
	cfg := config.LoadConfig()
	sintegraService := service.NewSintegraService(cfg)

	// Criar aplicação Fiber
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": err.Error(),
			})
		},
	})

	// Middlewares
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New())

	// Configurar rotas
	api.SetupRoutes(app, sintegraService)

	// Obter porta do ambiente ou usar padrão
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("🚀 Servidor iniciado na porta %s", port)
	log.Fatal(app.Listen(":" + port))
}

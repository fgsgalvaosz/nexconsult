package api

import (
	"nexconsult/internal/api/handlers"
	"nexconsult/internal/service"

	"github.com/gofiber/fiber/v2"
)

// SetupRoutes configura as rotas da API
func SetupRoutes(app *fiber.App, sintegraService *service.SintegraService) {
	// Criar handlers
	consultaHandler := handlers.NewConsultaHandler(sintegraService)

	// Grupo de rotas da API
	api := app.Group("/api/v1")

	// Rota para consulta de CNPJ via GET
	api.Get("/consulta/:cnpj", consultaHandler.ConsultaCNPJ)

	// Rota de health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "nexconsult-api",
			"version": "1.0.0",
		})
	})
}

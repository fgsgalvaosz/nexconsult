package main

import (
	"os"

	"nexconsult/internal/api"
	"nexconsult/internal/config"
	"nexconsult/internal/logger"
	"nexconsult/internal/service/container"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberLogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	// Carregar configuração
	cfg := config.LoadConfig()

	// Inicializar logger centralizado
	logger.InitGlobalLogger(cfg.DebugMode)
	appLogger := logger.GetLogger().With(logger.String("component", "main"))

	// Criar serviço usando container
	cont := container.NewContainer(cfg)
	factory := container.NewServiceFactory(cont)
	sintegraService := factory.CreateSintegraService()

	// Criar aplicação Fiber
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			appLogger.Error("Erro na aplicação",
				logger.Int("statusCode", code),
				logger.Error(err))
			return c.Status(code).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": err.Error(),
			})
		},
	})

	// Middlewares
	app.Use(recover.New())
	app.Use(fiberLogger.New())
	app.Use(cors.New())

	// Configurar rotas
	api.SetupRoutes(app, sintegraService)

	// Obter porta do ambiente ou usar padrão
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	appLogger.Info("🚀 Servidor iniciado", logger.String("porta", port))
	appLogger.Fatal("Erro ao iniciar servidor", logger.Error(app.Listen(":"+port)))
}

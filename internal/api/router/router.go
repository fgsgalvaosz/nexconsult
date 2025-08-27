package router

import (
	"fmt"
	"nexconsult-sintegra-ma/internal/api/handlers"
	"nexconsult-sintegra-ma/internal/api/middleware"
	"nexconsult-sintegra-ma/internal/service"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	_ "nexconsult-sintegra-ma/docs"
)

// SetupRoutes configura todas as rotas da API e retorna o SintegraService para graceful shutdown
func SetupRoutes(app *fiber.App, logger zerolog.Logger) *service.SintegraService {
	// Configurar middlewares globais
	setupMiddlewares(app)

	// Inicializar serviços
	// Usar configuração de timeout padrão
	sintegraService := service.NewSintegraService(logger, nil)

	// Iniciar worker pool para processamento paralelo
	sintegraService.StartWorkerPool()

	// Inicializar handlers
	healthHandler := handlers.NewHealthHandler()
	sintegraHandler := handlers.NewSintegraHandler(sintegraService, logger)

	// Configurar rotas básicas
	setupBasicRoutes(app, healthHandler)

	// Configurar grupo de rotas da API
	setupAPIRoutes(app, sintegraHandler)

	// Configurar rotas 404
	setup404Handler(app)

	return sintegraService
}

// setupMiddlewares configura middlewares globais
func setupMiddlewares(app *fiber.App) {
	// Recovery middleware (deve ser o primeiro)
	app.Use(middleware.RecoveryConfig())

	// Logger middleware
	app.Use(middleware.LoggerConfig())

	// CORS middleware
	app.Use(middleware.CORSConfig())

	// Rate limiter middleware - aplicado globalmente
	// Cada tipo de endpoint terá seu próprio limite conforme configurado no middleware
	app.Use(middleware.RateLimiterConfig())
}

// setupBasicRoutes configura rotas básicas (health, docs, welcome)
func setupBasicRoutes(app *fiber.App, healthHandler *handlers.HealthHandler) {
	// Rota raiz - welcome
	app.Get("/", healthHandler.Welcome)

	// Health check
	app.Get("/health", healthHandler.HealthCheck)

	// Documentação JSON
	app.Get("/docs", healthHandler.Docs)

	// Swagger UI - Documentação interativa com recursos CDN
	// Rota personalizada para Swagger UI com recursos estáticos do CDN
	app.Get("/swagger/", func(c *fiber.Ctx) error {
		html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>API Documentation - NexConsult Sintegra MA</title>
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui.css" />
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.9.0/index.css" />
  <link rel="icon" type="image/png" href="https://unpkg.com/swagger-ui-dist@5.9.0/favicon-32x32.png" sizes="32x32" />
  <link rel="icon" type="image/png" href="https://unpkg.com/swagger-ui-dist@5.9.0/favicon-16x16.png" sizes="16x16" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-bundle.js" charset="UTF-8"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-standalone-preset.js" charset="UTF-8"></script>
  <script>
    window.onload = function() {
      const ui = SwaggerUIBundle({
        url: '/swagger.json',
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout",
        docExpansion: "list",
        defaultModelExpandDepth: 3,
        defaultModelsExpandDepth: 1,
        showExtensions: true,
        showCommonExtensions: true,
        onComplete: function() {
          console.log('Swagger UI carregado com sucesso!');
        }
      });
      window.ui = ui;
    }
  </script>
</body>
</html>`
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(html)
	})

	// Redirecionamento para compatibilidade com /swagger/index.html
	app.Get("/swagger/index.html", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/", fiber.StatusMovedPermanently)
	})

	// Redirecionamento para /swagger sem barra final
	app.Get("/swagger", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/", fiber.StatusMovedPermanently)
	})

	// Servir arquivos estáticos da documentação
	app.Static("/swagger-assets", "./docs")

	// Rota direta para o arquivo JSON da documentação
	app.Get("/swagger.json", func(c *fiber.Ctx) error {
		// Ler arquivo diretamente
		filePath := "./docs/swagger.json"
		data, err := os.ReadFile(filePath)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Arquivo swagger.json não encontrado",
			})
		}

		// Configurar headers corretos
		c.Set("Content-Type", "application/json; charset=utf-8")
		c.Set("Content-Length", fmt.Sprintf("%d", len(data)))
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Content-Type")

		// Retornar dados como bytes para garantir integridade
		return c.Send(data)
	})
}

// setupAPIRoutes configura rotas da API versionadas
func setupAPIRoutes(app *fiber.App, sintegraHandler *handlers.SintegraHandler) {
	// Grupo de rotas da API v1
	v1 := app.Group("/api/v1")

	// Grupo específico para Sintegra
	sintegra := v1.Group("/sintegra")

	// Endpoints do Sintegra
	sintegra.Post("/consultar", sintegraHandler.ConsultarCNPJ)
	sintegra.Get("/consultar/:cnpj", sintegraHandler.ConsultarCNPJByPath)
	sintegra.Post("/consultar-lote", sintegraHandler.ConsultarCNPJEmLote)
	sintegra.Post("/status", sintegraHandler.VerificarStatusConsulta)
}

// setup404Handler configura handler para rotas não encontradas
func setup404Handler(app *fiber.App) {
	app.Use("*", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   "Endpoint não encontrado",
			"message": "Verifique a documentação em /docs",
			"path":    c.Path(),
			"method":  c.Method(),
		})
	})
}

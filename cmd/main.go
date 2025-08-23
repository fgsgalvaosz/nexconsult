// Package main CNPJ Consultor API
// @title CNPJ Consultor API
// @version 1.0
// @description API para consulta automatizada de CNPJs na Receita Federal com resolução automática de captcha
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email support@cnpjconsultor.com
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
// @host localhost:3000
// @BasePath /api/v1
// @schemes http https
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"

	"nexconsult/internal/api"
	"nexconsult/internal/captcha"
	"nexconsult/internal/config"
	"nexconsult/internal/logger"
	"nexconsult/internal/worker"

	_ "nexconsult/docs" // Swagger docs
)

func main() {
	// Inicializa configuração
	cfg := config.LoadConfig()

	// Inicializa logger centralizado
	appLogger := logger.NewLogger(logger.Config{
		Level:      cfg.Logging.Level,
		Format:     cfg.Logging.Format,
		Output:     cfg.Logging.Output,
		FilePath:   cfg.Logging.FilePath,
		MaxSize:    cfg.Logging.MaxSize,
		MaxBackups: cfg.Logging.MaxBackups,
		MaxAge:     cfg.Logging.MaxAge,
		Compress:   cfg.Logging.Compress,
		Sampling:   cfg.Logging.Sampling,
	})
	logger.SetGlobalLogger(appLogger)

	log := appLogger.WithComponent("main")

	// Inicializa componentes (sem cache - sempre busca direta)
	captchaClient := captcha.NewSolveCaptchaClient(cfg.SolveCaptcha.APIKey)
	workerPool := worker.NewWorkerPool(cfg.Workers.Count, captchaClient)

	// Inicia worker pool
	workerPool.Start()
	defer workerPool.Stop()

	// Configura Fiber
	app := fiber.New(fiber.Config{
		Prefork:      cfg.Server.Prefork,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
		ErrorHandler: errorHandler,
	})

	// Configura rotas da API
	api.SetupRoutes(app, workerPool, cfg)

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Info("Gracefully shutting down...")
		app.ShutdownWithTimeout(30 * time.Second)
	}()

	// Inicia servidor
	port := fmt.Sprintf(":%d", cfg.Server.Port)
	log.InfoFields("🚀 CNPJ Consultor iniciado", logger.Fields{
		"port": port,
	})

	if err := app.Listen(port); err != nil {
		log.ErrorFields("Failed to start server", logger.Fields{
			"error": err.Error(),
			"port":  port,
		})
		os.Exit(1)
	}
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	logrus.WithFields(logrus.Fields{
		"error":  err.Error(),
		"path":   c.Path(),
		"method": c.Method(),
	}).Error("Request error")

	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
		"code":  code,
	})
}

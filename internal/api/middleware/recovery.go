package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog/log"
)

// RecoveryConfig configura middleware de recovery com logging
func RecoveryConfig() fiber.Handler {
	return recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
			// Log estruturado do panic
			log.Error().
				Interface("panic", e).
				Str("method", c.Method()).
				Str("path", c.Path()).
				Str("ip", c.IP()).
				Msg("Panic capturado pelo recovery middleware")
		},
		// Personalizar resposta de erro
		Next: func(c *fiber.Ctx) bool {
			return false // Aplicar a todos os endpoints
		},
	})
}

// ErrorHandler middleware para tratamento global de erros
func ErrorHandler() fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError

		// Verificar se é um erro do Fiber
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}

		// Não logar erros de timeout 408 para evitar spam nos logs
		if code != fiber.StatusRequestTimeout {
			// Log do erro apenas para erros relevantes
			log.Error().
				Err(err).
				Int("status_code", code).
				Str("method", c.Method()).
				Str("path", c.Path()).
				Str("ip", c.IP()).
				Msg("Erro capturado pelo error handler")
		}

		// Personalizar mensagem de erro para timeout
		errorMessage := err.Error()
		if code == fiber.StatusRequestTimeout {
			errorMessage = "Requisição expirou. Tente novamente."
		}

		// Retornar resposta padronizada
		return c.Status(code).JSON(fiber.Map{
			"success":   false,
			"error":     errorMessage,
			"timestamp": time.Now(),
		})
	}
}
package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// LoggerConfig cria middleware personalizado para logging de requisições
func LoggerConfig() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Processar a requisição
		err := c.Next()

		// Calcular duração
		duration := time.Since(start)

		// Obter informações da requisição
		method := c.Method()
		path := c.Path()
		statusCode := c.Response().StatusCode()
		ip := c.IP()
		userAgent := c.Get("User-Agent")

		// Determinar nível de log baseado no status code
		var event *zerolog.Event
		switch {
		case statusCode >= 500:
			event = log.Error()
		case statusCode >= 400:
			event = log.Warn()
		default:
			event = log.Info()
		}

		// Registrar log estruturado
		event.
			Str("method", method).
			Str("path", path).
			Int("status", statusCode).
			Dur("duration", duration).
			Str("ip", ip).
			Str("user_agent", userAgent).
			Msgf("%s %s - %d (%s)", method, path, statusCode, duration)

		return err
	}
}
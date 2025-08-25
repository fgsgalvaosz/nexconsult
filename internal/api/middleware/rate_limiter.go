package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RateLimiterConfig configura rate limiting baseado em IP
func RateLimiterConfig() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        10,                 // Máximo 10 requisições
		Expiration: 1 * time.Minute,    // Por minuto
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP() // Usar IP como chave
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success":   false,
				"error":     "Rate limit excedido. Máximo 10 requisições por minuto por IP",
				"timestamp": time.Now(),
				"retry_after": "1 minute",
			})
		},
		SkipFailedRequests:     false,
		SkipSuccessfulRequests: false,
		Storage:                nil, // Usar storage em memória (padrão)
	})
}
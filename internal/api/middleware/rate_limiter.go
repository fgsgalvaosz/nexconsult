package middleware

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// Configurações de rate limit para diferentes tipos de endpoints
type RateLimitConfig struct {
	Max        int
	Expiration time.Duration
	Message    string
}

// RateLimiterConfig configura o middleware de rate limiting com diferentes limites por tipo de endpoint
func RateLimiterConfig() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        10,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			// Criar chave baseada no IP e tipo de endpoint
			endpointType := getEndpointType(c.Path())
			return c.IP() + ":" + endpointType
		},
		LimitReached: func(c *fiber.Ctx) error {
			// Determinar configuração baseada no tipo de endpoint
			endpointType := getEndpointType(c.Path())
			var message string
			var maxRequests int

			switch endpointType {
			case "critical":
				maxRequests = 15
				message = "Muitas consultas críticas. Tente novamente em alguns segundos."
			case "batch":
				maxRequests = 5
				message = "Muitas consultas em lote. Tente novamente em alguns segundos."
			default:
				maxRequests = 30
				message = "Muitas requisições. Tente novamente em alguns segundos."
			}

			// Calcular tempo de reset
			resetTime := time.Now().Add(time.Minute)

			// Adicionar headers informativos
			c.Set("X-RateLimit-Limit", strconv.Itoa(maxRequests))
			c.Set("X-RateLimit-Remaining", "0")
			c.Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
			c.Set("Retry-After", "60")

			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success":     false,
				"error":       message,
				"timestamp":   time.Now(),
				"retry_after": 60,
				"reset_at":    resetTime.Format(time.RFC3339),
			})
		},
	})
}

// getEndpointType determina o tipo de endpoint com base no caminho
func getEndpointType(path string) string {
	// Endpoints de consulta em lote (mais pesados)
	if strings.Contains(path, "/consultar-lote") {
		return "batch"
	}

	// Endpoints de consulta individual ao Sintegra
	if strings.Contains(path, "/consultar") {
		return "critical"
	}

	// Endpoints de status de consulta
	if strings.Contains(path, "/status") {
		return "critical"
	}

	// Outros endpoints (documentação, health check, etc)
	return "default"
}

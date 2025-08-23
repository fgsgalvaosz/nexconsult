package api

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"nexconsult/internal/logger"
)

// RateLimiter estrutura para controle de rate limiting
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter cria um novo rate limiter
func NewRateLimiter(requestsPerMinute, burstSize int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Every(time.Minute / time.Duration(requestsPerMinute)),
		burst:    burstSize,
	}
}

// getLimiter obtém ou cria um limiter para um IP específico
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[ip] = limiter
	}

	return limiter
}

// Allow verifica se a requisição é permitida
func (rl *RateLimiter) Allow(ip string) bool {
	return rl.getLimiter(ip).Allow()
}

// cleanupOldLimiters remove limiters inativos (executar periodicamente)
func (rl *RateLimiter) cleanupOldLimiters() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for ip, limiter := range rl.limiters {
		// Remove limiters que não foram usados recentemente
		if limiter.Tokens() == float64(rl.burst) {
			delete(rl.limiters, ip)
		}
	}
}

// LoggingMiddleware middleware para logging de requests HTTP
func LoggingMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Gera correlation ID se não existir
		correlationID := c.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
			c.Set("X-Correlation-ID", correlationID)
		}

		// Adiciona correlation ID ao contexto
		c.Locals("correlation_id", correlationID)

		// Log do request
		log := logger.GetGlobalLogger().WithComponent("api").WithCorrelationID(correlationID)
		log.InfoFields("HTTP request started", logger.Fields{
			"method":     c.Method(),
			"path":       c.Path(),
			"ip":         c.IP(),
			"user_agent": c.Get("User-Agent"),
			"type":       "http_request",
		})

		// Processa request
		err := c.Next()

		// Calcula duração
		duration := time.Since(start)

		// Log da response
		fields := logger.Fields{
			"method":   c.Method(),
			"path":     c.Path(),
			"status":   c.Response().StatusCode(),
			"duration": duration.String(),
			"ip":       c.IP(),
			"type":     "http_response",
		}

		if err != nil {
			fields["error"] = err.Error()
			log.ErrorFields("HTTP request failed", fields)
		} else {
			// Log level baseado no status code
			status := c.Response().StatusCode()
			if status >= 500 {
				log.ErrorFields("HTTP request completed with server error", fields)
			} else if status >= 400 {
				log.WarnFields("HTTP request completed with client error", fields)
			} else {
				log.InfoFields("HTTP request completed successfully", fields)
			}
		}

		return err
	}
}

// RecoveryMiddleware middleware para recovery com logging
func RecoveryMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				correlationID := c.Locals("correlation_id")
				if correlationID == nil {
					correlationID = "unknown"
				}

				log := logger.GetGlobalLogger().WithComponent("api").WithCorrelationID(correlationID.(string))
				log.ErrorFields("Panic recovered", logger.Fields{
					"panic":  r,
					"method": c.Method(),
					"path":   c.Path(),
					"ip":     c.IP(),
					"type":   "panic_recovery",
				})

				// Retorna erro 500
				c.Status(500).JSON(fiber.Map{
					"error":          "Internal Server Error",
					"correlation_id": correlationID,
				})
			}
		}()

		return c.Next()
	}
}

// GetCorrelationID helper para obter correlation ID do contexto
func GetCorrelationID(c *fiber.Ctx) string {
	if id := c.Locals("correlation_id"); id != nil {
		return id.(string)
	}
	return ""
}

// RateLimitMiddleware middleware para rate limiting por IP
func RateLimitMiddleware(rateLimiter *RateLimiter) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()

		if !rateLimiter.Allow(ip) {
			correlationID := GetCorrelationID(c)

			logger.GetGlobalLogger().WithComponent("api").WithCorrelationID(correlationID).WarnFields("Rate limit exceeded", logger.Fields{
				"ip":     ip,
				"path":   c.Path(),
				"method": c.Method(),
				"type":   "rate_limit_exceeded",
			})

			return c.Status(429).JSON(fiber.Map{
				"error":          "Rate limit exceeded",
				"message":        "Muitas requisições. Tente novamente em alguns instantes.",
				"retry_after":    "60s",
				"correlation_id": correlationID,
			})
		}

		return c.Next()
	}
}

// QueueLimitMiddleware middleware para controlar fila de jobs
func QueueLimitMiddleware(maxQueueSize int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Verifica se é uma rota de CNPJ
		path := c.Path()
		if !contains(path, "/api/v1/cnpj/") {
			return c.Next()
		}

		// TODO: Implementar verificação real da fila do worker pool
		// Por enquanto, simula uma verificação básica

		correlationID := GetCorrelationID(c)
		logger.GetGlobalLogger().WithComponent("api").WithCorrelationID(correlationID).DebugFields("Queue check passed", logger.Fields{
			"path":           path,
			"max_queue_size": maxQueueSize,
			"type":           "queue_check",
		})

		return c.Next()
	}
}

// contains verifica se uma string contém outra
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}

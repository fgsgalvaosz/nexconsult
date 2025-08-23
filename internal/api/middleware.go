package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	
	"nexconsult/internal/logger"
)

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
			"method":     c.Method(),
			"path":       c.Path(),
			"status":     c.Response().StatusCode(),
			"duration":   duration.String(),
			"ip":         c.IP(),
			"type":       "http_response",
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

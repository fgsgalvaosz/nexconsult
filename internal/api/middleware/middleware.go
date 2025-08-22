package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nexconsult/cnpj-api/internal/config"
	"github.com/sirupsen/logrus"
)

// RequestID adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// Recovery returns a middleware that recovers from panics
func Recovery(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				requestID := c.GetString("request_id")

				logger.WithFields(logrus.Fields{
					"request_id": requestID,
					"method":     c.Request.Method,
					"path":       c.Request.URL.Path,
					"panic":      err,
				}).Error("Panic recovered")

				c.JSON(http.StatusInternalServerError, gin.H{
					"error":      "Internal Server Error",
					"message":    "An unexpected error occurred",
					"request_id": requestID,
					"timestamp":  time.Now(),
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}

// CORS returns a middleware that handles CORS
func CORS(corsConfig config.CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range corsConfig.AllowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", strings.Join(corsConfig.AllowedMethods, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(corsConfig.AllowedHeaders, ", "))

		if corsConfig.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		c.Header("Access-Control-Max-Age", "86400") // 24 hours

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Security adds security headers
func Security() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// More permissive CSP for development (allows Swagger to work)
		// In production, you should use more restrictive policies
		if c.Request.URL.Path == "/swagger/" ||
			c.Request.URL.Path == "/swagger/index.html" ||
			strings.HasPrefix(c.Request.URL.Path, "/swagger/") {
			c.Header("Content-Security-Policy", "default-src 'self' 'unsafe-inline' 'unsafe-eval'; connect-src 'self' http://localhost:8080; img-src 'self' data:; font-src 'self' data:")
		} else {
			c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'")
		}

		c.Next()
	}
}

// AdminAuth middleware for admin-only endpoints
func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// For now, just check for a simple admin token
		// In production, implement proper JWT or API key authentication
		adminToken := c.GetHeader("X-Admin-Token")
		if adminToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":     "Unauthorized",
				"message":   "Admin token required",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// TODO: Validate admin token against database or JWT
		// For now, accept any non-empty token
		c.Next()
	}
}

// APIKeyAuth middleware for API key authentication
func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":     "Unauthorized",
				"message":   "API key required",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// TODO: Validate API key against database
		// For now, accept any non-empty key
		c.Set("api_key", apiKey)
		c.Next()
	}
}

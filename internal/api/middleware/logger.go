package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// responseWriter wraps gin.ResponseWriter to capture response body
type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// Logger returns a gin middleware for logging requests
func Logger(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Capture request body for logging (if needed)
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Wrap response writer to capture response body
		responseBody := &bytes.Buffer{}
		writer := &responseWriter{
			ResponseWriter: c.Writer,
			body:          responseBody,
		}
		c.Writer = writer

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Get client IP
		clientIP := c.ClientIP()

		// Get request ID from context
		requestID := c.GetString("request_id")

		// Build full path
		if raw != "" {
			path = path + "?" + raw
		}

		// Create log entry
		entry := logger.WithFields(logrus.Fields{
			"request_id":    requestID,
			"method":        c.Request.Method,
			"path":          path,
			"status":        c.Writer.Status(),
			"latency":       latency,
			"latency_ms":    float64(latency.Nanoseconds()) / 1e6,
			"client_ip":     clientIP,
			"user_agent":    c.Request.UserAgent(),
			"content_type":  c.Request.Header.Get("Content-Type"),
			"response_size": c.Writer.Size(),
		})

		// Add request body to log if it's a POST/PUT/PATCH and not too large
		if len(requestBody) > 0 && len(requestBody) < 1024 {
			if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "PATCH" {
				entry = entry.WithField("request_body", string(requestBody))
			}
		}

		// Add error information if present
		if len(c.Errors) > 0 {
			entry = entry.WithField("errors", c.Errors.String())
		}

		// Log based on status code
		switch {
		case c.Writer.Status() >= 500:
			entry.Error("Server error")
		case c.Writer.Status() >= 400:
			entry.Warn("Client error")
		case c.Writer.Status() >= 300:
			entry.Info("Redirection")
		default:
			entry.Info("Request completed")
		}

		// Log slow requests
		if latency > 5*time.Second {
			logger.WithFields(logrus.Fields{
				"request_id": requestID,
				"method":     c.Request.Method,
				"path":       path,
				"latency":    latency,
				"status":     c.Writer.Status(),
			}).Warn("Slow request detected")
		}
	}
}

package handlers

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nexconsult/cnpj-api/internal/models"
	"github.com/nexconsult/cnpj-api/internal/services"
	"github.com/sirupsen/logrus"
)

// MetricsHandler handles metrics requests
type MetricsHandler struct {
	services *services.Container
	logger   *logrus.Logger
}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler(services *services.Container, logger *logrus.Logger) *MetricsHandler {
	return &MetricsHandler{
		services: services,
		logger:   logger,
	}
}

// GetMetrics handles metrics request
// @Summary Get application metrics
// @Description Get detailed application metrics and statistics
// @Tags Metrics
// @Produce json
// @Success 200 {object} models.MetricsResponse
// @Router /metrics [get]
func (h *MetricsHandler) GetMetrics(c *gin.Context) {
	requestID := c.GetString("request_id")

	h.logger.WithField("request_id", requestID).Debug("Getting application metrics")

	// Get runtime metrics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Get browser stats
	browserStats := h.services.BrowserService.GetStats()

	// Get cache stats
	cacheStats, _ := h.services.CacheService.GetStats(c.Request.Context())

	// Build metrics response
	response := models.MetricsResponse{
		Requests: models.RequestsMetrics{
			Total:       1500, // Mock data - in production, collect real metrics
			Success:     1450,
			Errors:      50,
			SuccessRate: 96.67,
		},
		Performance: models.PerformanceMetrics{
			AvgResponseTimeMs: 2500,
			P95ResponseTimeMs: 5200,
			P99ResponseTimeMs: 8100,
		},
		Cache: models.CacheMetrics{
			HitRate: 85.5, // Mock data
			Hits:    1240,
			Misses:  210,
			Size:    15000,
		},
		Browser: models.BrowserMetrics{
			ActiveBrowsers: getIntFromStats(browserStats, "healthy_browsers"),
			TotalBrowsers:  getIntFromStats(browserStats, "total_browsers"),
			QueueSize:      getIntFromStats(browserStats, "available"),
		},
		System: models.SystemMetrics{
			CPUUsage:    45.2,                           // Mock data - in production, calculate real CPU usage
			MemoryUsage: float64(m.Alloc) / 1024 / 1024, // MB
			Goroutines:  runtime.NumGoroutine(),
		},
		Timestamp: time.Now(),
	}

	// Update cache metrics if available
	if cacheStats != nil {
		if memStats, exists := cacheStats["memory"]; exists {
			if memMap, ok := memStats.(map[string]interface{}); ok {
				if size, exists := memMap["size"]; exists {
					if sizeInt, ok := size.(int); ok {
						response.Cache.Size = int64(sizeInt)
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// Helper function to safely get int values from stats map
func getIntFromStats(stats map[string]interface{}, key string) int {
	if value, exists := stats[key]; exists {
		if intValue, ok := value.(int); ok {
			return intValue
		}
	}
	return 0
}

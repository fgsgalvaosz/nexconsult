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

// HealthHandler handles health check requests
type HealthHandler struct {
	services  *services.Container
	logger    *logrus.Logger
	startTime time.Time
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(services *services.Container, logger *logrus.Logger) *HealthHandler {
	return &HealthHandler{
		services:  services,
		logger:    logger,
		startTime: time.Now(),
	}
}

// GetHealth handles general health check
// @Summary Health check
// @Description Get the health status of the API and its dependencies
// @Tags Health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Failure 503 {object} models.ErrorResponse
// @Router /health [get]
func (h *HealthHandler) GetHealth(c *gin.Context) {
	// Get health status from all services
	servicesHealth := h.services.Health()

	// Determine overall status
	status := "healthy"
	for _, serviceHealth := range servicesHealth {
		if healthMap, ok := serviceHealth.(map[string]interface{}); ok {
			if serviceStatus, exists := healthMap["status"]; exists {
				if serviceStatus == "unhealthy" {
					status = "unhealthy"
					break
				} else if serviceStatus == "degraded" && status == "healthy" {
					status = "degraded"
				}
			}
		}
	}

	// Calculate uptime
	uptime := time.Since(h.startTime)

	response := models.HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Version:   "2.0.0",
		Services:  make(map[string]models.ServiceInfo),
		Uptime:    uptime.String(),
	}

	// Convert services health to ServiceInfo format
	for serviceName, serviceHealth := range servicesHealth {
		if healthMap, ok := serviceHealth.(map[string]interface{}); ok {
			serviceInfo := models.ServiceInfo{
				LastCheck: time.Now(),
			}

			if serviceStatus, exists := healthMap["status"]; exists {
				serviceInfo.Status = serviceStatus.(string)
			}

			if errorMsg, exists := healthMap["error"]; exists {
				serviceInfo.Error = errorMsg.(string)
			}

			// Mock response time for now
			serviceInfo.ResponseTimeMs = 50

			response.Services[serviceName] = serviceInfo
		}
	}

	// Set HTTP status based on health
	httpStatus := http.StatusOK
	if status == "unhealthy" {
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, response)
}

// GetReadiness handles readiness probe
// @Summary Readiness check
// @Description Check if the API is ready to serve requests
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} models.ErrorResponse
// @Router /health/ready [get]
func (h *HealthHandler) GetReadiness(c *gin.Context) {
	// Check if critical services are ready
	servicesHealth := h.services.Health()

	ready := true
	issues := make([]string, 0)

	// Check browser service
	if browserHealth, exists := servicesHealth["browser"]; exists {
		if healthMap, ok := browserHealth.(map[string]interface{}); ok {
			if status, exists := healthMap["status"]; exists && status == "unhealthy" {
				ready = false
				issues = append(issues, "browser service is unhealthy")
			}
		}
	}

	// Check CNPJ service
	if cnpjHealth, exists := servicesHealth["cnpj"]; exists {
		if healthMap, ok := cnpjHealth.(map[string]interface{}); ok {
			if status, exists := healthMap["status"]; exists && status == "unhealthy" {
				ready = false
				issues = append(issues, "CNPJ service is unhealthy")
			}
		}
	}

	response := map[string]interface{}{
		"ready":     ready,
		"timestamp": time.Now(),
		"services":  servicesHealth,
	}

	if len(issues) > 0 {
		response["issues"] = issues
	}

	httpStatus := http.StatusOK
	if !ready {
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, response)
}

// GetLiveness handles liveness probe
// @Summary Liveness check
// @Description Check if the API is alive and responding
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health/live [get]
func (h *HealthHandler) GetLiveness(c *gin.Context) {
	// Basic liveness check - if we can respond, we're alive
	response := map[string]interface{}{
		"alive":     true,
		"timestamp": time.Now(),
		"uptime":    time.Since(h.startTime).String(),
		"version":   "2.0.0",
	}

	c.JSON(http.StatusOK, response)
}

// GetMetrics handles metrics endpoint
// @Summary Get metrics
// @Description Get application metrics and statistics
// @Tags Metrics
// @Produce json
// @Success 200 {object} models.MetricsResponse
// @Router /metrics [get]
func (h *HealthHandler) GetMetrics(c *gin.Context) {
	// Get runtime metrics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Mock metrics for now - in production, you'd collect real metrics
	response := models.MetricsResponse{
		Requests: models.RequestsMetrics{
			Total:       1500,
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
			HitRate: 85.5,
			Hits:    1240,
			Misses:  210,
			Size:    15000,
		},
		Browser: models.BrowserMetrics{
			ActiveBrowsers: 8,
			TotalBrowsers:  15,
			QueueSize:      3,
		},
		System: models.SystemMetrics{
			CPUUsage:    45.2,
			MemoryUsage: float64(m.Alloc) / 1024 / 1024, // MB
			Goroutines:  runtime.NumGoroutine(),
		},
		Timestamp: time.Now(),
	}

	c.JSON(http.StatusOK, response)
}

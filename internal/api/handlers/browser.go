package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nexconsult/cnpj-api/internal/models"
	"github.com/nexconsult/cnpj-api/internal/services"
	"github.com/sirupsen/logrus"
)

// BrowserHandler handles browser pool management requests
type BrowserHandler struct {
	browserService services.BrowserServiceInterface
	logger         *logrus.Logger
}

// NewBrowserHandler creates a new browser handler
func NewBrowserHandler(browserService services.BrowserServiceInterface, logger *logrus.Logger) *BrowserHandler {
	return &BrowserHandler{
		browserService: browserService,
		logger:         logger,
	}
}

// GetStats handles browser pool statistics request
// @Summary Get browser pool statistics
// @Description Get detailed browser pool statistics and metrics
// @Tags Browser
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} models.ErrorResponse
// @Router /browser/stats [get]
func (h *BrowserHandler) GetStats(c *gin.Context) {
	requestID := c.GetString("request_id")

	h.logger.WithField("request_id", requestID).Info("Getting browser pool statistics")

	stats := h.browserService.GetStats()

	response := map[string]interface{}{
		"stats":     stats,
		"timestamp": time.Now(),
		"health":    h.browserService.Health(),
	}

	c.JSON(http.StatusOK, response)
}

// Restart handles browser pool restart request
// @Summary Restart browser pool
// @Description Restart all browsers in the pool
// @Tags Browser
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} models.ErrorResponse
// @Router /browser/restart [post]
func (h *BrowserHandler) Restart(c *gin.Context) {
	requestID := c.GetString("request_id")

	h.logger.WithField("request_id", requestID).Info("Restarting browser pool")

	err := h.browserService.Restart()
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"error":      err.Error(),
		}).Error("Failed to restart browser pool")

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "Internal server error",
			Message:   "Failed to restart browser pool",
			Code:      "BROWSER_RESTART_ERROR",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	h.logger.WithField("request_id", requestID).Info("Browser pool restarted successfully")

	response := map[string]interface{}{
		"message":   "Browser pool restarted successfully",
		"timestamp": time.Now(),
		"success":   true,
		"stats":     h.browserService.GetStats(),
	}

	c.JSON(http.StatusOK, response)
}

// GetHealth handles browser pool health check request
// @Summary Get browser pool health
// @Description Get the health status of the browser pool
// @Tags Browser
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} models.ErrorResponse
// @Router /browser/health [get]
func (h *BrowserHandler) GetHealth(c *gin.Context) {
	requestID := c.GetString("request_id")

	h.logger.WithField("request_id", requestID).Info("Checking browser pool health")

	health := h.browserService.Health()
	stats := h.browserService.GetStats()

	response := map[string]interface{}{
		"health":    health,
		"stats":     stats,
		"timestamp": time.Now(),
	}

	// Determine HTTP status based on health
	httpStatus := http.StatusOK
	if healthStatus, exists := health["status"]; exists {
		if healthStatus == "unhealthy" {
			httpStatus = http.StatusServiceUnavailable
		}
	}

	c.JSON(httpStatus, response)
}

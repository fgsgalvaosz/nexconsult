package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nexconsult/cnpj-api/internal/models"
	"github.com/nexconsult/cnpj-api/internal/services"
	"github.com/nexconsult/cnpj-api/internal/utils"
	"github.com/sirupsen/logrus"
)

// CacheHandler handles cache management requests
type CacheHandler struct {
	cacheService services.CacheServiceInterface
	logger       *logrus.Logger
}

// NewCacheHandler creates a new cache handler
func NewCacheHandler(cacheService services.CacheServiceInterface, logger *logrus.Logger) *CacheHandler {
	return &CacheHandler{
		cacheService: cacheService,
		logger:       logger,
	}
}

// GetStats handles cache statistics request
// @Summary Get cache statistics
// @Description Get detailed cache statistics and metrics
// @Tags Cache
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} models.ErrorResponse
// @Router /cache/stats [get]
func (h *CacheHandler) GetStats(c *gin.Context) {
	requestID := c.GetString("request_id")

	h.logger.WithField("request_id", requestID).Info("Getting cache statistics")

	stats, err := h.cacheService.GetStats(c.Request.Context())
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"error":      err.Error(),
		}).Error("Failed to get cache statistics")

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "Internal server error",
			Message:   "Failed to retrieve cache statistics",
			Code:      "CACHE_STATS_ERROR",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	// Add additional metadata
	response := map[string]interface{}{
		"stats":     stats,
		"timestamp": time.Now(),
		"health":    h.cacheService.Health(),
	}

	c.JSON(http.StatusOK, response)
}

// Clear handles cache clear request
// @Summary Clear all cache
// @Description Clear all cached CNPJ data
// @Tags Cache
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} models.ErrorResponse
// @Router /cache/clear [delete]
func (h *CacheHandler) Clear(c *gin.Context) {
	requestID := c.GetString("request_id")

	h.logger.WithField("request_id", requestID).Info("Clearing all cache")

	err := h.cacheService.Clear(c.Request.Context())
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"error":      err.Error(),
		}).Error("Failed to clear cache")

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "Internal server error",
			Message:   "Failed to clear cache",
			Code:      "CACHE_CLEAR_ERROR",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	h.logger.WithField("request_id", requestID).Info("Cache cleared successfully")

	response := map[string]interface{}{
		"message":   "Cache cleared successfully",
		"timestamp": time.Now(),
		"success":   true,
	}

	c.JSON(http.StatusOK, response)
}

// Delete handles specific cache entry deletion
// @Summary Delete specific CNPJ from cache
// @Description Delete a specific CNPJ entry from cache
// @Tags Cache
// @Param cnpj path string true "CNPJ number to delete from cache"
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /cache/{cnpj} [delete]
func (h *CacheHandler) Delete(c *gin.Context) {
	requestID := c.GetString("request_id")
	cnpjParam := c.Param("cnpj")

	// Clean and validate CNPJ
	cnpj := utils.CleanCNPJ(cnpjParam)
	if !utils.IsValidCNPJ(cnpj) {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"cnpj":       cnpjParam,
			"cleaned":    cnpj,
		}).Warn("Invalid CNPJ format for cache deletion")

		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:     "Invalid CNPJ format",
			Message:   "CNPJ must contain exactly 14 digits",
			Code:      "INVALID_CNPJ",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"request_id": requestID,
		"cnpj":       cnpj,
	}).Info("Deleting CNPJ from cache")

	// Create cache key
	cacheKey := "cnpj:" + cnpj

	// Check if key exists
	exists, err := h.cacheService.Exists(c.Request.Context(), cacheKey)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"cnpj":       cnpj,
			"error":      err.Error(),
		}).Error("Failed to check cache key existence")

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "Internal server error",
			Message:   "Failed to check cache",
			Code:      "CACHE_CHECK_ERROR",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	if !exists {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"cnpj":       cnpj,
		}).Info("CNPJ not found in cache")

		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:     "Not found",
			Message:   "CNPJ not found in cache",
			Code:      "CNPJ_NOT_IN_CACHE",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	// Delete from cache
	err = h.cacheService.Delete(c.Request.Context(), cacheKey)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"cnpj":       cnpj,
			"error":      err.Error(),
		}).Error("Failed to delete CNPJ from cache")

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "Internal server error",
			Message:   "Failed to delete from cache",
			Code:      "CACHE_DELETE_ERROR",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"request_id": requestID,
		"cnpj":       cnpj,
	}).Info("CNPJ deleted from cache successfully")

	response := map[string]interface{}{
		"message":   "CNPJ deleted from cache successfully",
		"cnpj":      utils.FormatCNPJ(cnpj),
		"timestamp": time.Now(),
		"success":   true,
	}

	c.JSON(http.StatusOK, response)
}

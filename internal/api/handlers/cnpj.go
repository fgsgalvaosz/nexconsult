package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nexconsult/cnpj-api/internal/models"
	"github.com/nexconsult/cnpj-api/internal/services"
	"github.com/nexconsult/cnpj-api/internal/utils"
	"github.com/sirupsen/logrus"
)

// CNPJHandler handles CNPJ-related requests
type CNPJHandler struct {
	cnpjService services.CNPJServiceInterface
	logger      *logrus.Logger
}

// NewCNPJHandler creates a new CNPJ handler
func NewCNPJHandler(cnpjService services.CNPJServiceInterface, logger *logrus.Logger) *CNPJHandler {
	return &CNPJHandler{
		cnpjService: cnpjService,
		logger:      logger,
	}
}

// GetCNPJ handles single CNPJ consultation
// @Summary Get CNPJ information
// @Description Retrieve detailed information about a CNPJ from Receita Federal
// @Tags CNPJ
// @Accept json
// @Produce json
// @Param cnpj path string true "CNPJ number (14 digits)" example(11222333000181)
// @Success 200 {object} models.CNPJResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 429 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /cnpj/{cnpj} [get]
func (h *CNPJHandler) GetCNPJ(c *gin.Context) {
	start := time.Now()
	requestID := c.GetString("request_id")

	// Get CNPJ from URL parameter
	cnpjParam := c.Param("cnpj")

	// Clean and validate CNPJ
	cnpj := utils.CleanCNPJ(cnpjParam)
	if !utils.IsValidCNPJ(cnpj) {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"cnpj":       cnpjParam,
			"cleaned":    cnpj,
		}).Warn("Invalid CNPJ format")

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
	}).Info("Processing CNPJ consultation")

	// Call CNPJ service
	result, err := h.cnpjService.GetCNPJ(c.Request.Context(), cnpj)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"cnpj":       cnpj,
			"error":      err.Error(),
			"duration":   time.Since(start),
		}).Error("Failed to get CNPJ information")

		// Handle different types of errors
		switch {
		case strings.Contains(err.Error(), "not found"):
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:     "CNPJ not found",
				Message:   "The requested CNPJ was not found in Receita Federal database",
				Code:      "CNPJ_NOT_FOUND",
				Timestamp: time.Now(),
				Path:      c.Request.URL.Path,
			})
		case strings.Contains(err.Error(), "timeout"):
			c.JSON(http.StatusRequestTimeout, models.ErrorResponse{
				Error:     "Request timeout",
				Message:   "The request took too long to process. Please try again later",
				Code:      "TIMEOUT",
				Timestamp: time.Now(),
				Path:      c.Request.URL.Path,
			})
		case strings.Contains(err.Error(), "captcha"):
			c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{
				Error:     "Captcha service unavailable",
				Message:   "Unable to solve captcha. Please try again later",
				Code:      "CAPTCHA_ERROR",
				Timestamp: time.Now(),
				Path:      c.Request.URL.Path,
			})
		default:
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:     "Internal server error",
				Message:   "An unexpected error occurred while processing your request",
				Code:      "INTERNAL_ERROR",
				Timestamp: time.Now(),
				Path:      c.Request.URL.Path,
			})
		}
		return
	}

	duration := time.Since(start)
	h.logger.WithFields(logrus.Fields{
		"request_id": requestID,
		"cnpj":       cnpj,
		"duration":   duration,
		"cache":      result.Cache,
	}).Info("CNPJ consultation completed successfully")

	// Set cache headers
	if result.Cache {
		c.Header("X-Cache", "HIT")
		c.Header("Cache-Control", "public, max-age=3600")
	} else {
		c.Header("X-Cache", "MISS")
		c.Header("Cache-Control", "public, max-age=3600")
	}

	c.JSON(http.StatusOK, result)
}

// GetBatchCNPJ handles batch CNPJ consultation
// @Summary Get multiple CNPJ information
// @Description Retrieve detailed information about multiple CNPJs from Receita Federal
// @Tags CNPJ
// @Accept json
// @Produce json
// @Param request body models.BatchRequest true "Batch CNPJ request"
// @Success 200 {object} models.BatchResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 429 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /cnpj/batch [post]
func (h *CNPJHandler) GetBatchCNPJ(c *gin.Context) {
	start := time.Now()
	requestID := c.GetString("request_id")

	var request models.BatchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"error":      err.Error(),
		}).Warn("Invalid batch request format")

		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:     "Invalid request format",
			Message:   err.Error(),
			Code:      "INVALID_REQUEST",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	// Validate CNPJs
	validCNPJs := make([]string, 0, len(request.CNPJs))
	for _, cnpj := range request.CNPJs {
		cleaned := utils.CleanCNPJ(cnpj)
		if utils.IsValidCNPJ(cleaned) {
			validCNPJs = append(validCNPJs, cleaned)
		}
	}

	if len(validCNPJs) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:     "No valid CNPJs provided",
			Message:   "All provided CNPJs are invalid",
			Code:      "NO_VALID_CNPJS",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	h.logger.WithFields(logrus.Fields{
		"request_id":  requestID,
		"total_cnpjs": len(request.CNPJs),
		"valid_cnpjs": len(validCNPJs),
	}).Info("Processing batch CNPJ consultation")

	// Call batch CNPJ service
	results, err := h.cnpjService.GetBatchCNPJ(c.Request.Context(), validCNPJs)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"request_id": requestID,
			"error":      err.Error(),
			"duration":   time.Since(start),
		}).Error("Failed to process batch CNPJ consultation")

		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "Internal server error",
			Message:   "An unexpected error occurred while processing batch request",
			Code:      "BATCH_ERROR",
			Timestamp: time.Now(),
			Path:      c.Request.URL.Path,
		})
		return
	}

	duration := time.Since(start)
	successCount := 0
	errorCount := 0

	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			errorCount++
		}
	}

	h.logger.WithFields(logrus.Fields{
		"request_id": requestID,
		"total":      len(results),
		"success":    successCount,
		"errors":     errorCount,
		"duration":   duration,
	}).Info("Batch CNPJ consultation completed")

	response := models.BatchResponse{
		Results:    results,
		Total:      len(results),
		Success:    successCount,
		Errors:     errorCount,
		DurationMs: duration.Milliseconds(),
		Timestamp:  time.Now(),
	}

	c.JSON(http.StatusOK, response)
}

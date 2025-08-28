package handlers

import (
	"strings"

	"nexconsult/internal/api/dto"
	"nexconsult/internal/logger"
	"nexconsult/internal/service/container"

	"github.com/gofiber/fiber/v2"
)

type ConsultaHandler struct {
	sintegraService *container.SintegraService
	logger          logger.Logger
}

// NewConsultaHandler cria uma nova instância do handler de consulta
func NewConsultaHandler(sintegraService *container.SintegraService) *ConsultaHandler {
	return &ConsultaHandler{
		sintegraService: sintegraService,
		logger:          logger.GetLogger().With(logger.String("component", "handler")),
	}
}

// ConsultaCNPJ processa requisições de consulta de CNPJ via GET
func (h *ConsultaHandler) ConsultaCNPJ(c *fiber.Ctx) error {
	// Obter CNPJ do parâmetro da URL
	cnpj := strings.TrimSpace(c.Params("cnpj"))

	// Validar CNPJ
	if cnpj == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   "Validation Error",
			Message: "CNPJ é obrigatório",
		})
	}

	// Limpar CNPJ (remover formatação)
	cnpj = h.cleanCNPJ(cnpj)

	// Validar formato do CNPJ (14 dígitos)
	if len(cnpj) != 14 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   "Validation Error",
			Message: "CNPJ deve ter 14 dígitos",
		})
	}

	h.logger.Info("Iniciando consulta para CNPJ", logger.String("cnpj", cnpj))

	// Executar scraping para obter dados completos (uma única consulta)
	data, err := h.sintegraService.ScrapeCNPJComplete(cnpj)
	if err != nil {
		h.logger.Error("Erro na consulta", logger.String("cnpj", cnpj), logger.Error(err))

		// Determinar tipo de erro e status code apropriado
		statusCode := fiber.StatusInternalServerError
		errorType := "Consultation Error"

		errorMsg := err.Error()
		if strings.Contains(errorMsg, "timeout") {
			statusCode = fiber.StatusRequestTimeout
			errorType = "Timeout Error"
			errorMsg = "A consulta demorou mais que o esperado. Tente novamente em alguns minutos."
		} else if strings.Contains(errorMsg, "CAPTCHA") {
			errorType = "CAPTCHA Error"
			errorMsg = "Erro na resolução do CAPTCHA. Tente novamente."
		} else if strings.Contains(errorMsg, "websocket") {
			statusCode = fiber.StatusServiceUnavailable
			errorType = "Connection Error"
			errorMsg = "Problema de conexão com o sistema SINTEGRA. Tente novamente."
		}

		return c.Status(statusCode).JSON(dto.ErrorResponse{
			Error:   errorType,
			Message: errorMsg,
		})
	}

	// Converter para DTO de resposta
	response := dto.ToConsultaResponseFromData(data)

	return c.JSON(dto.SuccessResponse{
		Success: true,
		Data:    response,
		Message: "Consulta realizada com sucesso",
	})
}

// cleanCNPJ remove formatação do CNPJ
func (h *ConsultaHandler) cleanCNPJ(cnpj string) string {
	cnpj = strings.ReplaceAll(cnpj, ".", "")
	cnpj = strings.ReplaceAll(cnpj, "/", "")
	cnpj = strings.ReplaceAll(cnpj, "-", "")
	cnpj = strings.ReplaceAll(cnpj, " ", "")
	return cnpj
}

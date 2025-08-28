package handlers

import (
	"log"
	"strings"

	"nexconsult/internal/api/dto"
	"nexconsult/internal/service"

	"github.com/gofiber/fiber/v2"
)

type ConsultaHandler struct {
	sintegraService *service.SintegraService
}

// NewConsultaHandler cria uma nova instância do handler de consulta
func NewConsultaHandler(sintegraService *service.SintegraService) *ConsultaHandler {
	return &ConsultaHandler{
		sintegraService: sintegraService,
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

	log.Printf("Iniciando consulta para CNPJ: %s", cnpj)

	// Executar scraping para obter HTML
	result, err := h.sintegraService.ScrapeCNPJ(cnpj)
	if err != nil {
		log.Printf("Erro na consulta: %v", err)

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

	// Verificar se houve erro na consulta
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error:   "Consultation Error",
			Message: result.Error.Error(),
		})
	}

	// Obter dados completos através de uma nova extração
	// Vamos modificar o ScrapeCNPJ para retornar os dados completos
	data, err := h.getCompleteData(cnpj)
	if err != nil {
		log.Printf("Erro ao obter dados completos: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error:   "Data Extraction Error",
			Message: err.Error(),
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

// getCompleteData executa scraping e retorna dados completos
func (h *ConsultaHandler) getCompleteData(cnpj string) (*service.SintegraData, error) {
	// Executar scraping para obter HTML
	result, err := h.sintegraService.ScrapeCNPJ(cnpj)
	if err != nil {
		return nil, err
	}

	if result.Error != nil {
		return nil, result.Error
	}

	// Como o ScrapeCNPJ já faz a extração internamente, vamos acessar os dados
	// Vou modificar o serviço para expor os dados completos
	return h.sintegraService.GetLastExtractedData(), nil
}

// cleanCNPJ remove formatação do CNPJ
func (h *ConsultaHandler) cleanCNPJ(cnpj string) string {
	cnpj = strings.ReplaceAll(cnpj, ".", "")
	cnpj = strings.ReplaceAll(cnpj, "/", "")
	cnpj = strings.ReplaceAll(cnpj, "-", "")
	cnpj = strings.ReplaceAll(cnpj, " ", "")
	return cnpj
}

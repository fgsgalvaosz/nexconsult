package handlers

import (
	"nexconsult-sintegra-ma/internal/models"
	"nexconsult-sintegra-ma/internal/service"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

// SintegraHandler gerencia os endpoints relacionados ao Sintegra MA
type SintegraHandler struct {
	service *service.SintegraService
	logger  zerolog.Logger
}

// NewSintegraHandler cria uma nova instância do handler
func NewSintegraHandler(service *service.SintegraService, logger zerolog.Logger) *SintegraHandler {
	return &SintegraHandler{
		service: service,
		logger:  logger,
	}
}

// ConsultarCNPJ realiza consulta no Sintegra MA via POST
// @Summary Consultar CNPJ no Sintegra MA
// @Description Executa consulta automatizada no Sintegra MA com resolução de CAPTCHA e extração de dados estruturados
// @Tags Sintegra
// @Accept json
// @Produce json
// @Param request body models.SintegraRequest true "Dados do CNPJ para consulta"
// @Success 200 {object} models.APIResponse{data=models.SintegraResponse} "Consulta realizada com sucesso"
// @Failure 400 {object} models.APIResponse "Requisição inválida"
// @Failure 429 {object} models.APIResponse "Rate limit excedido"
// @Failure 500 {object} models.APIResponse "Erro interno do servidor"
// @Router /api/v1/sintegra/consultar [post]
func (h *SintegraHandler) ConsultarCNPJ(c *fiber.Ctx) error {
	var req models.SintegraRequest

	// Parse JSON body
	if err := c.BodyParser(&req); err != nil {
		h.logger.Error().Err(err).Msg("❌ Erro ao parsear requisição")
		return c.Status(400).JSON(models.APIResponse{
			Success:   false,
			Error:     "Formato de requisição inválido",
			Timestamp: time.Now(),
		})
	}

	// Limpar e validar CNPJ
	req.CleanCNPJ()
	if !req.ValidateCNPJ() {
		h.logger.Warn().Str("cnpj", req.CNPJ).Msg("⚠️ CNPJ inválido")
		return c.Status(400).JSON(models.APIResponse{
			Success:   false,
			Error:     "CNPJ inválido. Deve conter 14 dígitos",
			Timestamp: time.Now(),
		})
	}

	h.logger.Info().
		Str("cnpj", req.CNPJ).
		Str("ip", c.IP()).
		Str("user_agent", c.Get("User-Agent")).
		Msg("🎯 Recebida requisição de consulta")

	// Executar consulta
	start := time.Now()
	result, err := h.service.ConsultarCNPJ(req.CNPJ)
	if err != nil {
		duration := time.Since(start)
		h.logger.Error().
			Err(err).
			Str("cnpj", req.CNPJ).
			Dur("duration", duration).
			Msg("❌ Erro na consulta")
		
		return c.Status(500).JSON(models.APIResponse{
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
	}

	duration := time.Since(start)
	h.logger.Info().
		Str("cnpj", req.CNPJ).
		Dur("duration", duration).
		Str("status", result.Status).
		Bool("captcha_solved", result.CaptchaSolved).
		Msg("✅ Consulta realizada com sucesso")

	// Retornar resultado
	return c.JSON(models.APIResponse{
		Success:   true,
		Message:   "Consulta realizada com sucesso",
		Data:      result,
		Timestamp: time.Now(),
	})
}

// ConsultarCNPJByPath consulta CNPJ via parâmetro de rota
// @Summary Consultar CNPJ via URL
// @Description Executa consulta automatizada no Sintegra MA passando CNPJ diretamente na URL
// @Tags Sintegra
// @Produce json
// @Param cnpj path string true "CNPJ (14 dígitos numéricos)" minlength(14) maxlength(14)
// @Success 200 {object} models.APIResponse{data=models.SintegraResponse} "Consulta realizada com sucesso"
// @Failure 400 {object} models.APIResponse "CNPJ inválido ou não informado"
// @Failure 429 {object} models.APIResponse "Rate limit excedido"
// @Failure 500 {object} models.APIResponse "Erro interno do servidor"
// @Router /api/v1/sintegra/consultar/{cnpj} [get]
func (h *SintegraHandler) ConsultarCNPJByPath(c *fiber.Ctx) error {
	cnpj := c.Params("cnpj")

	if cnpj == "" {
		return c.Status(400).JSON(models.APIResponse{
			Success:   false,
			Error:     "CNPJ não informado",
			Timestamp: time.Now(),
		})
	}

	// Criar request object e reutilizar lógica do método POST
	req := models.SintegraRequest{CNPJ: cnpj}
	req.CleanCNPJ()

	if !req.ValidateCNPJ() {
		h.logger.Warn().Str("cnpj", req.CNPJ).Msg("⚠️ CNPJ inválido via URL")
		return c.Status(400).JSON(models.APIResponse{
			Success:   false,
			Error:     "CNPJ inválido. Deve conter 14 dígitos",
			Timestamp: time.Now(),
		})
	}

	h.logger.Info().
		Str("cnpj", req.CNPJ).
		Str("ip", c.IP()).
		Str("method", "GET").
		Msg("🎯 Recebida consulta via URL")

	// Executar consulta
	start := time.Now()
	result, err := h.service.ConsultarCNPJ(req.CNPJ)
	if err != nil {
		duration := time.Since(start)
		h.logger.Error().
			Err(err).
			Str("cnpj", req.CNPJ).
			Dur("duration", duration).
			Msg("❌ Erro na consulta via URL")
		
		return c.Status(500).JSON(models.APIResponse{
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
	}

	duration := time.Since(start)
	h.logger.Info().
		Str("cnpj", req.CNPJ).
		Dur("duration", duration).
		Str("status", result.Status).
		Msg("✅ Consulta via URL realizada com sucesso")

	return c.JSON(models.APIResponse{
		Success:   true,
		Message:   "Consulta realizada com sucesso",
		Data:      result,
		Timestamp: time.Now(),
	})
}
package handlers

import (
	"fmt"
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
	// Limite máximo de CNPJs por consulta em lote
	maxBatchSize int
}

// NewSintegraHandler cria uma nova instância do handler
func NewSintegraHandler(service *service.SintegraService, logger zerolog.Logger) *SintegraHandler {
	return &SintegraHandler{
		service:      service,
		logger:       logger,
		maxBatchSize: 10, // Máximo de 10 CNPJs por consulta em lote
	}
}

// ConsultarCNPJ realiza consulta no Sintegra MA via POST
// @Summary Consultar CNPJ no Sintegra MA
// @Description Executa consulta automatizada no Sintegra MA com resolução de CAPTCHA e extração de dados estruturados
// @Tags Sintegra
// @Accept json
// @Produce json
// @Param request body models.SintegraRequest true "Dados do CNPJ para consulta"
// @Success 200 {object} models.StandardResponse{data=models.SintegraResponse} "Consulta realizada com sucesso"
// @Failure 400 {object} models.StandardResponse "Requisição inválida"
// @Failure 429 {object} models.StandardResponse "Rate limit excedido"
// @Failure 500 {object} models.StandardResponse "Erro interno do servidor"
// @Router /api/v1/sintegra/consultar [post]
func (h *SintegraHandler) ConsultarCNPJ(c *fiber.Ctx) error {
	var req models.SintegraRequest

	// Parse JSON body
	if err := c.BodyParser(&req); err != nil {
		h.logger.Error().Err(err).Msg("❌ Erro ao parsear requisição")
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidRequest,
			"Formato de requisição inválido",
			nil,
		))
	}

	// Limpar e validar CNPJ
	req.CleanCNPJ()
	if !req.ValidateCNPJ() {
		h.logger.Warn().Str("cnpj", req.CNPJ).Msg("⚠️ CNPJ inválido")
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidCNPJ,
			"CNPJ inválido. Deve conter 14 dígitos",
			map[string]string{"cnpj": req.CNPJ},
		))
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

		response := models.NewErrorResponse(
			models.ErrorCodeInternalError,
			err.Error(),
			map[string]string{"cnpj": req.CNPJ},
		)
		response.SetExecutionTime(duration)
		return c.Status(500).JSON(response)
	}

	duration := time.Since(start)
	h.logger.Info().
		Str("cnpj", req.CNPJ).
		Dur("duration", duration).
		Str("status", result.Status).
		Bool("captcha_solved", result.CaptchaSolved).
		Msg("✅ Consulta realizada com sucesso")

	// Retornar resultado
	response := models.NewSuccessResponse("Consulta realizada com sucesso", result)
	response.SetExecutionTime(duration)
	return c.JSON(response)
}

// ConsultarCNPJByPath consulta CNPJ via parâmetro de rota
// @Summary Consultar CNPJ via URL
// @Description Executa consulta automatizada no Sintegra MA passando CNPJ diretamente na URL
// @Tags Sintegra
// @Produce json
// @Param cnpj path string true "CNPJ (14 dígitos numéricos)" minlength(14) maxlength(14)
// @Success 200 {object} models.StandardResponse{data=models.SintegraResponse} "Consulta realizada com sucesso"
// @Failure 400 {object} models.StandardResponse "CNPJ inválido ou não informado"
// @Failure 429 {object} models.StandardResponse "Rate limit excedido"
// @Failure 500 {object} models.StandardResponse "Erro interno do servidor"
// @Router /api/v1/sintegra/consultar/{cnpj} [get]

// ConsultarCNPJEmLote realiza consulta em lote de múltiplos CNPJs
// @Summary Consultar múltiplos CNPJs em lote
// @Description Executa consulta automatizada em lote no Sintegra MA para múltiplos CNPJs simultaneamente
// @Tags Sintegra
// @Accept json
// @Produce json
// @Param request body models.BatchSintegraRequest true "Lista de CNPJs para consulta em lote (máximo 10)"
// @Success 200 {object} models.StandardResponse{data=models.BatchSintegraResponse} "Consulta em lote realizada com sucesso"
// @Failure 400 {object} models.StandardResponse "Requisição inválida ou CNPJs inválidos"
// @Failure 429 {object} models.StandardResponse "Rate limit excedido"
// @Failure 500 {object} models.StandardResponse "Erro interno do servidor"
// @Router /api/v1/sintegra/consultar-lote [post]
func (h *SintegraHandler) ConsultarCNPJEmLote(c *fiber.Ctx) error {
	var req models.BatchSintegraRequest

	// Parse JSON body
	if err := c.BodyParser(&req); err != nil {
		h.logger.Error().Err(err).Msg("❌ Erro ao parsear requisição de lote")
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidRequest,
			"Formato de requisição inválido",
			nil,
		))
	}

	// Verificar se a lista de CNPJs está vazia
	if len(req.CNPJs) == 0 {
		h.logger.Warn().Msg("⚠️ Lista de CNPJs vazia")
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidRequest,
			"A lista de CNPJs não pode estar vazia",
			nil,
		))
	}

	// Verificar se a lista de CNPJs excede o limite
	if len(req.CNPJs) > h.maxBatchSize {
		h.logger.Warn().Int("size", len(req.CNPJs)).Int("max", h.maxBatchSize).Msg("⚠️ Lista de CNPJs excede o limite")
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidRequest,
			fmt.Sprintf("A lista de CNPJs não pode exceder %d itens", h.maxBatchSize),
			map[string]interface{}{"max_size": h.maxBatchSize, "provided_size": len(req.CNPJs)},
		))
	}

	// Validar e limpar CNPJs
	validCNPJs, invalidCNPJs := req.ValidateAndCleanCNPJs()

	// Verificar se há CNPJs válidos
	if len(validCNPJs) == 0 {
		h.logger.Warn().Strs("invalid_cnpjs", invalidCNPJs).Msg("⚠️ Nenhum CNPJ válido")
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidCNPJ,
			"Nenhum CNPJ válido foi fornecido",
			map[string]interface{}{"invalid_cnpjs": invalidCNPJs},
		))
	}

	h.logger.Info().
		Int("total_cnpjs", len(validCNPJs)).
		Int("invalid_cnpjs", len(invalidCNPJs)).
		Str("ip", c.IP()).
		Str("user_agent", c.Get("User-Agent")).
		Msg("🎯 Recebida requisição de consulta em lote")

	// Executar consulta em lote
	start := time.Now()
	result := h.service.ConsultarCNPJEmLote(validCNPJs)

	// Adicionar CNPJs inválidos ao resultado
	if len(invalidCNPJs) > 0 {
		result.InvalidCNPJs = invalidCNPJs
	}

	duration := time.Since(start)
	h.logger.Info().
		Int("total", result.Total).
		Int("success", result.SuccessCount).
		Int("errors", result.ErrorCount).
		Int("invalid", len(invalidCNPJs)).
		Dur("duration", duration).
		Msg("✅ Consulta em lote concluída")

	// Retornar resultado
	response := models.NewSuccessResponse("Consulta em lote realizada com sucesso", result)
	response.SetExecutionTime(duration)
	return c.JSON(response)
}
func (h *SintegraHandler) ConsultarCNPJByPath(c *fiber.Ctx) error {
	cnpj := c.Params("cnpj")

	if cnpj == "" {
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidRequest,
			"CNPJ não informado",
			nil,
		))
	}

	// Criar request object e reutilizar lógica do método POST
	req := models.SintegraRequest{CNPJ: cnpj}
	req.CleanCNPJ()

	if !req.ValidateCNPJ() {
		h.logger.Warn().Str("cnpj", req.CNPJ).Msg("⚠️ CNPJ inválido via URL")
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidCNPJ,
			"CNPJ inválido. Deve conter 14 dígitos",
			map[string]string{"cnpj": req.CNPJ},
		))
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

		response := models.NewErrorResponse(
			models.ErrorCodeInternalError,
			err.Error(),
			map[string]string{"cnpj": req.CNPJ},
		)
		response.SetExecutionTime(duration)
		return c.Status(500).JSON(response)
	}

	duration := time.Since(start)
	h.logger.Info().
		Str("cnpj", req.CNPJ).
		Dur("duration", duration).
		Str("status", result.Status).
		Msg("✅ Consulta via URL realizada com sucesso")

	response := models.NewSuccessResponse("Consulta realizada com sucesso", result)
	response.SetExecutionTime(duration)
	return c.JSON(response)
}

// VerificarStatusConsulta verifica o status de uma consulta de CNPJ
// @Summary Verifica o status de uma consulta de CNPJ
// @Description Verifica se uma consulta de CNPJ está em andamento ou concluída
// @Tags Sintegra
// @Accept json
// @Produce json
// @Param request body models.StatusRequest true "CNPJ para verificar status"
// @Success 200 {object} models.StandardResponse{data=models.StatusResponse} "Status da consulta"
// @Failure 400 {object} models.StandardResponse "Requisição inválida"
// @Failure 500 {object} models.StandardResponse "Erro interno do servidor"
// @Router /api/v1/sintegra/status [post]
func (h *SintegraHandler) VerificarStatusConsulta(c *fiber.Ctx) error {
	var req models.StatusRequest

	// Parse JSON body
	if err := c.BodyParser(&req); err != nil {
		h.logger.Error().Err(err).Msg("❌ Erro ao parsear requisição de status")
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidRequest,
			"Formato de requisição inválido",
			nil,
		))
	}

	// Limpar e validar CNPJ
	req.CleanCNPJ()
	if !req.ValidateCNPJ() {
		h.logger.Warn().Str("cnpj", req.CNPJ).Msg("⚠️ CNPJ inválido na verificação de status")
		return c.Status(400).JSON(models.NewErrorResponse(
			models.ErrorCodeInvalidCNPJ,
			"CNPJ inválido. Deve conter 14 dígitos",
			map[string]string{"cnpj": req.CNPJ},
		))
	}

	h.logger.Info().
		Str("cnpj", req.CNPJ).
		Str("ip", c.IP()).
		Msg("🔍 Verificando status de consulta")

	// Verificar status da consulta
	result, err := h.service.VerificarStatusConsulta(req.CNPJ)
	if err != nil {
		h.logger.Error().
			Err(err).
			Str("cnpj", req.CNPJ).
			Msg("❌ Erro ao verificar status da consulta")

		return c.Status(500).JSON(models.NewErrorResponse(
			models.ErrorCodeInternalError,
			err.Error(),
			map[string]string{"cnpj": req.CNPJ},
		))
	}

	h.logger.Info().
		Str("cnpj", req.CNPJ).
		Str("status", result.Status).
		Msg("✅ Status verificado com sucesso")

	return c.JSON(models.NewSuccessResponse("Status verificado com sucesso", result))
}

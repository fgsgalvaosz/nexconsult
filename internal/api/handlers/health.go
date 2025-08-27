package handlers

import (
	"nexconsult-sintegra-ma/internal/models"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HealthHandler gerencia endpoints de saúde e status da API
type HealthHandler struct {
	startTime time.Time
}

// NewHealthHandler cria uma nova instância do handler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		startTime: time.Now(),
	}
}

// HealthCheck verifica se a API está funcionando
// @Summary Health Check
// @Description Verifica o status de saúde da API, tempo de atividade e versão
// @Tags System
// @Produce json
// @Success 200 {object} models.StandardResponse{data=models.HealthResponse} "API funcionando corretamente"
// @Router /health [get]
func (h *HealthHandler) HealthCheck(c *fiber.Ctx) error {
	uptime := time.Since(h.startTime)

	healthData := models.HealthResponse{
		Status:  "healthy",
		Service: "nexconsult-sintegra-ma",
		Version: "1.0.0",
		Uptime:  uptime.String(),
	}

	return c.JSON(models.NewSuccessResponse("API Sintegra MA está funcionando", healthData))
}

// Welcome exibe informações básicas da API
// @Summary API Information
// @Description Exibe informações básicas sobre a API, endpoints disponíveis e exemplos de uso
// @Tags System
// @Produce json
// @Success 200 {object} models.StandardResponse "Informações da API"
// @Router / [get]
func (h *HealthHandler) Welcome(c *fiber.Ctx) error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	baseURL := "http://localhost:" + port

	info := map[string]interface{}{
		"api_name":    "NexConsult Sintegra MA API",
		"version":     "1.0.0",
		"description": "API para consulta automatizada no Sintegra MA com resolução de CAPTCHA",
		"uptime":      time.Since(h.startTime).String(),
		"endpoints": map[string]interface{}{
			"health": map[string]string{
				"GET /health": "Verificar saúde da API",
			},
			"docs": map[string]string{
				"GET /docs": "Documentação da API",
			},
			"swagger": map[string]string{
				"GET /swagger/": "Interface Swagger UI",
			},
			"sintegra": map[string]string{
				"POST /api/v1/sintegra/consultar":      "Consultar CNPJ (JSON body)",
				"GET /api/v1/sintegra/consultar/:cnpj": "Consultar CNPJ via URL",
			},
		},
		"example_usage": map[string]interface{}{
			"post_request": map[string]string{
				"method":  "POST",
				"url":     baseURL + "/api/v1/sintegra/consultar",
				"body":    `{"cnpj": "38139407000177"}`,
				"headers": "Content-Type: application/json",
			},
			"get_request": map[string]string{
				"method": "GET",
				"url":    baseURL + "/api/v1/sintegra/consultar/38139407000177",
			},
		},
	}

	return c.JSON(models.NewSuccessResponse("Bem-vindo à API do Sintegra MA", info))
}

// Docs exibe documentação detalhada da API
// @Summary API Documentation
// @Description Documentação detalhada dos endpoints disponíveis, formatos de resposta e exemplos
// @Tags System
// @Produce json
// @Success 200 {object} models.StandardResponse "Documentação completa da API"
// @Router /docs [get]
func (h *HealthHandler) Docs(c *fiber.Ctx) error {
	docs := map[string]interface{}{
		"api_info": map[string]string{
			"name":        "NexConsult Sintegra MA API",
			"version":     "1.0.0",
			"description": "API REST para consulta automatizada no Sintegra do Maranhão",
		},
		"features": []string{
			"Consulta automatizada no Sintegra MA",
			"Resolução automática de reCAPTCHA v2",
			"Extração estruturada de dados",
			"Rate limiting por IP",
			"Logging estruturado",
			"CORS habilitado",
		},
		"endpoints": map[string]interface{}{
			"/health": map[string]interface{}{
				"method":      "GET",
				"description": "Verificar saúde da API",
				"response":    "Status e informações de uptime",
			},
			"/api/v1/sintegra/consultar": map[string]interface{}{
				"method":      "POST",
				"description": "Consultar CNPJ no Sintegra MA",
				"body":        `{"cnpj": "14_digits"}`,
				"response":    "Dados estruturados da empresa",
				"example":     `curl -X POST http://localhost:3000/api/v1/sintegra/consultar -H "Content-Type: application/json" -d '{"cnpj": "38139407000177"}'`,
			},
			"/api/v1/sintegra/consultar/:cnpj": map[string]interface{}{
				"method":      "GET",
				"description": "Consultar CNPJ via parâmetro de URL",
				"params":      "cnpj: string (14 dígitos)",
				"response":    "Dados estruturados da empresa",
				"example":     `curl http://localhost:3000/api/v1/sintegra/consultar/38139407000177`,
			},
		},
		"rate_limits": map[string]string{
			"limit":    "10 requisições por minuto por IP",
			"policy":   "Rate limiting baseado em IP",
			"exceeded": "HTTP 429 - Too Many Requests",
		},
		"response_format": map[string]interface{}{
			"success": map[string]interface{}{
				"success":   true,
				"message":   "Mensagem de sucesso",
				"data":      "Dados da consulta",
				"timestamp": "ISO 8601 timestamp",
			},
			"error": map[string]interface{}{
				"success":   false,
				"error":     "Mensagem de erro",
				"timestamp": "ISO 8601 timestamp",
			},
		},
	}

	return c.JSON(models.NewSuccessResponse("Documentação da API", docs))
}

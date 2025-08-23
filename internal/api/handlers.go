package api

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"nexconsult/internal/logger"
	"nexconsult/internal/types"
	"nexconsult/internal/worker"
)

// Handlers contém os handlers da API
type Handlers struct {
	workerPool *worker.WorkerPool
}

// NewHandlers cria novos handlers
func NewHandlers(workerPool *worker.WorkerPool) *Handlers {
	return &Handlers{
		workerPool: workerPool,
	}
}

// GetCNPJ godoc
// @Summary Consulta dados de um CNPJ
// @Description Consulta dados de um CNPJ na Receita Federal com resolução automática de captcha
// @Tags CNPJ
// @Accept json
// @Produce json
// @Param cnpj path string true "CNPJ (com ou sem formatação)"
// @Param cache query bool false "Usar cache (sempre false - busca direta)"
// @Success 200 {object} types.CNPJData
// @Failure 400 {object} types.ErrorResponse
// @Failure 500 {object} types.ErrorResponse
// @Router /cnpj/{cnpj} [get]
func (h *Handlers) GetCNPJ(c *fiber.Ctx) error {
	cnpj := c.Params("cnpj")
	if cnpj == "" {
		return c.Status(400).JSON(types.ErrorResponse{
			Error:   "CNPJ é obrigatório",
			Message: "Forneça um CNPJ válido",
		})
	}

	// Cria job
	job := &types.Job{
		ID:       generateJobID(),
		CNPJ:     cnpj,
		UseCache: false, // Sempre busca direta
		Result:   make(chan types.CNPJResult, 1),
	}

	// Submete job
	select {
	case h.workerPool.GetJobQueue() <- job:
		correlationID := GetCorrelationID(c)
		logger.GetGlobalLogger().WithComponent("api").WithCorrelationID(correlationID).InfoFields("Job submitted", logger.Fields{
			"cnpj":   cnpj,
			"job_id": job.ID,
		})
	case <-time.After(5 * time.Second):
		return c.Status(503).JSON(types.ErrorResponse{
			Error:   "Sistema sobrecarregado",
			Message: "Tente novamente em alguns instantes",
		})
	}

	// Aguarda resultado
	select {
	case result := <-job.Result:
		if result.Status == "success" {
			return c.JSON(result.Data)
		} else {
			return c.Status(500).JSON(types.ErrorResponse{
				Error:   "Erro na consulta",
				Message: result.Error,
			})
		}
	case <-time.After(5 * time.Minute):
		return c.Status(408).JSON(types.ErrorResponse{
			Error:   "Timeout",
			Message: "Consulta demorou mais que o esperado",
		})
	}
}

// GetStatus godoc
// @Summary Status do sistema
// @Description Retorna estatísticas e status do sistema
// @Tags Sistema
// @Accept json
// @Produce json
// @Success 200 {object} types.WorkerStats
// @Router /status [get]
func (h *Handlers) GetStatus(c *fiber.Ctx) error {
	status := h.workerPool.GetStats()
	return c.JSON(status)
}

// HealthCheck godoc
// @Summary Health check
// @Description Verifica se o sistema está funcionando
// @Tags Sistema
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func (h *Handlers) HealthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func generateJobID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(6)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

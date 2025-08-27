package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const (
	// API endpoints
	solveCaptchaSubmitURL = "https://api.solvecaptcha.com/in.php"
	solveCaptchaResultURL = "https://api.solvecaptcha.com/res.php"

	// Configurações de retry
	maxRetryAttempts = 3
	retryDelay       = 10 * time.Second
	processingDelay  = 20 * time.Second

	// Timeouts
	defaultTimeout = 30 * time.Second
	maxPollTimeout = 5 * time.Minute
	pollInterval   = 5 * time.Second

	// Status codes
	statusOK         = 1
	statusProcessing = 0
	statusError      = -1

	// Error messages
	errorNoSlotAvailable   = "ERROR_NO_SLOT_AVAILABLE"
	errorCaptchaUnsolvable = "ERROR_CAPTCHA_UNSOLVABLE"
)

// CaptchaSolverInterface define a interface para resolução de CAPTCHA
type CaptchaSolverInterface interface {
	SolveCaptcha(ctx context.Context, sitekey, url string) (string, error)
}

// CaptchaSubmitRequest representa a requisição para submissão de CAPTCHA
type CaptchaSubmitRequest struct {
	Key       string `json:"key"`
	Method    string `json:"method"`
	GoogleKey string `json:"googlekey"`
	PageURL   string `json:"pageurl"`
	JSON      string `json:"json"`
}

// CaptchaSubmitResponse representa a resposta da submissão
type CaptchaSubmitResponse struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
	Error   string `json:"error"`
}

// CaptchaResultResponse representa a resposta do resultado
type CaptchaResultResponse struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
	Error   string `json:"error"`
}

// CaptchaSolver gerencia a resolução de CAPTCHA via API
type CaptchaSolver struct {
	apiKey string
	client *http.Client
	logger zerolog.Logger
}

// NewCaptchaSolver cria uma nova instância do CaptchaSolver
func NewCaptchaSolver(apiKey string, logger zerolog.Logger) *CaptchaSolver {
	if apiKey == "" {
		logger.Warn().Msg("API key do CAPTCHA não configurada")
	}

	return &CaptchaSolver{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:       10,
				IdleConnTimeout:    30 * time.Second,
				DisableCompression: true,
			},
		},
		logger: logger,
	}
}

// SolveCaptcha resolve o CAPTCHA usando a API SolveCaptcha
func (c *CaptchaSolver) SolveCaptcha(ctx context.Context, sitekey, pageURL string) (string, error) {
	if err := c.validateInputs(sitekey, pageURL); err != nil {
		return "", err
	}

	c.logger.Info().
		Str("sitekey", sitekey).
		Str("url", pageURL).
		Msg("Iniciando resolução do CAPTCHA via API")

	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		taskID, err := c.submitCaptcha(ctx, sitekey, pageURL)
		if err != nil {
			if c.shouldRetry(err, attempt) {
				c.logger.Warn().
					Int("attempt", attempt).
					Err(err).
					Msg("Tentando novamente após erro")

				if waitErr := c.waitWithContext(ctx, retryDelay); waitErr != nil {
			return "", waitErr
		}
				continue
			}
			return "", fmt.Errorf("erro na submissão do CAPTCHA: %w", err)
		}

		// Aguardar processamento inicial
		if err := c.waitWithContext(ctx, processingDelay); err != nil {
			return "", err
		}

		return c.getCaptchaResult(ctx, taskID)
	}

	return "", fmt.Errorf("falha após %d tentativas", maxRetryAttempts)
}

// validateInputs valida os parâmetros de entrada
func (c *CaptchaSolver) validateInputs(sitekey, pageURL string) error {
	if c.apiKey == "" {
		return fmt.Errorf("API key não configurada")
	}
	if sitekey == "" {
		return fmt.Errorf("sitekey não pode ser vazio")
	}
	if pageURL == "" {
		return fmt.Errorf("pageURL não pode ser vazio")
	}
	return nil
}

// shouldRetry determina se deve tentar novamente baseado no erro
func (c *CaptchaSolver) shouldRetry(err error, attempt int) bool {
	if attempt >= maxRetryAttempts {
		return false
	}

	errorMsg := err.Error()
	return strings.Contains(errorMsg, errorNoSlotAvailable) ||
		strings.Contains(errorMsg, "temporariamente indisponível") ||
		strings.Contains(errorMsg, "timeout")
}

// waitWithContext aguarda com possibilidade de cancelamento
func (c *CaptchaSolver) waitWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// submitCaptcha submete um CAPTCHA para resolução
func (c *CaptchaSolver) submitCaptcha(ctx context.Context, googleKey, pageURL string) (string, error) {
	c.logger.Info().
		Str("googlekey", googleKey).
		Str("pageurl", pageURL).
		Msg("Submetendo CAPTCHA para resolução")

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	// Adicionar campos do formulário
	fields := map[string]string{
		"key":       c.apiKey,
		"method":    "userrecaptcha",
		"googlekey": googleKey,
		"pageurl":   pageURL,
		"json":      "1",
	}

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return "", fmt.Errorf("erro ao adicionar campo %s: %w", key, err)
		}
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("erro ao preparar payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", solveCaptchaSubmitURL, payload)
	if err != nil {
		return "", fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", "NexConsult-API/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status HTTP inválido: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta: %w", err)
	}

	return c.parseSubmitResponse(body)
}

// parseSubmitResponse processa a resposta da submissão
func (c *CaptchaSolver) parseSubmitResponse(body []byte) (string, error) {
	bodyStr := string(body)
	c.logger.Info().Str("submit_response", bodyStr).Msg("Resposta da submissão")

	if len(body) == 0 {
		return "", fmt.Errorf("resposta vazia da API")
	}

	// Tentar parsear como JSON primeiro
	var response CaptchaSubmitResponse
	if err := json.Unmarshal(body, &response); err == nil {
		return c.handleJSONSubmitResponse(&response)
	}

	// Se não conseguir parsear como JSON, tentar como texto simples
	return c.handleTextSubmitResponse(body)
}

func (c *CaptchaSolver) handleJSONSubmitResponse(resp *CaptchaSubmitResponse) (string, error) {
	switch resp.Status {
	case statusOK:
		c.logger.Info().Str("task_id", resp.Request).Msg("CAPTCHA submetido com sucesso")
		return resp.Request, nil
	case statusError:
		c.logger.Error().Str("error", resp.Error).Msg("Erro na submissão do CAPTCHA")
		return "", c.mapAPIError(resp.Error)
	default:
		return "", fmt.Errorf("status desconhecido: %d", resp.Status)
	}
}

func (c *CaptchaSolver) handleTextSubmitResponse(body []byte) (string, error) {
	responseText := strings.TrimSpace(string(body))

	if strings.HasPrefix(responseText, "OK|") {
		taskID := strings.TrimPrefix(responseText, "OK|")
		if taskID == "" {
			return "", fmt.Errorf("task ID vazio na resposta")
		}
		c.logger.Info().Str("task_id", taskID).Msg("CAPTCHA submetido com sucesso")
		return taskID, nil
	}

	c.logger.Error().Str("response", responseText).Msg("Resposta inesperada da API")
	return "", c.mapAPIError(responseText)
}

func (c *CaptchaSolver) mapAPIError(errorMsg string) error {
	switch {
	case strings.Contains(errorMsg, errorNoSlotAvailable):
		return fmt.Errorf("serviço temporariamente indisponível: %s", errorMsg)
	case strings.Contains(errorMsg, "ERROR_WRONG_USER_KEY"):
		return fmt.Errorf("API key inválida: %s", errorMsg)
	case strings.Contains(errorMsg, "ERROR_KEY_DOES_NOT_EXIST"):
		return fmt.Errorf("API key não existe: %s", errorMsg)
	case strings.Contains(errorMsg, "ERROR_ZERO_BALANCE"):
		return fmt.Errorf("saldo insuficiente: %s", errorMsg)
	default:
		return fmt.Errorf("erro da API: %s", errorMsg)
	}
}

// getCaptchaResult obtém o resultado do CAPTCHA com polling
func (c *CaptchaSolver) getCaptchaResult(ctx context.Context, taskID string) (string, error) {
	c.logger.Info().Str("task_id", taskID).Msg("Obtendo resultado do CAPTCHA")

	if taskID == "" {
		return "", fmt.Errorf("task ID não pode ser vazio")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, maxPollTimeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return "", fmt.Errorf("timeout ao aguardar resultado do CAPTCHA após %v", maxPollTimeout)
		case <-ticker.C:
			result, err := c.checkCaptchaStatus(timeoutCtx, taskID)
			if err != nil {
				return "", err
			}

			if result != "" {
				c.logger.Info().Str("result", result).Msg("CAPTCHA resolvido com sucesso")
				return result, nil
			}
		}
	}
}

// checkCaptchaStatus verifica o status atual do CAPTCHA
func (c *CaptchaSolver) checkCaptchaStatus(ctx context.Context, taskID string) (string, error) {
	url := fmt.Sprintf("%s?key=%s&action=get&id=%s&json=1",
		solveCaptchaResultURL, c.apiKey, taskID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("erro ao criar requisição de status: %w", err)
	}

	req.Header.Set("User-Agent", "NexConsult-API/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro na requisição de status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status HTTP inválido ao verificar CAPTCHA: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta de status: %w", err)
	}

	return c.parseStatusResponse(body)
}

// parseStatusResponse processa a resposta do status
func (c *CaptchaSolver) parseStatusResponse(body []byte) (string, error) {
	bodyStr := string(body)
	c.logger.Debug().Str("status_response", bodyStr).Msg("Resposta do status")

	if len(body) == 0 {
		return "", fmt.Errorf("resposta vazia ao verificar status")
	}

	// Tentar parsear como JSON primeiro
	var response CaptchaResultResponse
	if err := json.Unmarshal(body, &response); err == nil {
		return c.handleJSONStatusResponse(&response)
	}

	// Se não conseguir parsear como JSON, tentar como texto simples
	return c.handleTextStatusResponse(body)
}

func (c *CaptchaSolver) handleJSONStatusResponse(resp *CaptchaResultResponse) (string, error) {
	switch resp.Status {
	case statusOK:
		if resp.Request == "" {
			return "", fmt.Errorf("resultado vazio recebido")
		}
		return resp.Request, nil
	case statusProcessing:
		// Ainda processando
		return "", nil
	case statusError:
		c.logger.Error().Str("error", resp.Error).Msg("Erro no resultado do CAPTCHA")
		if strings.Contains(resp.Error, errorCaptchaUnsolvable) {
			return "", fmt.Errorf("CAPTCHA não pode ser resolvido: %s", resp.Error)
		}
		return "", fmt.Errorf("erro da API: %s", resp.Error)
	default:
		return "", fmt.Errorf("status desconhecido: %d", resp.Status)
	}
}

func (c *CaptchaSolver) handleTextStatusResponse(body []byte) (string, error) {
	responseText := strings.TrimSpace(string(body))

	switch {
	case strings.HasPrefix(responseText, "OK|"):
		result := strings.TrimPrefix(responseText, "OK|")
		if result == "" {
			return "", fmt.Errorf("resultado vazio na resposta")
		}
		return result, nil
	case responseText == "CAPCHA_NOT_READY":
		// Ainda processando
		return "", nil
	case strings.HasPrefix(responseText, "ERROR_"):
		return "", c.mapAPIError(responseText)
	default:
		c.logger.Error().Str("response", responseText).Msg("Resposta inesperada do status")
		return "", fmt.Errorf("resposta inesperada: %s", responseText)
	}
}

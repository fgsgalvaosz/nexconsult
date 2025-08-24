package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"nexconsult/internal/logger"
)

// CachedToken representa um token em cache
type CachedToken struct {
	Token     string
	ExpiresAt time.Time
	SiteKey   string
	PageURL   string
}

// SolveCaptchaClient cliente para API do SolveCaptcha
type SolveCaptchaClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter
	maxRetries int
	timeout    time.Duration
	mu         sync.RWMutex
	stats      CaptchaStats

	// Cache de tokens
	tokenCache map[string]*CachedToken
	cacheTTL   time.Duration

	// Pool de workers para paralelização
	workerPool chan struct{}
}

// CaptchaStats estatísticas do cliente captcha
type CaptchaStats struct {
	TotalRequests   int64         `json:"total_requests"`
	SuccessRequests int64         `json:"success_requests"`
	FailedRequests  int64         `json:"failed_requests"`
	AverageTime     time.Duration `json:"average_time"`
	LastRequest     time.Time     `json:"last_request"`
}

// CaptchaResponse resposta da API SolveCaptcha
type CaptchaResponse struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
	Error   string `json:"error_text,omitempty"`
}

// NewSolveCaptchaClient cria novo cliente SolveCaptcha
func NewSolveCaptchaClient(apiKey string) *SolveCaptchaClient {
	return &SolveCaptchaClient{
		apiKey:  apiKey,
		baseURL: "https://api.solvecaptcha.com",
		httpClient: &http.Client{
			Timeout: 15 * time.Second, // Reduzido para requests mais rápidos
			Transport: &http.Transport{
				MaxIdleConns:        20,               // Aumentado para melhor reutilização
				MaxIdleConnsPerHost: 20,               // Aumentado para melhor reutilização
				IdleConnTimeout:     60 * time.Second, // Aumentado para manter conexões
			},
		},
		limiter:    rate.NewLimiter(rate.Every(1*time.Second), 2), // 2 requests por segundo (otimizado)
		maxRetries: 2,                                             // Reduzido para falhar mais rápido
		timeout:    240 * time.Second,                             // 4 minutos timeout (reduzido)
		stats:      CaptchaStats{},
	}
}

// SolveHCaptcha resolve hCaptcha e retorna o token
func (c *SolveCaptchaClient) SolveHCaptcha(sitekey, pageURL string) (string, error) {
	start := time.Now()

	c.mu.Lock()
	c.stats.TotalRequests++
	c.stats.LastRequest = start
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.stats.AverageTime = (c.stats.AverageTime + time.Since(start)) / 2
		c.mu.Unlock()
	}()

	var lastErr error

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			// Backoff exponencial: 2^attempt * 2 segundos (2s, 4s, 8s...)
			backoffDuration := time.Duration(1<<uint(attempt)) * 2 * time.Second

			logger.GetGlobalLogger().WithComponent("captcha").WarnFields("Retrying captcha resolution", logger.Fields{
				"attempt":         attempt + 1,
				"sitekey":         sitekey,
				"backoff_seconds": backoffDuration.Seconds(),
			})

			time.Sleep(backoffDuration)
		}

		token, err := c.solveCaptchaAttempt(sitekey, pageURL)
		if err == nil {
			c.mu.Lock()
			c.stats.SuccessRequests++
			c.mu.Unlock()

			logger.GetGlobalLogger().WithComponent("captcha").InfoFields("Captcha resolved successfully", logger.Fields{
				"duration": time.Since(start),
				"attempt":  attempt + 1,
			})

			return token, nil
		}

		lastErr = err
		logger.GetGlobalLogger().WithComponent("captcha").ErrorFields("Captcha resolution failed", logger.Fields{
			"error":   err.Error(),
			"attempt": attempt + 1,
		})
	}

	c.mu.Lock()
	c.stats.FailedRequests++
	c.mu.Unlock()

	return "", fmt.Errorf("failed to solve captcha after %d attempts: %v", c.maxRetries, lastErr)
}

// solveCaptchaAttempt executa uma tentativa de resolução
func (c *SolveCaptchaClient) solveCaptchaAttempt(sitekey, pageURL string) (string, error) {
	// Rate limiting
	if err := c.limiter.Wait(context.TODO()); err != nil {
		return "", fmt.Errorf("rate limiter error: %v", err)
	}

	// Submete captcha
	captchaID, err := c.submitCaptcha(sitekey, pageURL)
	if err != nil {
		return "", fmt.Errorf("submit error: %v", err)
	}

	logger.GetGlobalLogger().WithComponent("captcha").InfoFields("Captcha submitted", logger.Fields{"captcha_id": captchaID})

	// Aguarda resolução
	token, err := c.waitForSolution(captchaID)
	if err != nil {
		return "", fmt.Errorf("wait error: %v", err)
	}

	return token, nil
}

// submitCaptcha submete captcha para resolução
func (c *SolveCaptchaClient) submitCaptcha(sitekey, pageURL string) (string, error) {
	data := url.Values{
		"key":     {c.apiKey},
		"method":  {"hcaptcha"},
		"sitekey": {sitekey},
		"pageurl": {pageURL},
		"json":    {"1"},
	}

	resp, err := c.httpClient.PostForm(c.baseURL+"/in.php", data)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	var result CaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %v", err)
	}

	if result.Status != 1 {
		return "", fmt.Errorf("API error: %s", result.Request)
	}

	return result.Request, nil
}

// waitForSolution aguarda a resolução do captcha
func (c *SolveCaptchaClient) waitForSolution(captchaID string) (string, error) {
	start := time.Now()
	ticker := time.NewTicker(2 * time.Second) // Reduzido para verificar mais frequentemente
	defer ticker.Stop()

	timeout := time.After(c.timeout)

	for {
		select {
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for captcha solution")

		case <-ticker.C:
			// Rate limiting para verificações
			if err := c.limiter.Wait(context.TODO()); err != nil {
				continue
			}

			token, status, err := c.checkSolution(captchaID)
			if err != nil {
				logger.GetGlobalLogger().WithComponent("captcha").WarnFields("Error checking captcha solution", logger.Fields{"error": err.Error()})
				continue
			}

			switch status {
			case "READY":
				logger.GetGlobalLogger().WithComponent("captcha").InfoFields("Captcha solution ready", logger.Fields{
					"captcha_id": captchaID,
					"duration":   time.Since(start),
				})
				return token, nil

			case "CAPCHA_NOT_READY":
				logger.GetGlobalLogger().WithComponent("captcha").DebugFields("Captcha not ready yet", logger.Fields{
					"captcha_id": captchaID,
					"elapsed":    time.Since(start),
				})
				continue

			default:
				return "", fmt.Errorf("captcha resolution failed: %s", status)
			}
		}
	}
}

// checkSolution verifica o status da resolução
func (c *SolveCaptchaClient) checkSolution(captchaID string) (string, string, error) {
	reqURL := fmt.Sprintf("%s/res.php?key=%s&action=get&id=%s&json=1",
		c.baseURL, c.apiKey, captchaID)

	resp, err := c.httpClient.Get(reqURL)
	if err != nil {
		return "", "", fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response: %v", err)
	}

	var result CaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("failed to parse JSON: %v", err)
	}

	if result.Status == 1 {
		return result.Request, "READY", nil
	}

	return "", result.Request, nil
}

// GetStats retorna estatísticas do cliente
func (c *SolveCaptchaClient) GetStats() CaptchaStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// IsHealthy verifica se o cliente está saudável
func (c *SolveCaptchaClient) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Considera saudável se:
	// 1. Teve pelo menos uma requisição bem-sucedida OU
	// 2. Não teve requisições ainda OU
	// 3. Taxa de sucesso > 50%
	if c.stats.TotalRequests == 0 {
		return true
	}

	if c.stats.SuccessRequests == 0 {
		return false
	}

	successRate := float64(c.stats.SuccessRequests) / float64(c.stats.TotalRequests)
	return successRate > 0.5
}

// Reset reseta as estatísticas
func (c *SolveCaptchaClient) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats = CaptchaStats{}
}

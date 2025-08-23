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

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

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
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		limiter:    rate.NewLimiter(rate.Every(2*time.Second), 1), // 1 request a cada 2 segundos
		maxRetries: 3,
		timeout:    300 * time.Second, // 5 minutos timeout para resolução
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
			logrus.WithFields(logrus.Fields{
				"attempt": attempt + 1,
				"sitekey": sitekey,
			}).Warn("Retrying captcha resolution")

			time.Sleep(time.Duration(attempt) * 5 * time.Second)
		}

		token, err := c.solveCaptchaAttempt(sitekey, pageURL)
		if err == nil {
			c.mu.Lock()
			c.stats.SuccessRequests++
			c.mu.Unlock()

			logrus.WithFields(logrus.Fields{
				"duration": time.Since(start),
				"attempt":  attempt + 1,
			}).Info("Captcha resolved successfully")

			return token, nil
		}

		lastErr = err
		logrus.WithFields(logrus.Fields{
			"error":   err.Error(),
			"attempt": attempt + 1,
		}).Error("Captcha resolution failed")
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

	logrus.WithField("captcha_id", captchaID).Info("Captcha submitted")

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
	ticker := time.NewTicker(3 * time.Second)
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
				logrus.WithError(err).Warn("Error checking captcha solution")
				continue
			}

			switch status {
			case "READY":
				logrus.WithFields(logrus.Fields{
					"captcha_id": captchaID,
					"duration":   time.Since(start),
				}).Info("Captcha solution ready")
				return token, nil

			case "CAPCHA_NOT_READY":
				logrus.WithFields(logrus.Fields{
					"captcha_id": captchaID,
					"elapsed":    time.Since(start),
				}).Debug("Captcha not ready yet")
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

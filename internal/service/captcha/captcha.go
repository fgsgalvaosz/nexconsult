package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"nexconsult/internal/logger"

	"github.com/chromedp/chromedp"
)

// CaptchaResult representa o resultado da API de CAPTCHA
type CaptchaResult struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
}

// CaptchaResolver gerencia a resolução de CAPTCHAs
type CaptchaResolver struct {
	apiKey     string
	httpClient *http.Client
	logger     logger.Logger
}

// NewCaptchaResolver cria uma nova instância do resolvedor de CAPTCHA
func NewCaptchaResolver(apiKey string, httpClient *http.Client) *CaptchaResolver {
	return &CaptchaResolver{
		apiKey:     apiKey,
		httpClient: httpClient,
		logger:     logger.GetLogger().With(logger.String("component", "captcha")),
	}
}

// ResolveCaptcha resolve um CAPTCHA reCAPTCHA v2
func (c *CaptchaResolver) ResolveCaptcha(ctx context.Context, siteKey, pageURL string) (string, error) {
	c.logger.Debug("Iniciando resolução do CAPTCHA",
		logger.String("siteKey", siteKey),
		logger.String("pageURL", pageURL))

	// Enviar CAPTCHA para resolução
	captchaID, err := c.submitCaptcha(siteKey, pageURL)
	if err != nil {
		return "", fmt.Errorf("erro ao enviar CAPTCHA: %v", err)
	}

	c.logger.Debug("CAPTCHA enviado com sucesso",
		logger.String("captchaID", captchaID))

	// Aguardar resolução
	token, err := c.waitForCaptchaSolution(ctx, captchaID)
	if err != nil {
		return "", fmt.Errorf("erro ao aguardar resolução: %v", err)
	}

	c.logger.Debug("CAPTCHA resolvido com sucesso",
		logger.String("captchaID", captchaID))

	return token, nil
}

// submitCaptcha envia o CAPTCHA para a API SolveCaptcha
func (c *CaptchaResolver) submitCaptcha(siteKey, pageURL string) (string, error) {
	c.logger.Debug("Enviando CAPTCHA para resolução...")

	data := url.Values{}
	data.Set("key", c.apiKey)
	data.Set("method", "userrecaptcha")
	data.Set("googlekey", siteKey)
	data.Set("pageurl", pageURL)
	data.Set("json", "1")

	resp, err := c.httpClient.PostForm("https://api.solvecaptcha.com/in.php", data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	c.logger.Debug("Resposta da API in.php",
		logger.String("response", string(body)))

	var result CaptchaResult
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("erro ao decodificar resposta: %v", err)
	}

	if result.Status != 1 {
		return "", fmt.Errorf("erro na API: %s", result.Request)
	}

	return result.Request, nil
}

// waitForCaptchaSolution aguarda a resolução do CAPTCHA
func (c *CaptchaResolver) waitForCaptchaSolution(ctx context.Context, captchaID string) (string, error) {
	c.logger.Debug("Aguardando resolução do CAPTCHA",
		logger.String("captchaID", captchaID))

	maxAttempts := 60
	baseDelay := 3 * time.Second
	maxDelay := 10 * time.Second

	for i := 0; i < maxAttempts; i++ {
		// Verificar se o contexto foi cancelado
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Calcular delay com backoff exponencial limitado
		delay := baseDelay
		if i > 5 {
			multiplier := 1.5
			if i > 15 {
				multiplier = 2.0
			}
			delay = time.Duration(float64(baseDelay) * multiplier)
			if delay > maxDelay {
				delay = maxDelay
			}
		}

		// Log de progresso
		if i%5 == 0 || i < 5 {
			c.logger.Debug("Aguardando CAPTCHA...",
				logger.Int("tentativa", i+1),
				logger.Int("maxTentativas", maxAttempts),
				logger.Duration("proximoCheck", delay))
		}

		// Aguardar antes da próxima verificação
		time.Sleep(delay)

		// Verificar resultado
		result, err := c.checkCaptchaResult(captchaID)
		if err != nil {
			c.logger.Warn("Erro ao verificar resultado do CAPTCHA",
				logger.String("captchaID", captchaID),
				logger.Error(err))
			continue
		}

		c.logger.Debug("Resposta da API res.php",
			logger.String("response", fmt.Sprintf(`{"status":%d,"request":"%s"}`, result.Status, result.Request)))

		if result.Status == 1 {
			c.logger.Debug("CAPTCHA resolvido com sucesso",
				logger.Int("tentativa", i+1))
			return result.Request, nil
		}

		if result.Request != "CAPCHA_NOT_READY" {
			return "", fmt.Errorf("erro no resultado do CAPTCHA: %s", result.Request)
		}
	}

	return "", fmt.Errorf("timeout: CAPTCHA não foi resolvido após %d tentativas", maxAttempts)
}

// checkCaptchaResult verifica o resultado do CAPTCHA
func (c *CaptchaResolver) checkCaptchaResult(captchaID string) (*CaptchaResult, error) {
	url := fmt.Sprintf("https://api.solvecaptcha.com/res.php?key=%s&action=get&id=%s&json=1", c.apiKey, captchaID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %v", err)
	}

	var result CaptchaResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("erro ao decodificar resposta: %v", err)
	}

	return &result, nil
}

// GetSiteKey obtém a site key do reCAPTCHA da página
func (c *CaptchaResolver) GetSiteKey(ctx context.Context) (string, error) {
	var siteKey string
	err := chromedp.Run(ctx,
		chromedp.AttributeValue(`div[data-sitekey]`, "data-sitekey", &siteKey, nil, chromedp.ByQuery),
	)
	if err != nil {
		return "", fmt.Errorf("erro ao obter site key: %v", err)
	}

	c.logger.Debug("Site key obtida", logger.String("siteKey", siteKey))
	return siteKey, nil
}

// IsCaptchaPresent verifica se há CAPTCHA na página
func (c *CaptchaResolver) IsCaptchaPresent(ctx context.Context) bool {
	ctxTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	
	err := chromedp.Run(ctxTimeout, chromedp.WaitVisible(`iframe[src*="recaptcha"]`, chromedp.ByQuery))
	if err == nil {
		c.logger.Debug("reCAPTCHA iframe encontrado")
		return true
	}
	return false
}

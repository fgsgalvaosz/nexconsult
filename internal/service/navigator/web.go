package navigator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nexconsult/internal/config"
	"nexconsult/internal/logger"

	"github.com/chromedp/chromedp"
)

// WebNavigator gerencia navegação e interação com páginas web
type WebNavigator struct {
	config *config.Config
	logger logger.Logger
}

// NewWebNavigator cria uma nova instância do navegador web
func NewWebNavigator(cfg *config.Config) *WebNavigator {
	return &WebNavigator{
		config: cfg,
		logger: logger.GetLogger().With(logger.String("component", "navigator")),
	}
}

// NavigateToSintegra navega para a página do SINTEGRA
func (w *WebNavigator) NavigateToSintegra(ctx context.Context) error {
	w.logger.Debug("Navegando para SINTEGRA", logger.String("url", w.config.SintegraURL))

	return chromedp.Run(ctx,
		chromedp.Navigate(w.config.SintegraURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	)
}

// FillCNPJForm preenche o formulário com o CNPJ
func (w *WebNavigator) FillCNPJForm(ctx context.Context, cnpj string) error {
	w.logger.Debug("Preenchendo formulário CNPJ", logger.String("cnpj", cnpj))

	return chromedp.Run(ctx,
		// Clicar no radio button CPF/CNPJ
		chromedp.Click(`td:nth-of-type(2) > label`, chromedp.ByQuery),
		chromedp.Sleep(200*time.Millisecond),
		// Aguardar campo aparecer e preencher
		chromedp.WaitVisible(`#form1\:cpfCnpj`, chromedp.ByQuery),
		chromedp.Clear(`#form1\:cpfCnpj`, chromedp.ByQuery),
		chromedp.SendKeys(`#form1\:cpfCnpj`, cnpj, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
	)
}

// SubmitConsultation submete a consulta e valida o resultado
func (w *WebNavigator) SubmitConsultation(ctx context.Context) (string, error) {
	w.logger.Debug("Submetendo consulta")

	var currentURL string
	err := chromedp.Run(ctx,
		// Clicar no botão consultar
		chromedp.Click(`#form1\:pnlPrincipal4 input:nth-of-type(2)`, chromedp.ByQuery),
		// Aguardar navegação
		chromedp.Sleep(2*time.Second),
		// Verificar URL atual
		chromedp.Location(&currentURL),
	)

	if err != nil {
		return "", fmt.Errorf("erro ao submeter consulta: %v", err)
	}

	w.logger.Debug("Consulta submetida", logger.String("currentURL", currentURL))
	return currentURL, nil
}

// ValidateConsultationResult valida se a consulta foi bem-sucedida
func (w *WebNavigator) ValidateConsultationResult(currentURL string) bool {
	isValid := strings.Contains(currentURL, "consultaSintegraResultadoListaConsulta.jsf")
	
	w.logger.Debug("Validando resultado da consulta",
		logger.String("url", currentURL),
		logger.Bool("valid", isValid))
	
	return isValid
}

// GetErrorPageHTML obtém o HTML da página de erro
func (w *WebNavigator) GetErrorPageHTML(ctx context.Context) (string, error) {
	w.logger.Debug("Obtendo HTML da página de erro")

	var errorHTML string
	err := chromedp.Run(ctx,
		chromedp.Sleep(1*time.Second),
		chromedp.OuterHTML(`html`, &errorHTML, chromedp.ByQuery),
	)

	if err != nil {
		return "", fmt.Errorf("erro ao obter HTML da página de erro: %v", err)
	}

	return errorHTML, nil
}

// NavigateToResultsList navega para a página de lista de resultados
func (w *WebNavigator) NavigateToResultsList(ctx context.Context) error {
	w.logger.Debug("Navegando para lista de resultados")

	return chromedp.Run(ctx,
		// Aguardar página de lista carregar
		chromedp.WaitVisible(`#j_id6\:pnlCadastro`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			w.logger.Debug("Página de lista carregada, clicando no ícone de consulta")
			return nil
		}),
		// Clicar no ícone de consulta para ir para página de detalhes
		chromedp.Click(`#j_id6\:pnlCadastro img`, chromedp.ByQuery),
		// Aguardar navegação para página de detalhes
		chromedp.Sleep(2*time.Second),
		// Aguardar página de detalhes carregar
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
	)
}

// ExtractPageHTML extrai o HTML completo da página atual
func (w *WebNavigator) ExtractPageHTML(ctx context.Context) (string, error) {
	w.logger.Debug("Extraindo HTML da página")

	var htmlContent string
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			w.logger.Debug("Página de detalhes carregada, extraindo HTML")
			return nil
		}),
		chromedp.OuterHTML(`html`, &htmlContent, chromedp.ByQuery),
	)

	if err != nil {
		return "", fmt.Errorf("erro ao extrair HTML: %v", err)
	}

	w.logger.Debug("HTML extraído com sucesso",
		logger.Int("tamanho", len(htmlContent)))

	return htmlContent, nil
}

// WaitForCaptcha aguarda elementos de CAPTCHA aparecerem
func (w *WebNavigator) WaitForCaptcha(ctx context.Context) error {
	w.logger.Debug("Aguardando elementos de CAPTCHA")

	return chromedp.Run(ctx,
		chromedp.Sleep(1*time.Second),
		chromedp.ActionFunc(func(ctx context.Context) error {
			w.logger.Debug("Procurando por elementos de CAPTCHA")
			
			// Verificação do reCAPTCHA
			var found bool
			ctxTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			
			err := chromedp.Run(ctxTimeout, chromedp.WaitVisible(`iframe[src*="recaptcha"]`, chromedp.ByQuery))
			if err == nil {
				found = true
				w.logger.Debug("reCAPTCHA iframe encontrado")
			} else {
				// Tentar outros seletores
				err = chromedp.Run(ctx, chromedp.WaitVisible(`[data-sitekey]`, chromedp.ByQuery))
				if err == nil {
					found = true
					w.logger.Debug("Elemento com data-sitekey encontrado")
				}
			}
			
			if !found {
				return fmt.Errorf("nenhum elemento de CAPTCHA encontrado")
			}
			
			return nil
		}),
	)
}

// InjectCaptchaToken injeta o token do CAPTCHA na página
func (w *WebNavigator) InjectCaptchaToken(ctx context.Context, token string) error {
	w.logger.Debug("Injetando token do CAPTCHA",
		logger.String("token", token[:50]+"..."))

	script := fmt.Sprintf(`
		var textarea = document.getElementById('g-recaptcha-response');
		if (!textarea) {
			textarea = document.createElement('textarea');
			textarea.id = 'g-recaptcha-response';
			textarea.name = 'g-recaptcha-response';
			textarea.style.display = 'none';
			document.body.appendChild(textarea);
		}
		textarea.value = '%s';
		if (typeof grecaptcha !== 'undefined' && grecaptcha.getResponse) {
			grecaptcha.getResponse = function() { return '%s'; };
		}
	`, token, token)

	err := chromedp.Run(ctx, chromedp.Evaluate(script, nil))
	if err != nil {
		return fmt.Errorf("erro ao injetar token: %v", err)
	}

	w.logger.Debug("Token do CAPTCHA injetado com sucesso")
	return nil
}

// GetSiteKey obtém a site key do reCAPTCHA
func (w *WebNavigator) GetSiteKey(ctx context.Context) (string, error) {
	w.logger.Debug("Obtendo site key do reCAPTCHA")

	var siteKey string
	err := chromedp.Run(ctx,
		chromedp.AttributeValue(`[data-sitekey]`, "data-sitekey", &siteKey, nil, chromedp.ByQuery),
	)
	
	if err != nil {
		return "", fmt.Errorf("erro ao obter site key: %v", err)
	}

	w.logger.Debug("Site key obtida", logger.String("siteKey", siteKey))
	return siteKey, nil
}

// ExecuteWithRetry executa uma ação com retry
func (w *WebNavigator) ExecuteWithRetry(ctx context.Context, fn func(context.Context) error, maxRetries int) error {
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		w.logger.Debug("Executando ação com retry",
			logger.Int("tentativa", attempt),
			logger.Int("maxTentativas", maxRetries))

		err := fn(ctx)
		if err == nil {
			if attempt > 1 {
				w.logger.Info("Ação executada com sucesso após retry",
					logger.Int("tentativa", attempt))
			}
			return nil
		}

		lastErr = err
		w.logger.Warn("Erro na execução",
			logger.Int("tentativa", attempt),
			logger.Error(err))

		if attempt == maxRetries {
			return err
		}

		// Aguardar antes da próxima tentativa
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	return lastErr
}

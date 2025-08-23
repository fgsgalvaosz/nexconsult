package browser

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"nexconsult/internal/captcha"
	"nexconsult/internal/logger"
	"nexconsult/internal/types"
)

// Constantes de configuração
const (
	DefaultMaxIdleTime    = 30 * time.Minute
	DefaultPageTimeout    = 45 * time.Second
	DefaultElementTimeout = 10 * time.Second
	DefaultViewportWidth  = 1200
	DefaultViewportHeight = 800

	// URLs da Receita Federal
	ReceitaBaseURL    = "https://solucoes.receita.fazenda.gov.br"
	ReceitaCNPJURL    = ReceitaBaseURL + "/Servicos/cnpjreva/Cnpjreva_Solicitacao.asp"
	ReceitaCaptchaURL = ReceitaBaseURL + "/Servicos/cnpjreva/captcha.asp"
)

// Recursos bloqueados para performance
var blockedResources = []string{"*.css", "*.png", "*.jpg", "*.gif", "*.svg", "*.ico"}

// BrowserManager gerencia instâncias de browser
type BrowserManager struct {
	browsers    []*rod.Browser
	mu          sync.RWMutex
	index       int
	size        int
	headless    bool
	inUse       []bool      // Track which browsers are in use
	lastUsed    []time.Time // Track last usage for cleanup
	maxIdleTime time.Duration
	logger      logger.Logger
}

// NewBrowserManager cria um novo gerenciador de browsers
func NewBrowserManager(size int, headless bool) *BrowserManager {
	return &BrowserManager{
		browsers:    make([]*rod.Browser, 0, size),
		size:        size,
		headless:    headless,
		inUse:       make([]bool, size),
		lastUsed:    make([]time.Time, size),
		maxIdleTime: DefaultMaxIdleTime,
		logger:      logger.GetGlobalLogger().WithComponent("browser-manager"),
	}
}

// Start inicializa o pool de browsers
func (bm *BrowserManager) Start() error {
	start := time.Now()
	bm.logger.InfoFields("Starting browser pool initialization", logger.Fields{
		"pool_size": bm.size,
		"headless":  bm.headless,
	})

	bm.mu.Lock()
	defer bm.mu.Unlock()

	for i := 0; i < bm.size; i++ {
		browser, err := bm.createBrowser()
		if err != nil {
			bm.logger.ErrorFields("Failed to create browser during pool initialization", logger.Fields{
				"browser_index": i,
				"error":         err.Error(),
				"created_count": len(bm.browsers),
			})

			// Cleanup browsers já criados
			for _, b := range bm.browsers {
				b.Close()
			}
			return fmt.Errorf("failed to create browser %d: %v", i, err)
		}
		bm.browsers = append(bm.browsers, browser)

		bm.logger.DebugFields("Browser created successfully", logger.Fields{
			"browser_index": i,
			"total_created": len(bm.browsers),
		})
	}

	duration := time.Since(start)
	bm.logger.InfoFields("Browser pool initialized successfully", logger.Fields{
		"pool_size": bm.size,
		"duration":  duration.String(),
	})

	return nil
}

// GetBrowser retorna um browser do pool (round-robin otimizado)
func (bm *BrowserManager) GetBrowser() *rod.Browser {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if len(bm.browsers) == 0 {
		bm.logger.Error("No browsers available in pool")
		return nil
	}

	// Conta browsers em uso para métricas
	inUseCount := 0
	for _, used := range bm.inUse {
		if used {
			inUseCount++
		}
	}

	// Procura por um browser não em uso
	for i := 0; i < len(bm.browsers); i++ {
		idx := (bm.index + i) % len(bm.browsers)
		if !bm.inUse[idx] {
			bm.inUse[idx] = true
			bm.lastUsed[idx] = time.Now()
			bm.index = (idx + 1) % len(bm.browsers)

			bm.logger.DebugFields("Browser allocated from pool", logger.Fields{
				"browser_index": idx,
				"in_use_count":  inUseCount + 1,
				"pool_size":     len(bm.browsers),
				"allocation":    "available",
			})

			return bm.browsers[idx]
		}
	}

	// Se todos estão em uso, retorna o próximo na sequência (round-robin)
	browser := bm.browsers[bm.index]
	bm.lastUsed[bm.index] = time.Now()
	oldIndex := bm.index
	bm.index = (bm.index + 1) % len(bm.browsers)

	bm.logger.WarnFields("All browsers in use, sharing browser instance", logger.Fields{
		"browser_index": oldIndex,
		"in_use_count":  inUseCount,
		"pool_size":     len(bm.browsers),
		"allocation":    "shared",
	})

	return browser
}

// ReleaseBrowser marca um browser como não em uso
func (bm *BrowserManager) ReleaseBrowser(browser *rod.Browser) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for i, b := range bm.browsers {
		if b == browser {
			wasInUse := bm.inUse[i]
			bm.inUse[i] = false
			bm.lastUsed[i] = time.Now()

			// Conta browsers ainda em uso
			inUseCount := 0
			for _, used := range bm.inUse {
				if used {
					inUseCount++
				}
			}

			bm.logger.DebugFields("Browser released to pool", logger.Fields{
				"browser_index":   i,
				"was_in_use":      wasInUse,
				"in_use_count":    inUseCount,
				"pool_size":       len(bm.browsers),
				"available_count": len(bm.browsers) - inUseCount,
			})
			break
		}
	}
}

// Stop fecha todos os browsers
func (bm *BrowserManager) Stop() {
	bm.logger.InfoFields("Stopping browser pool", logger.Fields{
		"pool_size": len(bm.browsers),
	})

	bm.mu.Lock()
	defer bm.mu.Unlock()

	closedCount := 0
	for i, browser := range bm.browsers {
		if browser != nil {
			browser.Close()
			closedCount++
			bm.logger.DebugFields("Browser closed", logger.Fields{
				"browser_index": i,
				"closed_count":  closedCount,
			})
		}
	}

	bm.browsers = nil
	bm.logger.InfoFields("Browser pool stopped successfully", logger.Fields{
		"closed_count": closedCount,
	})
}

// createBrowser cria uma nova instância de browser otimizada
func (bm *BrowserManager) createBrowser() (*rod.Browser, error) {
	start := time.Now()
	bm.logger.Debug("Creating new browser instance")

	// Configurações do launcher com cookies habilitados
	l := launcher.New().
		Headless(bm.headless).
		NoSandbox(true).
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("disable-extensions").
		Set("disable-web-security").
		Set("disable-features", "VizDisplayCompositor").
		Set("disable-background-timer-throttling").
		Set("disable-backgrounding-occluded-windows").
		Set("disable-renderer-backgrounding").
		Set("enable-cookies").
		Set("accept-cookies").
		Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	bm.logger.Debug("Launching browser process")
	launchStart := time.Now()
	url, err := l.Launch()
	if err != nil {
		bm.logger.ErrorFields("Failed to launch browser", logger.Fields{
			"error":    err.Error(),
			"headless": bm.headless,
			"duration": time.Since(launchStart).String(),
		})
		return nil, fmt.Errorf("failed to launch browser: %v", err)
	}

	bm.logger.DebugFields("Browser launched, connecting", logger.Fields{
		"url":             url,
		"launch_duration": time.Since(launchStart).String(),
	})

	connectStart := time.Now()
	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		bm.logger.ErrorFields("Failed to connect to browser", logger.Fields{
			"error":            err.Error(),
			"url":              url,
			"connect_duration": time.Since(connectStart).String(),
		})
		return nil, fmt.Errorf("failed to connect to browser: %v", err)
	}

	totalDuration := time.Since(start)
	bm.logger.DebugFields("Browser created successfully", logger.Fields{
		"total_duration":   totalDuration.String(),
		"launch_duration":  time.Since(launchStart).String(),
		"connect_duration": time.Since(connectStart).String(),
		"headless":         bm.headless,
	})

	return browser, nil
}

// CNPJExtractor extrai dados de CNPJ da página da Receita Federal
type CNPJExtractor struct {
	captchaClient *captcha.SolveCaptchaClient
	browserMgr    *BrowserManager
	logger        logger.Logger
}

// NewCNPJExtractor cria um novo extrator
func NewCNPJExtractor(captchaClient *captcha.SolveCaptchaClient, browserMgr *BrowserManager) *CNPJExtractor {
	return &CNPJExtractor{
		captchaClient: captchaClient,
		browserMgr:    browserMgr,
		logger:        logger.GetGlobalLogger().WithComponent("cnpj-extractor"),
	}
}

// ExtractCNPJData extrai dados de um CNPJ
func (e *CNPJExtractor) ExtractCNPJData(cnpj string) (*types.CNPJData, error) {
	start := time.Now()
	correlationID := fmt.Sprintf("cnpj-%s-%d", cnpj, start.Unix())

	e.logger.InfoFields("Starting CNPJ data extraction", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
	})

	browser := e.browserMgr.GetBrowser()
	if browser == nil {
		e.logger.ErrorFields("No browser available for CNPJ extraction", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
		})
		return nil, fmt.Errorf("no browser available")
	}
	defer e.browserMgr.ReleaseBrowser(browser) // Libera browser após uso

	// Cria nova página isolada com timeout otimizado
	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		e.logger.ErrorFields("Failed to create browser page", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to create page: %v", err)
	}
	defer page.Close()

	// Configura página para performance
	if err := e.configurePagePerformance(page); err != nil {
		e.logger.WarnFields("Failed to configure page performance", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to configure page: %v", err)
	}

	// Navega para página de consulta
	url := fmt.Sprintf("https://solucoes.receita.fazenda.gov.br/servicos/cnpjreva/Cnpjreva_Solicitacao.asp?cnpj=%s", cnpj)

	e.logger.DebugFields("Navigating to CNPJ consultation page", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
		"url":            url,
	})

	err = page.Navigate(url)
	if err != nil {
		e.logger.ErrorFields("Failed to navigate to consultation page", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"url":            url,
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to navigate: %v", err)
	}

	err = page.WaitLoad()
	if err != nil {
		e.logger.ErrorFields("Failed to wait for page load", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"url":            url,
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to wait for page load: %v", err)
	}

	e.logger.DebugFields("Page loaded successfully", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
		"load_duration":  time.Since(start).String(),
	})

	// Habilitar cookies explicitamente
	e.logger.Debug("Enabling cookies support")
	if cookieErr := e.enableCookies(page); cookieErr != nil {
		e.logger.WarnFields("Failed to enable cookies", logger.Fields{
			"error": cookieErr.Error(),
		})
		// Não falha aqui, apenas avisa
	}

	// Resolve captcha
	captchaStart := time.Now()
	e.logger.DebugFields("Starting captcha resolution", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
	})

	err = e.solveCaptcha(page)
	if err != nil {
		e.logger.ErrorFields("Captcha resolution failed", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"duration":       time.Since(captchaStart).String(),
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to solve captcha: %v", err)
	}

	e.logger.InfoFields("Captcha resolved successfully", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
		"duration":       time.Since(captchaStart).String(),
	})

	// Submete formulário
	formStart := time.Now()
	e.logger.DebugFields("Starting form submission", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
	})

	// Submete formulário com retry automático
	maxFormRetries := 3 // Aumentado para 3 tentativas
	var formErr error

	for formAttempt := 1; formAttempt <= maxFormRetries; formAttempt++ {
		e.logger.DebugFields("Form submission attempt", logger.Fields{
			"attempt":     formAttempt,
			"max_retries": maxFormRetries,
			"cnpj":        cnpj,
		})

		formErr = e.submitForm(page, cnpj)
		if formErr == nil {
			// Sucesso!
			break
		}

		// Verifica se é erro de cookies desativados (não pode ser resolvido com retry)
		if strings.Contains(formErr.Error(), "cookies_disabled_error") {
			e.logger.ErrorFields("Cookies disabled error - cannot retry", logger.Fields{
				"attempt": formAttempt,
				"error":   formErr.Error(),
				"cnpj":    cnpj,
			})
			// Para erro de cookies, não tenta novamente
			break
		}

		// Determina o tipo de erro
		errorType := "unknown"
		if strings.Contains(formErr.Error(), "captcha_incorrect_error") {
			errorType = "captcha_incorrect"
		} else if strings.Contains(formErr.Error(), "campos não preenchidos") ||
			strings.Contains(formErr.Error(), "form_validation_error") {
			errorType = "form_validation"
		} else if strings.Contains(formErr.Error(), "invalid_parameters_error") {
			errorType = "invalid_parameters"
		}

		// Verifica se é erro de parâmetros inválidos (pode ser resolvido com restart)
		if errorType == "invalid_parameters" {
			e.logger.ErrorFields("Invalid parameters error - will restart browser", logger.Fields{
				"attempt": formAttempt,
				"error":   formErr.Error(),
				"cnpj":    cnpj,
			})
		}

		// Verifica se é erro que pode ser resolvido com retry
		shouldRetry := errorType == "captcha_incorrect" ||
			errorType == "form_validation" ||
			errorType == "invalid_parameters"

		if shouldRetry {

			e.logger.WarnFields("Form submission failed, retrying", logger.Fields{
				"attempt":    formAttempt,
				"error":      formErr.Error(),
				"error_type": errorType,
				"cnpj":       cnpj,
			})

			if formAttempt < maxFormRetries {
				// Para erros de formulário e parâmetros inválidos, reinicia o navegador
				if errorType == "form_validation" || errorType == "invalid_parameters" {
					e.logger.InfoFields("Restarting browser due to form validation error", logger.Fields{
						"attempt":    formAttempt,
						"error_type": errorType,
						"cnpj":       cnpj,
					})
					newBrowser := e.restartBrowser()
					if newBrowser == nil {
						formErr = fmt.Errorf("failed to restart browser")
						break
					}

					// Cria nova página com o navegador reiniciado
					newPage, pageErr := newBrowser.Page(proto.TargetCreateTarget{})
					if pageErr != nil {
						e.logger.ErrorFields("Failed to create new page after browser restart", logger.Fields{
							"error": pageErr.Error(),
						})
						formErr = pageErr
						break
					}

					// Atualiza a página para a nova instância
					page = newPage

					e.logger.Info("Browser and page restarted, navigating to CNPJ page")

					// Navega novamente para a página
					if navErr := page.Navigate(url); navErr != nil {
						e.logger.ErrorFields("Failed to navigate after browser restart", logger.Fields{
							"error": navErr.Error(),
						})
						formErr = navErr
						break
					}

					// Aguarda carregamento
					if waitErr := page.WaitLoad(); waitErr != nil {
						e.logger.ErrorFields("Failed to wait for page load after restart", logger.Fields{
							"error": waitErr.Error(),
						})
						formErr = waitErr
						break
					}

					// Habilita cookies novamente
					if cookieErr := e.enableCookies(page); cookieErr != nil {
						e.logger.WarnFields("Failed to enable cookies after restart", logger.Fields{
							"error": cookieErr.Error(),
						})
					}
				} else {
					// Para outros tipos de erro, aguarda com delay progressivo
					retryDelay := time.Duration(formAttempt) * 5 * time.Second // Aumentado de 3s para 5s
					e.logger.DebugFields("Waiting before retry", logger.Fields{
						"delay":      retryDelay.String(),
						"error_type": errorType,
						"attempt":    formAttempt,
					})
					time.Sleep(retryDelay)
				}

				// Re-resolve captcha para nova tentativa
				e.logger.Debug("Re-resolving captcha for retry")
				if captchaErr := e.solveCaptcha(page); captchaErr != nil {
					e.logger.ErrorFields("Captcha re-resolution failed", logger.Fields{
						"attempt": formAttempt,
						"error":   captchaErr.Error(),
					})
					formErr = captchaErr
					break
				}
				continue
			}
		}

		// Para outros tipos de erro, não tenta novamente
		break
	}

	if formErr != nil {
		e.logger.ErrorFields("Form submission failed after all attempts", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"duration":       time.Since(formStart).String(),
			"attempts":       maxFormRetries,
			"error":          formErr.Error(),
		})
		return nil, fmt.Errorf("failed to submit form after %d attempts: %v", maxFormRetries, formErr)
	}

	e.logger.InfoFields("Form submitted successfully", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
		"duration":       time.Since(formStart).String(),
	})

	// Extrai dados
	extractStart := time.Now()
	e.logger.DebugFields("Starting data extraction", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
	})

	data, err := e.extractData(page)
	if err != nil {
		e.logger.ErrorFields("Data extraction failed", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"duration":       time.Since(extractStart).String(),
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to extract data: %v", err)
	}

	e.logger.InfoFields("Data extraction completed successfully", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
		"duration":       time.Since(extractStart).String(),
	})

	// Adiciona metadados
	totalDuration := time.Since(start)
	data.Metadados.Timestamp = time.Now()
	data.Metadados.Duracao = totalDuration.String()
	data.Metadados.URLConsulta = page.MustInfo().URL
	data.Metadados.Fonte = "online"
	data.Metadados.Sucesso = true

	e.logger.InfoFields("CNPJ data extraction completed successfully", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
		"total_duration": totalDuration.String(),
		"url_consulta":   data.Metadados.URLConsulta,
		"empresa":        data.Empresa.RazaoSocial,
	})

	return data, nil
}

// configurePagePerformance configura viewport e bloqueia recursos para performance
func (e *CNPJExtractor) configurePagePerformance(page *rod.Page) error {
	// Define timeout global para a página
	page = page.Timeout(DefaultPageTimeout)

	// Configura viewport
	err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  DefaultViewportWidth,
		Height: DefaultViewportHeight,
	})
	if err != nil {
		// Log warning but continue
	}

	// Bloqueia recursos desnecessários para performance
	router := page.HijackRequests()
	for _, resource := range blockedResources {
		router.MustAdd(resource, func(ctx *rod.Hijack) {
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		})
	}
	go router.Run()

	return nil
}

// injectCaptchaToken injeta token de captcha de forma robusta
func (e *CNPJExtractor) injectCaptchaToken(page *rod.Page, token string) (map[string]any, error) {
	if token == "" {
		return nil, fmt.Errorf("empty token")
	}

	js := `(token, timeoutMs = 2000) => {
		if (!token) return { ok: false, err: 'empty_token' };

		function setAndFire(el) {
			if (!el) return false;
			try {
				el.value = token;
				el.dispatchEvent(new Event('input', { bubbles: true }));
				el.dispatchEvent(new Event('change', { bubbles: true }));
				el.dispatchEvent(new Event('blur', { bubbles: true }));
				return true;
			} catch (e) {
				return false;
			}
		}

		const selectors = [
			'textarea[name="h-captcha-response"]',
			'textarea[name="g-recaptcha-response"]',
			'textarea[id^="g-recaptcha-response"]',
			'input[name="h-captcha-response"]',
			'input[name="g-recaptcha-response"]'
		];

		// 1) tenta no documento principal
		for (const s of selectors) {
			const el = document.querySelector(s);
			if (el && setAndFire(el)) {
				return { ok: true, method: 'document', selector: s };
			}
		}

		// 2) tenta em iframes acessíveis
		const iframes = Array.from(document.querySelectorAll('iframe'));
		for (const f of iframes) {
			try {
				const doc = f.contentDocument;
				if (!doc) continue;
				for (const s of selectors) {
					const el = doc.querySelector(s);
					if (el && setAndFire(el)) {
						return { ok: true, method: 'iframe', iframeSrc: f.src || null, selector: s };
					}
				}
			} catch (e) {
				// cross-origin: não podemos acessar o doc
			}
		}

		// 3) tentativa retardada (pequeno polling)
		const start = Date.now();
		while (Date.now() - start < timeoutMs) {
			for (const s of selectors) {
				const el = document.querySelector(s);
				if (el && setAndFire(el)) {
					return { ok: true, method: 'delayed-document', selector: s };
				}
			}
			// espera 150 ms
			const waitUntil = Date.now() + 150;
			while (Date.now() < waitUntil) {}
		}

		return {
			ok: false,
			err: 'injection_failed',
			hints: [
				'textarea pode estar em iframe cross-origin',
				'token pode ter expirado',
				'verifique se o selector correto existe no DOM'
			],
			iframeCount: iframes.length
		};
	}`

	// Chamada segura: passa token como argumento
	res, err := page.Eval(js, token, 2000)
	if err != nil {
		return nil, fmt.Errorf("page.Eval failed: %w", err)
	}

	// res.Value é do tipo gson.JSON do Rod
	var out map[string]any
	err = res.Value.Unmarshal(&out)
	if err != nil {
		// fallback: criar estrutura básica
		out = map[string]any{
			"ok":  false,
			"err": "failed_to_unmarshal_result",
			"raw": res.Value.String(),
		}
	}

	return out, nil
}

// solveCaptcha resolve o captcha na página
func (e *CNPJExtractor) solveCaptcha(page *rod.Page) (err error) {
	start := time.Now()

	// Adiciona recovery para capturar panics
	defer func() {
		if r := recover(); r != nil {
			e.logger.ErrorFields("Panic during captcha solving", logger.Fields{
				"panic":    r,
				"duration": time.Since(start).String(),
			})
			err = fmt.Errorf("panic during captcha solving: %v", r)
		}
	}()

	e.logger.Debug("Looking for captcha element")

	// Aguarda elemento do captcha
	captchaEl, err := page.Timeout(10 * time.Second).Element("[data-sitekey]")
	if err != nil {
		e.logger.ErrorFields("Captcha element not found", logger.Fields{
			"timeout": "10s",
			"error":   err.Error(),
		})
		return fmt.Errorf("captcha element not found: %v", err)
	}

	sitekey, err := captchaEl.Attribute("data-sitekey")
	if err != nil {
		e.logger.ErrorFields("Failed to get captcha sitekey", logger.Fields{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to get sitekey: %v", err)
	}

	if sitekey == nil {
		e.logger.Error("Captcha sitekey is empty")
		return fmt.Errorf("sitekey is empty")
	}

	e.logger.DebugFields("Found captcha, starting resolution", logger.Fields{
		"sitekey": *sitekey,
		"url":     page.MustInfo().URL,
	})

	// Resolve captcha
	resolveStart := time.Now()
	token, err := e.captchaClient.SolveHCaptcha(*sitekey, page.MustInfo().URL)
	if err != nil {
		e.logger.ErrorFields("Captcha resolution failed", logger.Fields{
			"sitekey":  *sitekey,
			"duration": time.Since(resolveStart).String(),
			"error":    err.Error(),
		})
		return fmt.Errorf("captcha resolution failed: %v", err)
	}

	e.logger.InfoFields("Captcha token received", logger.Fields{
		"sitekey":          *sitekey,
		"resolve_duration": time.Since(resolveStart).String(),
		"token_length":     len(token),
	})

	// Injeta token com retry automático
	e.logger.Debug("Starting token injection with retry")
	injectStart := time.Now()

	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		e.logger.DebugFields("Token injection attempt", logger.Fields{
			"attempt":     attempt,
			"max_retries": maxRetries,
		})

		result, err := e.injectCaptchaToken(page, token)
		if err != nil {
			lastErr = err
			e.logger.WarnFields("Token injection attempt failed", logger.Fields{
				"attempt": attempt,
				"error":   err.Error(),
			})

			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
				continue
			}
			break
		}

		e.logger.DebugFields("Token injection result", logger.Fields{
			"attempt": attempt,
			"result":  result,
		})

		if ok, _ := result["ok"].(bool); !ok {
			errMsg, _ := result["err"].(string)
			lastErr = fmt.Errorf("captcha injection failed: %s", errMsg)
			e.logger.WarnFields("Captcha injection failed", logger.Fields{
				"attempt":   attempt,
				"error_msg": errMsg,
				"result":    result,
			})

			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
				continue
			}
			break
		}

		// Aguarda um pouco para garantir que o token foi aplicado
		e.logger.Debug("Waiting for token to be applied")
		time.Sleep(3 * time.Second)

		// Verifica se o token foi realmente aplicado
		if validateErr := e.validateCaptchaToken(page); validateErr != nil {
			lastErr = validateErr
			e.logger.WarnFields("Captcha token validation failed after injection", logger.Fields{
				"attempt": attempt,
				"error":   validateErr.Error(),
			})

			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
				continue
			}

			e.saveDebugInfo(page, "captcha_validation_failed", "unknown")
			break
		}

		// Sucesso!
		e.logger.InfoFields("Captcha token injection and validation successful", logger.Fields{
			"attempt":  attempt,
			"duration": time.Since(injectStart).String(),
		})
		return nil
	}

	// Se chegou aqui, todas as tentativas falharam
	e.logger.ErrorFields("All captcha injection attempts failed", logger.Fields{
		"max_retries": maxRetries,
		"last_error":  lastErr.Error(),
		"duration":    time.Since(injectStart).String(),
	})
	return fmt.Errorf("failed to inject and validate captcha token after %d attempts: %v", maxRetries, lastErr)
}

// submitForm submete o formulário de consulta
func (e *CNPJExtractor) submitForm(page *rod.Page, cnpj string) error {
	start := time.Now()
	e.logger.Debug("Starting form submission")

	// Aguarda JavaScript terminar de executar
	e.logger.Debug("Waiting for JavaScript execution to complete")
	time.Sleep(3 * time.Second)

	// Verifica se o formulário está realmente pronto
	if err := e.waitForFormReady(page, cnpj); err != nil {
		e.logger.ErrorFields("Form not ready for submission", logger.Fields{
			"error": err.Error(),
		})
		return fmt.Errorf("form not ready: %v", err)
	}

	// NOVA: Validar formulário antes de submeter
	e.logger.Debug("Validating form before submission")
	if err := e.validateForm(page, cnpj); err != nil {
		e.logger.ErrorFields("Form validation failed", logger.Fields{
			"error": err.Error(),
			"cnpj":  cnpj,
		})

		// Salvar debug info
		e.saveDebugInfo(page, "form_validation_failed", cnpj)
		return fmt.Errorf("form validation failed: %v", err)
	}

	// Aguarda mais tempo para garantir que o captcha foi processado pelo servidor
	waitTime := 8 * time.Second
	e.logger.DebugFields("Extended wait for captcha server processing", logger.Fields{
		"wait_time": waitTime.String(),
	})
	time.Sleep(waitTime)

	// Verifica se o token captcha ainda é válido
	e.logger.Debug("Validating captcha token before submission")
	if err := e.validateCaptchaToken(page); err != nil {
		e.logger.ErrorFields("Captcha token invalid before submission", logger.Fields{
			"error": err.Error(),
		})
		return fmt.Errorf("captcha token invalid: %v", err)
	}

	// Verifica novamente se o formulário ainda está válido
	e.logger.Debug("Final form validation before submission")
	if err := e.validateForm(page, cnpj); err != nil {
		e.logger.ErrorFields("Final form validation failed", logger.Fields{
			"error": err.Error(),
			"cnpj":  cnpj,
		})
		return fmt.Errorf("final form validation failed: %v", err)
	}

	// Procura botão de consulta
	e.logger.Debug("Looking for submit button")
	button, err := page.Timeout(10 * time.Second).Element("button.btn-primary")
	if err != nil {
		e.logger.ErrorFields("Submit button not found", logger.Fields{
			"timeout": "10s",
			"error":   err.Error(),
		})

		// Salvar debug info
		e.saveDebugInfo(page, "submit_button_not_found", cnpj)
		return fmt.Errorf("submit button not found: %v", err)
	}

	e.logger.Debug("Submit button found, clicking")

	// Clica no botão
	clickStart := time.Now()
	err = button.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		e.logger.ErrorFields("Failed to click submit button", logger.Fields{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to click submit button: %v", err)
	}

	e.logger.DebugFields("Form submitted, waiting for navigation", logger.Fields{
		"click_duration": time.Since(clickStart).String(),
	})

	// Aguarda navegação para página de resultado
	navStart := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	e.logger.Debug("Waiting for navigation to result page")

	// Tenta aguardar pela URL de comprovante
	page.Context(ctx).WaitNavigation(proto.PageLifecycleEventNameLoad)()

	currentURL := page.MustInfo().URL
	e.logger.DebugFields("Navigation completed", logger.Fields{
		"current_url":         currentURL,
		"navigation_duration": time.Since(navStart).String(),
	})

	// Se chegou aqui, verifica se é a página de resultado
	e.logger.Debug("Looking for result page content")
	verifyStart := time.Now()

	// Primeiro, verifica se há erro na página
	if errorElement, errorErr := page.Element("*"); errorErr == nil {
		if pageText, textErr := errorElement.Text(); textErr == nil {
			// Verifica tipos específicos de erro da Receita Federal
			if strings.Contains(pageText, "Erro na Consulta") || strings.Contains(pageText, "Campos não preenchidos") {
				e.logger.ErrorFields("Form submission error detected", logger.Fields{
					"current_url": currentURL,
					"error_type":  "form_validation_error",
				})

				// Salvar debug info específico para erro de formulário
				e.saveDebugInfo(page, "form_submission_error", cnpj)
				return fmt.Errorf("form submission failed: campos não preenchidos ou erro na consulta")
			}

			// Verifica erro de captcha incorreto
			if strings.Contains(pageText, "digitou os caracteres fornecidos na imagem incorretamente") ||
				strings.Contains(pageText, "Erro na Emissão de Comprovante") {
				e.logger.ErrorFields("Captcha error detected", logger.Fields{
					"current_url": currentURL,
					"error_type":  "captcha_incorrect",
				})

				// Salvar debug info específico para erro de captcha
				e.saveDebugInfo(page, "captcha_incorrect_error", cnpj)
				return fmt.Errorf("captcha_incorrect_error: caracteres do captcha digitados incorretamente")
			}

			// Verifica erro de cookies desativados
			if strings.Contains(pageText, "navegador está com a opção de gravação de cookies desativada") ||
				strings.Contains(pageText, "Cookie estiver desativado") {
				e.logger.ErrorFields("Cookies disabled error detected", logger.Fields{
					"current_url": currentURL,
					"error_type":  "cookies_disabled",
				})

				// Salvar debug info específico para erro de cookies
				e.saveDebugInfo(page, "cookies_disabled_error", cnpj)
				return fmt.Errorf("cookies_disabled_error: cookies estão desativados no navegador")
			}

			// Verifica erro de parâmetros inválidos
			if strings.Contains(pageText, "Parâmetros Inválidos") ||
				strings.Contains(currentURL, "Cnpjreva_Erro.asp") {
				e.logger.ErrorFields("Invalid parameters error detected", logger.Fields{
					"current_url": currentURL,
					"error_type":  "invalid_parameters",
				})

				// Salvar debug info específico para erro de parâmetros
				e.saveDebugInfo(page, "invalid_parameters_error", cnpj)
				return fmt.Errorf("invalid_parameters_error: parâmetros inválidos detectados")
			}
		}
	}

	_, err = page.Timeout(15*time.Second).ElementR("*", "COMPROVANTE DE INSCRIÇÃO")
	if err != nil {
		e.logger.ErrorFields("Result page content not found", logger.Fields{
			"error":           err.Error(),
			"current_url":     currentURL,
			"verify_duration": time.Since(verifyStart).String(),
		})

		// Tenta capturar o conteúdo da página para debug
		if bodyText, textErr := page.Element("body"); textErr == nil {
			if pageContent, textErr := bodyText.Text(); textErr == nil {
				previewLength := min(500, len(pageContent))
				e.logger.DebugFields("Current page content for debugging", logger.Fields{
					"content_length":  len(pageContent),
					"content_preview": pageContent[:previewLength],
				})
			}
		}

		// Salvar debug info quando falha na navegação
		e.saveDebugInfo(page, "navigation_failed", cnpj)
		return fmt.Errorf("failed to wait for result page: %v", err)
	}

	totalDuration := time.Since(start)
	e.logger.InfoFields("Form submission completed successfully", logger.Fields{
		"total_duration":      totalDuration.String(),
		"navigation_duration": time.Since(navStart).String(),
		"verify_duration":     time.Since(verifyStart).String(),
		"result_url":          currentURL,
	})

	return nil
}

// validateForm verifica se todos os campos estão preenchidos corretamente
func (e *CNPJExtractor) validateForm(page *rod.Page, cnpj string) error {
	e.logger.Debug("Starting form validation")

	// 1. Verificar campo CNPJ
	if err := e.validateCNPJField(page, cnpj); err != nil {
		return fmt.Errorf("CNPJ field validation failed: %v", err)
	}

	// 2. Verificar token hCaptcha
	if err := e.validateCaptchaToken(page); err != nil {
		return fmt.Errorf("captcha token validation failed: %v", err)
	}

	e.logger.Debug("Form validation completed successfully")
	return nil
}

// validateCNPJField verifica se o campo CNPJ está preenchido corretamente
func (e *CNPJExtractor) validateCNPJField(page *rod.Page, expectedCNPJ string) error {
	// Possíveis seletores para o campo CNPJ
	selectors := []string{
		"input[name='cnpj']",
		"input[id='cnpj']",
		"input[name='txtCNPJ']",
		"input[type='text']", // fallback
	}

	for _, selector := range selectors {
		element, err := page.Element(selector)
		if err != nil {
			continue // tenta próximo seletor
		}

		// Verifica valor atual
		currentValue, err := element.Property("value")
		if err != nil {
			continue
		}

		currentStr := currentValue.String()
		cleanCurrent := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(currentStr, ".", ""), "/", ""), "-", "")
		cleanExpected := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(expectedCNPJ, ".", ""), "/", ""), "-", "")

		if cleanCurrent == cleanExpected {
			e.logger.DebugFields("CNPJ field validated successfully", logger.Fields{
				"selector":       selector,
				"value":          currentStr,
				"clean_current":  cleanCurrent,
				"clean_expected": cleanExpected,
			})
			return nil
		}

		// Se não está preenchido, tenta preencher
		e.logger.WarnFields("CNPJ field not filled, attempting to fill", logger.Fields{
			"selector":       selector,
			"current_value":  currentStr,
			"expected_value": expectedCNPJ,
		})

		err = element.Input(expectedCNPJ)
		if err != nil {
			continue
		}

		// Verifica se foi preenchido
		newValue, err := element.Property("value")
		if err == nil {
			newStr := newValue.String()
			cleanNew := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(newStr, ".", ""), "/", ""), "-", "")
			if cleanNew == cleanExpected {
				e.logger.InfoFields("CNPJ field filled successfully", logger.Fields{
					"selector":       selector,
					"value":          newStr,
					"clean_new":      cleanNew,
					"clean_expected": cleanExpected,
				})
				return nil
			}
		}
	}

	return fmt.Errorf("CNPJ field not found or could not be filled")
}

// validateCaptchaToken verifica se o token hCaptcha foi aplicado corretamente
func (e *CNPJExtractor) validateCaptchaToken(page *rod.Page) error {
	selectors := []string{
		"textarea[name='h-captcha-response']",
		"textarea[name='g-recaptcha-response']",
		"textarea[id^='g-recaptcha-response']",
		"input[name='h-captcha-response']",
		"input[name='g-recaptcha-response']",
	}

	var foundElements []string
	var tokenInfo []string

	for _, selector := range selectors {
		element, err := page.Element(selector)
		if err != nil {
			continue // tenta próximo seletor
		}

		foundElements = append(foundElements, selector)

		// Verifica se tem valor
		value, err := element.Property("value")
		if err != nil {
			tokenInfo = append(tokenInfo, fmt.Sprintf("%s: erro ao ler valor", selector))
			continue
		}

		valueStr := value.String()
		tokenInfo = append(tokenInfo, fmt.Sprintf("%s: length=%d", selector, len(valueStr)))

		if len(valueStr) > 50 { // tokens hCaptcha são longos
			e.logger.DebugFields("Captcha token validated successfully", logger.Fields{
				"selector":       selector,
				"token_length":   len(valueStr),
				"token_preview":  valueStr[:50] + "...",
				"found_elements": foundElements,
			})
			return nil
		}
	}

	// Log detalhado sobre elementos encontrados
	e.logger.ErrorFields("Captcha token validation failed", logger.Fields{
		"found_elements":   foundElements,
		"token_info":       tokenInfo,
		"selectors_tested": selectors,
	})

	return fmt.Errorf("captcha token not found or invalid - found %d elements but no valid tokens", len(foundElements))
}

// enableCookies força a habilitação de cookies na página
func (e *CNPJExtractor) enableCookies(page *rod.Page) error {
	e.logger.Debug("Enabling cookies support")

	// Usa a sintaxe correta do Rod: função arrow
	_, err := page.Eval(`() => {
		document.cookie = "test_cookie=enabled; path=/; SameSite=Lax";
		console.log('Cookie test set, navigator.cookieEnabled:', navigator.cookieEnabled);
	}`)

	if err != nil {
		e.logger.ErrorFields("Failed to enable cookies", logger.Fields{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to enable cookies: %v", err)
	}

	// Verifica se cookies estão funcionando
	result, err := page.Eval(`() => document.cookie.indexOf('test_cookie=enabled') !== -1`)
	if err != nil {
		e.logger.WarnFields("Failed to verify cookies", logger.Fields{
			"error": err.Error(),
		})
		return nil // Não falha, apenas avisa
	}

	cookieWorking := false
	if result != nil {
		cookieWorking = result.Value.Bool()
	}

	e.logger.DebugFields("Cookie verification result", logger.Fields{
		"cookie_working": cookieWorking,
	})

	return nil
}

// waitForFormReady aguarda o formulário estar completamente pronto para submissão
func (e *CNPJExtractor) waitForFormReady(page *rod.Page, cnpj string) error {
	e.logger.Debug("Waiting for form to be ready")

	maxAttempts := 10
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		e.logger.DebugFields("Checking form readiness", logger.Fields{
			"attempt":      attempt,
			"max_attempts": maxAttempts,
		})

		// 1. Verifica se o campo CNPJ está preenchido
		cnpjField, err := page.Element("input[name='cnpj']")
		if err != nil {
			e.logger.WarnFields("CNPJ field not found", logger.Fields{
				"attempt": attempt,
				"error":   err.Error(),
			})
			time.Sleep(1 * time.Second)
			continue
		}

		// Verifica o valor do campo
		value, err := cnpjField.Property("value")
		if err != nil || value.String() == "" {
			e.logger.WarnFields("CNPJ field empty", logger.Fields{
				"attempt": attempt,
				"value":   value.String(),
			})
			time.Sleep(1 * time.Second)
			continue
		}

		// Normaliza ambos os valores para comparação (remove formatação)
		fieldValue := strings.ReplaceAll(strings.ReplaceAll(value.String(), ".", ""), "/", "")
		fieldValue = strings.ReplaceAll(fieldValue, "-", "")
		expectedValue := strings.ReplaceAll(strings.ReplaceAll(cnpj, ".", ""), "/", "")
		expectedValue = strings.ReplaceAll(expectedValue, "-", "")

		if fieldValue != expectedValue {
			e.logger.WarnFields("CNPJ field mismatch", logger.Fields{
				"attempt":             attempt,
				"field_value":         value.String(),
				"expected_cnpj":       cnpj,
				"normalized_field":    fieldValue,
				"normalized_expected": expectedValue,
			})
			time.Sleep(1 * time.Second)
			continue
		}

		// 2. Verifica se o captcha está carregado
		captchaLoaded, err := page.Eval(`() => {
			const iframe = document.querySelector('iframe[src*="hcaptcha"]');
			return iframe !== null && iframe.style.display !== 'none';
		}`)
		if err != nil || !captchaLoaded.Value.Bool() {
			e.logger.WarnFields("Captcha not loaded", logger.Fields{
				"attempt": attempt,
				"loaded":  captchaLoaded.Value.Bool(),
			})
			time.Sleep(1 * time.Second)
			continue
		}

		// 3. Verifica se há token de captcha
		hasToken, err := page.Eval(`() => {
			const textarea = document.querySelector('textarea[name="h-captcha-response"]');
			return textarea && textarea.value && textarea.value.length > 50;
		}`)
		if err != nil || !hasToken.Value.Bool() {
			e.logger.WarnFields("Captcha token not ready", logger.Fields{
				"attempt":   attempt,
				"has_token": hasToken.Value.Bool(),
			})
			time.Sleep(1 * time.Second)
			continue
		}

		// 4. Verifica se não há validações JavaScript pendentes
		jsReady, err := page.Eval(`() => {
			// Verifica se jQuery está carregado e não há requests pendentes
			if (typeof jQuery !== 'undefined') {
				return jQuery.active === 0;
			}
			// Se não há jQuery, assume que está pronto
			return true;
		}`)
		if err != nil || !jsReady.Value.Bool() {
			e.logger.WarnFields("JavaScript validations pending", logger.Fields{
				"attempt":  attempt,
				"js_ready": jsReady.Value.Bool(),
			})
			time.Sleep(1 * time.Second)
			continue
		}

		// Se chegou aqui, o formulário está pronto
		e.logger.InfoFields("Form is ready for submission", logger.Fields{
			"attempt":        attempt,
			"cnpj_filled":    true,
			"captcha_loaded": true,
			"token_ready":    true,
			"js_ready":       true,
		})
		return nil
	}

	return fmt.Errorf("form not ready after %d attempts", maxAttempts)
}

// restartBrowser força o restart do navegador para limpar estado corrompido
func (e *CNPJExtractor) restartBrowser() *rod.Browser {
	e.logger.Info("Restarting browser to clear corrupted state")

	// Solicita um novo navegador do pool
	newBrowser := e.browserMgr.GetBrowser()
	if newBrowser == nil {
		e.logger.Error("Failed to get new browser from pool")
		return nil
	}

	e.logger.Info("Browser restarted successfully")
	return newBrowser
}

// saveDebugInfo salva screenshot e HTML para análise
func (e *CNPJExtractor) saveDebugInfo(page *rod.Page, errorType string, cnpj string) {
	timestamp := time.Now().Format("20060102_150405")

	// Screenshot
	screenshotPath := fmt.Sprintf("debug_%s_%s_%s.png", errorType, cnpj, timestamp)
	screenshotData, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})
	if err == nil {
		// Salvar screenshot
		if writeErr := os.WriteFile(screenshotPath, screenshotData, 0644); writeErr == nil {
			e.logger.InfoFields("Debug screenshot saved", logger.Fields{
				"path":       screenshotPath,
				"error_type": errorType,
				"cnpj":       cnpj,
			})
		}
	}

	// HTML
	htmlPath := fmt.Sprintf("debug_%s_%s_%s.html", errorType, cnpj, timestamp)
	html := page.MustHTML()
	if writeErr := os.WriteFile(htmlPath, []byte(html), 0644); writeErr == nil {
		e.logger.InfoFields("Debug HTML saved", logger.Fields{
			"path":       htmlPath,
			"error_type": errorType,
			"cnpj":       cnpj,
		})
	}

	// URL atual
	currentURL := page.MustInfo().URL

	e.logger.ErrorFields("Debug info saved", logger.Fields{
		"error_type":      errorType,
		"cnpj":            cnpj,
		"current_url":     currentURL,
		"screenshot_path": screenshotPath,
		"html_path":       htmlPath,
		"timestamp":       timestamp,
	})
}

// extractData extrai os dados da página de resultado
func (e *CNPJExtractor) extractData(page *rod.Page) (*types.CNPJData, error) {
	start := time.Now()
	e.logger.Debug("Starting data extraction from result page")

	// Obtém todo o texto da página
	e.logger.Debug("Getting page body element")
	bodyElement, err := page.Element("body")
	if err != nil {
		e.logger.ErrorFields("Failed to find body element", logger.Fields{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to find body element: %v", err)
	}

	e.logger.Debug("Extracting text from page body")
	textStart := time.Now()
	text, err := bodyElement.Text()
	if err != nil {
		e.logger.ErrorFields("Failed to get page text", logger.Fields{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to get page text: %v", err)
	}

	e.logger.DebugFields("Page text extracted", logger.Fields{
		"text_length":      len(text),
		"extract_duration": time.Since(textStart).String(),
	})

	// Usa o mesmo parser do Python (adaptado para Go)
	e.logger.Debug("Starting text parsing")
	parseStart := time.Now()
	data := e.parseTextData(text)

	totalDuration := time.Since(start)
	e.logger.InfoFields("Data extraction completed", logger.Fields{
		"total_duration": totalDuration.String(),
		"parse_duration": time.Since(parseStart).String(),
		"cnpj":           data.CNPJ.Numero,
		"empresa":        data.Empresa.RazaoSocial,
		"situacao":       data.Situacao.Cadastral,
	})

	return data, nil
}

// parseTextData converte texto da página em estrutura de dados
func (e *CNPJExtractor) parseTextData(text string) *types.CNPJData {
	cleanLines := e.cleanTextLines(text)
	data := e.createEmptyCNPJData()
	fieldMap := e.createFieldMap(data)

	e.processTextLines(cleanLines, fieldMap, data)

	return data
}

// cleanTextLines remove linhas vazias e faz trim
func (e *CNPJExtractor) cleanTextLines(text string) []string {
	lines := strings.Split(text, "\n")
	var cleanLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return cleanLines
}

// createEmptyCNPJData cria estrutura vazia de dados CNPJ
func (e *CNPJExtractor) createEmptyCNPJData() *types.CNPJData {
	return &types.CNPJData{
		CNPJ:        types.CNPJInfo{},
		Empresa:     types.EmpresaInfo{},
		Atividades:  types.AtividadesInfo{Secundarias: []types.Atividade{}},
		Endereco:    types.EnderecoInfo{},
		Contato:     types.ContatoInfo{},
		Situacao:    types.SituacaoInfo{},
		Comprovante: types.ComprovanteInfo{},
		Metadados:   types.MetadadosInfo{},
	}
}

// createFieldMap cria mapa de campos para extração
func (e *CNPJExtractor) createFieldMap(data *types.CNPJData) map[string]func(string) {
	return map[string]func(string){
		"NÚMERO DE INSCRIÇÃO":                          func(v string) { data.CNPJ.Numero = v },
		"DATA DE ABERTURA":                             func(v string) { data.CNPJ.DataAbertura = v },
		"NOME EMPRESARIAL":                             func(v string) { data.Empresa.RazaoSocial = v },
		"TÍTULO DO ESTABELECIMENTO (NOME DE FANTASIA)": func(v string) { data.Empresa.NomeFantasia = v },
		"PORTE":      func(v string) { data.Empresa.Porte = v },
		"LOGRADOURO": func(v string) { data.Endereco.Logradouro = v },
		"NÚMERO": func(v string) {
			if data.Endereco.Numero == "" {
				data.Endereco.Numero = v
			}
		},
		"COMPLEMENTO":                func(v string) { data.Endereco.Complemento = v },
		"CEP":                        func(v string) { data.Endereco.CEP = v },
		"BAIRRO/DISTRITO":            func(v string) { data.Endereco.Bairro = v },
		"MUNICÍPIO":                  func(v string) { data.Endereco.Municipio = v },
		"UF":                         func(v string) { data.Endereco.UF = v },
		"ENDEREÇO ELETRÔNICO":        func(v string) { data.Contato.Email = v },
		"TELEFONE":                   func(v string) { data.Contato.Telefone = v },
		"SITUAÇÃO CADASTRAL":         func(v string) { data.Situacao.Cadastral = v },
		"DATA DA SITUAÇÃO CADASTRAL": func(v string) { data.Situacao.DataSituacao = v },
	}
}

// processTextLines processa as linhas de texto extraindo dados
func (e *CNPJExtractor) processTextLines(cleanLines []string, fieldMap map[string]func(string), data *types.CNPJData) {
	for i, line := range cleanLines {
		nextLine := ""
		if i+1 < len(cleanLines) {
			nextLine = cleanLines[i+1]
		}

		// Campos simples
		if setter, exists := fieldMap[line]; exists && nextLine != "" {
			setter(nextLine)
		}

		// Campos especiais
		switch line {
		case "MATRIZ":
			data.CNPJ.Tipo = "MATRIZ"
		case "FILIAL":
			data.CNPJ.Tipo = "FILIAL"
		}

		// Natureza jurídica
		if strings.Contains(line, "CÓDIGO E DESCRIÇÃO DA NATUREZA JURÍDICA") && nextLine != "" {
			if parts := strings.SplitN(nextLine, " - ", 2); len(parts) == 2 {
				data.Empresa.NaturezaJuridica.Codigo = strings.TrimSpace(parts[0])
				data.Empresa.NaturezaJuridica.Descricao = strings.TrimSpace(parts[1])
			}
		}

		// Atividade principal
		if strings.Contains(line, "CÓDIGO E DESCRIÇÃO DA ATIVIDADE ECONÔMICA PRINCIPAL") && nextLine != "" {
			if parts := strings.SplitN(nextLine, " - ", 2); len(parts) == 2 {
				data.Atividades.Principal.Codigo = strings.TrimSpace(parts[0])
				data.Atividades.Principal.Descricao = strings.TrimSpace(parts[1])
			}
		}

		// Atividades secundárias
		if strings.Contains(line, "CÓDIGO E DESCRIÇÃO DAS ATIVIDADES ECONÔMICAS SECUNDÁRIAS") {
			j := i + 1
			for j < len(cleanLines) {
				if strings.Contains(cleanLines[j], "NATUREZA JURÍDICA") || strings.Contains(cleanLines[j], "LOGRADOURO") {
					break
				}
				if parts := strings.SplitN(cleanLines[j], " - ", 2); len(parts) == 2 {
					data.Atividades.Secundarias = append(data.Atividades.Secundarias, types.Atividade{
						Codigo:    strings.TrimSpace(parts[0]),
						Descricao: strings.TrimSpace(parts[1]),
					})
				}
				j++
			}
		}

		// Data/hora de emissão
		if strings.Contains(line, "Emitido no dia") {
			re := regexp.MustCompile(`(\d{2}/\d{2}/\d{4}) às (\d{2}:\d{2}:\d{2})`)
			matches := re.FindStringSubmatch(line)
			if len(matches) >= 3 {
				data.Comprovante.DataEmissao = matches[1]
				data.Comprovante.HoraEmissao = matches[2]
			}
		}
	}
}

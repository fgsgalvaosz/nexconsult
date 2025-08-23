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

// minInt retorna o menor entre dois inteiros
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Constantes de configuração otimizadas para velocidade
const (
	DefaultMaxIdleTime    = 30 * time.Minute
	DefaultPageTimeout    = 30 * time.Second // Reduzido de 45s
	DefaultElementTimeout = 5 * time.Second  // Reduzido de 10s
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

	// Configurações do launcher com cookies habilitados e sem leakless
	l := launcher.New().
		Headless(bm.headless).
		NoSandbox(true).
		Leakless(false). // Desabilita leakless para evitar problemas com antivírus
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

	bm.logger.Debug("Launching browser process (without leakless)")
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
		"leakless":         false,
	})

	return browser, nil
}

// CNPJExtractor extrai dados de CNPJ da página da Receita Federal
type CNPJExtractor struct {
	captchaClient    *captcha.SolveCaptchaClient
	browserMgr       *BrowserManager
	logger           logger.Logger
	lastCaptchaToken string // Armazena o último token para re-injeção
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
func (e *CNPJExtractor) ExtractCNPJData(cnpj string) (data *types.CNPJData, err error) {
	start := time.Now()
	correlationID := fmt.Sprintf("cnpj-%s-%d", cnpj, start.Unix())

	// Recovery de panic para evitar crash do programa
	defer func() {
		if r := recover(); r != nil {
			e.logger.ErrorFields("Panic recovered during CNPJ extraction", logger.Fields{
				"cnpj":  cnpj,
				"panic": r,
				"type":  "panic_recovery",
			})
			err = fmt.Errorf("panic during CNPJ extraction: %v", r)
			data = nil
		}
	}()

	e.logger.InfoFields("Starting CNPJ data extraction", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
	})

	// Configura página
	page, err := e.setupPage(cnpj, correlationID)
	if err != nil {
		return nil, err
	}
	defer page.Close()

	// Resolve captcha
	if err := e.solveCaptcha(page); err != nil {
		e.logger.ErrorFields("Captcha resolution failed", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to solve captcha: %v", err)
	}

	// Submete formulário com retry
	if err := e.submitFormWithRetry(page, cnpj, correlationID); err != nil {
		return nil, err
	}

	// Extrai dados
	data, err = e.extractData(page)
	if err != nil {
		e.logger.ErrorFields("Data extraction failed", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to extract data: %v", err)
	}

	// Finaliza
	return e.finalizeCNPJData(data, page, cnpj, correlationID, start)
}

// setupPage configura e navega para a página do CNPJ
func (e *CNPJExtractor) setupPage(cnpj, correlationID string) (*rod.Page, error) {
	browser := e.browserMgr.GetBrowser()
	if browser == nil {
		return nil, fmt.Errorf("no browser available")
	}
	defer e.browserMgr.ReleaseBrowser(browser)

	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %v", err)
	}

	page.EnableDomain(proto.NetworkEnable{})
	page.EnableDomain(proto.RuntimeEnable{})

	go e.monitorNetworkRequests(page, cnpj, correlationID)
	go e.monitorConsole(page, cnpj, correlationID)

	if err := e.configurePagePerformance(page); err != nil {
		page.Close()
		return nil, fmt.Errorf("failed to configure page: %v", err)
	}

	url := fmt.Sprintf("https://solucoes.receita.fazenda.gov.br/servicos/cnpjreva/Cnpjreva_Solicitacao.asp?cnpj=%s", cnpj)
	if err := page.Navigate(url); err != nil {
		page.Close()
		return nil, fmt.Errorf("failed to navigate: %v", err)
	}

	if err := page.WaitLoad(); err != nil {
		page.Close()
		return nil, fmt.Errorf("failed to wait for load: %v", err)
	}

	return page, nil
}

// submitFormWithRetry submete formulário com retry automático
func (e *CNPJExtractor) submitFormWithRetry(page *rod.Page, cnpj, correlationID string) error {
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		e.logger.DebugFields("Form submission attempt", logger.Fields{
			"attempt":        attempt,
			"max_retries":    maxRetries,
			"cnpj":           cnpj,
			"correlation_id": correlationID,
		})

		if err := e.submitForm(page, cnpj); err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("form submission failed after %d attempts: %v", maxRetries, err)
			}

			e.logger.WarnFields("Form submission failed, retrying", logger.Fields{
				"error":          err.Error(),
				"attempt":        attempt,
				"cnpj":           cnpj,
				"correlation_id": correlationID,
				"error_type":     "form_validation",
			})

			// Restart browser para limpar estado
			newBrowser := e.restartBrowser()
			if newBrowser == nil {
				return fmt.Errorf("failed to restart browser")
			}

			// Cria nova página
			newPage, err := newBrowser.Page(proto.TargetCreateTarget{})
			if err != nil {
				return fmt.Errorf("failed to create new page: %v", err)
			}

			page.Close()
			page = newPage

			// Navega novamente
			url := fmt.Sprintf("https://solucoes.receita.fazenda.gov.br/servicos/cnpjreva/Cnpjreva_Solicitacao.asp?cnpj=%s", cnpj)
			if err := page.Navigate(url); err != nil {
				return fmt.Errorf("failed to navigate after restart: %v", err)
			}
			if err := page.WaitLoad(); err != nil {
				return fmt.Errorf("failed to wait for load after restart: %v", err)
			}

			// Resolve captcha novamente
			if err := e.solveCaptcha(page); err != nil {
				return fmt.Errorf("failed to solve captcha after restart: %v", err)
			}
			continue
		}

		// Sucesso
		return nil
	}

	return fmt.Errorf("unexpected end of retry loop")
}

// finalizeCNPJData finaliza os dados extraídos
func (e *CNPJExtractor) finalizeCNPJData(data *types.CNPJData, page *rod.Page, cnpj, correlationID string, start time.Time) (*types.CNPJData, error) {
	totalDuration := time.Since(start)
	data.Metadados.Timestamp = time.Now()
	data.Metadados.Duracao = totalDuration.String()
	if info, err := page.Info(); err == nil {
		data.Metadados.URLConsulta = info.URL
	} else {
		data.Metadados.URLConsulta = "unknown"
	}
	data.Metadados.Fonte = "online"
	data.Metadados.Sucesso = true

	e.logger.InfoFields("CNPJ data extraction completed successfully", logger.Fields{
		"cnpj":           cnpj,
		"correlation_id": correlationID,
		"total_duration": totalDuration.String(),
		"url_consulta":   data.Metadados.URLConsulta,
		"empresa":        data.Empresa.RazaoSocial,
		"method":         "puppeteer-pattern",
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
				console.log('Setting token on element:', el.id || el.name, 'Current value:', el.value);
				el.value = token;
				el.dispatchEvent(new Event('input', { bubbles: true }));
				el.dispatchEvent(new Event('change', { bubbles: true }));
				el.dispatchEvent(new Event('blur', { bubbles: true }));
				console.log('Token set successfully. New value length:', el.value.length);
				return true;
			} catch (e) {
				console.error('Error setting token:', e);
				return false;
			}
		}

		const selectors = [
			'textarea[id^="h-captcha-response"]',
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

	var pageURL string
	if info, err := page.Info(); err == nil {
		pageURL = info.URL
	} else {
		pageURL = "unknown"
	}

	e.logger.DebugFields("Found captcha, starting resolution", logger.Fields{
		"sitekey": *sitekey,
		"url":     pageURL,
	})

	// Resolve captcha
	resolveStart := time.Now()
	token, err := e.captchaClient.SolveHCaptcha(*sitekey, pageURL)
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

	// Armazena o token para possível re-injeção
	e.lastCaptchaToken = token

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

		// Aguarda um pouco para garantir que o token foi aplicado (otimizado)
		e.logger.Debug("Waiting for token to be applied")
		time.Sleep(1 * time.Second)

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

	// Aguarda JavaScript terminar de executar (reduzido para velocidade)
	e.logger.Debug("Waiting for JavaScript execution to complete")
	time.Sleep(1 * time.Second)

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

	// Aguarda otimizada para processamento do captcha
	waitTime := 3 * time.Second
	e.logger.DebugFields("Optimized wait for captcha server processing", logger.Fields{
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

	// NOVO: Detectar e limpar estados de erro pré-existentes
	e.logger.Debug("Checking for pre-existing error states")
	if err := e.clearPreExistingErrors(page); err != nil {
		e.logger.WarnFields("Failed to clear pre-existing errors", logger.Fields{
			"error": err.Error(),
		})
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

	// NOVO: Salva HTML antes da submissão (com token aplicado)
	e.logger.Debug("Saving HTML before form submission for debug")
	e.saveDebugInfo(page, "before_submission", cnpj)

	// Procura botão de consulta (timeout otimizado)
	e.logger.Debug("Looking for submit button")
	button, err := page.Timeout(5 * time.Second).Element("button.btn-primary")
	if err != nil {
		e.logger.ErrorFields("Submit button not found", logger.Fields{
			"timeout": "10s",
			"error":   err.Error(),
		})

		// Salvar debug info
		e.saveDebugInfo(page, "submit_button_not_found", cnpj)
		return fmt.Errorf("submit button not found: %v", err)
	}

	e.logger.Info("Submit button found, re-injecting token before click")

	// CRÍTICO: Verificação de saúde do captcha e re-injeção robusta
	e.logger.InfoFields("Performing captcha health check before submission", logger.Fields{
		"token_length": len(e.lastCaptchaToken),
		"token_empty":  e.lastCaptchaToken == "",
	})

	if e.lastCaptchaToken == "" {
		e.logger.Error("Cannot proceed: lastCaptchaToken is empty")
		return fmt.Errorf("lastCaptchaToken is empty")
	}

	// Verifica se o widget hCaptcha ainda está ativo
	captchaHealth, err := page.Eval(`() => {
		const iframe = document.querySelector('iframe[src*="hcaptcha.com"]');
		const textarea = document.querySelector('textarea[id^="h-captcha-response"]');

		return {
			iframe_present: iframe !== null,
			textarea_present: textarea !== null,
			textarea_value_length: textarea ? textarea.value.length : 0,
			widget_visible: iframe ? iframe.style.display !== 'none' : false
		};
	}`)

	if err == nil {
		healthData := captchaHealth.Value.Map()
		e.logger.InfoFields("Captcha health check results", logger.Fields{
			"iframe_present":        healthData["iframe_present"].Bool(),
			"textarea_present":      healthData["textarea_present"].Bool(),
			"textarea_value_length": healthData["textarea_value_length"].Int(),
			"widget_visible":        healthData["widget_visible"].Bool(),
		})

		// Se o textarea não tem valor, força re-injeção
		if healthData["textarea_value_length"].Int() == 0 {
			e.logger.Warn("Captcha textarea is empty, forcing re-injection")
		}
	}

	// Re-injeção robusta com múltiplas tentativas
	maxReInjectAttempts := 3
	var reInjectSuccess bool

	for attempt := 1; attempt <= maxReInjectAttempts; attempt++ {
		e.logger.InfoFields("Re-injecting token before submission", logger.Fields{
			"attempt":      attempt,
			"max_attempts": maxReInjectAttempts,
		})

		if _, err := e.injectCaptchaToken(page, e.lastCaptchaToken); err != nil {
			e.logger.ErrorFields("Token re-injection attempt failed", logger.Fields{
				"attempt": attempt,
				"error":   err.Error(),
			})

			if attempt < maxReInjectAttempts {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			return fmt.Errorf("failed to re-inject token after %d attempts: %v", maxReInjectAttempts, err)
		}

		// Verifica se a re-injeção foi bem-sucedida
		if validateErr := e.validateCaptchaToken(page); validateErr == nil {
			reInjectSuccess = true
			e.logger.InfoFields("Token re-injection successful", logger.Fields{
				"attempt": attempt,
			})
			break
		} else {
			e.logger.WarnFields("Token re-injection validation failed", logger.Fields{
				"attempt": attempt,
				"error":   validateErr.Error(),
			})
		}
	}

	if !reInjectSuccess {
		return fmt.Errorf("failed to successfully re-inject and validate token")
	}

	// Proteção simples: apenas salva token para re-injeção
	e.logger.Debug("Saving token for potential re-injection")
	_, err = page.Eval(`() => {
		const currentToken = document.querySelector('textarea[id^="h-captcha-response"]');
		if (currentToken && currentToken.value) {
			window._tokenBackup = currentToken.value;
			console.log('Token saved for re-injection, length:', currentToken.value.length);
			return true;
		}
		return false;
	}`)
	if err != nil {
		e.logger.WarnFields("Failed to freeze hCaptcha", logger.Fields{
			"error": err.Error(),
		})
	}

	e.logger.Info("Token re-injected successfully, clicking submit button (Puppeteer-style)")

	// Verificação final: confirma que o token ainda está presente
	finalCheck, err := page.Eval(`() => {
		const textarea = document.querySelector('textarea[id^="h-captcha-response"]');
		return {
			present: textarea !== null,
			length: textarea ? textarea.value.length : 0,
			id: textarea ? textarea.id : 'none'
		};
	}`)
	if err == nil {
		checkData := finalCheck.Value.Map()
		e.logger.InfoFields("Final token check before click", logger.Fields{
			"token_present": checkData["present"].Bool(),
			"token_length":  checkData["length"].Int(),
			"element_id":    checkData["id"].Str(),
		})
	}

	// Estratégia avançada: submissão de formulário direta para evitar detecção do hCaptcha
	clickStart := time.Now()
	e.logger.Info("Using advanced form submission strategy to bypass hCaptcha detection")

	// Hover rápido para parecer humano (otimizado)
	err = button.Hover()
	if err != nil {
		e.logger.WarnFields("Failed to hover button", logger.Fields{"error": err.Error()})
	}

	// Pausa mínima para simular comportamento humano
	time.Sleep(100 * time.Millisecond)

	// Estratégia principal: submissão direta do formulário
	submitResult, err := page.Eval(`() => {
		const form = document.querySelector('#frmConsulta');
		const button = document.querySelector('button.btn-primary');
		const textarea = document.querySelector('textarea[id^="h-captcha-response"]');

		if (!form || !button) return { success: false, error: 'form_or_button_not_found' };

		// Salva o token atual
		const currentToken = textarea ? textarea.value : '';

		try {
			// Estratégia 1: Submissão direta do formulário (mais eficaz)
			if (form.submit && currentToken.length > 0) {
				// Garante que o token está presente
				if (textarea) {
					textarea.value = currentToken;
					textarea.dispatchEvent(new Event('change', { bubbles: true }));
				}

				// Submete o formulário diretamente sem clique
				form.submit();
				return { success: true, method: 'direct_form_submit', token_length: currentToken.length };
			}

			// Estratégia 2: Clique programático no botão
			if (button.click) {
				// Re-injeta token imediatamente antes do clique
				if (textarea && currentToken) {
					textarea.value = currentToken;
				}

				button.click();
				return { success: true, method: 'programmatic_click', token_length: currentToken.length };
			}

			return { success: false, error: 'no_submission_method_available' };
		} catch (e) {
			return { success: false, error: e.message };
		}
	}`)

	if err != nil {
		e.logger.ErrorFields("Advanced submission strategy failed", logger.Fields{
			"error": err.Error(),
		})

		// Fallback para clique Rod tradicional
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = button.Context(ctx).Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			e.logger.ErrorFields("Failed to click submit button", logger.Fields{
				"error": err.Error(),
			})
			return fmt.Errorf("failed to click submit button: %v", err)
		}

		e.logger.Info("Used fallback Rod click method")
	} else {
		result := submitResult.Value.Map()
		e.logger.InfoFields("Advanced submission strategy executed", logger.Fields{
			"success":      result["success"].Bool(),
			"method":       result["method"].Str(),
			"token_length": result["token_length"].Int(),
		})

		if !result["success"].Bool() {
			return fmt.Errorf("form submission failed: %s", result["error"].Str())
		}
	}

	// CRÍTICO: Estratégia agressiva de re-injeção pós-clique
	e.logger.Debug("Starting aggressive token re-injection strategy")

	// Múltiplas tentativas de re-injeção em intervalos otimizados
	for attempt := 1; attempt <= 3; attempt++ {
		time.Sleep(time.Duration(attempt*100) * time.Millisecond)

		postClickCheck, err := page.Eval(`() => {
			const textarea = document.querySelector('textarea[id^="h-captcha-response"]');
			return {
				present: textarea !== null,
				length: textarea ? textarea.value.length : 0,
				id: textarea ? textarea.id : 'none',
				timestamp: Date.now()
			};
		}`)

		if err == nil {
			checkData := postClickCheck.Value.Map()
			e.logger.InfoFields("Post-click token verification", logger.Fields{
				"attempt":        attempt,
				"token_present":  checkData["present"].Bool(),
				"token_length":   checkData["length"].Int(),
				"element_id":     checkData["id"].Str(),
				"ms_after_click": time.Since(clickStart).Milliseconds(),
			})

			// Se o token foi limpo, tenta re-injeção agressiva
			if checkData["length"].Int() == 0 && e.lastCaptchaToken != "" {
				e.logger.WarnFields("Token cleared after click, attempting aggressive re-injection", logger.Fields{
					"attempt": attempt,
				})

				// Re-injeção com JavaScript direto para ser mais rápido
				reInjectResult, reInjectErr := page.Eval(`(token) => {
					const selectors = [
						'textarea[id^="h-captcha-response"]',
						'textarea[name="h-captcha-response"]',
						'input[name="h-captcha-response"]'
					];

					for (const selector of selectors) {
						const el = document.querySelector(selector);
						if (el) {
							el.value = token;
							el.dispatchEvent(new Event('input', { bubbles: true }));
							el.dispatchEvent(new Event('change', { bubbles: true }));
							return { success: true, selector: selector, length: el.value.length };
						}
					}
					return { success: false, error: 'no_element_found' };
				}`, e.lastCaptchaToken)

				if reInjectErr != nil {
					e.logger.ErrorFields("Aggressive re-injection failed", logger.Fields{
						"attempt": attempt,
						"error":   reInjectErr.Error(),
					})
				} else {
					result := reInjectResult.Value.Map()
					if result["success"].Bool() {
						e.logger.InfoFields("Aggressive re-injection successful", logger.Fields{
							"attempt":      attempt,
							"selector":     result["selector"].Str(),
							"token_length": result["length"].Int(),
						})
						break // Sucesso, para de tentar
					}
				}
			} else if checkData["length"].Int() > 0 {
				// Token ainda presente, não precisa re-injetar
				e.logger.InfoFields("Token still present, no re-injection needed", logger.Fields{
					"attempt":      attempt,
					"token_length": checkData["length"].Int(),
				})
				break
			}
		}
	}

	e.logger.DebugFields("Form submitted, waiting for navigation (Puppeteer pattern)", logger.Fields{
		"click_duration": time.Since(clickStart).String(),
	})

	// Aguarda navegação para página de resultado com timeout otimizado
	navStart := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	e.logger.Debug("Waiting for navigation to result page")

	// Aguarda navegação com estratégia mais robusta
	navigationDone := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				navigationDone <- fmt.Errorf("navigation panic: %v", r)
			}
		}()

		// Aguarda mudança na URL ou conteúdo específico
		page.Context(ctx).WaitNavigation(proto.PageLifecycleEventNameLoad)()
		navigationDone <- nil
	}()

	// Aguarda navegação ou timeout
	select {
	case navErr := <-navigationDone:
		if navErr != nil {
			e.logger.WarnFields("Navigation wait failed", logger.Fields{
				"error":    navErr.Error(),
				"duration": time.Since(navStart).String(),
			})
			// Continua mesmo com erro de navegação, pode ter carregado
		}
	case <-ctx.Done():
		e.logger.WarnFields("Navigation timeout", logger.Fields{
			"timeout":  "15s",
			"duration": time.Since(navStart).String(),
		})
		// Continua mesmo com timeout, pode ter carregado
	}

	var currentURL string
	if info, err := page.Info(); err == nil {
		currentURL = info.URL
	} else {
		currentURL = "unknown"
	}

	e.logger.DebugFields("Navigation completed", logger.Fields{
		"current_url":         currentURL,
		"navigation_duration": time.Since(navStart).String(),
	})

	// Se chegou aqui, verifica se é a página de resultado
	e.logger.Debug("Looking for result page content")
	verifyStart := time.Now()

	// Verifica se houve mudança significativa na URL (indicando submissão bem-sucedida)
	if strings.Contains(currentURL, "cnpj=") && !strings.Contains(currentURL, "Cnpjreva_Solicitacao.asp") {
		e.logger.InfoFields("URL change detected, form submission likely successful", logger.Fields{
			"current_url": currentURL,
		})
	}

	// Verifica se chegou na página de comprovante (sucesso garantido)
	if strings.Contains(currentURL, "Cnpjreva_Comprovante.asp") {
		e.logger.InfoFields("Comprovante page reached - submission successful", logger.Fields{
			"current_url": currentURL,
		})
		// Pula verificações adicionais, já é sucesso
		return nil
	}

	// Primeiro, verifica se há erro na página (melhorada para evitar falsos positivos)
	if errorElement, errorErr := page.Element("*"); errorErr == nil {
		if pageText, textErr := errorElement.Text(); textErr == nil {
			// Verifica se é uma página de sucesso (comprovante) - PRIORIDADE
			if strings.Contains(pageText, "COMPROVANTE DE INSCRIÇÃO") ||
				strings.Contains(pageText, "Situação Cadastral") ||
				strings.Contains(currentURL, "Cnpjreva_Comprovante.asp") {
				e.logger.InfoFields("Success page detected - comprovante found", logger.Fields{
					"current_url": currentURL,
					"page_type":   "comprovante_success",
				})
				// É uma página de sucesso, continua o processamento
			} else if strings.Contains(pageText, "Erro na Consulta") ||
				strings.Contains(pageText, "Campos não preenchidos") ||
				strings.Contains(pageText, "CNPJ Inválido") ||
				strings.Contains(pageText, "Captcha inválido") ||
				strings.Contains(pageText, "Dados incorretos") {
				// Verifica estado do token no momento do erro
				tokenInfo := e.getTokenDebugInfo(page)

				e.logger.ErrorFields("Form submission error detected", logger.Fields{
					"current_url":    currentURL,
					"error_type":     "form_validation_error",
					"error_content":  strings.TrimSpace(pageText[:minInt(200, len(pageText))]),
					"token_present":  tokenInfo["token_present"],
					"token_length":   tokenInfo["token_length"],
					"token_selector": tokenInfo["token_selector"],
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

	_, err = page.Timeout(8*time.Second).ElementR("*", "COMPROVANTE DE INSCRIÇÃO")
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
		"textarea[id^='h-captcha-response']",
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
			const selectors = [
				'textarea[id^="h-captcha-response"]',
				'textarea[name="h-captcha-response"]'
			];
			for (const selector of selectors) {
				const textarea = document.querySelector(selector);
				if (textarea && textarea.value && textarea.value.length > 50) {
					return true;
				}
			}
			return false;
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

	// Obter informações do token para incluir nos logs
	tokenInfo := e.getTokenDebugInfo(page)

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
		"token_present":   tokenInfo["token_present"],
		"token_length":    tokenInfo["token_length"],
		"token_selector":  tokenInfo["token_selector"],
		"element_id":      tokenInfo["element_id"],
		"element_name":    tokenInfo["element_name"],
	})
}

// extractData extrai os dados da página de resultado
func (e *CNPJExtractor) extractData(page *rod.Page) (data *types.CNPJData, err error) {
	start := time.Now()
	e.logger.Debug("Starting data extraction from result page")

	// Recovery de panic para evitar crash do programa
	defer func() {
		if r := recover(); r != nil {
			e.logger.ErrorFields("Panic recovered during data extraction", logger.Fields{
				"panic": r,
				"type":  "panic_recovery",
			})
			err = fmt.Errorf("panic during data extraction: %v", r)
			data = nil
		}
	}()

	// Cria novo contexto para extração de dados (evita cancelamento)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verifica se estamos na página correta (com tratamento de erro)
	var currentURL string
	if info, err := page.Info(); err != nil {
		e.logger.WarnFields("Failed to get page info, using fallback", logger.Fields{
			"error": err.Error(),
		})
		currentURL = "unknown"
	} else {
		currentURL = info.URL
	}

	e.logger.DebugFields("Extracting data from page", logger.Fields{
		"current_url": currentURL,
	})

	// Obtém todo o texto da página com novo contexto
	e.logger.Debug("Getting page body element")
	bodyElement, err := page.Context(ctx).Element("body")
	if err != nil {
		e.logger.ErrorFields("Failed to find body element", logger.Fields{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to find body element: %v", err)
	}

	e.logger.Debug("Extracting text from page body")
	textStart := time.Now()

	// Aguarda um pouco para garantir que a página carregou completamente
	time.Sleep(500 * time.Millisecond)

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
	data = e.parseTextData(text)

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

// clearPreExistingErrors detecta e limpa estados de erro pré-existentes
func (e *CNPJExtractor) clearPreExistingErrors(page *rod.Page) error {
	e.logger.Debug("Checking for pre-existing error states")

	// Verifica se há erros visíveis na página
	errorCheck, err := page.Eval(`() => {
		const errors = [];

		// Verifica msgErroCaptcha (erro específico que causa falhas)
		const captchaError = document.querySelector('#msgErroCaptcha');
		if (captchaError && !captchaError.classList.contains('collapse')) {
			errors.push({
				id: 'msgErroCaptcha',
				visible: true,
				text: captchaError.textContent.trim()
			});
		}

		// Verifica outros erros visíveis
		const generalError = document.querySelector('#msgErro');
		if (generalError && !generalError.classList.contains('collapse')) {
			errors.push({
				id: 'msgErro',
				visible: true,
				text: generalError.textContent.trim()
			});
		}

		return {
			hasErrors: errors.length > 0,
			errors: errors
		};
	}`)

	if err != nil {
		return fmt.Errorf("failed to check for errors: %v", err)
	}

	errorData := errorCheck.Value.Map()
	hasErrors := errorData["hasErrors"].Bool()

	if hasErrors {
		e.logger.WarnFields("Pre-existing errors detected", logger.Fields{
			"errors": errorData["errors"].String(),
		})

		// Tenta limpar os erros
		_, clearErr := page.Eval(`() => {
			// Esconde msgErroCaptcha
			const captchaError = document.querySelector('#msgErroCaptcha');
			if (captchaError) {
				captchaError.classList.add('collapse');
				captchaError.style.display = 'none';
			}

			// Esconde msgErro
			const generalError = document.querySelector('#msgErro');
			if (generalError) {
				generalError.classList.add('collapse');
				generalError.style.display = 'none';
			}

			return true;
		}`)

		if clearErr != nil {
			return fmt.Errorf("failed to clear errors: %v", clearErr)
		}

		e.logger.Info("Pre-existing errors cleared successfully")
	}

	return nil
}

// monitorNetworkRequests monitora requisições de rede
func (e *CNPJExtractor) monitorNetworkRequests(page *rod.Page, cnpj, correlationID string) {
	page.EachEvent(func(e *proto.NetworkRequestWillBeSent) {
		if e.Request.URL != "" {
			logger.GetGlobalLogger().WithComponent("network").DebugFields("Network request", logger.Fields{
				"cnpj":           cnpj,
				"correlation_id": correlationID,
				"method":         e.Request.Method,
				"url":            e.Request.URL,
				"request_id":     string(e.RequestID),
			})
		}
	})()

	page.EachEvent(func(e *proto.NetworkResponseReceived) {
		if e.Response.URL != "" {
			logger.GetGlobalLogger().WithComponent("network").DebugFields("Network response", logger.Fields{
				"cnpj":           cnpj,
				"correlation_id": correlationID,
				"status":         e.Response.Status,
				"url":            e.Response.URL,
				"request_id":     string(e.RequestID),
				"mime_type":      e.Response.MIMEType,
			})
		}
	})()

	page.EachEvent(func(e *proto.NetworkLoadingFailed) {
		logger.GetGlobalLogger().WithComponent("network").WarnFields("Network request failed", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"request_id":     string(e.RequestID),
			"error_text":     e.ErrorText,
			"type":           string(e.Type),
		})
	})()
}

// monitorConsole monitora logs do console do navegador
func (e *CNPJExtractor) monitorConsole(page *rod.Page, cnpj, correlationID string) {
	page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		if len(e.Args) > 0 {
			var message string
			if e.Args[0].Value.String() != "" {
				message = e.Args[0].Value.String()
			}

			logLevel := "info"
			switch e.Type {
			case proto.RuntimeConsoleAPICalledTypeError:
				logLevel = "error"
			case proto.RuntimeConsoleAPICalledTypeWarning:
				logLevel = "warning"
			case proto.RuntimeConsoleAPICalledTypeLog:
				logLevel = "info"
			case proto.RuntimeConsoleAPICalledTypeDebug:
				logLevel = "debug"
			}

			logger.GetGlobalLogger().WithComponent("console").InfoFields("Browser console", logger.Fields{
				"cnpj":           cnpj,
				"correlation_id": correlationID,
				"level":          logLevel,
				"message":        message,
				"type":           string(e.Type),
			})
		}
	})()

	page.EachEvent(func(e *proto.RuntimeExceptionThrown) {
		logger.GetGlobalLogger().WithComponent("console").ErrorFields("JavaScript exception", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"message":        e.ExceptionDetails.Text,
			"line_number":    e.ExceptionDetails.LineNumber,
			"column_number":  e.ExceptionDetails.ColumnNumber,
			"url":            e.ExceptionDetails.URL,
		})
	})()
}

// getTokenDebugInfo obtém informações de debug sobre o token hCaptcha
func (e *CNPJExtractor) getTokenDebugInfo(page *rod.Page) map[string]any {
	result := make(map[string]any)

	// Verifica se há token presente
	tokenInfo, err := page.Eval(`() => {
		const selectors = [
			'textarea[id^="h-captcha-response"]',
			'textarea[name="h-captcha-response"]',
			'textarea[name="g-recaptcha-response"]'
		];

		for (const selector of selectors) {
			const element = document.querySelector(selector);
			if (element) {
				return {
					selector: selector,
					present: true,
					value_length: element.value ? element.value.length : 0,
					value_preview: element.value ? element.value.substring(0, 50) + '...' : '',
					element_id: element.id,
					element_name: element.name
				};
			}
		}

		return {
			selector: 'none',
			present: false,
			value_length: 0,
			value_preview: '',
			element_id: '',
			element_name: ''
		};
	}`)

	if err != nil {
		result["error"] = err.Error()
		result["token_present"] = false
		result["token_length"] = 0
		result["token_selector"] = "error"
	} else {
		tokenData := tokenInfo.Value.Map()
		result["token_present"] = tokenData["present"].Bool()
		result["token_length"] = tokenData["value_length"].Int()
		result["token_selector"] = tokenData["selector"].Str()
		result["token_preview"] = tokenData["value_preview"].Str()
		result["element_id"] = tokenData["element_id"].Str()
		result["element_name"] = tokenData["element_name"].Str()
	}

	return result
}

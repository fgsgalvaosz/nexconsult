package browser

import (
	"context"
	"fmt"
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

	// Configurações do launcher
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

	err = e.submitForm(page, cnpj)
	if err != nil {
		e.logger.ErrorFields("Form submission failed", logger.Fields{
			"cnpj":           cnpj,
			"correlation_id": correlationID,
			"duration":       time.Since(formStart).String(),
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to submit form: %v", err)
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

	// Injeta token usando método robusto
	e.logger.Debug("Starting token injection")
	injectStart := time.Now()

	result, err := e.injectCaptchaToken(page, token)
	if err != nil {
		e.logger.ErrorFields("Token injection failed", logger.Fields{
			"error":    err.Error(),
			"duration": time.Since(injectStart).String(),
		})
		return fmt.Errorf("failed to inject captcha token: %v", err)
	}

	e.logger.DebugFields("Token injection result", logger.Fields{
		"result":   result,
		"duration": time.Since(injectStart).String(),
	})

	if ok, _ := result["ok"].(bool); !ok {
		errMsg, _ := result["err"].(string)
		e.logger.ErrorFields("Captcha injection failed", logger.Fields{
			"error_msg": errMsg,
			"result":    result,
		})
		return fmt.Errorf("captcha injection failed: %s", errMsg)
	}

	e.logger.Info("Token injected successfully")

	// Aguarda processamento do token
	e.logger.Debug("Waiting for token processing")
	time.Sleep(2 * time.Second)

	totalDuration := time.Since(start)
	e.logger.InfoFields("Captcha resolution completed successfully", logger.Fields{
		"total_duration":   totalDuration.String(),
		"resolve_duration": time.Since(resolveStart).String(),
		"inject_duration":  time.Since(injectStart).String(),
	})

	return nil
}

// submitForm submete o formulário de consulta
func (e *CNPJExtractor) submitForm(page *rod.Page) error {
	start := time.Now()
	e.logger.Debug("Starting form submission")

	// Procura botão de consulta
	e.logger.Debug("Looking for submit button")
	button, err := page.Timeout(10 * time.Second).Element("button.btn-primary")
	if err != nil {
		e.logger.ErrorFields("Submit button not found", logger.Fields{
			"timeout": "10s",
			"error":   err.Error(),
		})
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

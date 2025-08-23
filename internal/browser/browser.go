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
	}
}

// Start inicializa o pool de browsers
func (bm *BrowserManager) Start() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for i := 0; i < bm.size; i++ {
		browser, err := bm.createBrowser()
		if err != nil {
			// Cleanup browsers já criados
			for _, b := range bm.browsers {
				b.Close()
			}
			return fmt.Errorf("failed to create browser %d: %v", i, err)
		}
		bm.browsers = append(bm.browsers, browser)
	}

	// logger.GetGlobalLogger().WithComponent("browser").InfoFields("Browser pool initialized", logger.Fields{"count": bm.size})
	return nil
}

// GetBrowser retorna um browser do pool (round-robin otimizado)
func (bm *BrowserManager) GetBrowser() *rod.Browser {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if len(bm.browsers) == 0 {
		return nil
	}

	// Procura por um browser não em uso
	for i := 0; i < len(bm.browsers); i++ {
		idx := (bm.index + i) % len(bm.browsers)
		if !bm.inUse[idx] {
			bm.inUse[idx] = true
			bm.lastUsed[idx] = time.Now()
			bm.index = (idx + 1) % len(bm.browsers)
			return bm.browsers[idx]
		}
	}

	// Se todos estão em uso, retorna o próximo na sequência (round-robin)
	browser := bm.browsers[bm.index]
	bm.lastUsed[bm.index] = time.Now()
	bm.index = (bm.index + 1) % len(bm.browsers)
	return browser
}

// ReleaseBrowser marca um browser como não em uso
func (bm *BrowserManager) ReleaseBrowser(browser *rod.Browser) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for i, b := range bm.browsers {
		if b == browser {
			bm.inUse[i] = false
			bm.lastUsed[i] = time.Now()
			break
		}
	}
}

// Stop fecha todos os browsers
func (bm *BrowserManager) Stop() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, browser := range bm.browsers {
		browser.Close()
	}
	bm.browsers = nil
	// logger.GetGlobalLogger().WithComponent("browser").Info("Browser pool stopped")
}

// createBrowser cria uma nova instância de browser otimizada
func (bm *BrowserManager) createBrowser() (*rod.Browser, error) {
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

	url, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %v", err)
	}

	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %v", err)
	}

	return browser, nil
}

// CNPJExtractor extrai dados de CNPJ da página da Receita Federal
type CNPJExtractor struct {
	captchaClient *captcha.SolveCaptchaClient
	browserMgr    *BrowserManager
}

// NewCNPJExtractor cria um novo extrator
func NewCNPJExtractor(captchaClient *captcha.SolveCaptchaClient, browserMgr *BrowserManager) *CNPJExtractor {
	return &CNPJExtractor{
		captchaClient: captchaClient,
		browserMgr:    browserMgr,
	}
}

// ExtractCNPJData extrai dados de um CNPJ
func (e *CNPJExtractor) ExtractCNPJData(cnpj string) (*types.CNPJData, error) {
	start := time.Now()

	browser := e.browserMgr.GetBrowser()
	if browser == nil {
		return nil, fmt.Errorf("no browser available")
	}
	defer e.browserMgr.ReleaseBrowser(browser) // Libera browser após uso

	// Cria nova página isolada com timeout otimizado
	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %v", err)
	}
	defer page.Close()

	// Configura página para performance
	if err := e.configurePagePerformance(page); err != nil {
		return nil, fmt.Errorf("failed to configure page: %v", err)
	}

	// Navega para página de consulta
	url := fmt.Sprintf("https://solucoes.receita.fazenda.gov.br/servicos/cnpjreva/Cnpjreva_Solicitacao.asp?cnpj=%s", cnpj)

	err = page.Navigate(url)
	if err != nil {
		return nil, fmt.Errorf("failed to navigate: %v", err)
	}

	err = page.WaitLoad()
	if err != nil {
		return nil, fmt.Errorf("failed to wait for page load: %v", err)
	}

	// 	// // logger.GetGlobalLogger().WithComponent("browser").DebugFields("Page loaded", logger.Fields{
	// 		"cnpj": cnpj,
	// 		"url":  url,
	// 	}).Debug("Page loaded")

	// Resolve captcha
	// logger.GetGlobalLogger().WithComponent("browser").Debug("Starting captcha resolution")
	err = e.solveCaptcha(page)
	if err != nil {
		// logger.GetGlobalLogger().WithComponent("browser").WithError(err).Error("Captcha resolution failed")
		return nil, fmt.Errorf("failed to solve captcha: %v", err)
	}
	// logger.GetGlobalLogger().WithComponent("browser").Info("Captcha resolved, proceeding to form submission")

	// Submete formulário
	// logger.GetGlobalLogger().WithComponent("browser").Debug("Starting form submission")
	err = e.submitForm(page)
	if err != nil {
		// logger.GetGlobalLogger().WithComponent("browser").WithError(err).Error("Form submission failed")
		return nil, fmt.Errorf("failed to submit form: %v", err)
	}
	// logger.GetGlobalLogger().WithComponent("browser").Info("Form submitted successfully, proceeding to data extraction")

	// Extrai dados
	// logger.GetGlobalLogger().WithComponent("browser").Debug("Starting data extraction")
	data, err := e.extractData(page)
	if err != nil {
		// logger.GetGlobalLogger().WithComponent("browser").WithError(err).Error("Data extraction failed")
		return nil, fmt.Errorf("failed to extract data: %v", err)
	}
	// logger.GetGlobalLogger().WithComponent("browser").Info("Data extraction completed successfully")

	// Adiciona metadados
	data.Metadados.Timestamp = time.Now()
	data.Metadados.Duracao = time.Since(start).String()
	data.Metadados.URLConsulta = page.MustInfo().URL
	data.Metadados.Fonte = "online"
	data.Metadados.Sucesso = true

	// // logger.GetGlobalLogger().WithComponent("browser").DebugFields("Page loaded", logger.Fields{
	// 		"cnpj":     cnpj,
	// 		"duration": time.Since(start),
	// 	}).Info("CNPJ data extracted successfully")

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
func (e *CNPJExtractor) injectCaptchaToken(page *rod.Page, token string) (map[string]interface{}, error) {
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
	var out map[string]interface{}
	err = res.Value.Unmarshal(&out)
	if err != nil {
		// fallback: criar estrutura básica
		out = map[string]interface{}{
			"ok":  false,
			"err": "failed_to_unmarshal_result",
			"raw": res.Value.String(),
		}
	}

	return out, nil
}

// solveCaptcha resolve o captcha na página
func (e *CNPJExtractor) solveCaptcha(page *rod.Page) (err error) {
	// Adiciona recovery para capturar panics
	defer func() {
		if r := recover(); r != nil {
			// logger.GetGlobalLogger().WithComponent("browser").Info("Browser action").Error("🚨 PANIC during captcha solving")
			err = fmt.Errorf("panic during captcha solving: %v", r)
		}
	}()
	// Aguarda elemento do captcha
	captchaEl, err := page.Timeout(10 * time.Second).Element("[data-sitekey]")
	if err != nil {
		return fmt.Errorf("captcha element not found: %v", err)
	}

	sitekey, err := captchaEl.Attribute("data-sitekey")
	if err != nil {
		return fmt.Errorf("failed to get sitekey: %v", err)
	}

	if sitekey == nil {
		return fmt.Errorf("sitekey is empty")
	}

	// logger.GetGlobalLogger().WithComponent("browser").DebugFields("Solving captcha", logger.Fields{"sitekey": *sitekey})

	// Resolve captcha
	token, err := e.captchaClient.SolveHCaptcha(*sitekey, page.MustInfo().URL)
	if err != nil {
		return fmt.Errorf("captcha resolution failed: %v", err)
	}

	// 	// logger.GetGlobalLogger().WithComponent("browser").Info("Browser action") > 0).Info("🎯 CAPTCHA TOKEN RECEIVED - Starting injection process")

	// Injeta token usando método robusto (sem fmt.Sprintf)
	// 	// logger.GetGlobalLogger().WithComponent("browser").Info("Browser action")).Info("🔧 STARTING TOKEN INJECTION")

	result, err := e.injectCaptchaToken(page, token)
	if err != nil {
		// logger.GetGlobalLogger().WithComponent("browser").WithError(err).Error("❌ Token injection failed")
		return fmt.Errorf("failed to inject captcha token: %v", err)
	}

	// logger.GetGlobalLogger().WithComponent("browser").InfoFields("📋 Injection result", logger.Fields{"inject_result": result})

	if ok, _ := result["ok"].(bool); !ok {
		errMsg, _ := result["err"].(string)
		// logger.GetGlobalLogger().WithComponent("browser").Info("Browser action").Error("❌ Captcha injection failed")
		return fmt.Errorf("captcha injection failed: %s", errMsg)
	}

	// logger.GetGlobalLogger().WithComponent("browser").Info("✅ Token injected successfully")

	// Token injection já foi feito acima

	// Aguarda um pouco para garantir que o token foi processado (igual ao Python)
	// logger.GetGlobalLogger().WithComponent("browser").Info("⏳ Waiting 2 seconds for token processing...")
	time.Sleep(2 * time.Second)

	// logger.GetGlobalLogger().WithComponent("browser").Info("✅ CAPTCHA TOKEN INJECTION COMPLETED SUCCESSFULLY")
	return nil
}

// submitForm submete o formulário de consulta
func (e *CNPJExtractor) submitForm(page *rod.Page) error {
	// logger.GetGlobalLogger().WithComponent("browser").Info("🚀 STARTING FORM SUBMISSION")

	// Procura botão de consulta
	// logger.GetGlobalLogger().WithComponent("browser").Info("🔍 Looking for submit button...")
	button, err := page.Timeout(10 * time.Second).Element("button.btn-primary")
	if err != nil {
		// logger.GetGlobalLogger().WithComponent("browser").WithError(err).Error("Submit button not found")
		return fmt.Errorf("submit button not found: %v", err)
	}

	// logger.GetGlobalLogger().WithComponent("browser").Debug("Submit button found, clicking...")

	// Clica no botão
	err = button.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		// logger.GetGlobalLogger().WithComponent("browser").WithError(err).Error("Failed to click submit button")
		return fmt.Errorf("failed to click submit button: %v", err)
	}

	// logger.GetGlobalLogger().WithComponent("browser").Info("Form submitted successfully, waiting for navigation")

	// Aguarda navegação para página de resultado
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// logger.GetGlobalLogger().WithComponent("browser").Debug("Waiting for navigation to result page")

	// Tenta aguardar pela URL de comprovante
	page.Context(ctx).WaitNavigation(proto.PageLifecycleEventNameLoad)()

	_ = page.MustInfo().URL
	// logger.GetGlobalLogger().WithComponent("browser").DebugFields("Navigation completed", logger.Fields{"current_url": currentURL})

	// Se chegou aqui, verifica se é a página de resultado
	// logger.GetGlobalLogger().WithComponent("browser").Debug("Looking for result page content")
	_, err = page.Timeout(15*time.Second).ElementR("*", "COMPROVANTE DE INSCRIÇÃO")
	if err != nil {
		// logger.GetGlobalLogger().WithComponent("browser").WithError(err).Info("Browser action").Error("Result page content not found")

		// Tenta capturar o conteúdo da página para debug
		if bodyText, textErr := page.Element("body"); textErr == nil {
			if _, textErr := bodyText.Text(); textErr == nil {
				// 				// logger.GetGlobalLogger().WithComponent("browser").Info("Browser action"))]).Debug("Current page content")
			}
		}

		return fmt.Errorf("failed to wait for result page: %v", err)
	}

	// logger.GetGlobalLogger().WithComponent("browser").Info("Result page loaded successfully")
	return nil
}

// extractData extrai os dados da página de resultado
func (e *CNPJExtractor) extractData(page *rod.Page) (*types.CNPJData, error) {
	// Obtém todo o texto da página
	bodyElement, err := page.Element("body")
	if err != nil {
		return nil, fmt.Errorf("failed to find body element: %v", err)
	}

	text, err := bodyElement.Text()
	if err != nil {
		return nil, fmt.Errorf("failed to get page text: %v", err)
	}

	// Usa o mesmo parser do Python (adaptado para Go)
	data := e.parseTextData(text)

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

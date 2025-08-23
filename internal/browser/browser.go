package browser

import (
	"cnpj-consultor/internal/captcha"
	"cnpj-consultor/internal/types"
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sirupsen/logrus"
)

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
		maxIdleTime: 30 * time.Minute, // Browsers idle por mais de 30min são reciclados
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

	logrus.WithField("count", bm.size).Info("Browser pool initialized")
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
	logrus.Info("Browser pool stopped")
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

	// Define timeout global para a página (otimizado para busca direta)
	page = page.Timeout(45 * time.Second)

	// Configura página para performance
	err = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  1200,
		Height: 800,
	})
	if err != nil {
		logrus.WithError(err).Warn("Failed to set viewport")
	}

	// Bloqueia recursos desnecessários
	router := page.HijackRequests()
	defer router.Stop()

	router.MustAdd("*.css", func(ctx *rod.Hijack) {
		ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	})
	router.MustAdd("*.png", func(ctx *rod.Hijack) {
		ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	})
	router.MustAdd("*.jpg", func(ctx *rod.Hijack) {
		ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	})
	router.MustAdd("*.gif", func(ctx *rod.Hijack) {
		ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	})

	go router.Run()

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

	logrus.WithFields(logrus.Fields{
		"cnpj": cnpj,
		"url":  url,
	}).Debug("Page loaded")

	// Resolve captcha
	logrus.Debug("Starting captcha resolution")
	err = e.solveCaptcha(page)
	if err != nil {
		logrus.WithError(err).Error("Captcha resolution failed")
		return nil, fmt.Errorf("failed to solve captcha: %v", err)
	}
	logrus.Info("Captcha resolved, proceeding to form submission")

	// Submete formulário
	logrus.Debug("Starting form submission")
	err = e.submitForm(page)
	if err != nil {
		logrus.WithError(err).Error("Form submission failed")
		return nil, fmt.Errorf("failed to submit form: %v", err)
	}
	logrus.Info("Form submitted successfully, proceeding to data extraction")

	// Extrai dados
	logrus.Debug("Starting data extraction")
	data, err := e.extractData(page)
	if err != nil {
		logrus.WithError(err).Error("Data extraction failed")
		return nil, fmt.Errorf("failed to extract data: %v", err)
	}
	logrus.Info("Data extraction completed successfully")

	// Adiciona metadados
	data.Metadados.Timestamp = time.Now()
	data.Metadados.Duracao = time.Since(start).String()
	data.Metadados.URLConsulta = page.MustInfo().URL
	data.Metadados.Fonte = "online"
	data.Metadados.Sucesso = true

	logrus.WithFields(logrus.Fields{
		"cnpj":     cnpj,
		"duration": time.Since(start),
	}).Info("CNPJ data extracted successfully")

	return data, nil
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
			logrus.WithField("panic", r).Error("🚨 PANIC during captcha solving")
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

	logrus.WithField("sitekey", *sitekey).Debug("Solving captcha")

	// Resolve captcha
	token, err := e.captchaClient.SolveHCaptcha(*sitekey, page.MustInfo().URL)
	if err != nil {
		return fmt.Errorf("captcha resolution failed: %v", err)
	}

	logrus.WithField("token_received", len(token) > 0).Info("🎯 CAPTCHA TOKEN RECEIVED - Starting injection process")

	// Injeta token usando método robusto (sem fmt.Sprintf)
	logrus.WithField("token_length", len(token)).Info("🔧 STARTING TOKEN INJECTION")

	result, err := e.injectCaptchaToken(page, token)
	if err != nil {
		logrus.WithError(err).Error("❌ Token injection failed")
		return fmt.Errorf("failed to inject captcha token: %v", err)
	}

	logrus.WithField("inject_result", result).Info("📋 Injection result")

	if ok, _ := result["ok"].(bool); !ok {
		errMsg, _ := result["err"].(string)
		logrus.WithField("error", errMsg).Error("❌ Captcha injection failed")
		return fmt.Errorf("captcha injection failed: %s", errMsg)
	}

	logrus.Info("✅ Token injected successfully")

	// Token injection já foi feito acima

	// Aguarda um pouco para garantir que o token foi processado (igual ao Python)
	logrus.Info("⏳ Waiting 2 seconds for token processing...")
	time.Sleep(2 * time.Second)

	logrus.Info("✅ CAPTCHA TOKEN INJECTION COMPLETED SUCCESSFULLY")
	return nil
}

// submitForm submete o formulário de consulta
func (e *CNPJExtractor) submitForm(page *rod.Page) error {
	logrus.Info("🚀 STARTING FORM SUBMISSION")

	// Procura botão de consulta
	logrus.Info("🔍 Looking for submit button...")
	button, err := page.Timeout(10 * time.Second).Element("button.btn-primary")
	if err != nil {
		logrus.WithError(err).Error("Submit button not found")
		return fmt.Errorf("submit button not found: %v", err)
	}

	logrus.Debug("Submit button found, clicking...")

	// Clica no botão
	err = button.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		logrus.WithError(err).Error("Failed to click submit button")
		return fmt.Errorf("failed to click submit button: %v", err)
	}

	logrus.Info("Form submitted successfully, waiting for navigation")

	// Aguarda navegação para página de resultado
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logrus.Debug("Waiting for navigation to result page")

	// Tenta aguardar pela URL de comprovante
	page.Context(ctx).WaitNavigation(proto.PageLifecycleEventNameLoad)()

	currentURL := page.MustInfo().URL
	logrus.WithField("current_url", currentURL).Debug("Navigation completed")

	// Se chegou aqui, verifica se é a página de resultado
	logrus.Debug("Looking for result page content")
	_, err = page.Timeout(15*time.Second).ElementR("*", "COMPROVANTE DE INSCRIÇÃO")
	if err != nil {
		logrus.WithError(err).WithField("url", currentURL).Error("Result page content not found")

		// Tenta capturar o conteúdo da página para debug
		if bodyText, textErr := page.Element("body"); textErr == nil {
			if text, textErr := bodyText.Text(); textErr == nil {
				logrus.WithField("page_content_preview", text[:min(500, len(text))]).Debug("Current page content")
			}
		}

		return fmt.Errorf("failed to wait for result page: %v", err)
	}

	logrus.Info("Result page loaded successfully")
	return nil
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
	lines := strings.Split(text, "\n")

	// Remove linhas vazias e trim
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	data := &types.CNPJData{
		CNPJ:        types.CNPJInfo{},
		Empresa:     types.EmpresaInfo{},
		Atividades:  types.AtividadesInfo{Secundarias: []types.Atividade{}},
		Endereco:    types.EnderecoInfo{},
		Contato:     types.ContatoInfo{},
		Situacao:    types.SituacaoInfo{},
		Comprovante: types.ComprovanteInfo{},
		Metadados:   types.MetadadosInfo{},
	}

	// Mapa de campos para extração
	fieldMap := map[string]func(string){
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

	// Processa linhas
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

	return data
}

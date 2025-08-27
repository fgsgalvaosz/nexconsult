package service

import (
	"context"
	"fmt"
	"io"
	"nexconsult-sintegra-ma/internal/config"
	"nexconsult-sintegra-ma/internal/models"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

const (
	// URLs base
	sintegraFormURL = "https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraFiltro.jsf"

	// Configurações padrão
	defaultWorkerCount    = 3
	defaultViewportWidth  = 1920
	defaultViewportHeight = 1080
	defaultUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	// Timeouts
	defaultWaitBetween     = 2 * time.Second
	defaultRetryDelay      = 5 * time.Second
	defaultMaxRetries      = 3
	resultPageTimeout      = 10 * time.Second

	// Seletores CSS
	selectorResultsTable = "table"
	selectorErrorMessage = ".error"

	// Caminhos do Chrome
	chromePathWindows64 = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
	chromePathWindows32 = "C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe"

	// Padrões de validação
	cnpjNumbersPattern = `^\d{14}$`
)

// Config contém as configurações da aplicação
type Config struct {
	SolveCaptchaAPIKey string                `json:"solvecaptcha_api_key"`
	Headless           bool                  `json:"headless"`
	Timeout            time.Duration         `json:"timeout"`
	WaitBetweenSteps   time.Duration         `json:"wait_between_steps"`
	UserAgent          string                `json:"user_agent"`
	ViewportWidth      int                   `json:"viewport_width"`
	ViewportHeight     int                   `json:"viewport_height"`
	MaxRetries         int                   `json:"max_retries"`
	RetryDelay         time.Duration         `json:"retry_delay"`
	TimeoutConfig      *config.TimeoutConfig `json:"-"` // Configurações de timeout centralizadas
}

// NewDefaultConfig cria uma configuração padrão
func NewDefaultConfig() *Config {
	return &Config{
		Headless:         true,
		Timeout:          defaultTimeout,
		WaitBetweenSteps: defaultWaitBetween,
		UserAgent:        defaultUserAgent,
		ViewportWidth:    defaultViewportWidth,
		ViewportHeight:   defaultViewportHeight,
		MaxRetries:       defaultMaxRetries,
		RetryDelay:       defaultRetryDelay,
	}
}

// SolveCaptchaRequest representa a requisição para a API SolveCaptcha
type SolveCaptchaRequest struct {
	Key       string `json:"key"`
	Method    string `json:"method"`
	GoogleKey string `json:"googlekey"`
	PageURL   string `json:"pageurl"`
	JSON      string `json:"json"`
}

// SolveCaptchaResponse representa a resposta da API SolveCaptcha
type SolveCaptchaResponse struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
	Error   string `json:"error"`
}

// SintegraMAResult representa o resultado da consulta
type SintegraMAResult struct {
	CNPJ          string        `json:"cnpj"`
	Status        string        `json:"status"`
	URL           string        `json:"url"`
	Data          *SintegraData `json:"data"`
	ExecutionTime time.Duration `json:"execution_time"`
	Timestamp     time.Time     `json:"timestamp"`
	CaptchaSolved bool          `json:"captcha_solved"`
}

// SintegraData representa os dados estruturados da consulta
type SintegraData struct {
	// Identificação
	CGC               string `json:"cgc"`
	InscricaoEstadual string `json:"inscricao_estadual"`
	RazaoSocial       string `json:"razao_social"`
	RegimeApuracao    string `json:"regime_apuracao"`

	// Endereço
	Endereco *EnderecoData `json:"endereco"`

	// CNAE
	CNAEPrincipal   string     `json:"cnae_principal"`
	CNAESecundarios []CNAEData `json:"cnae_secundarios"`

	// Situação
	SituacaoCadastral     string `json:"situacao_cadastral"`
	DataSituacaoCadastral string `json:"data_situacao_cadastral"`

	// Obrigações
	Obrigacoes *ObrigacoesData `json:"obrigacoes"`

	// Metadados
	DataConsulta   string `json:"data_consulta"`
	NumeroConsulta string `json:"numero_consulta"`
	Observacao     string `json:"observacao"`
}

type EnderecoData struct {
	Logradouro  string `json:"logradouro"`
	Numero      string `json:"numero"`
	Complemento string `json:"complemento"`
	Bairro      string `json:"bairro"`
	Municipio   string `json:"municipio"`
	UF          string `json:"uf"`
	CEP         string `json:"cep"`
	DDD         string `json:"ddd"`
	Telefone    string `json:"telefone"`
}

type CNAEData struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
}

type ObrigacoesData struct {
	NFeAPartirDe string `json:"nfe_a_partir_de"`
	EDFAPartirDe string `json:"edf_a_partir_de"`
	CTEAPartirDe string `json:"cte_a_partir_de"`
}

// SintegraMAScraper scraper para consulta no Sintegra MA
type SintegraMAScraper struct {
	config        *Config
	browser       *rod.Browser
	page          *rod.Page
	captchaSolver *CaptchaSolver
	// Removed undefined RecaptchaSolver field since we're using CaptchaSolver instead
	logger zerolog.Logger
	result *SintegraMAResult
}

// SintegraService gerencia as operações de consulta no Sintegra MA
type SintegraService struct {
	logger     zerolog.Logger
	workerPool *WorkerPool
	// Mapa para rastrear consultas em andamento
	consultasEmAndamento    map[string]bool
	consultasEmAndamentoMux sync.RWMutex
	// Configurações de timeout
	timeoutConfig *config.TimeoutConfig
}

// NewSintegraService cria uma nova instância do serviço
func NewSintegraService(logger zerolog.Logger, timeoutConfig *config.TimeoutConfig) *SintegraService {
	if logger.GetLevel() == zerolog.Disabled {
		logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}

	// Se não for fornecido um config de timeout, usar o padrão
	if timeoutConfig == nil {
		timeoutConfig = config.DefaultTimeoutConfig()
	}

	service := &SintegraService{
		logger:               logger,
		consultasEmAndamento: make(map[string]bool),
		timeoutConfig:        timeoutConfig,
	}

	// Criar worker pool com número padrão de workers
	service.workerPool = NewWorkerPool(service, defaultWorkerCount, timeoutConfig)

	return service
}

// StartWorkerPool inicia o worker pool para processamento paralelo
func (s *SintegraService) StartWorkerPool() {
	s.logger.Info().Msg("🚀 Iniciando worker pool para processamento paralelo")
	s.workerPool.Start()
}

// StopWorkerPool para o worker pool gracefully
func (s *SintegraService) StopWorkerPool() {
	s.logger.Info().Msg("🛑 Parando worker pool...")
	s.workerPool.Stop()
}

// Initialize inicializa o navegador
func (s *SintegraMAScraper) Initialize() error {
	s.logger.Info().Msg("Inicializando navegador Chrome")

	chromePath := s.findChromePath()
	launcher := s.createLauncher(chromePath)

	browser, err := s.initializeBrowser(launcher)
	if err != nil {
		return err
	}

	page, err := s.createPage(browser)
	if err != nil {
		return err
	}

	s.browser = browser
	s.page = page

	s.logger.Info().Msg("Navegador inicializado com sucesso")
	return nil
}

// min retorna o menor valor entre dois inteiros
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// findChromePath encontra o caminho do Chrome instalado
func (s *SintegraMAScraper) findChromePath() string {
	chromePaths := []string{
		chromePathWindows64,
		chromePathWindows32,
	}

	for _, path := range chromePaths {
		if s.isFileExists(path) {
			s.logger.Info().Str("chrome_path", path).Msg("Chrome encontrado no sistema")
			return path
		}
	}

	s.logger.Warn().Msg("Chrome não encontrado, usando Chromium")
	return ""
}

// isFileExists verifica se um arquivo existe
func (s *SintegraMAScraper) isFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// createLauncher cria o launcher do navegador
func (s *SintegraMAScraper) createLauncher(chromePath string) *launcher.Launcher {
	l := launcher.New().
		Headless(s.config.Headless).
		Leakless(false).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-web-security").
		Set("disable-extensions").
		Set("user-agent", s.config.UserAgent)

	if chromePath != "" {
		l = l.Bin(chromePath)
	}

	return l
}

// initializeBrowser inicializa o navegador
func (s *SintegraMAScraper) initializeBrowser(launcher *launcher.Launcher) (*rod.Browser, error) {
	url, err := launcher.Launch()
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao iniciar o navegador")
		return nil, fmt.Errorf("erro ao iniciar o navegador: %v", err)
	}

	browser := rod.New().
		ControlURL(url).
		Timeout(s.config.Timeout)

	err = browser.Connect()
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao conectar ao navegador")
		return nil, fmt.Errorf("erro ao conectar ao navegador: %v", err)
	}

	return browser, nil
}

// createPage cria uma nova página no navegador
func (s *SintegraMAScraper) createPage(browser *rod.Browser) (*rod.Page, error) {
	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao criar página")
		return nil, fmt.Errorf("erro ao criar página: %v", err)
	}

	err = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             s.config.ViewportWidth,
		Height:            s.config.ViewportHeight,
		DeviceScaleFactor: 1,
		Mobile:            false,
	})
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao configurar viewport")
		return nil, fmt.Errorf("erro ao configurar viewport: %v", err)
	}

	return page, nil
}

// ConsultarCNPJ executa a consulta completa
func (s *SintegraMAScraper) ConsultarCNPJ(cnpj string) error {
	start := time.Now()
	s.result.CNPJ = cnpj

	s.logger.Info().
		Str("cnpj", cnpj).
		Msg("Iniciando consulta no Sintegra MA")

	// Navegar para página inicial
	if err := s.navigateToInitialPage(); err != nil {
		s.result.Status = "erro_navegacao"
		return err
	}

	// Configurar formulário
	if err := s.setupForm(cnpj); err != nil {
		s.result.Status = "erro_formulario"
		return err
	}

	// Tentar resolver reCAPTCHA automaticamente
	s.tryResolveCaptcha()

	// Submeter formulário
	if err := s.submitForm(); err != nil {
		s.result.Status = "erro_submit"
		return err
	}

	// Aguardar e navegar para página de resultados
	if err := s.waitForResultsPage(); err != nil {
		s.result.Status = "erro_resultados"
		return err
	}

	// Navegar para página de detalhes
	if err := s.navigateToDetailsPage(); err != nil {
		s.result.Status = "erro_detalhes"
		return err
	}

	// Atualizar URL do resultado
	s.result.URL = s.page.MustEval(`() => window.location.href`).String()
	s.logger.Info().Msg("✓ Página de resultados carregada, prosseguindo para extração")

	// Extrair detalhes
	if err := s.extrairDetalhes(); err != nil {
		s.result.Status = "erro_extracao"
		return fmt.Errorf("erro na extração: %v", err)
	}

	// Finalizar consulta
	s.finalizarConsulta(start)
	return nil
}

// navigateToInitialPage navega para a página inicial do Sintegra
func (s *SintegraMAScraper) navigateToInitialPage() error {
	s.logger.Info().Msg("Navegando para página inicial do Sintegra MA")

	ctx, cancel := context.WithTimeout(context.Background(), s.config.Timeout)
	defer cancel()

	err := s.page.Context(ctx).Navigate(sintegraFormURL)
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao navegar para página inicial")
		return fmt.Errorf("erro ao navegar para página inicial: %w", err)
	}

	// Aguardar página carregar completamente
	err = s.page.WaitLoad()
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao aguardar carregamento da página")
		return fmt.Errorf("erro ao aguardar carregamento: %w", err)
	}

	return nil
}

// setupForm configura o formulário com tipo de emissão e CNPJ
func (s *SintegraMAScraper) setupForm(cnpj string) error {
	s.logger.Info().Str("cnpj", cnpj).Msg("Configurando formulário")

	if err := s.validateCNPJ(cnpj); err != nil {
		return fmt.Errorf("CNPJ inválido: %w", err)
	}

	// Selecionar tipo de emissão
	s.logger.Info().Msg("Selecionando tipo de emissão")
	tipoEmissao := s.page.MustElement("#form1\\:tipoEmissao\\:1")
	tipoEmissao.MustClick()
	time.Sleep(1 * time.Second)

	// Aguardar e localizar campo CNPJ
	s.logger.Info().Str("cnpj", cnpj).Msg("Preenchendo CNPJ")
	campoCNPJ, err := s.page.Timeout(s.config.Timeout).Element("#form1\\:cpfCnpj")
	if err != nil {
		s.logger.Error().Err(err).Msg("Campo CNPJ não encontrado")
		return fmt.Errorf("campo CNPJ não encontrado: %w", err)
	}

	// Limpar campo e inserir CNPJ
	err = campoCNPJ.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao clicar no campo CNPJ")
		return fmt.Errorf("erro ao clicar no campo CNPJ: %w", err)
	}
	err = campoCNPJ.SelectAllText()
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao selecionar texto do campo CNPJ")
		return fmt.Errorf("erro ao selecionar texto: %w", err)
	}
	err = campoCNPJ.Input(cnpj)
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao preencher campo CNPJ")
		return fmt.Errorf("erro ao preencher CNPJ: %w", err)
	}

	time.Sleep(1 * time.Second)
	return nil
}

// validateCNPJ valida o formato do CNPJ
func (s *SintegraMAScraper) validateCNPJ(cnpj string) error {
	if cnpj == "" {
		return fmt.Errorf("CNPJ não pode ser vazio")
	}

	// Remover caracteres especiais para validação
	cnpjNumbers := regexp.MustCompile(`[^\d]`).ReplaceAllString(cnpj, "")

	// Verificar se tem 14 dígitos
	if matched, _ := regexp.MatchString(cnpjNumbersPattern, cnpjNumbers); !matched {
		return fmt.Errorf("CNPJ deve conter exatamente 14 dígitos")
	}

	return nil
}

// tryResolveCaptcha tenta resolver o reCAPTCHA automaticamente
func (s *SintegraMAScraper) tryResolveCaptcha() {
	recaptchaResolvido := false
	recaptchaCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	go func() {
		if recaptchaErr := s.resolverRecaptcha(); recaptchaErr != nil {
			// Verificar se é erro de indisponibilidade do serviço
			if strings.Contains(recaptchaErr.Error(), "temporariamente indisponível") {
				s.logger.Warn().Err(recaptchaErr).Msg("Serviço de CAPTCHA temporariamente indisponível")
			} else {
				s.logger.Warn().Err(recaptchaErr).Msg("Erro na resolução automática do CAPTCHA")
			}
		} else {
			recaptchaResolvido = true
		}
		cancel()
	}()

	// Aguardar resolução automática ou timeout
	<-recaptchaCtx.Done()
	if !recaptchaResolvido {
		s.logger.Info().Msg("⚠️ CAPTCHA automático falhou ou timeout. Continuando sem CAPTCHA...")
	}
}

// submitForm submete o formulário de consulta
func (s *SintegraMAScraper) submitForm() error {
	s.logger.Info().Msg("🚀 Submetendo formulário...")

	// Aguardar o processamento do reCAPTCHA
	time.Sleep(2 * time.Second)

	// Verificar estado da página e botão
	if err := s.checkPageAndButtonState(); err != nil {
		return err
	}

	// Executar clique no botão com verificação adicional
	s.logger.Info().Msg("Clicando no botão de consulta...")

	// Usar seletor mais robusto para encontrar o botão
	btnSelector := "#form1\\:pnlPrincipal4 input:nth-of-type(2), form#form1 button[type=submit], #botaoConsultar, button[type=submit], input[value='Consultar']"
	botaoConsultar := s.page.MustElement(btnSelector)

	// Verificar se o botão está realmente clicável
	buttonReady := s.page.MustEval(`() => {
		var selectors = ["#form1\\:pnlPrincipal4 input:nth-of-type(2)", "form#form1 button[type=submit]", "#botaoConsultar", "button[type=submit]", "input[value='Consultar']"];
		for (var i = 0; i < selectors.length; i++) {
			var btn = document.querySelector(selectors[i]);
			if (btn && !btn.disabled && btn.offsetParent !== null) {
				return true;
			}
		}
		return false;
	}`).Bool()

	if !buttonReady {
		s.logger.Warn().Msg("Botão não está pronto, aguardando...")
		time.Sleep(1 * time.Second)
	}

	botaoConsultar.MustClick()
	s.logger.Info().Msg("✓ Botão de consulta clicado!")

	// Aguardar carregamento da página
	s.page.MustWaitLoad()
	time.Sleep(s.config.WaitBetweenSteps)

	return nil
}

// checkPageAndButtonState verifica o estado da página e botão antes do clique
func (s *SintegraMAScraper) checkPageAndButtonState() error {
	s.logger.Debug().Msg("Verificando estado da página e botão")

	// Verificar se a página carregou corretamente
	pageInfo, err := s.page.Info()
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao obter informações da página")
		return fmt.Errorf("erro ao verificar página: %w", err)
	}

	s.logger.Debug().Str("page_title", pageInfo.Title).Msg("Título da página obtido")

	// Verificar se a página está responsiva
	pageReady := s.page.MustEval(`() => {
		return document.readyState === 'complete' && !document.querySelector('.loading');
	}`).Bool()
	s.logger.Info().Bool("page_ready", pageReady).Msg("Estado da página antes do clique")

	// Verificar se o botão está visível e clicável
	btnExists := s.page.MustEval(`() => {
		// Verificar se o botão existe e está habilitado usando seletores múltiplos
		var selectors = ["#form1\\:pnlPrincipal4 input:nth-of-type(2)", "form#form1 button[type=submit]", "#botaoConsultar", "button[type=submit]", "input[value='Consultar']"];
		var btn = null;
		for (var i = 0; i < selectors.length; i++) {
			btn = document.querySelector(selectors[i]);
			if (btn) break;
		}
		if (!btn) return false;
		return btn.offsetParent !== null && !btn.disabled;
	}`).Bool()
	s.logger.Info().Bool("btn_exists", btnExists).Msg("Estado do botão")

	if !btnExists {
		s.logger.Warn().Msg("Botão não está visível ou habilitado, aguardando...")
		time.Sleep(2 * time.Second)
	}
	s.logger.Debug().Msg("Página e botão estão em estado válido")
	return nil
}

// waitForResultsPage aguarda o carregamento da página de resultados
func (s *SintegraMAScraper) waitForResultsPage() error {
	urlEsperada := "https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraResultadoListaConsulta.jsf"
	s.logger.Info().Str("url_esperada", urlEsperada).Msg("🔍 Aguardando página de resultados...")

	ctx, cancel := context.WithTimeout(context.Background(), resultPageTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			urlAtual := s.page.MustInfo().URL
			errorMsg := s.page.MustEval(`() => {
				// Procurar por mensagens de erro comuns
				var errorSelectors = ['.error', '.alert', '.warning', '[class*="error"]', '[class*="alert"]'];
				for (var i = 0; i < errorSelectors.length; i++) {
					var el = document.querySelector(errorSelectors[i]);
					if (el && el.textContent.trim()) {
						return el.textContent.trim();
					}
				}
				return 'Nenhuma mensagem de erro encontrada';
			}`).String()

			s.logger.Warn().Str("error_check", errorMsg).Str("url_atual", urlAtual).Msg("⚠ Página de resultados não carregou")
			return fmt.Errorf("timeout aguardando página de resultados: %w", ctx.Err())
		case <-ticker.C:
			urlAtual := s.page.MustInfo().URL
			if strings.Contains(urlAtual, "consultaSintegraResultadoListaConsulta.jsf") {
				s.logger.Info().Str("url_resultado", urlAtual).Msg("✓ Página de resultados carregada!")
				return nil
			}
			// Verificar se há resultados ou erro usando hasResults
			if s.hasResults() {
				s.logger.Info().Msg("Página de resultados carregada")
				return nil
			}
		}
	}
}

// hasResults verifica se a página tem resultados ou erro
func (s *SintegraMAScraper) hasResults() bool {
	return s.page.MustHas(selectorResultsTable) || s.page.MustHas(selectorErrorMessage)
}

// navigateToDetailsPage navega para a página de detalhes
func (s *SintegraMAScraper) navigateToDetailsPage() error {
	s.logger.Info().Msg("🔍 Procurando elemento para acessar detalhes...")

	// Usar o seletor específico do main.go
	detailElement, err := s.page.Timeout(s.config.Timeout).Element("#j_id6\\:pnlCadastro img")
	if err != nil {
		s.logger.Error().Err(err).Msg("Elemento de detalhes não encontrado")
		return fmt.Errorf("elemento de detalhes não encontrado: %w", err)
	}

	err = detailElement.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao clicar no elemento de detalhes")
		return fmt.Errorf("erro ao clicar no elemento de detalhes: %w", err)
	}

	s.logger.Info().Msg("✓ Clicou no elemento de detalhes, aguardando página final...")

	// Aguardar carregamento da página de detalhes
	err = s.page.WaitLoad()
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao aguardar carregamento da página de detalhes")
		return fmt.Errorf("erro ao aguardar carregamento: %w", err)
	}

	// Verificar se chegou na página de detalhes
	for j := 0; j < 10; j++ {
		detailPageLoaded := s.page.MustEval(`() => {
			return window.location.href.includes('consultaSintegraResultadoConsulta.jsf');
		}`).Bool()

		if detailPageLoaded {
			s.logger.Info().Msg("✓ Página de detalhes carregada com sucesso!")
			return nil
		}

		if j == 9 {
			s.logger.Warn().Msg("⚠️ Timeout aguardando página de detalhes")
			return fmt.Errorf("timeout aguardando página de detalhes após clique")
		}

		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// finalizarConsulta finaliza a consulta com sucesso
func (s *SintegraMAScraper) finalizarConsulta(start time.Time) {
	s.result.ExecutionTime = time.Since(start)
	s.result.Status = "sucesso"
	s.logger.Info().Dur("execution_time", s.result.ExecutionTime).Msg("Consulta concluída com sucesso")
}

// GetResult retorna o resultado da consulta
func (s *SintegraMAScraper) GetResult() *SintegraMAResult {
	return s.result
}

// Close fecha recursos
func (s *SintegraMAScraper) Close() {
	if s.browser != nil {
		s.browser.MustClose()
		s.logger.Info().Msg("Navegador fechado")
	}
}

// resolverRecaptcha resolve o reCAPTCHA usando SolveCaptcha API e aplica na página
func (s *SintegraMAScraper) resolverRecaptcha() error {
	s.logger.Info().Msg("Iniciando resolução do reCAPTCHA")

	// Se não tiver API key, retorna erro para resolução manual
	if s.config.SolveCaptchaAPIKey == "" {
		return fmt.Errorf("API key não configurada")
	}

	// Encontrar o elemento do reCAPTCHA para extrair a sitekey
	recaptchaFrame, err := s.page.Element("iframe[src*='recaptcha']")
	if err != nil {
		return fmt.Errorf("iframe do reCAPTCHA não encontrado: %v", err)
	}

	src, err := recaptchaFrame.Attribute("src")
	if err != nil {
		return fmt.Errorf("erro ao obter src do iframe: %v", err)
	}

	// Extrair sitekey da URL do iframe
	var sitekey string
	if strings.Contains(*src, "k=") {
		parts := strings.Split(*src, "k=")
		if len(parts) > 1 {
			keyPart := strings.Split(parts[1], "&")[0]
			sitekey = keyPart
		}
	}

	if sitekey == "" {
		return fmt.Errorf("sitekey do reCAPTCHA não encontrada")
	}

	s.logger.Info().
		Str("sitekey", sitekey).
		Msg("Sitekey extraída, resolvendo CAPTCHA")

	// Resolver CAPTCHA usando SolveCaptcha API
	currentURL := s.page.MustInfo().URL
	ctx, cancel := context.WithTimeout(context.Background(), s.config.Timeout)
	defer cancel()
	token, err := s.captchaSolver.SolveCaptcha(ctx, sitekey, currentURL)
	if err != nil {
		return fmt.Errorf("erro na resolução do CAPTCHA: %v", err)
	}

	s.result.CaptchaSolved = true
	s.logger.Info().Msg("CAPTCHA resolvido, iniciando injeção robusta")

	// Encontrar o elemento g-recaptcha-response
	s.logger.Info().Msg("Procurando elemento g-recaptcha-response...")
	responseElement := s.page.MustElement("#g-recaptcha-response")
	s.logger.Info().Msg("Elemento g-recaptcha-response encontrado")

	// Injetar token seguindo padrão dos solvers (2Captcha, CapMonster, etc)
	s.logger.Info().Str("token_preview", token[:min(20, len(token))]+"...").Int("token_length", len(token)).Msg("🔍 Iniciando injeção (padrão solver)")

	// Injetar token com segurança no contexto do elemento e disparar eventos
	res, err := responseElement.Eval(`(token) => {
		if (typeof token !== 'string' || token.length === 0) {
			return {ok:false, msg:'token_invalid'};
		}
		
		// Definir valor no textarea
		this.value = token;
		
		// Garantir hidden no form (padrão dos solvers)
		var form = (this.closest ? this.closest('form') : (function(){
			var e=this; 
			while(e && e.nodeName && e.nodeName.toLowerCase()!=='form') e=e.parentElement; 
			return e;
		}).call(this));
		
		if (form) {
			var h = form.querySelector('input[name="g-recaptcha-response"]');
			if (!h) { 
				h = document.createElement('input'); 
				h.type='hidden'; 
				h.name='g-recaptcha-response'; 
				form.appendChild(h); 
			}
			h.value = token;
		}
		
		// Disparar eventos que os scripts da página escutam
		try { 
			this.dispatchEvent(new Event('input',{bubbles:true})); 
			this.dispatchEvent(new Event('change',{bubbles:true})); 
		} catch(e){}
		
		// Verificar e chamar callbacks específicos (padrão solver avançado)
		if (typeof window.grecaptcha !== 'undefined') {
			// Sobrescrever getResponse
			window.grecaptcha.getResponse = function() { return token; };
			
			// Procurar por callbacks registrados no site
			if (window.onRecaptchaSuccess && typeof window.onRecaptchaSuccess === 'function') {
				try { window.onRecaptchaSuccess(token); } catch(e){}
			}
			if (window.recaptchaCallback && typeof window.recaptchaCallback === 'function') {
				try { window.recaptchaCallback(token); } catch(e){}
			}
			if (window.onCaptchaSuccess && typeof window.onCaptchaSuccess === 'function') {
				try { window.onCaptchaSuccess(token); } catch(e){}
			}
		}
		
		// Verificar se há listeners de submit que precisamos acionar
		if (form && form.onsubmit && typeof form.onsubmit === 'function') {
			try { form.onsubmit(); } catch(e){}
		}
		
		return {ok:true, msg:'injected'};
	}`, token)
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao injetar token (Eval)")
		return fmt.Errorf("erro ao injetar token: %v", err)
	}

	// Verificar retorno do Eval
	resultStr := res.Value.String()
	s.logger.Info().Str("eval_result", resultStr).Msg("Resultado da injeção")

	// Se contém "ok":true ou ok:true ou injected, foi sucesso
	if strings.Contains(resultStr, `"ok":true`) || strings.Contains(resultStr, `ok:true`) || strings.Contains(resultStr, `injected`) {
		s.logger.Info().Msg("✓ Token injetado com sucesso (padrão solver)")
	} else {
		s.logger.Error().Str("result", resultStr).Msg("✗ Falha na injeção")
		return fmt.Errorf("falha na injeção: %s", resultStr)
	}

	// Garantir processamento (padrão dos solvers: 300ms)
	time.Sleep(300 * time.Millisecond)

	// Verificar se o reCAPTCHA foi aceito pela página
	recaptchaStatus := s.page.MustEval(`() => {
		var responseElement = document.querySelector('#g-recaptcha-response');
		var hasValue = responseElement && responseElement.value && responseElement.value.length > 0;
		var grecaptchaReady = typeof window.grecaptcha !== 'undefined';
		return {
			hasToken: hasValue,
			tokenLength: hasValue ? responseElement.value.length : 0,
			grecaptchaReady: grecaptchaReady
		};
	}`).String()
	s.logger.Info().Str("recaptcha_status", recaptchaStatus).Msg("Status do reCAPTCHA verificado")

	return nil
}



// extrairDetalhes extrai os detalhes da consulta
func (s *SintegraMAScraper) extrairDetalhes() error {
	s.logger.Info().Msg("Extraindo detalhes da consulta")

	// Aguardar a página de resultados carregar completamente
	s.logger.Info().Msg("Aguardando página de resultados carregar...")
	time.Sleep(2 * time.Second)

	// Clicar diretamente no link de detalhes usando MustElement
	s.logger.Info().Msg("Procurando e clicando no link de detalhes...")
	linkDetalhes := s.page.MustElement("#j_id6\\:pnlCadastro img")
	linkDetalhes.MustClick()

	// Aguardar carregamento da página de detalhes
	s.logger.Info().Msg("Aguardando carregamento da página de detalhes...")
	s.page.MustWaitLoad()
	time.Sleep(s.config.WaitBetweenSteps)

	// Verificar URL de detalhes
	urlDetalhes := "https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraResultadoConsulta.jsf"
	currentURL := s.page.MustEval(`() => window.location.href`).String()
	if currentURL != urlDetalhes {
		s.logger.Warn().Str("url_atual", currentURL).Str("url_esperada", urlDetalhes).Msg("⚠️ URL de detalhes diferente da esperada")
		// Não retornar erro, apenas logar o aviso e continuar
	}

	// Extrair dados da página
	s.extrairDadosPagina()

	s.logger.Info().
		Str("razao_social", s.result.Data.RazaoSocial).
		Str("cgc", s.result.Data.CGC).
		Str("situacao", s.result.Data.SituacaoCadastral).
		Int("cnaes_secundarios", len(s.result.Data.CNAESecundarios)).
		Msg("Detalhes extraídos com sucesso")

	return nil
}



// extrairDadosPagina extrai dados da página usando goquery
func (s *SintegraMAScraper) extrairDadosPagina() {
	s.logger.Info().Msg("Extraindo dados da página usando goquery")

	// Extrair o HTML completo da página
	htmlCompleto := s.page.MustEval(`() => {
		return document.documentElement.outerHTML;
	}`).String()

	s.logger.Info().Int("html_length", len(htmlCompleto)).Msg("HTML extraído da página")

	// Usar goquery para parsing HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlCompleto))
	if err != nil {
		s.logger.Error().Err(err).Msg("Erro ao criar documento goquery")
		return
	}

	// Usar goquery para extrair os dados estruturados
	s.extrairIdentificacaoGoquery(doc)
	s.extrairEnderecoGoquery(doc)
	s.extrairCNAEGoquery(doc)
	s.extrairSituacaoGoquery(doc)
	s.extrairObrigacoesGoquery(doc)
	s.extrairMetadadosGoquery(doc)

	// Log de resumo dos dados extraídos
	s.logger.Info().
		Str("razao_social", s.result.Data.RazaoSocial).
		Str("cgc", s.result.Data.CGC).
		Str("situacao", s.result.Data.SituacaoCadastral).
		Int("cnaes_secundarios", len(s.result.Data.CNAESecundarios)).
		Msg("Dados estruturados extraídos com sucesso usando goquery")
}

func (s *SintegraMAScraper) extrairIdentificacaoGoquery(doc *goquery.Document) {
	// Extrair dados usando seletores CSS mais precisos
	doc.Find("td").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())

		// Extrair CGC - buscar por padrão de CNPJ
		if matches := regexp.MustCompile(`(\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2})`).FindStringSubmatch(text); len(matches) > 1 {
			s.result.Data.CGC = strings.TrimSpace(matches[1])
		}

		// Extrair Inscrição Estadual
		if strings.Contains(text, "Inscrição Estadual") || strings.Contains(text, "Inscri") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				ie := strings.TrimSpace(nextTd.Text())
				if ie != "" && ie != "Inscrição Estadual:" {
					s.result.Data.InscricaoEstadual = ie
				}
			}
		}

		// Extrair Razão Social
		if strings.Contains(text, "Razão Social") || strings.Contains(text, "Raz") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				rs := strings.TrimSpace(nextTd.Text())
				if rs != "" && rs != "Razão Social:" {
					s.result.Data.RazaoSocial = rs
				}
			}
		}

		// Extrair Regime de Apuração
		if strings.Contains(text, "Regime") && strings.Contains(text, "Apura") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				ra := strings.TrimSpace(nextTd.Text())
				if ra != "" && !strings.Contains(ra, "Regime") {
					s.result.Data.RegimeApuracao = ra
				}
			}
		}
	})
}

func (s *SintegraMAScraper) extrairEnderecoGoquery(doc *goquery.Document) {
	s.result.Data.Endereco = &EnderecoData{}

	// Extrair dados de endereço usando seletores CSS
	doc.Find("td").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())

		// Extrair Logradouro
		if strings.Contains(text, "Logradouro") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				logr := strings.TrimSpace(nextTd.Text())
				if logr != "" && !strings.Contains(logr, "Logradouro") {
					s.result.Data.Endereco.Logradouro = logr
				}
			}
		}

		// Extrair Número
		if strings.Contains(text, "Número") || strings.Contains(text, "mero") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				num := strings.TrimSpace(nextTd.Text())
				if num != "" && !strings.Contains(num, "mero") {
					s.result.Data.Endereco.Numero = num
				}
			}
		}

		// Extrair Complemento
		if strings.Contains(text, "Complemento") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				comp := strings.TrimSpace(nextTd.Text())
				if comp != "" && !strings.Contains(comp, "Complemento") {
					s.result.Data.Endereco.Complemento = comp
				}
			}
		}

		// Extrair Bairro
		if strings.Contains(text, "Bairro") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				bairro := strings.TrimSpace(nextTd.Text())
				if bairro != "" && !strings.Contains(bairro, "Bairro") {
					s.result.Data.Endereco.Bairro = bairro
				}
			}
		}

		// Extrair Município
		if strings.Contains(text, "Município") || strings.Contains(text, "pio") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				mun := strings.TrimSpace(nextTd.Text())
				if mun != "" && !strings.Contains(mun, "pio") {
					s.result.Data.Endereco.Municipio = mun
				}
			}
		}

		// Extrair UF
		if text == "UF:" {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				uf := strings.TrimSpace(nextTd.Text())
				if len(uf) == 2 {
					s.result.Data.Endereco.UF = uf
				}
			}
		}

		// Extrair CEP
		if strings.Contains(text, "CEP") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				cep := strings.TrimSpace(nextTd.Text())
				if cep != "" && !strings.Contains(cep, "CEP") {
					s.result.Data.Endereco.CEP = cep
				}
			}
		}

		// Extrair DDD
		if strings.Contains(text, "DDD") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				ddd := strings.TrimSpace(nextTd.Text())
				if ddd != "" && !strings.Contains(ddd, "DDD") {
					s.result.Data.Endereco.DDD = ddd
				}
			}
		}

		// Extrair Telefone
		if strings.Contains(text, "Telefone") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				tel := strings.TrimSpace(nextTd.Text())
				if tel != "" && !strings.Contains(tel, "Telefone") {
					s.result.Data.Endereco.Telefone = tel
				}
			}
		}
	})
}

func (s *SintegraMAScraper) extrairCNAEGoquery(doc *goquery.Document) {
	// Extrair CNAE Principal
	doc.Find("td").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())

		if strings.Contains(text, "CNAE Principal") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				cnae := strings.TrimSpace(nextTd.Text())
				if cnae != "" && !strings.Contains(cnae, "CNAE") {
					s.result.Data.CNAEPrincipal = cnae
				}
			}
		}
	})

	// Extrair CNAEs Secundários
	s.result.Data.CNAESecundarios = []CNAEData{}

	// Procurar por tabelas ou seções que contenham CNAEs secundários
	doc.Find("tr").Each(func(i int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() >= 2 {
			firstCell := strings.TrimSpace(cells.First().Text())
			secondCell := strings.TrimSpace(cells.Eq(1).Text())

			// Verificar se a primeira célula contém um código CNAE (7 dígitos)
			if matches := regexp.MustCompile(`^(\d{7})$`).FindStringSubmatch(firstCell); len(matches) > 1 {
				cnae := CNAEData{
					Codigo:    matches[1],
					Descricao: secondCell,
				}
				s.result.Data.CNAESecundarios = append(s.result.Data.CNAESecundarios, cnae)
			}
		}
	})
}

func (s *SintegraMAScraper) extrairSituacaoGoquery(doc *goquery.Document) {
	// Extrair Situação Cadastral
	doc.Find("td").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())

		if strings.Contains(text, "Situação Cadastral Vigente") || strings.Contains(text, "Situa") && strings.Contains(text, "Cadastral") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				sit := strings.TrimSpace(nextTd.Text())
				if sit != "" && !strings.Contains(sit, "Situa") {
					s.result.Data.SituacaoCadastral = sit
				}
			}
		}

		if strings.Contains(text, "Data desta Situação") || strings.Contains(text, "Data desta Situa") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				data := strings.TrimSpace(nextTd.Text())
				if data != "" && !strings.Contains(data, "Data") {
					s.result.Data.DataSituacaoCadastral = data
				}
			}
		}
	})
}

func (s *SintegraMAScraper) extrairObrigacoesGoquery(doc *goquery.Document) {
	s.result.Data.Obrigacoes = &ObrigacoesData{}

	// Extrair obrigações usando seletores CSS
	doc.Find("td").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())

		if strings.Contains(text, "NFe a partir de") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				nfe := strings.TrimSpace(nextTd.Text())
				if nfe != "" && !strings.Contains(nfe, "NFe") {
					s.result.Data.Obrigacoes.NFeAPartirDe = nfe
				}
			}
		}

		if strings.Contains(text, "EDF a partir de") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				edf := strings.TrimSpace(nextTd.Text())
				if edf != "" && !strings.Contains(edf, "EDF") {
					s.result.Data.Obrigacoes.EDFAPartirDe = edf
				}
			}
		}

		if strings.Contains(text, "CTE a partir de") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				cte := strings.TrimSpace(nextTd.Text())
				if cte != "" && !strings.Contains(cte, "CTE") {
					s.result.Data.Obrigacoes.CTEAPartirDe = cte
				}
			}
		}
	})
}

func (s *SintegraMAScraper) extrairMetadadosGoquery(doc *goquery.Document) {
	// Extrair metadados usando seletores CSS
	doc.Find("td").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())

		if strings.Contains(text, "Data da Consulta") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				data := strings.TrimSpace(nextTd.Text())
				if data != "" && !strings.Contains(data, "Data") {
					s.result.Data.DataConsulta = data
				}
			}
		}

		if strings.Contains(text, "Número da Consulta") || strings.Contains(text, "mero da Consulta") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				num := strings.TrimSpace(nextTd.Text())
				if num != "" && !strings.Contains(num, "mero") {
					s.result.Data.NumeroConsulta = num
				}
			}
		}

		if strings.Contains(text, "Observação") || strings.Contains(text, "Observa") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				obs := strings.TrimSpace(nextTd.Text())
				if obs != "" && !strings.Contains(obs, "Observa") {
					s.result.Data.Observacao = obs
				}
			}
		}
	})
}

// consultarCNPJInternal executa a consulta completa no Sintegra MA (método interno usado pelos workers)
func (s *SintegraService) consultarCNPJInternal(cnpj string) (*models.SintegraResponse, error) {
	s.logger.Info().Str("cnpj", cnpj).Msg("🚀 Iniciando consulta via API")

	// Validar CNPJ antes de prosseguir
	if err := s.validateCNPJFormat(cnpj); err != nil {
		return nil, fmt.Errorf("CNPJ inválido: %w", err)
	}

	// Carregar configuração
	config := s.loadConfig()

	// Criar scraper usando as funções do main.go
	scraper := s.createScraper(config)
	defer func() {
		scraper.Close()
		s.logger.Debug().Msg("Scraper fechado")
	}()

	// Inicializar navegador
	if err := scraper.Initialize(); err != nil {
		s.logger.Error().Err(err).Msg("❌ Erro na inicialização do navegador")
		return nil, fmt.Errorf("erro na inicialização: %w", err)
	}

	// Executar consulta com timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.timeoutConfig.SintegraRequestTimeout)
	defer cancel()

	resultChan := make(chan *SintegraMAResult, 1)
	errorChan := make(chan error, 1)

	go func() {
		if err := scraper.ConsultarCNPJ(cnpj); err != nil {
			errorChan <- err
			return
		}
		resultChan <- scraper.GetResult()
	}()

	select {
	case <-ctx.Done():
		s.logger.Error().Str("cnpj", cnpj).Msg("❌ Timeout na consulta")
		return nil, fmt.Errorf("timeout na consulta: %w", ctx.Err())
	case err := <-errorChan:
		s.logger.Error().Err(err).Str("cnpj", cnpj).Msg("❌ Erro na consulta")
		return nil, fmt.Errorf("erro na consulta: %w", err)
	case resultado := <-resultChan:
		// Converter para modelo da API
		response := s.convertToAPIResponse(resultado)

		s.logger.Info().
			Str("cnpj", cnpj).
			Str("status", response.Status).
			Str("execution_time", response.ExecutionTime).
			Msg("✅ Consulta concluída com sucesso")

		return response, nil
	}
}

// validateCNPJFormat valida o formato básico do CNPJ
func (s *SintegraService) validateCNPJFormat(cnpj string) error {
	if cnpj == "" {
		return fmt.Errorf("CNPJ não pode ser vazio")
	}

	// Remover caracteres especiais
	cnpjNumbers := regexp.MustCompile(`[^\d]`).ReplaceAllString(cnpj, "")

	// Verificar se tem 14 dígitos
	if matched, _ := regexp.MatchString(cnpjNumbersPattern, cnpjNumbers); !matched {
		return fmt.Errorf("CNPJ deve conter exatamente 14 dígitos")
	}

	return nil
}

// ConsultarCNPJ executa a consulta completa no Sintegra MA usando o worker pool
func (s *SintegraService) ConsultarCNPJ(cnpj string) (*models.SintegraResponse, error) {
	s.logger.Info().Str("cnpj", cnpj).Msg("🔄 Enfileirando consulta para processamento paralelo")

	// Registra a consulta como em andamento
	s.consultasEmAndamentoMux.Lock()
	s.consultasEmAndamento[cnpj] = true
	s.consultasEmAndamentoMux.Unlock()

	// Enfileirar job no worker pool
	timeout := s.timeoutConfig.SintegraRequestTimeout
	resultado, err := s.workerPool.EnqueueJob(cnpj, timeout)

	// Remove a consulta do mapa de consultas em andamento
	s.consultasEmAndamentoMux.Lock()
	delete(s.consultasEmAndamento, cnpj)
	s.consultasEmAndamentoMux.Unlock()

	return resultado, err
}

// ConsultarCNPJEmLote executa consultas em lote para múltiplos CNPJs
func (s *SintegraService) ConsultarCNPJEmLote(cnpjs []string) *models.BatchSintegraResponse {
	s.logger.Info().Int("total_cnpjs", len(cnpjs)).Msg("🔄 Iniciando consulta em lote")

	// Iniciar contagem de tempo
	start := time.Now()

	// Preparar resposta
	response := &models.BatchSintegraResponse{
		Total:        len(cnpjs),
		SuccessCount: 0,
		ErrorCount:   0,
		Results:      make(map[string]*models.SintegraResponse),
		Errors:       make(map[string]string),
		Timestamp:    time.Now(),
	}

	// Canais para coletar resultados
	type resultItem struct {
		cnpj     string
		response *models.SintegraResponse
		err      error
	}

	// Canal para receber resultados
	resultsChan := make(chan resultItem, len(cnpjs))

	// Iniciar consultas em paralelo
	for _, cnpj := range cnpjs {
		go func(cnpj string) {
			// Consultar CNPJ
			result, err := s.ConsultarCNPJ(cnpj)

			// Enviar resultado para o canal
			resultsChan <- resultItem{
				cnpj:     cnpj,
				response: result,
				err:      err,
			}
		}(cnpj)
	}

	// Coletar resultados
	for i := 0; i < len(cnpjs); i++ {
		result := <-resultsChan

		if result.err != nil {
			// Registrar erro
			response.ErrorCount++
			response.Errors[result.cnpj] = result.err.Error()
			s.logger.Error().Err(result.err).Str("cnpj", result.cnpj).Msg("❌ Erro na consulta em lote")
		} else {
			// Registrar sucesso
			response.SuccessCount++
			response.Results[result.cnpj] = result.response
			s.logger.Info().Str("cnpj", result.cnpj).Msg("✅ Consulta em lote concluída com sucesso")
		}
	}

	// Calcular tempo total
	duration := time.Since(start)
	response.ExecutionTime = duration.String()

	s.logger.Info().
		Int("total", response.Total).
		Int("success", response.SuccessCount).
		Int("errors", response.ErrorCount).
		Str("execution_time", response.ExecutionTime).
		Msg("✅ Processamento em lote concluído")

	return response
}

// VerificarStatusConsulta verifica se uma consulta está em andamento
func (s *SintegraService) VerificarStatusConsulta(cnpj string) (*models.StatusResponse, error) {
	// Verificar se o CNPJ está em processamento
	s.consultasEmAndamentoMux.RLock()
	emAndamento := s.consultasEmAndamento[cnpj]
	s.consultasEmAndamentoMux.RUnlock()

	response := &models.StatusResponse{
		CNPJ:      cnpj,
		Timestamp: time.Now(),
	}

	if emAndamento {
		// A consulta está em andamento
		response.Status = "em_andamento"
		response.Mensagem = "Consulta em processamento. Aguarde."
		// Tempo estimado baseado na média de consultas (15 segundos é um valor aproximado)
		response.TempoEstimado = 15
	} else {
		// Verificar se a consulta já foi realizada anteriormente
		// Aqui poderia ser implementado um cache ou banco de dados para consultas anteriores
		// Por enquanto, apenas informamos que a consulta não está em andamento
		response.Status = "nao_encontrada"
		response.Mensagem = "Consulta não está em andamento. Inicie uma nova consulta."
	}

	return response, nil
}

// loadConfig carrega configurações usando as variáveis de ambiente
func (s *SintegraService) loadConfig() *Config {
	// Tentar carregar arquivo .env se existir
	if _, err := os.Stat(".env"); err == nil {
		if err := s.loadEnvFile(".env"); err != nil {
			s.logger.Warn().Err(err).Msg("Aviso: erro ao carregar .env")
		}
	}

	// Obter configurações de timeout padrão
	timeoutConfig := s.timeoutConfig

	config := &Config{
		SolveCaptchaAPIKey: os.Getenv("SOLVECAPTCHA_API_KEY"),
		Headless:           false,                                // Forçando headless false conforme especificação
		Timeout:            timeoutConfig.SintegraRequestTimeout, // Usando timeout configurado
		WaitBetweenSteps:   2 * time.Second,
		UserAgent:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		ViewportWidth:      1115,
		ViewportHeight:     639,
		MaxRetries:         int(timeoutConfig.SintegraCaptchaTimeout.Seconds() / 5), // Calcular com base no timeout configurado
		RetryDelay:         5 * time.Second,
		TimeoutConfig:      timeoutConfig, // Adicionar configurações de timeout
	}

	// Validação da API key
	if config.SolveCaptchaAPIKey == "" {
		s.logger.Warn().Msg("AVISO: SOLVECAPTCHA_API_KEY não configurada. Tentando resolver CAPTCHA manualmente...")
	} else {
		s.logger.Info().Str("key_preview", config.SolveCaptchaAPIKey[:8]+"...").Msg("✓ SolveCaptcha API configurada")
	}

	return config
}

// createScraper cria uma instância do scraper real
func (s *SintegraService) createScraper(config *Config) *SintegraMAScraper {
	// Validar configuração antes de criar scraper
	if err := s.validateConfig(config); err != nil {
		s.logger.Error().Err(err).Msg("Configuração inválida")
		// Retornar scraper com configuração padrão em caso de erro
		config = NewDefaultConfig()
	}

	return &SintegraMAScraper{
		config:        config,
		captchaSolver: NewCaptchaSolver(config.SolveCaptchaAPIKey, s.logger),
		logger:        s.logger,
		result: &SintegraMAResult{
			Data:      &SintegraData{},
			Timestamp: time.Now(),
		},
	}
}

// validateConfig valida a configuração do serviço
func (s *SintegraService) validateConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("configuração não pode ser nula")
	}

	if config.Timeout <= 0 {
		return fmt.Errorf("timeout deve ser maior que zero")
	}

	if config.ViewportWidth <= 0 || config.ViewportHeight <= 0 {
		return fmt.Errorf("dimensões do viewport devem ser maiores que zero")
	}

	return nil
}

// convertToAPIResponse converte o resultado do scraper para o modelo da API
func (s *SintegraService) convertToAPIResponse(scraperResult *SintegraMAResult) *models.SintegraResponse {
	// Converter estrutura interna para estrutura da API
	apiData := &models.SintegraData{
		CGC:                   scraperResult.Data.CGC,
		InscricaoEstadual:     scraperResult.Data.InscricaoEstadual,
		RazaoSocial:           scraperResult.Data.RazaoSocial,
		RegimeApuracao:        scraperResult.Data.RegimeApuracao,
		SituacaoCadastral:     scraperResult.Data.SituacaoCadastral,
		DataSituacaoCadastral: scraperResult.Data.DataSituacaoCadastral,
		CNAEPrincipal:         scraperResult.Data.CNAEPrincipal,
		DataConsulta:          scraperResult.Data.DataConsulta,
		NumeroConsulta:        scraperResult.Data.NumeroConsulta,
		Observacao:            scraperResult.Data.Observacao,
	}

	// Converter endereço
	if scraperResult.Data.Endereco != nil {
		apiData.Endereco = &models.EnderecoData{
			Logradouro:  scraperResult.Data.Endereco.Logradouro,
			Numero:      scraperResult.Data.Endereco.Numero,
			Complemento: scraperResult.Data.Endereco.Complemento,
			Bairro:      scraperResult.Data.Endereco.Bairro,
			Municipio:   scraperResult.Data.Endereco.Municipio,
			UF:          scraperResult.Data.Endereco.UF,
			CEP:         scraperResult.Data.Endereco.CEP,
			DDD:         scraperResult.Data.Endereco.DDD,
			Telefone:    scraperResult.Data.Endereco.Telefone,
		}
	}

	// Converter CNAEs secundários
	for _, cnae := range scraperResult.Data.CNAESecundarios {
		apiData.CNAESecundarios = append(apiData.CNAESecundarios, models.CNAEData{
			Codigo:    cnae.Codigo,
			Descricao: cnae.Descricao,
		})
	}

	// Converter obrigações
	if scraperResult.Data.Obrigacoes != nil {
		apiData.Obrigacoes = &models.ObrigacoesData{
			NFeAPartirDe: scraperResult.Data.Obrigacoes.NFeAPartirDe,
			EDFAPartirDe: scraperResult.Data.Obrigacoes.EDFAPartirDe,
			CTEAPartirDe: scraperResult.Data.Obrigacoes.CTEAPartirDe,
		}
	}

	return &models.SintegraResponse{
		CNPJ:          scraperResult.CNPJ,
		Status:        scraperResult.Status,
		URL:           scraperResult.URL,
		ExecutionTime: scraperResult.ExecutionTime.String(),
		Timestamp:     scraperResult.Timestamp,
		CaptchaSolved: scraperResult.CaptchaSolved,
		Data:          apiData,
	}
}

// loadEnvFile carrega variáveis de um arquivo .env
func (s *SintegraService) loadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
		}
	}

	return nil
}

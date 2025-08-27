package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// Config contém as configurações da aplicação
type Config struct {
	SolveCaptchaAPIKey string        `json:"solvecaptcha_api_key"`
	Headless           bool          `json:"headless"`
	Timeout            time.Duration `json:"timeout"`
	WaitBetweenSteps   time.Duration `json:"wait_between_steps"`
	UserAgent          string        `json:"user_agent"`
	ViewportWidth      int           `json:"viewport_width"`
	ViewportHeight     int           `json:"viewport_height"`
	MaxRetries         int           `json:"max_retries"`
	RetryDelay         time.Duration `json:"retry_delay"`
}

// LoadConfig carrega configurações das variáveis de ambiente
func LoadConfig() *Config {
	// Tentar carregar arquivo .env se existir
	if _, err := os.Stat(".env"); err == nil {
		if err := loadEnvFile(".env"); err != nil {
			log.Printf("Aviso: erro ao carregar .env: %v", err)
		}
	}

	config := &Config{
		SolveCaptchaAPIKey: os.Getenv("SOLVECAPTCHA_API_KEY"),
		Headless:           true,              // Temporariamente headless true para debug
		Timeout:            180 * time.Second, // Reduzindo timeout para evitar travamentos
		WaitBetweenSteps:   2 * time.Second,
		UserAgent:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		ViewportWidth:      1115,
		ViewportHeight:     639,
		MaxRetries:         36, // 36 tentativas x 5 segundos = 180 segundos (3 minutos)
		RetryDelay:         5 * time.Second,
	}

	// Validação da API key
	if config.SolveCaptchaAPIKey == "" {
		log.Printf("AVISO: SOLVECAPTCHA_API_KEY não configurada. Tentando resolver CAPTCHA manualmente...")
	} else {
		log.Printf("✓ SolveCaptcha API configurada (key: %s...)", config.SolveCaptchaAPIKey[:8])
	}

	return config
}

// loadEnvFile carrega variáveis de um arquivo .env
func loadEnvFile(filename string) error {
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

// SolveCaptchaResultResponse representa a resposta do resultado
type SolveCaptchaResultResponse struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
	Error   string `json:"error"`
}

// CaptchaSolver gerencia a resolução de CAPTCHA
type CaptchaSolver struct {
	config *Config
	client *http.Client
	logger zerolog.Logger
}

// NewCaptchaSolver cria um novo resolver de CAPTCHA
func NewCaptchaSolver(config *Config, logger zerolog.Logger) *CaptchaSolver {
	return &CaptchaSolver{
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

// SubmitCaptcha submete um CAPTCHA para resolução
func (cs *CaptchaSolver) SubmitCaptcha(googleKey, pageURL string) (string, error) {
	if cs.config.SolveCaptchaAPIKey == "" {
		return "", fmt.Errorf("API key não configurada")
	}

	cs.logger.Info().
		Str("googlekey", googleKey).
		Str("pageurl", pageURL).
		Msg("Submetendo CAPTCHA para resolução")

	url := "https://api.solvecaptcha.com/in.php"
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	_ = writer.WriteField("key", cs.config.SolveCaptchaAPIKey)
	_ = writer.WriteField("method", "userrecaptcha")
	_ = writer.WriteField("googlekey", googleKey)
	_ = writer.WriteField("pageurl", pageURL)
	_ = writer.WriteField("json", "1")

	err := writer.Close()
	if err != nil {
		return "", fmt.Errorf("erro ao preparar payload: %v", err)
	}

	req, err := http.NewRequest("POST", url, payload)
	if err != nil {
		return "", fmt.Errorf("erro ao criar requisição: %v", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := cs.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro na requisição: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta: %v", err)
	}

	bodyStr := string(body)
	cs.logger.Info().Str("submit_response", bodyStr).Msg("Resposta da submissão")

	// Tentar parsear como JSON primeiro
	var response SolveCaptchaResponse
	if err := json.Unmarshal(body, &response); err == nil {
		// Resposta JSON
		if response.Status != 1 {
			return "", fmt.Errorf("erro na submissão (JSON): %s", response.Error)
		}
		cs.logger.Info().Str("task_id", response.Request).Msg("CAPTCHA submetido com sucesso (JSON)")
		return response.Request, nil
	} else {
		// Resposta texto simples (formato OK|taskid)
		if strings.HasPrefix(bodyStr, "OK|") {
			taskID := strings.TrimPrefix(bodyStr, "OK|")
			taskID = strings.TrimSpace(taskID)
			cs.logger.Info().Str("task_id", taskID).Msg("CAPTCHA submetido com sucesso (texto)")
			return taskID, nil
		}

		// Erro em formato texto
		return "", fmt.Errorf("erro na submissão (texto): %s", bodyStr)
	}
}

// GetCaptchaResult obtém o resultado da resolução do CAPTCHA
func (cs *CaptchaSolver) GetCaptchaResult(taskID string) (string, error) {
	url := fmt.Sprintf("https://api.solvecaptcha.com/res.php?key=%s&action=get&id=%s&json=1",
		cs.config.SolveCaptchaAPIKey, taskID)

	for i := 0; i < cs.config.MaxRetries; i++ {
		tempoDecorrido := time.Duration(i) * cs.config.RetryDelay
		tempoRestante := time.Duration(cs.config.MaxRetries-i) * cs.config.RetryDelay
		porcentagem := float64(i) / float64(cs.config.MaxRetries) * 100

		cs.logger.Info().
			Str("task_id", taskID).
			Int("attempt", i+1).
			Int("max_attempts", cs.config.MaxRetries).
			Dur("tempo_decorrido", tempoDecorrido).
			Dur("tempo_restante", tempoRestante).
			Float64("progresso_pct", porcentagem).
			Msg("Consultando resultado do CAPTCHA")

		resp, err := cs.client.Get(url)
		if err != nil {
			cs.logger.Error().Err(err).Msg("Erro na consulta HTTP")
			time.Sleep(cs.config.RetryDelay)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			cs.logger.Error().Err(err).Msg("Erro ao ler resposta")
			time.Sleep(cs.config.RetryDelay)
			continue
		}

		bodyStr := string(body)
		cs.logger.Info().Str("response_body", bodyStr).Msg("Resposta da API")

		// Tentar parsear como JSON primeiro
		var result SolveCaptchaResultResponse
		if err := json.Unmarshal(body, &result); err == nil {
			// Resposta JSON
			cs.logger.Info().
				Int("status", result.Status).
				Str("error", result.Error).
				Str("request", result.Request).
				Msg("Resposta JSON recebida")

			if result.Status == 1 {
				cs.logger.Info().
					Str("task_id", taskID).
					Str("token_preview", result.Request[:min(10, len(result.Request))]+"...").
					Msg("CAPTCHA resolvido com sucesso (JSON)")
				return result.Request, nil
			}

			// Status=0 significa "ainda processando" - continuar tentando
			if result.Status == 0 || result.Request == "CAPCHA_NOT_READY" {
				cs.logger.Info().
					Str("status_msg", result.Request).
					Msg("CAPTCHA ainda processando (JSON), aguardando...")
				time.Sleep(cs.config.RetryDelay)
				continue
			}

			// Outros erros com status diferente de 0 ou 1
			if result.Status != 1 && result.Status != 0 {
				return "", fmt.Errorf("erro da API SolveCaptcha (JSON): status=%d, error=%s", result.Status, result.Error)
			}
		} else {
			// Resposta texto simples (formato OK|result)
			cs.logger.Info().Msg("Tentando parsear resposta como texto simples")

			if strings.HasPrefix(bodyStr, "OK|") {
				token := strings.TrimPrefix(bodyStr, "OK|")
				token = strings.TrimSpace(token)
				cs.logger.Info().
					Str("task_id", taskID).
					Str("token_preview", token[:min(10, len(token))]+"...").
					Msg("CAPTCHA resolvido com sucesso (texto)")
				return token, nil
			}

			if bodyStr == "CAPCHA_NOT_READY" {
				cs.logger.Info().Msg("CAPTCHA ainda processando (texto)...")
				time.Sleep(cs.config.RetryDelay)
				continue
			}

			// Outros erros
			if bodyStr != "" {
				return "", fmt.Errorf("erro da API SolveCaptcha (texto): %s", bodyStr)
			}
		}

		return "", fmt.Errorf("resposta inválida da API: %s", bodyStr)
	}

	tempoTotalDecorrido := time.Duration(cs.config.MaxRetries) * cs.config.RetryDelay
	return "", fmt.Errorf("timeout na resolução do CAPTCHA após %d tentativas em %v (%.1f minutos)",
		cs.config.MaxRetries, tempoTotalDecorrido, tempoTotalDecorrido.Minutes())
}

// Funcção auxiliar para min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Função auxiliar para max
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Função auxiliar para truncar strings
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// SolveCaptcha resolve um CAPTCHA completo
func (cs *CaptchaSolver) SolveCaptcha(googleKey, pageURL string) (string, error) {
	taskID, err := cs.SubmitCaptcha(googleKey, pageURL)
	if err != nil {
		return "", err
	}

	// Aguardar 20 segundos para reCAPTCHA conforme documentação
	cs.logger.Info().Msg("Aguardando 20 segundos para processamento do reCAPTCHA...")
	time.Sleep(20 * time.Second)

	return cs.GetCaptchaResult(taskID)
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
	logger        zerolog.Logger
	result        *SintegraMAResult
}

// NewSintegraMAScraper cria um novo scraper
func NewSintegraMAScraper(config *Config, logger zerolog.Logger) *SintegraMAScraper {
	return &SintegraMAScraper{
		config:        config,
		captchaSolver: NewCaptchaSolver(config, logger),
		logger:        logger,
		result: &SintegraMAResult{
			Data:      &SintegraData{},
			Timestamp: time.Now(),
		},
	}
}

// Initialize inicializa o navegador
func (s *SintegraMAScraper) Initialize() error {
	s.logger.Info().Msg("Inicializando navegador Chrome")

	// Tentar encontrar Chrome instalado no sistema primeiro
	path, found := launcher.LookPath()
	if found {
		s.logger.Info().Str("chrome_path", path).Msg("Chrome encontrado no sistema")
	} else {
		s.logger.Info().Msg("Chrome não encontrado no sistema, usando download automático")
	}

	// Configurar launcher com Chrome encontrado ou download automático
	l := launcher.New().
		Headless(s.config.Headless).
		Leakless(false).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-setuid-sandbox").
		Set("disable-background-timer-throttling").
		Set("disable-backgrounding-occluded-windows").
		Set("disable-renderer-backgrounding").
		Set("disable-features", "VizDisplayCompositor").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-web-security").
		Set("disable-extensions").
		Set("user-agent", s.config.UserAgent)

	// Se Chrome foi encontrado no sistema, usar esse caminho
	if found {
		l = l.Bin(path)
	}

	url, err := l.Launch()
	if err != nil {
		return fmt.Errorf("falha ao inicializar navegador: %v", err)
	}

	s.logger.Info().Str("url", url).Msg("Navegador iniciado com sucesso")

	// Conectar ao navegador
	browser := rod.New().ControlURL(url).Timeout(s.config.Timeout)
	err = browser.Connect()
	if err != nil {
		return fmt.Errorf("falha ao conectar ao navegador: %v", err)
	}

	s.browser = browser

	// Criar página
	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		browser.Close()
		return fmt.Errorf("falha ao criar página: %v", err)
	}

	err = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  s.config.ViewportWidth,
		Height: s.config.ViewportHeight,
		DeviceScaleFactor: 1,
		Mobile: false,
	})
	if err != nil {
		browser.Close()
		return fmt.Errorf("falha ao configurar viewport: %v", err)
	}

	s.page = page
	s.logger.Info().Msg("Navegador inicializado com sucesso")
	return nil
}
// ConsultarCNPJ executa a consulta completa
func (s *SintegraMAScraper) ConsultarCNPJ(cnpj string) error {
	start := time.Now()
	s.result.CNPJ = cnpj

	s.logger.Info().
		Str("cnpj", cnpj).
		Msg("Iniciando consulta no Sintegra MA")

	// Navegar para página inicial
	baseURL := "https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraFiltro.jsf"
	err := s.page.Navigate(baseURL)
	if err != nil {
		s.result.Status = "erro_navegacao"
		return fmt.Errorf("erro ao navegar: %v", err)
	}

	s.page.MustWaitLoad()
	time.Sleep(s.config.WaitBetweenSteps)

	// Selecionar tipo de emissão
	s.logger.Info().Msg("Selecionando tipo de emissão")
	tipoEmissao := s.page.MustElement("#form1\\:tipoEmissao\\:1")
	tipoEmissao.MustClick()
	time.Sleep(1 * time.Second)

	// Preencher CNPJ
	s.logger.Info().Str("cnpj", cnpj).Msg("Preenchendo CNPJ")
	campoCNPJ := s.page.MustElement("#form1\\:cpfCnpj")
	campoCNPJ.MustClick()
	campoCNPJ.MustSelectAllText()
	campoCNPJ.MustInput(cnpj)
	time.Sleep(1 * time.Second)

	// Tentar resolver reCAPTCHA automaticamente ou manualmente
	if err := s.resolverRecaptcha(); err != nil {
		s.logger.Warn().Err(err).Msg("Erro na resolução automática, aguardando resolução manual")
		// Pausar para resolução manual se API falhar
		s.logger.Info().Msg("Por favor, resolva o CAPTCHA manualmente e pressione Enter para continuar...")
		fmt.Print("Pressione Enter após resolver o CAPTCHA: ")
		fmt.Scanln()
	}

	// Clique no botão real do form (padrão solver)
	s.logger.Info().Msg("🎯 Clicando no botão de consulta")

	// Seletores otimizados (ordem de prioridade) - conforme teste Playwright
	btnSelector := "#form1\\:pnlPrincipal4 input:nth-of-type(2), form#form1 button[type=submit], #botaoConsultar, button[type=submit]"
	btn, err := s.page.Timeout(15 * time.Second).Element(btnSelector)
	if err != nil {
		s.logger.Warn().Err(err).Msg("⚠ Botão não encontrado")
		return fmt.Errorf("botão de submit não encontrado")
	}

	// Aguardar um pouco antes do clique para garantir que o reCAPTCHA foi processado
	time.Sleep(1 * time.Second)

	// Verificar se a página ainda está responsiva antes do clique
	pageReady := s.page.MustEval(`() => {
		return document.readyState === 'complete' && !document.querySelector('.loading');
	}`).Bool()
	s.logger.Info().Bool("page_ready", pageReady).Msg("Estado da página antes do clique")

	// Verificar se o botão está visível e clicável
	btnVisible := btn.MustVisible()
	btnEnabled := btn.MustEval(`() => !this.disabled`).Bool()
	s.logger.Info().Bool("btn_visible", btnVisible).Bool("btn_enabled", btnEnabled).Msg("Estado do botão")

	if !btnVisible || !btnEnabled {
		s.logger.Warn().Msg("Botão não está visível ou habilitado, aguardando...")
		time.Sleep(2 * time.Second)
	}

	// Clicar no botão com timeout customizado
	s.logger.Info().Msg("Executando clique no botão...")
	// Tentar clique direto primeiro
	btn.MustClick()
	s.logger.Info().Msg("✓ Clique executado")

	// Aguardar carregamento/resultado (padrão solver)
	s.page.MustWaitLoad()
	time.Sleep(800 * time.Millisecond)

	// Debug: URL atual após submissão
	s.logger.Info().Str("url_atual", s.page.MustInfo().URL).Msg("🔍 Formulário submetido")

	// Aguardar especificamente pela página de resultados (baseado no teste Playwright)
	urlEsperada := "https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraResultadoListaConsulta.jsf"
	s.logger.Info().Str("url_esperada", urlEsperada).Msg("🔍 Aguardando página de resultados...")

	// Aguardar até 10 segundos pela mudança de URL
	for i := 0; i < 20; i++ {
		urlAtual := s.page.MustInfo().URL
		if strings.Contains(urlAtual, "consultaSintegraResultadoListaConsulta.jsf") {
			s.logger.Info().Str("url_resultado", urlAtual).Msg("✓ Página de resultados carregada!")
			break
		}

		if i == 19 {
			// Última tentativa - verificar se há mensagens de erro na página
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
			return fmt.Errorf("página de resultado não carregada após 10s. URL atual: %s", urlAtual)
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Se chegou até aqui, a página de resultados carregou com sucesso
	s.result.URL = s.page.MustInfo().URL
	s.logger.Info().Msg("✓ Página de resultados carregada, prosseguindo para extração")

	// Extrair detalhes
	if err := s.extrairDetalhes(); err != nil {
		s.result.Status = "erro_extracao"
		return fmt.Errorf("erro na extração: %v", err)
	}

	s.result.ExecutionTime = time.Since(start)
	s.result.Status = "sucesso"

	s.logger.Info().
		Dur("execution_time", s.result.ExecutionTime).
		Msg("Consulta concluída com sucesso")

	return nil
}

// resolverRecaptcha resolve o reCAPTCHA usando SolveCaptcha API ou manualmente
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
	token, err := s.captchaSolver.SolveCaptcha(sitekey, currentURL)
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

	// Aguardar e clicar no link de detalhes com timeout aumentado
	s.logger.Info().Msg("Procurando link de detalhes...")
	linkDetalhes, err := s.page.Timeout(15 * time.Second).Element("#j_id6\\:pnlCadastro img")
	if err != nil {
		return fmt.Errorf("link de detalhes não encontrado: %v", err)
	}

	s.logger.Info().Msg("Clicando no link de detalhes...")
	linkDetalhes.MustClick()
	s.page.MustWaitLoad()
	time.Sleep(s.config.WaitBetweenSteps)

	// Verificar URL de detalhes
	urlDetalhes := "https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraResultadoConsulta.jsf"
	if s.page.MustInfo().URL != urlDetalhes {
		return fmt.Errorf("página de detalhes não carregada")
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

// extrairDadosPagina extrai dados da página de forma estruturada
func (s *SintegraMAScraper) extrairDadosPagina() {
	s.logger.Info().Msg("Extraindo dados da página de forma estruturada")

	// Extrair o texto completo da página
	textoCompleto := s.page.MustEval(`() => {
		// Procurar pelo conteúdo principal
		var content = document.querySelector('body') || document;
		return content.innerText || content.textContent || '';
	}`).String()

	s.logger.Info().Int("texto_length", len(textoCompleto)).Msg("Texto extraído da página")

	// Usar regex para extrair os dados estruturados diretamente
	s.extrairIdentificacao(textoCompleto)
	s.extrairEndereco(textoCompleto)
	s.extrairCNAE(textoCompleto)
	s.extrairSituacao(textoCompleto)
	s.extrairObrigacoes(textoCompleto)
	s.extrairMetadados(textoCompleto)

	// Log de resumo dos dados extraídos
	s.logger.Info().
		Str("razao_social", s.result.Data.RazaoSocial).
		Str("cgc", s.result.Data.CGC).
		Str("situacao", s.result.Data.SituacaoCadastral).
		Int("cnaes_secundarios", len(s.result.Data.CNAESecundarios)).
		Msg("Dados estruturados extraídos com sucesso")
}

func (s *SintegraMAScraper) extrairIdentificacao(texto string) {
	// Extrair CGC
	if matches := regexp.MustCompile(`CGC:\s*([\d.-/]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.CGC = strings.TrimSpace(matches[1])
	}

	// Extrair Inscrição Estadual
	if matches := regexp.MustCompile(`Inscrição Estadual:\s*([\d.-]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.InscricaoEstadual = strings.TrimSpace(matches[1])
	}

	// Extrair Razão Social
	if matches := regexp.MustCompile(`Razão Social:\s*([^\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.RazaoSocial = strings.TrimSpace(matches[1])
	}

	// Extrair Regime de Apuração
	if matches := regexp.MustCompile(`Regime Apuração:\s*([^\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.RegimeApuracao = strings.TrimSpace(matches[1])
	}
}

func (s *SintegraMAScraper) extrairEndereco(texto string) {
	s.result.Data.Endereco = &EnderecoData{}

	// Extrair Logradouro
	if matches := regexp.MustCompile(`Logradouro:\s*([^\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Endereco.Logradouro = strings.TrimSpace(matches[1])
	}

	// Extrair Número
	if matches := regexp.MustCompile(`Número:\s*([^\t\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Endereco.Numero = strings.TrimSpace(matches[1])
	}

	// Extrair Complemento
	if matches := regexp.MustCompile(`Complemento:\s*([^\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Endereco.Complemento = strings.TrimSpace(matches[1])
	}

	// Extrair Bairro
	if matches := regexp.MustCompile(`Bairro:\s*([^\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Endereco.Bairro = strings.TrimSpace(matches[1])
	}

	// Extrair Município
	if matches := regexp.MustCompile(`Município:\s*([^\t\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Endereco.Municipio = strings.TrimSpace(matches[1])
	}

	// Extrair UF
	if matches := regexp.MustCompile(`UF:\s*([A-Z]{2})`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Endereco.UF = strings.TrimSpace(matches[1])
	}

	// Extrair CEP
	if matches := regexp.MustCompile(`CEP:\s*([\d-]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Endereco.CEP = strings.TrimSpace(matches[1])
	}

	// Extrair DDD
	if matches := regexp.MustCompile(`DDD:\s*([\d]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Endereco.DDD = strings.TrimSpace(matches[1])
	}

	// Extrair Telefone
	if matches := regexp.MustCompile(`Telefone:\s*([\d-]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Endereco.Telefone = strings.TrimSpace(matches[1])
	}
}

func (s *SintegraMAScraper) extrairCNAE(texto string) {
	// Extrair CNAE Principal
	if matches := regexp.MustCompile(`CNAE Principal:\s*([^\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.CNAEPrincipal = strings.TrimSpace(matches[1])
	}

	// Extrair CNAEs Secundários
	s.result.Data.CNAESecundarios = []CNAEData{}
	cnaeRegex := regexp.MustCompile(`(\d{7})\s+([A-ZÀ-ÿ\s\-\.,/]+)`)
	matches := cnaeRegex.FindAllStringSubmatch(texto, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			cnae := CNAEData{
				Codigo:    strings.TrimSpace(match[1]),
				Descricao: strings.TrimSpace(match[2]),
			}
			s.result.Data.CNAESecundarios = append(s.result.Data.CNAESecundarios, cnae)
		}
	}
}

func (s *SintegraMAScraper) extrairSituacao(texto string) {
	// Extrair Situação Cadastral
	if matches := regexp.MustCompile(`Situação Cadastral Vigente:\s*([^\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.SituacaoCadastral = strings.TrimSpace(matches[1])
	}

	// Extrair Data da Situação Cadastral
	if matches := regexp.MustCompile(`Data desta Situação Cadastral:\s*([\d/]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.DataSituacaoCadastral = strings.TrimSpace(matches[1])
	}
}

func (s *SintegraMAScraper) extrairObrigacoes(texto string) {
	s.result.Data.Obrigacoes = &ObrigacoesData{}

	// Extrair NFe a partir de
	if matches := regexp.MustCompile(`NFe a partir de \(CNAE's\):\s*([^\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Obrigacoes.NFeAPartirDe = strings.TrimSpace(matches[1])
	}

	// Extrair EDF a partir de
	if matches := regexp.MustCompile(`EDF a partir de:\s*([^\n\r]*)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Obrigacoes.EDFAPartirDe = strings.TrimSpace(matches[1])
	}

	// Extrair CTE a partir de
	if matches := regexp.MustCompile(`CTE a partir de:\s*([^\n\r]*)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Obrigacoes.CTEAPartirDe = strings.TrimSpace(matches[1])
	}
}

func (s *SintegraMAScraper) extrairMetadados(texto string) {
	// Extrair Data da Consulta
	if matches := regexp.MustCompile(`Data da Consulta:\s*([\d/]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.DataConsulta = strings.TrimSpace(matches[1])
	}

	// Extrair Número da Consulta
	if matches := regexp.MustCompile(`Número da Consulta:\s*([^\n\r]*)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.NumeroConsulta = strings.TrimSpace(matches[1])
	}

	// Extrair Observação
	if matches := regexp.MustCompile(`Observação: ([^\n\r]+)`).FindStringSubmatch(texto); len(matches) > 1 {
		s.result.Data.Observacao = strings.TrimSpace(matches[1])
	}
}

// GetResult retorna o resultado da consulta
func (s *SintegraMAScraper) GetResult() *SintegraMAResult {
	return s.result
}

// SaveResult salva o resultado em arquivo JSON
func (s *SintegraMAScraper) SaveResult(filename string) error {
	data, err := json.MarshalIndent(s.result, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// Close fecha recursos
func (s *SintegraMAScraper) Close() {
	if s.browser != nil {
		s.browser.MustClose()
		s.logger.Info().Msg("Navegador fechado")
	}
}

func main() {
	// Configurar logger estruturado conforme especificações do projeto
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().
		Timestamp().
		Caller().
		Logger()

	logger.Info().Msg("=== SINTEGRA MA SCRAPER ===\nHeadless: FALSE (modo visível)")

	// Carregar configuração
	config := LoadConfig()

	// CNPJ para teste (mesmo do exemplo.py)
	cnpj := "38139407000177"

	// Criar scraper
	scraper := NewSintegraMAScraper(config, logger)
	defer scraper.Close()

	// Inicializar
	if err := scraper.Initialize(); err != nil {
		logger.Fatal().Err(err).Msg("Erro na inicialização")
	}

	// Executar consulta
	if err := scraper.ConsultarCNPJ(cnpj); err != nil {
		logger.Fatal().Err(err).Msg("Erro na consulta")
	}

	// Obter resultado
	resultado := scraper.GetResult()

	// Salvar resultado
	filename := fmt.Sprintf("resultado_sintegra_ma_%s.json",
		time.Now().Format("20060102_150405"))
	if err := scraper.SaveResult(filename); err != nil {
		logger.Error().Err(err).Msg("Erro ao salvar resultado")
	}

	// Imprimir resumo
	fmt.Printf("\n=== RESULTADO DA CONSULTA SINTEGRA MA ===\n")
	fmt.Printf("CNPJ: %s\n", resultado.CNPJ)
	fmt.Printf("Status: %s\n", resultado.Status)
	fmt.Printf("URL: %s\n", resultado.URL)
	fmt.Printf("CAPTCHA Resolvido: %v\n", resultado.CaptchaSolved)
	fmt.Printf("Tempo de execução: %v\n", resultado.ExecutionTime)

	if resultado.Data != nil {
		fmt.Printf("\n=== DADOS ESTRUTURADOS ===\n")
		fmt.Printf("┌────────────────────────────────────────────────────────────────────────────────┐\n")
		fmt.Printf("│ IDENTIFICAÇÃO%66s │\n", "")
		fmt.Printf("├────────────────────────────────────────────────────────────────────────────────┤\n")
		fmt.Printf("│ Razão Social: %-58s │\n", truncateString(resultado.Data.RazaoSocial, 58))
		fmt.Printf("│ CGC: %-67s │\n", resultado.Data.CGC)
		fmt.Printf("│ Inscrição Estadual: %-49s │\n", resultado.Data.InscricaoEstadual)
		fmt.Printf("│ Regime: %-63s │\n", truncateString(resultado.Data.RegimeApuracao, 63))
		fmt.Printf("│ Situação: %-61s │\n", truncateString(resultado.Data.SituacaoCadastral, 61))
		fmt.Printf("│ Data Situação: %-56s │\n", resultado.Data.DataSituacaoCadastral)

		if resultado.Data.Endereco != nil {
			fmt.Printf("├────────────────────────────────────────────────────────────────────────────────┤\n")
			fmt.Printf("│ ENDEREÇO%69s │\n", "")
			fmt.Printf("├────────────────────────────────────────────────────────────────────────────────┤\n")
			fmt.Printf("│ %s, %s - %s%*s │\n",
				truncateString(resultado.Data.Endereco.Logradouro, 30),
				resultado.Data.Endereco.Numero,
				truncateString(resultado.Data.Endereco.Bairro, 25),
				maxInt(0, 79-len(resultado.Data.Endereco.Logradouro)-len(resultado.Data.Endereco.Numero)-len(resultado.Data.Endereco.Bairro)-6), "")
			fmt.Printf("│ %s/%s - CEP: %s%*s │\n",
				truncateString(resultado.Data.Endereco.Municipio, 30),
				resultado.Data.Endereco.UF,
				resultado.Data.Endereco.CEP,
				maxInt(0, 79-len(resultado.Data.Endereco.Municipio)-len(resultado.Data.Endereco.UF)-len(resultado.Data.Endereco.CEP)-11), "")
			if resultado.Data.Endereco.Telefone != "" {
				fmt.Printf("│ Telefone: %s%*s │\n",
					resultado.Data.Endereco.Telefone,
					maxInt(0, 79-len(resultado.Data.Endereco.Telefone)-11), "")
			}
		}

		fmt.Printf("├────────────────────────────────────────────────────────────────────────────────┤\n")
		fmt.Printf("│ ATIVIDADES%68s │\n", "")
		fmt.Printf("├────────────────────────────────────────────────────────────────────────────────┤\n")
		fmt.Printf("│ CNAE Principal: %-55s │\n", truncateString(resultado.Data.CNAEPrincipal, 55))
		fmt.Printf("│ CNAEs Secundários: %-52d │\n", len(resultado.Data.CNAESecundarios))

		if resultado.Data.Obrigacoes != nil {
			fmt.Printf("├────────────────────────────────────────────────────────────────────────────────┤\n")
			fmt.Printf("│ OBRIGAÇÕES%67s │\n", "")
			fmt.Printf("├────────────────────────────────────────────────────────────────────────────────┤\n")
			fmt.Printf("│ NFe a partir de: %-54s │\n", truncateString(resultado.Data.Obrigacoes.NFeAPartirDe, 54))
			fmt.Printf("│ EDF a partir de: %-54s │\n", truncateString(resultado.Data.Obrigacoes.EDFAPartirDe, 54))
			fmt.Printf("│ CTE a partir de: %-54s │\n", truncateString(resultado.Data.Obrigacoes.CTEAPartirDe, 54))
		}

		fmt.Printf("├────────────────────────────────────────────────────────────────────────────────┤\n")
		fmt.Printf("│ METADADOS%68s │\n", "")
		fmt.Printf("├────────────────────────────────────────────────────────────────────────────────┤\n")
		fmt.Printf("│ Data Consulta: %-56s │\n", resultado.Data.DataConsulta)
		fmt.Printf("│ Número Consulta: %-54s │\n", resultado.Data.NumeroConsulta)
		fmt.Printf("└────────────────────────────────────────────────────────────────────────────────┘\n")
	}

	fmt.Printf("\nArquivo salvo: %s\n", filename)

	logger.Info().Msg("Consulta Sintegra MA concluída com sucesso!")
}

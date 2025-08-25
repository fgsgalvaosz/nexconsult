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
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
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
		Headless:           false, // Forçando headless false conforme solicitado
		Timeout:            300 * time.Second,
		WaitBetweenSteps:   2 * time.Second,
		UserAgent:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		ViewportWidth:      1115,
		ViewportHeight:     639,
		MaxRetries:         3,
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

	var response SolveCaptchaResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("erro ao parsear resposta: %v", err)
	}

	if response.Status != 1 {
		return "", fmt.Errorf("erro na submissão: %s", response.Error)
	}

	cs.logger.Info().
		Str("task_id", response.Request).
		Msg("CAPTCHA submetido com sucesso")

	return response.Request, nil
}

// GetCaptchaResult obtém o resultado da resolução do CAPTCHA
func (cs *CaptchaSolver) GetCaptchaResult(taskID string) (string, error) {
	url := fmt.Sprintf("https://api.solvecaptcha.com/res.php?key=%s&action=get&id=%s&json=1",
		cs.config.SolveCaptchaAPIKey, taskID)

	for i := 0; i < cs.config.MaxRetries; i++ {
		cs.logger.Info().
			Str("task_id", taskID).
			Int("attempt", i+1).
			Msg("Consultando resultado do CAPTCHA")

		resp, err := cs.client.Get(url)
		if err != nil {
			cs.logger.Error().Err(err).Msg("Erro na consulta")
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

		var result SolveCaptchaResultResponse
		if err := json.Unmarshal(body, &result); err != nil {
			cs.logger.Error().Err(err).Msg("Erro ao parsear resultado")
			time.Sleep(cs.config.RetryDelay)
			continue
		}

		if result.Status == 1 {
			cs.logger.Info().
				Str("task_id", taskID).
				Msg("CAPTCHA resolvido com sucesso")
			return result.Request, nil
		}

		if result.Error == "CAPCHA_NOT_READY" {
			cs.logger.Info().Msg("CAPTCHA ainda processando, aguardando...")
			time.Sleep(cs.config.RetryDelay)
			continue
		}

		return "", fmt.Errorf("erro na resolução: %s", result.Error)
	}

	return "", fmt.Errorf("timeout na resolução do CAPTCHA após %d tentativas", cs.config.MaxRetries)
}

// SolveCaptcha resolve um CAPTCHA completo
func (cs *CaptchaSolver) SolveCaptcha(googleKey, pageURL string) (string, error) {
	taskID, err := cs.SubmitCaptcha(googleKey, pageURL)
	if err != nil {
		return "", err
	}

	// Aguardar um pouco antes de começar a consultar
	time.Sleep(10 * time.Second)

	return cs.GetCaptchaResult(taskID)
}

// SintegraMAResult representa o resultado da consulta
type SintegraMAResult struct {
	CNPJ          string            `json:"cnpj"`
	Status        string            `json:"status"`
	URL           string            `json:"url"`
	Data          map[string]string `json:"data"`
	ExecutionTime time.Duration     `json:"execution_time"`
	Timestamp     time.Time         `json:"timestamp"`
	CaptchaSolved bool              `json:"captcha_solved"`
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
			Data:      make(map[string]string),
			Timestamp: time.Now(),
		},
	}
}

// Initialize inicializa o navegador
func (s *SintegraMAScraper) Initialize() error {
	s.logger.Info().Msg("Inicializando navegador Chrome (headless=false)")

	// Tentar encontrar Chrome instalado no sistema primeiro
	chromePaths := []string{
		"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
		"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
	}

	var chromePath string
	for _, path := range chromePaths {
		if _, err := os.Stat(path); err == nil {
			chromePath = path
			s.logger.Info().Str("chrome_path", path).Msg("Chrome encontrado no sistema")
			break
		}
	}

	var l *launcher.Launcher
	if chromePath != "" {
		// Usar Chrome do sistema
		l = launcher.New().
			Bin(chromePath).
			Leakless(false). // DESABILITAR LEAKLESS
			Headless(false). // HEADLESS FALSE conforme solicitado
			Set("disable-gpu").
			Set("no-sandbox").
			Set("disable-dev-shm-usage").
			Set("disable-blink-features", "AutomationControlled").
			Set("disable-web-security").
			Set("disable-extensions").
			Set("user-agent", s.config.UserAgent)
	} else {
		// Fallback para Chromium baixado (sem leakless)
		s.logger.Warn().Msg("Chrome não encontrado, usando Chromium (pode ter problemas com antivírus)")
		l = launcher.New().
			Headless(false).
			Leakless(false). // DESABILITAR LEAKLESS para evitar problema antivírus
			Set("disable-gpu").
			Set("no-sandbox").
			Set("disable-dev-shm-usage").
			Set("disable-blink-features", "AutomationControlled").
			Set("disable-web-security").
			Set("disable-extensions").
			Set("user-agent", s.config.UserAgent)
	}

	// Iniciar navegador
	browser := rod.New().
		ControlURL(l.MustLaunch()).
		Timeout(s.config.Timeout).
		MustConnect()

	s.browser = browser

	// Criar página
	page := browser.MustPage()
	page.MustSetViewport(s.config.ViewportWidth, s.config.ViewportHeight, 1, false)
	s.page = page

	s.logger.Info().Msg("Navegador inicializado com sucesso (modo visível)")
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

	// Submeter formulário
	s.logger.Info().Msg("Submetendo formulário")
	botaoConsulta := s.page.MustElement("#form1\\:pnlPrincipal4 input:nth-of-type(2)")
	botaoConsulta.MustClick()
	s.page.MustWaitLoad()
	time.Sleep(s.config.WaitBetweenSteps)

	// Verificar resultado
	urlEsperada := "https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraResultadoListaConsulta.jsf"
	urlAtual := s.page.MustInfo().URL
	s.result.URL = urlAtual

	if urlAtual != urlEsperada {
		s.result.Status = "erro_resultado"
		return fmt.Errorf("página de resultado não carregada. URL: %s", urlAtual)
	}

	s.logger.Info().Msg("Página de resultados carregada")

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
	s.logger.Info().Msg("CAPTCHA resolvido, injetando token")

	// Injetar token no formulário
	script := fmt.Sprintf(`
		document.getElementById('g-recaptcha-response').innerHTML = '%s';
		if (typeof grecaptcha !== 'undefined') {
			grecaptcha.getResponse = function() { return '%s'; };
		}
	`, token, token)

	s.page.MustEval(script)
	time.Sleep(2 * time.Second)

	s.logger.Info().Msg("Token do CAPTCHA injetado com sucesso")
	return nil
}

// extrairDetalhes extrai os detalhes da consulta
func (s *SintegraMAScraper) extrairDetalhes() error {
	s.logger.Info().Msg("Extraindo detalhes da consulta")

	// Clicar no link de detalhes
	linkDetalhes, err := s.page.Element("#j_id6\\:pnlCadastro img")
	if err != nil {
		return fmt.Errorf("link de detalhes não encontrado: %v", err)
	}

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
		Int("campos_extraidos", len(s.result.Data)).
		Msg("Detalhes extraídos com sucesso")

	return nil
}

// extrairDadosPagina extrai dados da página atual
func (s *SintegraMAScraper) extrairDadosPagina() {
	// Extrair tabelas e campos de dados
	elementos := s.page.MustElements("table tr, div.campo, span.valor, td")

	for i, elemento := range elementos {
		texto := strings.TrimSpace(elemento.MustText())
		if texto != "" && len(texto) > 3 {
			chave := fmt.Sprintf("campo_%d", i)
			s.result.Data[chave] = texto

			// Tentar extrair campos específicos
			if strings.Contains(strings.ToLower(texto), "razão social") {
				s.result.Data["razao_social"] = texto
			}
			if strings.Contains(strings.ToLower(texto), "cnpj") {
				s.result.Data["cnpj_completo"] = texto
			}
			if strings.Contains(strings.ToLower(texto), "situação") {
				s.result.Data["situacao"] = texto
			}
		}
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
	fmt.Printf("Dados extraídos: %d campos\n", len(resultado.Data))
	fmt.Printf("Arquivo salvo: %s\n", filename)

	logger.Info().Msg("Consulta Sintegra MA concluída com sucesso!")
}

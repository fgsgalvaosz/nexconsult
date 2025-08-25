package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"nexconsult-sintegra-ma/internal/models"
	"os"
	"regexp"
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

// CaptchaSolver gerencia a resolução de CAPTCHA
type CaptchaSolver struct {
	config *Config
	client *http.Client
	logger zerolog.Logger
}

// SintegraMAResult representa o resultado da consulta
type SintegraMAResult struct {
	CNPJ          string                 `json:"cnpj"`
	Status        string                 `json:"status"`
	URL           string                 `json:"url"`
	Data          *SintegraData          `json:"data"`
	ExecutionTime time.Duration          `json:"execution_time"`
	Timestamp     time.Time              `json:"timestamp"`
	CaptchaSolved bool                   `json:"captcha_solved"`
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
	CNAEPrincipal   string      `json:"cnae_principal"`
	CNAESecundarios []CNAEData  `json:"cnae_secundarios"`

	// Situação
	SituacaoCadastral     string `json:"situacao_cadastral"`
	DataSituacaoCadastral string `json:"data_situacao_cadastral"`

	// Obrigações
	Obrigacoes *ObrigacoesData `json:"obrigacoes"`

	// Metadados
	DataConsulta    string `json:"data_consulta"`
	NumeroConsulta  string `json:"numero_consulta"`
	Observacao      string `json:"observacao"`
}

type EnderecoData struct {
	Logradouro   string `json:"logradouro"`
	Numero       string `json:"numero"`
	Complemento  string `json:"complemento"`
	Bairro       string `json:"bairro"`
	Municipio    string `json:"municipio"`
	UF           string `json:"uf"`
	CEP          string `json:"cep"`
	DDD          string `json:"ddd"`
	Telefone     string `json:"telefone"`
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

// SintegraService gerencia as operações de consulta no Sintegra MA
type SintegraService struct {
	logger zerolog.Logger
}

// NewSintegraService cria uma nova instância do serviço
func NewSintegraService(logger zerolog.Logger) *SintegraService {
	return &SintegraService{
		logger: logger,
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

	s.logger.Info().Dur("execution_time", s.result.ExecutionTime).Msg("Consulta concluída com sucesso")

	return nil
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

	s.logger.Info().Str("sitekey", sitekey).Msg("Sitekey extraída, resolvendo CAPTCHA")

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

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

// Métodos do CaptchaSolver

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

// GetCaptchaResult obtém o resultado de um CAPTCHA
func (cs *CaptchaSolver) GetCaptchaResult(taskID string) (string, error) {
	cs.logger.Info().Str("task_id", taskID).Msg("Iniciando busca por resultado")

	url := fmt.Sprintf("https://api.solvecaptcha.com/res.php?key=%s&action=get&id=%s&json=1",
		cs.config.SolveCaptchaAPIKey, taskID)

	// Fazer polling com retry
	for i := 0; i < cs.config.MaxRetries; i++ {
		cs.logger.Info().Int("tentativa", i+1).Int("max_tentativas", cs.config.MaxRetries).Msg("🔄 Verificando resultado...")

		resp, err := cs.client.Get(url)
		if err != nil {
			cs.logger.Error().Err(err).Msg("Erro na requisição de resultado")
			time.Sleep(cs.config.RetryDelay)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			cs.logger.Error().Err(err).Msg("Erro ao ler corpo da resposta")
			time.Sleep(cs.config.RetryDelay)
			continue
		}

		bodyStr := string(body)
		cs.logger.Info().Str("response", bodyStr).Msg("Resposta recebida")

		// Tentar parsear como JSON
		var result SolveCaptchaResponse
		if err := json.Unmarshal(body, &result); err == nil {
			// Resposta JSON
			if result.Status == 1 {
				cs.logger.Info().Str("token", result.Request[:min(20, len(result.Request))]+"...").Msg("✅ CAPTCHA resolvido com sucesso!")
				return result.Request, nil
			} else if result.Request == "CAPCHA_NOT_READY" {
				cs.logger.Info().Msg("⏳ CAPTCHA ainda processando...")
				time.Sleep(cs.config.RetryDelay)
				continue
			} else {
				return "", fmt.Errorf("erro no resultado (JSON): %s", result.Error)
			}
		} else {
			// Resposta texto simples
			if strings.HasPrefix(bodyStr, "OK|") {
				token := strings.TrimPrefix(bodyStr, "OK|")
				token = strings.TrimSpace(token)
				cs.logger.Info().Str("token", token[:min(20, len(token))]+"...").Msg("✅ CAPTCHA resolvido com sucesso!")
				return token, nil
			} else if strings.Contains(bodyStr, "CAPCHA_NOT_READY") {
				cs.logger.Info().Msg("⏳ CAPTCHA ainda processando...")
				time.Sleep(cs.config.RetryDelay)
				continue
			} else {
				return "", fmt.Errorf("erro no resultado (texto): %s", bodyStr)
			}
		}
	}

	return "", fmt.Errorf("timeout: CAPTCHA não foi resolvido após %d tentativas (%v)",
		cs.config.MaxRetries, time.Duration(cs.config.MaxRetries)*cs.config.RetryDelay)
}

// ConsultarCNPJ executa a consulta completa no Sintegra MA
func (s *SintegraService) ConsultarCNPJ(cnpj string) (*models.SintegraResponse, error) {
	s.logger.Info().Str("cnpj", cnpj).Msg("🚀 Iniciando consulta via API")

	// Carregar configuração
	config := s.loadConfig()

	// Criar scraper usando as funções do main.go
	scraper := s.createScraper(config)
	defer scraper.Close()

	// Inicializar navegador
	if err := scraper.Initialize(); err != nil {
		s.logger.Error().Err(err).Msg("❌ Erro na inicialização do navegador")
		return nil, fmt.Errorf("erro na inicialização: %v", err)
	}

	// Executar consulta
	if err := scraper.ConsultarCNPJ(cnpj); err != nil {
		s.logger.Error().Err(err).Str("cnpj", cnpj).Msg("❌ Erro na consulta")
		return nil, fmt.Errorf("erro na consulta: %v", err)
	}

	// Obter resultado
	resultado := scraper.GetResult()

	// Converter para modelo da API
	response := s.convertToAPIResponse(resultado)

	s.logger.Info().
		Str("cnpj", cnpj).
		Str("status", response.Status).
		Str("execution_time", response.ExecutionTime).
		Msg("✅ Consulta concluída com sucesso")

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

	config := &Config{
		SolveCaptchaAPIKey: os.Getenv("SOLVECAPTCHA_API_KEY"),
		Headless:           false, // Forçando headless false conforme especificação
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
		s.logger.Warn().Msg("AVISO: SOLVECAPTCHA_API_KEY não configurada. Tentando resolver CAPTCHA manualmente...")
	} else {
		s.logger.Info().Str("key_preview", config.SolveCaptchaAPIKey[:8]+"...").Msg("✓ SolveCaptcha API configurada")
	}

	return config
}

// createScraper cria uma instância do scraper real
func (s *SintegraService) createScraper(config *Config) *SintegraMAScraper {
	return &SintegraMAScraper{
		config:        config,
		captchaSolver: &CaptchaSolver{
			config: config,
			client: &http.Client{Timeout: 30 * time.Second},
			logger: s.logger,
		},
		logger: s.logger,
		result: &SintegraMAResult{
			Data:      &SintegraData{},
			Timestamp: time.Now(),
		},
	}
}

// convertToAPIResponse converte o resultado do scraper para o modelo da API
func (s *SintegraService) convertToAPIResponse(scraperResult *SintegraMAResult) *models.SintegraResponse {
	// Converter estrutura interna para estrutura da API
	apiData := &models.SintegraData{
		CGC:               scraperResult.Data.CGC,
		InscricaoEstadual: scraperResult.Data.InscricaoEstadual,
		RazaoSocial:       scraperResult.Data.RazaoSocial,
		RegimeApuracao:    scraperResult.Data.RegimeApuracao,
		SituacaoCadastral: scraperResult.Data.SituacaoCadastral,
		DataSituacaoCadastral: scraperResult.Data.DataSituacaoCadastral,
		CNAEPrincipal:     scraperResult.Data.CNAEPrincipal,
		DataConsulta:      scraperResult.Data.DataConsulta,
		NumeroConsulta:    scraperResult.Data.NumeroConsulta,
		Observacao:        scraperResult.Data.Observacao,
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


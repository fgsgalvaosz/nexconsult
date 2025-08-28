package container

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"nexconsult/internal/config"
	"nexconsult/internal/logger"
	"nexconsult/internal/service/browser"
	"nexconsult/internal/service/captcha"
	"nexconsult/internal/service/extractor"
	"nexconsult/internal/service/navigator"
)

// Container gerencia as dependências da aplicação
type Container struct {
	config         *config.Config
	logger         logger.Logger
	httpClient     *http.Client
	textCleanRegex *regexp.Regexp

	// Módulos
	chromeManager   *browser.ChromeManager
	captchaResolver *captcha.CaptchaResolver
	htmlExtractor   *extractor.HTMLExtractor
	webNavigator    *navigator.WebNavigator
}

// NewContainer cria uma nova instância do container de dependências
func NewContainer(cfg *config.Config) *Container {
	// Inicializar logger global
	logger.InitGlobalLogger(cfg.DebugMode)
	appLogger := logger.GetLogger().With(logger.String("component", "container"))

	// Cliente HTTP reutilizável com configurações otimizadas
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	// Regex pré-compilada para limpeza de texto
	textCleanRegex := regexp.MustCompile(`\s+`)

	container := &Container{
		config:         cfg,
		logger:         appLogger,
		httpClient:     httpClient,
		textCleanRegex: textCleanRegex,
	}

	// Inicializar módulos
	container.initializeModules()

	appLogger.Info("Container de dependências inicializado",
		logger.Bool("debugMode", cfg.DebugMode),
		logger.Bool("headless", cfg.Headless))

	return container
}

// initializeModules inicializa todos os módulos
func (c *Container) initializeModules() {
	c.chromeManager = browser.NewChromeManager(c.config)
	c.captchaResolver = captcha.NewCaptchaResolver(c.config.SolveCaptchaAPIKey, c.httpClient)
	c.htmlExtractor = extractor.NewHTMLExtractor()
	c.webNavigator = navigator.NewWebNavigator(c.config)
}

// GetConfig retorna a configuração
func (c *Container) GetConfig() *config.Config {
	return c.config
}

// GetLogger retorna o logger
func (c *Container) GetLogger() logger.Logger {
	return c.logger
}

// GetHTTPClient retorna o cliente HTTP
func (c *Container) GetHTTPClient() *http.Client {
	return c.httpClient
}

// GetTextCleanRegex retorna a regex de limpeza de texto
func (c *Container) GetTextCleanRegex() *regexp.Regexp {
	return c.textCleanRegex
}

// GetChromeManager retorna o gerenciador do Chrome
func (c *Container) GetChromeManager() *browser.ChromeManager {
	return c.chromeManager
}

// GetCaptchaResolver retorna o resolvedor de CAPTCHA
func (c *Container) GetCaptchaResolver() *captcha.CaptchaResolver {
	return c.captchaResolver
}

// GetHTMLExtractor retorna o extrator de HTML
func (c *Container) GetHTMLExtractor() *extractor.HTMLExtractor {
	return c.htmlExtractor
}

// GetWebNavigator retorna o navegador web
func (c *Container) GetWebNavigator() *navigator.WebNavigator {
	return c.webNavigator
}

// Cleanup limpa recursos do container
func (c *Container) Cleanup() {
	c.logger.Info("Limpando recursos do container")

	// Fechar cliente HTTP se necessário
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// ServiceFactory cria instâncias de serviços com dependências injetadas
type ServiceFactory struct {
	container *Container
}

// NewServiceFactory cria uma nova factory de serviços
func NewServiceFactory(container *Container) *ServiceFactory {
	return &ServiceFactory{
		container: container,
	}
}

// CreateSintegraService cria uma instância do SintegraService com todas as dependências
func (f *ServiceFactory) CreateSintegraService() *SintegraService {
	return &SintegraService{
		container:         f.container,
		config:            f.container.GetConfig(),
		logger:            f.container.GetLogger().With(logger.String("service", "sintegra")),
		httpClient:        f.container.GetHTTPClient(),
		textCleanRegex:    f.container.GetTextCleanRegex(),
		chromeManager:     f.container.GetChromeManager(),
		captchaResolver:   f.container.GetCaptchaResolver(),
		htmlExtractor:     f.container.GetHTMLExtractor(),
		webNavigator:      f.container.GetWebNavigator(),
		lastExtractedData: nil,
	}
}

// SintegraService representa o serviço principal refatorado
type SintegraService struct {
	container         *Container
	config            *config.Config
	logger            logger.Logger
	httpClient        *http.Client
	textCleanRegex    *regexp.Regexp
	chromeManager     *browser.ChromeManager
	captchaResolver   *captcha.CaptchaResolver
	htmlExtractor     *extractor.HTMLExtractor
	webNavigator      *navigator.WebNavigator
	lastExtractedData *extractor.SintegraData
}

// SintegraResult representa o resultado da consulta
type SintegraResult struct {
	CNPJ         string
	RazaoSocial  string
	Situacao     string
	DataConsulta string
	Error        error
}

// GetLastExtractedData retorna os últimos dados extraídos
func (s *SintegraService) GetLastExtractedData() *extractor.SintegraData {
	return s.lastExtractedData
}

// ScrapeCNPJComplete executa scraping e retorna dados completos diretamente
func (s *SintegraService) ScrapeCNPJComplete(cnpj string) (*extractor.SintegraData, error) {
	s.logger.Info("Iniciando consulta CNPJ completa", logger.String("cnpj", cnpj))

	// Executar scraping para obter dados
	result, err := s.ScrapeCNPJ(cnpj)
	if err != nil {
		return nil, err
	}

	if result.Error != nil {
		return nil, result.Error
	}

	// Retornar os dados completos extraídos
	return s.GetLastExtractedData(), nil
}

// ScrapeCNPJ executa o scraping principal usando os módulos
func (s *SintegraService) ScrapeCNPJ(cnpj string) (*SintegraResult, error) {
	s.logger.Info("Iniciando scraping para CNPJ", logger.String("cnpj", cnpj))

	result := &SintegraResult{
		CNPJ:         cnpj,
		DataConsulta: time.Now().Format("2006-01-02 15:04:05"),
	}

	// Criar contexto Chrome otimizado
	ctx, cleanup, err := s.chromeManager.CreateOptimizedContext()
	if err != nil {
		return nil, fmt.Errorf("erro ao criar contexto Chrome: %v", err)
	}
	defer cleanup()

	// Navegar para SINTEGRA
	err = s.webNavigator.ExecuteWithRetry(ctx, func(ctx context.Context) error {
		if err := s.webNavigator.NavigateToSintegra(ctx); err != nil {
			return err
		}
		return s.webNavigator.FillCNPJForm(ctx, cnpj)
	}, 2)

	if err != nil {
		return nil, fmt.Errorf("erro durante preenchimento inicial: %v", err)
	}

	s.logger.Debug("CNPJ preenchido, aguardando CAPTCHA")

	// Aguardar e resolver CAPTCHA
	if err := s.webNavigator.WaitForCaptcha(ctx); err != nil {
		return nil, fmt.Errorf("erro ao aguardar CAPTCHA: %v", err)
	}

	// Obter site key e resolver CAPTCHA
	siteKey, err := s.webNavigator.GetSiteKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter site key: %v", err)
	}

	token, err := s.captchaResolver.ResolveCaptcha(ctx, siteKey, s.config.SintegraURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao resolver CAPTCHA: %v", err)
	}

	// Injetar token do CAPTCHA
	if err := s.webNavigator.InjectCaptchaToken(ctx, token); err != nil {
		return nil, fmt.Errorf("erro ao injetar token: %v", err)
	}

	s.logger.Debug("CAPTCHA resolvido, submetendo consulta")

	// Submeter consulta
	currentURL, err := s.webNavigator.SubmitConsultation(ctx)
	if err != nil {
		return nil, err
	}

	// Validar resultado
	if !s.webNavigator.ValidateConsultationResult(currentURL) {
		// Obter HTML da página de erro
		errorHTML, err := s.webNavigator.GetErrorPageHTML(ctx)
		if err != nil {
			return nil, fmt.Errorf("erro ao obter HTML da página de erro: %v", err)
		}

		// Extrair mensagem de erro
		errorMsg := s.htmlExtractor.ExtractErrorMessage(errorHTML)
		if errorMsg != "" {
			s.logger.Debug("Mensagem de erro encontrada", logger.String("error", errorMsg))
			return nil, fmt.Errorf("%s", errorMsg)
		}

		return nil, fmt.Errorf("consulta não foi processada corretamente. URL atual: %s", currentURL)
	}

	s.logger.Debug("Consulta submetida com sucesso", logger.String("url", currentURL))

	// Navegar para página de detalhes
	if err := s.webNavigator.NavigateToResultsList(ctx); err != nil {
		return nil, fmt.Errorf("erro na navegação: %v", err)
	}

	// Extrair HTML da página
	htmlContent, err := s.webNavigator.ExtractPageHTML(ctx)
	if err != nil {
		return nil, err
	}

	// Extrair dados do HTML
	data, err := s.htmlExtractor.ExtractDataFromHTML(htmlContent)
	if err != nil {
		return nil, fmt.Errorf("erro na extração: %v", err)
	}

	// Armazenar dados completos
	s.lastExtractedData = data

	result.RazaoSocial = data.RazaoSocial
	result.Situacao = data.SituacaoCadastral

	s.logger.Info("Scraping concluído com sucesso",
		logger.String("cnpj", cnpj),
		logger.String("razaoSocial", data.RazaoSocial))

	return result, nil
}

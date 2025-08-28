package browser

import (
	"context"
	"time"

	"nexconsult/internal/config"
	"nexconsult/internal/logger"

	"github.com/chromedp/chromedp"
)

// ChromeManager gerencia configurações e contextos do Chrome
type ChromeManager struct {
	config *config.Config
	logger logger.Logger
}

// NewChromeManager cria uma nova instância do gerenciador Chrome
func NewChromeManager(cfg *config.Config) *ChromeManager {
	return &ChromeManager{
		config: cfg,
		logger: logger.GetLogger().With(logger.String("component", "chrome")),
	}
}

// CreateOptimizedContext cria um contexto Chrome otimizado para velocidade
func (c *ChromeManager) CreateOptimizedContext() (context.Context, context.CancelFunc, error) {
	c.logger.Debug("Criando contexto Chrome otimizado")

	// Configurar opções do Chrome otimizadas para velocidade
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", c.config.Headless),
		chromedp.Flag("disable-gpu", true), // Desabilitar GPU para velocidade
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("disable-features", "VizDisplayCompositor,TranslateUI,BlinkGenPropertyTrees"),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-ipc-flooding-protection", true),
		chromedp.Flag("disable-extensions", true),  // Desabilitar extensões
		chromedp.Flag("disable-plugins", true),     // Desabilitar plugins
		chromedp.Flag("disable-images", false),     // Manter imagens para CAPTCHA
		chromedp.Flag("disable-javascript", false), // Manter JS para funcionalidade
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-domain-reliability", true),
		chromedp.Flag("disable-features", "MediaRouter"),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-web-resources", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("aggressive-cache-discard", true),
		chromedp.WindowSize(1366, 768), // Tamanho menor para velocidade
	)

	// Criar contexto com configurações personalizadas
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	
	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	// Aumentar timeout para operações mais estáveis
	timeoutDuration := time.Duration(c.config.Timeout) * time.Second
	if timeoutDuration < 120*time.Second {
		timeoutDuration = 120 * time.Second // Mínimo de 2 minutos
	}
	
	ctx, timeoutCancel := context.WithTimeout(ctx, timeoutDuration)

	// Função de cleanup que cancela todos os contextos
	cleanup := func() {
		timeoutCancel()
		ctxCancel()
		cancel()
	}

	c.logger.Debug("Contexto Chrome criado com sucesso",
		logger.Duration("timeout", timeoutDuration),
		logger.Bool("headless", c.config.Headless))

	return ctx, cleanup, nil
}

// GetOptimizedOptions retorna as opções otimizadas do Chrome
func (c *ChromeManager) GetOptimizedOptions() []chromedp.ExecAllocatorOption {
	return []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", c.config.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("disable-features", "VizDisplayCompositor,TranslateUI,BlinkGenPropertyTrees"),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-ipc-flooding-protection", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-plugins", true),
		chromedp.Flag("disable-images", false),
		chromedp.Flag("disable-javascript", false),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-domain-reliability", true),
		chromedp.Flag("disable-features", "MediaRouter"),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-web-resources", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("aggressive-cache-discard", true),
		chromedp.WindowSize(1366, 768),
	}
}

// ExecuteWithRetry executa uma função com retry em caso de erro
func (c *ChromeManager) ExecuteWithRetry(ctx context.Context, fn func(context.Context) error, maxRetries int) error {
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		c.logger.Debug("Executando ação",
			logger.Int("tentativa", attempt),
			logger.Int("maxTentativas", maxRetries))

		err := fn(ctx)
		if err == nil {
			if attempt > 1 {
				c.logger.Info("Ação executada com sucesso após retry",
					logger.Int("tentativa", attempt))
			}
			return nil
		}

		lastErr = err
		c.logger.Warn("Erro na execução",
			logger.Int("tentativa", attempt),
			logger.Error(err))

		if attempt == maxRetries {
			return err
		}

		// Aguardar antes da próxima tentativa
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	return lastErr
}

package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/nexconsult/cnpj-api/internal/config"
	"github.com/sirupsen/logrus"
)

// BrowserService manages a pool of browser contexts
type BrowserService struct {
	config   config.BrowserConfig
	logger   *logrus.Logger
	pool     chan *ChromeBrowserContext
	contexts []*ChromeBrowserContext
	mu       sync.RWMutex
	closed   bool
}

// ChromeBrowserContext implements BrowserContext interface
type ChromeBrowserContext struct {
	id       string
	ctx      context.Context
	cancel   context.CancelFunc
	chromedp context.Context
	healthy  bool
	mu       sync.RWMutex
}

// NewBrowserService creates a new browser service
func NewBrowserService(config config.BrowserConfig, logger *logrus.Logger) (BrowserServiceInterface, error) {
	service := &BrowserService{
		config:   config,
		logger:   logger,
		pool:     make(chan *ChromeBrowserContext, config.MaxBrowsers),
		contexts: make([]*ChromeBrowserContext, 0, config.MaxBrowsers),
	}

	// Initialize minimum browsers
	for i := 0; i < config.MinBrowsers; i++ {
		browserCtx, err := service.createBrowser()
		if err != nil {
			logger.WithError(err).Error("Failed to create initial browser")
			continue
		}
		service.contexts = append(service.contexts, browserCtx)
		service.pool <- browserCtx
	}

	logger.WithField("browsers", len(service.contexts)).Info("Browser service initialized")
	return service, nil
}

// GetBrowser gets an available browser context
func (s *BrowserService) GetBrowser(ctx context.Context) (BrowserContext, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("browser service is closed")
	}
	s.mu.RUnlock()

	select {
	case browserCtx := <-s.pool:
		if browserCtx.IsHealthy() {
			return browserCtx, nil
		}
		// Browser is unhealthy, create a new one
		s.logger.WithField("browser_id", browserCtx.GetID()).Warn("Unhealthy browser detected, creating new one")
		browserCtx.Close()

		newBrowser, err := s.createBrowser()
		if err != nil {
			return nil, fmt.Errorf("failed to create new browser: %w", err)
		}
		return newBrowser, nil

	case <-time.After(10 * time.Second):
		// No browser available, try to create a new one if under limit
		s.mu.Lock()
		if len(s.contexts) < s.config.MaxBrowsers {
			browserCtx, err := s.createBrowser()
			if err != nil {
				s.mu.Unlock()
				return nil, fmt.Errorf("failed to create browser: %w", err)
			}
			s.contexts = append(s.contexts, browserCtx)
			s.mu.Unlock()
			return browserCtx, nil
		}
		s.mu.Unlock()
		return nil, fmt.Errorf("no browser available and pool is at maximum capacity")

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReleaseBrowser releases a browser context back to the pool
func (s *BrowserService) ReleaseBrowser(browserCtx BrowserContext) error {
	chromeBrowser, ok := browserCtx.(*ChromeBrowserContext)
	if !ok {
		return fmt.Errorf("invalid browser context type")
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		chromeBrowser.Close()
		return nil
	}
	s.mu.RUnlock()

	select {
	case s.pool <- chromeBrowser:
		return nil
	default:
		// Pool is full, close the browser
		chromeBrowser.Close()
		return nil
	}
}

// createBrowser creates a new browser context
func (s *BrowserService) createBrowser() (*ChromeBrowserContext, error) {
	// Chrome options for headless operation
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-features", "TranslateUI"),
		chromedp.Flag("disable-ipc-flooding-protection", true),
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("disable-features", "VizDisplayCompositor"),
		chromedp.WindowSize(1920, 1080),
		chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36"),
	}

	if s.config.Headless {
		opts = append(opts, chromedp.Headless)
	}

	// Create allocator context with proper parent context
	parentCtx := context.Background()
	allocCtx, cancel := chromedp.NewExecAllocator(parentCtx, opts...)

	// Create browser context with proper parent
	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	browserCtx := &ChromeBrowserContext{
		id:       fmt.Sprintf("browser-%d", time.Now().UnixNano()),
		ctx:      ctx,
		cancel:   func() { ctxCancel(); cancel() },
		chromedp: ctx,
		healthy:  true,
	}

	// Test browser health with a simple navigation
	testCtx, testCancel := context.WithTimeout(ctx, 15*time.Second)
	defer testCancel()

	err := chromedp.Run(testCtx, chromedp.Navigate("about:blank"))
	if err != nil {
		browserCtx.Close()
		return nil, fmt.Errorf("browser health check failed: %w", err)
	}

	s.logger.WithField("browser_id", browserCtx.id).Debug("Browser created successfully")
	return browserCtx, nil
}

// GetStats returns browser pool statistics
func (s *BrowserService) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	healthy := 0
	for _, ctx := range s.contexts {
		if ctx.IsHealthy() {
			healthy++
		}
	}

	return map[string]interface{}{
		"total_browsers":   len(s.contexts),
		"healthy_browsers": healthy,
		"available":        len(s.pool),
		"max_browsers":     s.config.MaxBrowsers,
		"min_browsers":     s.config.MinBrowsers,
	}
}

// Health returns browser service health status
func (s *BrowserService) Health() map[string]interface{} {
	stats := s.GetStats()

	status := "healthy"
	if stats["healthy_browsers"].(int) == 0 {
		status = "unhealthy"
	} else if stats["healthy_browsers"].(int) < s.config.MinBrowsers {
		status = "degraded"
	}

	return map[string]interface{}{
		"status": status,
		"stats":  stats,
	}
}

// Restart restarts the browser pool
func (s *BrowserService) Restart() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close all existing browsers
	for _, ctx := range s.contexts {
		ctx.Close()
	}

	// Clear the pool
	for len(s.pool) > 0 {
		<-s.pool
	}

	s.contexts = s.contexts[:0]

	// Create new browsers
	for i := 0; i < s.config.MinBrowsers; i++ {
		browserCtx, err := s.createBrowser()
		if err != nil {
			s.logger.WithError(err).Error("Failed to create browser during restart")
			continue
		}
		s.contexts = append(s.contexts, browserCtx)
		s.pool <- browserCtx
	}

	s.logger.Info("Browser pool restarted")
	return nil
}

// Close closes all browsers and releases resources
func (s *BrowserService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true

	// Close all browsers
	for _, ctx := range s.contexts {
		ctx.Close()
	}

	// Clear the pool
	for len(s.pool) > 0 {
		<-s.pool
	}

	close(s.pool)
	s.logger.Info("Browser service closed")
	return nil
}

// ChromeBrowserContext methods

// Navigate navigates to a URL
func (c *ChromeBrowserContext) Navigate(ctx context.Context, url string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.healthy {
		return fmt.Errorf("browser context is not healthy")
	}

	return chromedp.Run(c.chromedp, chromedp.Navigate(url))
}

// WaitForSelector waits for an element to appear
func (c *ChromeBrowserContext) WaitForSelector(ctx context.Context, selector string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.healthy {
		return fmt.Errorf("browser context is not healthy")
	}

	return chromedp.Run(c.chromedp, chromedp.WaitVisible(selector))
}

// Click clicks on an element
func (c *ChromeBrowserContext) Click(ctx context.Context, selector string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.healthy {
		return fmt.Errorf("browser context is not healthy")
	}

	return chromedp.Run(c.chromedp, chromedp.Click(selector))
}

// Type types text into an element
func (c *ChromeBrowserContext) Type(ctx context.Context, selector, text string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.healthy {
		return fmt.Errorf("browser context is not healthy")
	}

	return chromedp.Run(ctx, chromedp.SendKeys(selector, text))
}

// GetText gets text content from an element
func (c *ChromeBrowserContext) GetText(ctx context.Context, selector string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.healthy {
		return "", fmt.Errorf("browser context is not healthy")
	}

	var text string
	err := chromedp.Run(ctx, chromedp.Text(selector, &text))
	return text, err
}

// GetHTML gets HTML content from the page
func (c *ChromeBrowserContext) GetHTML(ctx context.Context) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.healthy {
		return "", fmt.Errorf("browser context is not healthy")
	}

	var html string
	err := chromedp.Run(ctx, chromedp.OuterHTML("html", &html))
	return html, err
}

// Screenshot takes a screenshot
func (c *ChromeBrowserContext) Screenshot(ctx context.Context) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.healthy {
		return nil, fmt.Errorf("browser context is not healthy")
	}

	var buf []byte
	err := chromedp.Run(ctx, chromedp.Screenshot("body", &buf, chromedp.NodeVisible))
	return buf, err
}

// ExecuteScript executes JavaScript
func (c *ChromeBrowserContext) ExecuteScript(ctx context.Context, script string) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.healthy {
		return nil, fmt.Errorf("browser context is not healthy")
	}

	var result interface{}
	err := chromedp.Run(ctx, chromedp.Evaluate(script, &result))
	return result, err
}

// SetCookies sets cookies
func (c *ChromeBrowserContext) SetCookies(ctx context.Context, cookies []Cookie) error {
	// TODO: Implement cookie setting
	return fmt.Errorf("not implemented")
}

// GetCookies gets cookies
func (c *ChromeBrowserContext) GetCookies(ctx context.Context) ([]Cookie, error) {
	// TODO: Implement cookie getting
	return nil, fmt.Errorf("not implemented")
}

// Close closes the browser context
func (c *ChromeBrowserContext) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.healthy = false
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// IsHealthy checks if the browser context is healthy
func (c *ChromeBrowserContext) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthy
}

// GetID returns the browser context ID
func (c *ChromeBrowserContext) GetID() string {
	return c.id
}

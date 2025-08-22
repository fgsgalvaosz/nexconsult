package services

import (
	"context"

	"github.com/nexconsult/cnpj-api/internal/models"
)

// CNPJServiceInterface defines the interface for CNPJ service
type CNPJServiceInterface interface {
	// GetCNPJ retrieves CNPJ information
	GetCNPJ(ctx context.Context, cnpj string) (*models.CNPJResponse, error)
	
	// GetBatchCNPJ retrieves multiple CNPJ information
	GetBatchCNPJ(ctx context.Context, cnpjs []string) ([]models.BatchResult, error)
	
	// Health returns service health status
	Health() map[string]interface{}
	
	// Close closes the service and releases resources
	Close() error
}

// CacheServiceInterface defines the interface for cache service
type CacheServiceInterface interface {
	// Get retrieves a value from cache
	Get(ctx context.Context, key string) (string, error)
	
	// Set stores a value in cache with TTL
	Set(ctx context.Context, key string, value string) error
	
	// Delete removes a value from cache
	Delete(ctx context.Context, key string) error
	
	// Clear clears all cache entries
	Clear(ctx context.Context) error
	
	// Exists checks if a key exists in cache
	Exists(ctx context.Context, key string) (bool, error)
	
	// GetStats returns cache statistics
	GetStats(ctx context.Context) (map[string]interface{}, error)
	
	// Health returns cache service health status
	Health() map[string]interface{}
}

// BrowserServiceInterface defines the interface for browser service
type BrowserServiceInterface interface {
	// GetBrowser gets an available browser context
	GetBrowser(ctx context.Context) (BrowserContext, error)
	
	// ReleaseBrowser releases a browser context back to the pool
	ReleaseBrowser(browserCtx BrowserContext) error
	
	// GetStats returns browser pool statistics
	GetStats() map[string]interface{}
	
	// Health returns browser service health status
	Health() map[string]interface{}
	
	// Restart restarts the browser pool
	Restart() error
	
	// Close closes all browsers and releases resources
	Close() error
}

// BrowserContext represents a browser context for automation
type BrowserContext interface {
	// Navigate navigates to a URL
	Navigate(ctx context.Context, url string) error
	
	// WaitForSelector waits for an element to appear
	WaitForSelector(ctx context.Context, selector string) error
	
	// Click clicks on an element
	Click(ctx context.Context, selector string) error
	
	// Type types text into an element
	Type(ctx context.Context, selector, text string) error
	
	// GetText gets text content from an element
	GetText(ctx context.Context, selector string) (string, error)
	
	// GetHTML gets HTML content from the page
	GetHTML(ctx context.Context) (string, error)
	
	// Screenshot takes a screenshot
	Screenshot(ctx context.Context) ([]byte, error)
	
	// ExecuteScript executes JavaScript
	ExecuteScript(ctx context.Context, script string) (interface{}, error)
	
	// SetCookies sets cookies
	SetCookies(ctx context.Context, cookies []Cookie) error
	
	// GetCookies gets cookies
	GetCookies(ctx context.Context) ([]Cookie, error)
	
	// Close closes the browser context
	Close() error
	
	// IsHealthy checks if the browser context is healthy
	IsHealthy() bool
	
	// GetID returns the browser context ID
	GetID() string
}

// Cookie represents an HTTP cookie
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Expires  int64   `json:"expires,omitempty"`
	HTTPOnly bool    `json:"httpOnly,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	SameSite string  `json:"sameSite,omitempty"`
}

// CaptchaServiceInterface defines the interface for captcha solving service
type CaptchaServiceInterface interface {
	// SolveCaptcha solves a captcha image
	SolveCaptcha(ctx context.Context, imageData []byte) (string, error)
	
	// GetBalance gets the current balance
	GetBalance(ctx context.Context) (float64, error)
	
	// Health returns captcha service health status
	Health() map[string]interface{}
}

// ExtractorServiceInterface defines the interface for data extraction service
type ExtractorServiceInterface interface {
	// ExtractCNPJData extracts CNPJ data from HTML
	ExtractCNPJData(ctx context.Context, html string) (*models.CNPJResponse, error)
	
	// ValidateExtractedData validates extracted data
	ValidateExtractedData(data *models.CNPJResponse) error
	
	// Health returns extractor service health status
	Health() map[string]interface{}
}

// MetricsServiceInterface defines the interface for metrics service
type MetricsServiceInterface interface {
	// RecordRequest records a request metric
	RecordRequest(method, endpoint string, statusCode int, duration float64)
	
	// RecordCacheHit records a cache hit
	RecordCacheHit(hit bool)
	
	// RecordBrowserUsage records browser usage
	RecordBrowserUsage(browserID string, duration float64)
	
	// GetMetrics returns current metrics
	GetMetrics() map[string]interface{}
	
	// Health returns metrics service health status
	Health() map[string]interface{}
}

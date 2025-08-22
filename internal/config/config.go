package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the application
type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Redis    RedisConfig    `json:"redis"`
	CNPJ     CNPJConfig     `json:"cnpj"`
	Log      LogConfig      `json:"log"`
	Security SecurityConfig `json:"security"`
	Browser  BrowserConfig  `json:"browser"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port         int    `json:"port"`
	Environment  string `json:"environment"`
	ReadTimeout  int    `json:"read_timeout"`
	WriteTimeout int    `json:"write_timeout"`
	IdleTimeout  int    `json:"idle_timeout"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	User            string        `json:"user"`
	Password        string        `json:"password"`
	Name            string        `json:"name"`
	SSLMode         string        `json:"ssl_mode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	Password     string        `json:"password"`
	DB           int           `json:"db"`
	PoolSize     int           `json:"pool_size"`
	DialTimeout  time.Duration `json:"dial_timeout"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
}

// CNPJConfig holds CNPJ service configuration
type CNPJConfig struct {
	SolveCaptchaAPIKey string        `json:"solve_captcha_api_key"`
	BaseURL            string        `json:"base_url"`
	Timeout            time.Duration `json:"timeout"`
	MaxRetries         int           `json:"max_retries"`
	RetryDelay         time.Duration `json:"retry_delay"`
	CacheTTL           time.Duration `json:"cache_ttl"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	RateLimit RateLimitConfig `json:"rate_limit"`
	CORS      CORSConfig      `json:"cors"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int           `json:"requests_per_minute"`
	BurstSize         int           `json:"burst_size"`
	CleanupInterval   time.Duration `json:"cleanup_interval"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string `json:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
}

// BrowserConfig holds browser automation configuration
type BrowserConfig struct {
	MinBrowsers       int           `json:"min_browsers"`
	MaxBrowsers       int           `json:"max_browsers"`
	MaxPagesPerBrowser int          `json:"max_pages_per_browser"`
	BrowserTimeout    time.Duration `json:"browser_timeout"`
	PageTimeout       time.Duration `json:"page_timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout"`
	Headless          bool          `json:"headless"`
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         getEnvAsInt("PORT", 8080),
			Environment:  getEnv("ENVIRONMENT", "development"),
			ReadTimeout:  getEnvAsInt("READ_TIMEOUT", 30),
			WriteTimeout: getEnvAsInt("WRITE_TIMEOUT", 30),
			IdleTimeout:  getEnvAsInt("IDLE_TIMEOUT", 60),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnvAsInt("DB_PORT", 5432),
			User:            getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", ""),
			Name:            getEnv("DB_NAME", "cnpj_api"),
			SSLMode:         getEnv("DB_SSL_MODE", "disable"),
			MaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: time.Duration(getEnvAsInt("DB_CONN_MAX_LIFETIME", 300)) * time.Second,
		},
		Redis: RedisConfig{
			Host:         getEnv("REDIS_HOST", "localhost"),
			Port:         getEnvAsInt("REDIS_PORT", 6379),
			Password:     getEnv("REDIS_PASSWORD", ""),
			DB:           getEnvAsInt("REDIS_DB", 0),
			PoolSize:     getEnvAsInt("REDIS_POOL_SIZE", 10),
			DialTimeout:  time.Duration(getEnvAsInt("REDIS_DIAL_TIMEOUT", 5)) * time.Second,
			ReadTimeout:  time.Duration(getEnvAsInt("REDIS_READ_TIMEOUT", 3)) * time.Second,
			WriteTimeout: time.Duration(getEnvAsInt("REDIS_WRITE_TIMEOUT", 3)) * time.Second,
		},
		CNPJ: CNPJConfig{
			SolveCaptchaAPIKey: getEnv("SOLVE_CAPTCHA_API_KEY", ""),
			BaseURL:            getEnv("CNPJ_BASE_URL", "https://solucoes.receita.fazenda.gov.br/servicos/cnpjreva/cnpjreva_solicitacao.asp"),
			Timeout:            time.Duration(getEnvAsInt("CNPJ_TIMEOUT", 300)) * time.Second,
			MaxRetries:         getEnvAsInt("CNPJ_MAX_RETRIES", 3),
			RetryDelay:         time.Duration(getEnvAsInt("CNPJ_RETRY_DELAY", 5)) * time.Second,
			CacheTTL:           time.Duration(getEnvAsInt("CNPJ_CACHE_TTL", 3600)) * time.Second,
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		Security: SecurityConfig{
			RateLimit: RateLimitConfig{
				RequestsPerMinute: getEnvAsInt("RATE_LIMIT_RPM", 100),
				BurstSize:         getEnvAsInt("RATE_LIMIT_BURST", 10),
				CleanupInterval:   time.Duration(getEnvAsInt("RATE_LIMIT_CLEANUP", 60)) * time.Second,
			},
			CORS: CORSConfig{
				AllowedOrigins:   []string{"*"},
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders:   []string{"*"},
				AllowCredentials: false,
			},
		},
		Browser: BrowserConfig{
			MinBrowsers:        getEnvAsInt("BROWSER_MIN", 3),
			MaxBrowsers:        getEnvAsInt("BROWSER_MAX", 15),
			MaxPagesPerBrowser: getEnvAsInt("BROWSER_MAX_PAGES", 3),
			BrowserTimeout:     time.Duration(getEnvAsInt("BROWSER_TIMEOUT", 60)) * time.Second,
			PageTimeout:        time.Duration(getEnvAsInt("PAGE_TIMEOUT", 30)) * time.Second,
			IdleTimeout:        time.Duration(getEnvAsInt("BROWSER_IDLE_TIMEOUT", 300)) * time.Second,
			Headless:           getEnvAsBool("BROWSER_HEADLESS", true),
		},
	}

	// Validate required fields
	if cfg.CNPJ.SolveCaptchaAPIKey == "" {
		return nil, fmt.Errorf("SOLVE_CAPTCHA_API_KEY is required")
	}

	return cfg, nil
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

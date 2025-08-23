package config

import (
	"os"
	"strconv"

	"nexconsult/internal/types"
)

// LoadConfig carrega configuração do ambiente
func LoadConfig() *types.Config {
	config := &types.Config{
		Server: types.ServerConfig{
			Port:         getEnvInt("PORT", 3000),
			Prefork:      getEnvBool("PREFORK", false),
			ReadTimeout:  getEnvInt("READ_TIMEOUT", 30),
			WriteTimeout: getEnvInt("WRITE_TIMEOUT", 30),
			IdleTimeout:  getEnvInt("IDLE_TIMEOUT", 120),
		},
		Workers: types.WorkersConfig{
			Count:          getEnvInt("WORKERS_COUNT", 5),
			MaxConcurrent:  getEnvInt("MAX_CONCURRENT", 10),
			TimeoutSeconds: getEnvInt("WORKER_TIMEOUT", 300),
		},
		// Cache removido - sempre busca direta
		SolveCaptcha: types.SolveCaptchaConfig{
			APIKey:         getEnv("SOLVECAPTCHA_API_KEY", "bd238cb2bace2dd234e32a8df23486f1"),
			TimeoutSeconds: getEnvInt("CAPTCHA_TIMEOUT", 300),
			MaxRetries:     getEnvInt("CAPTCHA_MAX_RETRIES", 3),
		},
		RateLimit: types.RateLimitConfig{
			RequestsPerMinute: getEnvInt("RATE_LIMIT_RPM", 100),
		},
		Browser: types.BrowserConfig{
			PageTimeoutSeconds:       getEnvInt("BROWSER_PAGE_TIMEOUT", 30),
			NavigationTimeoutSeconds: getEnvInt("BROWSER_NAV_TIMEOUT", 30),
			ElementTimeoutSeconds:    getEnvInt("BROWSER_ELEMENT_TIMEOUT", 15),
			MaxIdleMinutes:           getEnvInt("BROWSER_MAX_IDLE_MINUTES", 30),
		},
		Logging: types.LoggingConfig{
			Level:      getEnv("LOG_LEVEL", "info"),
			Format:     getEnv("LOG_FORMAT", "console"),
			Output:     getEnv("LOG_OUTPUT", "stdout"),
			FilePath:   getEnv("LOG_FILE_PATH", ""),
			MaxSize:    getEnvInt("LOG_MAX_SIZE", 100),
			MaxBackups: getEnvInt("LOG_MAX_BACKUPS", 3),
			MaxAge:     getEnvInt("LOG_MAX_AGE", 7),
			Compress:   getEnvBool("LOG_COMPRESS", true),
			Sampling:   getEnvBool("LOG_SAMPLING", false),
		},
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	return config
}

// getEnv retorna variável de ambiente ou valor padrão
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt retorna variável de ambiente como int ou valor padrão
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvBool retorna variável de ambiente como bool ou valor padrão
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

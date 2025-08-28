package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	SolveCaptchaAPIKey string
	SintegraURL        string
	Timeout            int // em segundos
	DebugMode          bool
	Headless           bool
}

func LoadConfig() *Config {
	// Carregar variáveis do arquivo .env
	if err := godotenv.Load(); err != nil {
		log.Printf("Aviso: Arquivo .env não encontrado ou erro ao carregar: %v", err)
	}

	apiKey := getEnv("SOLVE_CAPTCHA_API_KEY", "")
	log.Printf("[DEBUG] Chave da API carregada: %s", apiKey)

	return &Config{
		SolveCaptchaAPIKey: apiKey,
		SintegraURL:        getEnv("SINTEGRA_URL", "https://sistemas1.sefaz.ma.gov.br/sintegra/jsp/consultaSintegra/consultaSintegraFiltro.jsf"),
		Timeout:            900, // Aumentar timeout para 15 minutos (CAPTCHA + navegação)
		DebugMode:          getEnv("DEBUG", "true") == "true", // Ativar debug por padrão
		Headless:           getEnv("HEADLESS", "false") == "true", // Headless false por padrão para visualização
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
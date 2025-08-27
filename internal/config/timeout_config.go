package config

import "time"

// TimeoutConfig contém todas as configurações de timeout da aplicação
type TimeoutConfig struct {
	// Timeouts do servidor HTTP
	ServerReadTimeout  time.Duration
	ServerWriteTimeout time.Duration
	ServerIdleTimeout  time.Duration

	// Timeouts para consultas ao Sintegra
	SintegraRequestTimeout  time.Duration
	SintegraPageLoadTimeout time.Duration
	SintegraCaptchaTimeout  time.Duration

	// Timeouts para clientes HTTP
	HTTPClientTimeout time.Duration

	// Timeouts para operações em lote
	BatchOperationTimeout time.Duration
}

// DefaultTimeoutConfig retorna a configuração padrão de timeouts
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		// Timeouts do servidor HTTP
		ServerReadTimeout:  120 * time.Second, // 2 minutos
		ServerWriteTimeout: 120 * time.Second, // 2 minutos
		ServerIdleTimeout:  300 * time.Second, // 5 minutos

		// Timeouts para consultas ao Sintegra
		SintegraRequestTimeout:  180 * time.Second, // 3 minutos
		SintegraPageLoadTimeout: 30 * time.Second,  // 30 segundos
		SintegraCaptchaTimeout:  180 * time.Second, // 3 minutos

		// Timeouts para clientes HTTP
		HTTPClientTimeout: 60 * time.Second, // 1 minuto

		// Timeouts para operações em lote
		BatchOperationTimeout: 300 * time.Second, // 5 minutos
	}
}

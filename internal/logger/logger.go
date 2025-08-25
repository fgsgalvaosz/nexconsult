package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup inicializa o logger com configurações otimizadas
func Setup() zerolog.Logger {
	// Configurar output do console com cores
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	// Configurar nível de log baseado na variável de ambiente
	level := zerolog.InfoLevel
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		if parsedLevel, err := zerolog.ParseLevel(logLevel); err == nil {
			level = parsedLevel
		}
	}

	// Criar logger
	logger := zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Caller().
		Logger()

	// Definir logger global
	log.Logger = logger

	return logger
}

// GetLogger retorna uma instância do logger configurado
func GetLogger() zerolog.Logger {
	return log.Logger
}

// Info cria um evento de log de nível info
func Info() *zerolog.Event {
	return log.Info()
}

// Error cria um evento de log de nível error
func Error() *zerolog.Event {
	return log.Error()
}

// Warn cria um evento de log de nível warning
func Warn() *zerolog.Event {
	return log.Warn()
}

// Debug cria um evento de log de nível debug
func Debug() *zerolog.Event {
	return log.Debug()
}
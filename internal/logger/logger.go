package logger

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger interface abstrai diferentes implementações de logging
type Logger interface {
	// Métodos básicos de logging
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	Fatal(msg string)

	// Métodos com campos estruturados
	DebugFields(msg string, fields Fields)
	InfoFields(msg string, fields Fields)
	WarnFields(msg string, fields Fields)
	ErrorFields(msg string, fields Fields)
	FatalFields(msg string, fields Fields)

	// Métodos context-aware
	DebugCtx(ctx context.Context, msg string)
	InfoCtx(ctx context.Context, msg string)
	WarnCtx(ctx context.Context, msg string)
	ErrorCtx(ctx context.Context, msg string)
	FatalCtx(ctx context.Context, msg string)

	// Métodos context-aware com campos
	DebugCtxFields(ctx context.Context, msg string, fields Fields)
	InfoCtxFields(ctx context.Context, msg string, fields Fields)
	WarnCtxFields(ctx context.Context, msg string, fields Fields)
	ErrorCtxFields(ctx context.Context, msg string, fields Fields)
	FatalCtxFields(ctx context.Context, msg string, fields Fields)

	// Métodos para criar sub-loggers
	WithFields(fields Fields) Logger
	WithComponent(component string) Logger
	WithCorrelationID(id string) Logger
}

// Fields representa campos estruturados para logging
type Fields map[string]interface{}

// Config representa a configuração do logger
type Config struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"` // "json" ou "console"
	Output     string `mapstructure:"output"` // "stdout", "stderr", "file"
	FilePath   string `mapstructure:"file_path"`
	MaxSize    int    `mapstructure:"max_size"` // MB
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"` // dias
	Compress   bool   `mapstructure:"compress"`
	Sampling   bool   `mapstructure:"sampling"`
}

// ZerologLogger implementa a interface Logger usando zerolog
type ZerologLogger struct {
	logger zerolog.Logger
}

// NewLogger cria uma nova instância do logger
func NewLogger(config Config) Logger {
	// Configura nível de log
	level := parseLevel(config.Level)
	zerolog.SetGlobalLevel(level)

	// Configura output
	var writer io.Writer
	switch config.Output {
	case "stderr":
		writer = os.Stderr
	case "file":
		if config.FilePath != "" {
			file, err := os.OpenFile(config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to open log file")
			}
			writer = file
		} else {
			writer = os.Stdout
		}
	default:
		writer = os.Stdout
	}

	// Configura formato
	var logger zerolog.Logger
	if config.Format == "console" {
		// Formato colorido para desenvolvimento
		writer = zerolog.ConsoleWriter{
			Out:        writer,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
	}

	logger = zerolog.New(writer).With().
		Timestamp().
		Caller().
		Logger()

	// Configura sampling se habilitado
	if config.Sampling {
		logger = logger.Sample(&zerolog.BasicSampler{N: 10})
	}

	return &ZerologLogger{logger: logger}
}

// parseLevel converte string para zerolog.Level
func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}

// Implementação dos métodos básicos
func (z *ZerologLogger) Debug(msg string) {
	z.logger.Debug().Msg(msg)
}

func (z *ZerologLogger) Info(msg string) {
	z.logger.Info().Msg(msg)
}

func (z *ZerologLogger) Warn(msg string) {
	z.logger.Warn().Msg(msg)
}

func (z *ZerologLogger) Error(msg string) {
	z.logger.Error().Msg(msg)
}

func (z *ZerologLogger) Fatal(msg string) {
	z.logger.Fatal().Msg(msg)
}

// Implementação dos métodos com campos
func (z *ZerologLogger) DebugFields(msg string, fields Fields) {
	event := z.logger.Debug()
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

func (z *ZerologLogger) InfoFields(msg string, fields Fields) {
	event := z.logger.Info()
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

func (z *ZerologLogger) WarnFields(msg string, fields Fields) {
	event := z.logger.Warn()
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

func (z *ZerologLogger) ErrorFields(msg string, fields Fields) {
	event := z.logger.Error()
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

func (z *ZerologLogger) FatalFields(msg string, fields Fields) {
	event := z.logger.Fatal()
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

// Implementação dos métodos context-aware
func (z *ZerologLogger) DebugCtx(ctx context.Context, msg string) {
	z.logger.Debug().Ctx(ctx).Msg(msg)
}

func (z *ZerologLogger) InfoCtx(ctx context.Context, msg string) {
	z.logger.Info().Ctx(ctx).Msg(msg)
}

func (z *ZerologLogger) WarnCtx(ctx context.Context, msg string) {
	z.logger.Warn().Ctx(ctx).Msg(msg)
}

func (z *ZerologLogger) ErrorCtx(ctx context.Context, msg string) {
	z.logger.Error().Ctx(ctx).Msg(msg)
}

func (z *ZerologLogger) FatalCtx(ctx context.Context, msg string) {
	z.logger.Fatal().Ctx(ctx).Msg(msg)
}

// Implementação dos métodos context-aware com campos
func (z *ZerologLogger) DebugCtxFields(ctx context.Context, msg string, fields Fields) {
	event := z.logger.Debug().Ctx(ctx)
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

func (z *ZerologLogger) InfoCtxFields(ctx context.Context, msg string, fields Fields) {
	event := z.logger.Info().Ctx(ctx)
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

func (z *ZerologLogger) WarnCtxFields(ctx context.Context, msg string, fields Fields) {
	event := z.logger.Warn().Ctx(ctx)
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

func (z *ZerologLogger) ErrorCtxFields(ctx context.Context, msg string, fields Fields) {
	event := z.logger.Error().Ctx(ctx)
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

func (z *ZerologLogger) FatalCtxFields(ctx context.Context, msg string, fields Fields) {
	event := z.logger.Fatal().Ctx(ctx)
	for k, v := range fields {
		event = event.Interface(k, v)
	}
	event.Msg(msg)
}

// Métodos para criar sub-loggers
func (z *ZerologLogger) WithFields(fields Fields) Logger {
	ctx := z.logger.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return &ZerologLogger{logger: ctx.Logger()}
}

func (z *ZerologLogger) WithComponent(component string) Logger {
	return &ZerologLogger{
		logger: z.logger.With().Str("component", component).Logger(),
	}
}

func (z *ZerologLogger) WithCorrelationID(id string) Logger {
	return &ZerologLogger{
		logger: z.logger.With().Str("correlation_id", id).Logger(),
	}
}

// Variável global para o logger (será inicializada no main)
var globalLogger Logger

// SetGlobalLogger define o logger global
func SetGlobalLogger(logger Logger) {
	globalLogger = logger
}

// GetGlobalLogger retorna o logger global
func GetGlobalLogger() Logger {
	if globalLogger == nil {
		// Fallback para um logger básico se não foi inicializado
		globalLogger = NewLogger(Config{
			Level:  "info",
			Format: "console",
			Output: "stdout",
		})
	}
	return globalLogger
}

// Funções helper para logs específicos
func LogCNPJRequest(cnpj string, correlationID string) {
	GetGlobalLogger().WithCorrelationID(correlationID).InfoFields("CNPJ request started", Fields{
		"cnpj": cnpj,
		"type": "cnpj_request",
	})
}

func LogCNPJResponse(cnpj string, correlationID string, success bool, duration time.Duration) {
	logger := GetGlobalLogger().WithCorrelationID(correlationID)
	fields := Fields{
		"cnpj":     cnpj,
		"success":  success,
		"duration": duration.String(),
		"type":     "cnpj_response",
	}

	if success {
		logger.InfoFields("CNPJ request completed successfully", fields)
	} else {
		logger.WarnFields("CNPJ request failed", fields)
	}
}

func LogCaptchaResolution(correlationID string, success bool, attempts int, duration time.Duration) {
	logger := GetGlobalLogger().WithCorrelationID(correlationID)
	fields := Fields{
		"success":  success,
		"attempts": attempts,
		"duration": duration.String(),
		"type":     "captcha_resolution",
	}

	if success {
		logger.InfoFields("Captcha resolved successfully", fields)
	} else {
		logger.ErrorFields("Captcha resolution failed", fields)
	}
}

func LogWorkerJob(workerID int, jobID string, action string, fields Fields) {
	logger := GetGlobalLogger().WithComponent("worker").WithFields(Fields{
		"worker_id": workerID,
		"job_id":    jobID,
		"action":    action,
		"type":      "worker_job",
	})

	if fields != nil {
		logger = logger.WithFields(fields)
	}

	switch action {
	case "started":
		logger.Info("Worker job started")
	case "completed":
		logger.Info("Worker job completed")
	case "failed":
		logger.Error("Worker job failed")
	default:
		logger.Info("Worker job action")
	}
}

func LogBrowserAction(action string, correlationID string, fields Fields) {
	logger := GetGlobalLogger().WithComponent("browser").WithCorrelationID(correlationID)

	if fields != nil {
		logger = logger.WithFields(fields)
	}

	logger.InfoFields("Browser action", Fields{
		"action": action,
		"type":   "browser_action",
	})
}

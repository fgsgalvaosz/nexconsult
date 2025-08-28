package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger é a interface para logging centralizado
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	With(fields ...Field) Logger
}

// Field representa um campo de log
type Field struct {
	Key   string
	Value interface{}
}

// AppLogger implementa a interface Logger usando zerolog
type AppLogger struct {
	logger zerolog.Logger
}

// NewLogger cria uma nova instância do logger
func NewLogger(debugMode bool) Logger {
	// Configurar output colorizado para console
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006/01/02 15:04:05",
		NoColor:    false,
		FormatLevel: func(i interface{}) string {
			return colorizeLevel(i.(string))
		},
		FormatMessage: func(i interface{}) string {
			return colorizeMessage(i.(string))
		},
		FormatFieldName: func(i interface{}) string {
			return colorizeFieldName(i.(string)) + "="
		},
		FormatFieldValue: func(i interface{}) string {
			return colorizeFieldValue(i)
		},
		PartsOrder: []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
			zerolog.MessageFieldName,
		},
	}

	// Configurar nível de log
	level := zerolog.InfoLevel
	if debugMode {
		level = zerolog.DebugLevel
	}

	// Criar logger
	logger := zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Logger()

	return &AppLogger{logger: logger}
}

// Debug registra uma mensagem de debug
func (l *AppLogger) Debug(msg string, fields ...Field) {
	event := l.logger.Debug()
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	event.Msg(msg)
}

// Info registra uma mensagem informativa
func (l *AppLogger) Info(msg string, fields ...Field) {
	event := l.logger.Info()
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	event.Msg(msg)
}

// Warn registra uma mensagem de aviso
func (l *AppLogger) Warn(msg string, fields ...Field) {
	event := l.logger.Warn()
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	event.Msg(msg)
}

// Error registra uma mensagem de erro
func (l *AppLogger) Error(msg string, fields ...Field) {
	event := l.logger.Error()
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	event.Msg(msg)
}

// Fatal registra uma mensagem fatal e termina o programa
func (l *AppLogger) Fatal(msg string, fields ...Field) {
	event := l.logger.Fatal()
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	event.Msg(msg)
}

// With cria um novo logger com campos adicionais
func (l *AppLogger) With(fields ...Field) Logger {
	ctx := l.logger.With()
	for _, field := range fields {
		ctx = ctx.Interface(field.Key, field.Value)
	}
	return &AppLogger{logger: ctx.Logger()}
}

// Funções auxiliares para colorização
func colorizeLevel(level string) string {
	switch level {
	case "debug":
		return "\033[36m[DEBUG]\033[0m" // Cyan
	case "info":
		return "\033[32m[INFO]\033[0m" // Green
	case "warn":
		return "\033[33m[WARN]\033[0m" // Yellow
	case "error":
		return "\033[31m[ERROR]\033[0m" // Red
	case "fatal":
		return "\033[35m[FATAL]\033[0m" // Magenta
	default:
		return level
	}
}

func colorizeMessage(msg string) string {
	return "\033[37m" + msg + "\033[0m" // White
}

func colorizeFieldName(name string) string {
	return "\033[34m" + name + "\033[0m" // Blue
}

func colorizeFieldValue(value interface{}) string {
	// Formatar valores de forma mais legível
	switch v := value.(type) {
	case string:
		return "\033[36m" + v + "\033[0m" // Cyan
	case bool:
		return "\033[36m" + fmt.Sprintf("%t", v) + "\033[0m"
	case int, int64, int32:
		return "\033[36m" + fmt.Sprintf("%d", v) + "\033[0m"
	case float64, float32:
		return "\033[36m" + fmt.Sprintf("%.2f", v) + "\033[0m"
	case time.Duration:
		return "\033[36m" + v.String() + "\033[0m"
	default:
		return "\033[36m" + fmt.Sprintf("%v", v) + "\033[0m"
	}
}

// Funções de conveniência para criar campos
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value}
}

func Error(err error) Field {
	return Field{Key: "error", Value: err}
}

func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// Instância global do logger
var globalLogger Logger

// InitGlobalLogger inicializa o logger global
func InitGlobalLogger(debugMode bool) {
	globalLogger = NewLogger(debugMode)
}

// GetLogger retorna a instância global do logger
func GetLogger() Logger {
	if globalLogger == nil {
		globalLogger = NewLogger(false)
	}
	return globalLogger
}

// Funções globais de conveniência
func Debug(msg string, fields ...Field) {
	GetLogger().Debug(msg, fields...)
}

func Info(msg string, fields ...Field) {
	GetLogger().Info(msg, fields...)
}

func Warn(msg string, fields ...Field) {
	GetLogger().Warn(msg, fields...)
}

func ErrorLog(msg string, fields ...Field) {
	GetLogger().Error(msg, fields...)
}

func Fatal(msg string, fields ...Field) {
	GetLogger().Fatal(msg, fields...)
}

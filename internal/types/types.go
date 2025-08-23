package types

import (
	"time"
)

// CNPJData representa os dados completos de um CNPJ
type CNPJData struct {
	CNPJ        CNPJInfo        `json:"cnpj"`
	Empresa     EmpresaInfo     `json:"empresa"`
	Atividades  AtividadesInfo  `json:"atividades"`
	Endereco    EnderecoInfo    `json:"endereco"`
	Contato     ContatoInfo     `json:"contato"`
	Situacao    SituacaoInfo    `json:"situacao"`
	Comprovante ComprovanteInfo `json:"comprovante"`
	Metadados   MetadadosInfo   `json:"metadados"`
}

// CNPJInfo contém informações básicas do CNPJ
type CNPJInfo struct {
	Numero       string `json:"numero"`
	Tipo         string `json:"tipo"` // MATRIZ ou FILIAL
	DataAbertura string `json:"data_abertura"`
}

// EmpresaInfo contém informações da empresa
type EmpresaInfo struct {
	RazaoSocial      string           `json:"razao_social"`
	NomeFantasia     string           `json:"nome_fantasia"`
	Porte            string           `json:"porte"`
	NaturezaJuridica NaturezaJuridica `json:"natureza_juridica"`
}

// NaturezaJuridica representa o código e descrição da natureza jurídica
type NaturezaJuridica struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
}

// AtividadesInfo contém as atividades econômicas
type AtividadesInfo struct {
	Principal   Atividade   `json:"principal"`
	Secundarias []Atividade `json:"secundarias"`
}

// Atividade representa uma atividade econômica
type Atividade struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
}

// EnderecoInfo contém o endereço completo
type EnderecoInfo struct {
	Logradouro  string `json:"logradouro"`
	Numero      string `json:"numero"`
	Complemento string `json:"complemento"`
	CEP         string `json:"cep"`
	Bairro      string `json:"bairro"`
	Municipio   string `json:"municipio"`
	UF          string `json:"uf"`
}

// ContatoInfo contém informações de contato
type ContatoInfo struct {
	Email    string `json:"email"`
	Telefone string `json:"telefone"`
}

// SituacaoInfo contém informações da situação cadastral
type SituacaoInfo struct {
	Cadastral            string `json:"cadastral"`
	DataSituacao         string `json:"data_situacao"`
	Motivo               string `json:"motivo"`
	SituacaoEspecial     string `json:"situacao_especial"`
	DataSituacaoEspecial string `json:"data_situacao_especial"`
}

// ComprovanteInfo contém informações do comprovante
type ComprovanteInfo struct {
	DataEmissao string `json:"data_emissao"`
	HoraEmissao string `json:"hora_emissao"`
}

// MetadadosInfo contém metadados da consulta
type MetadadosInfo struct {
	URLConsulta string    `json:"url_consulta"`
	Timestamp   time.Time `json:"timestamp"`
	Sucesso     bool      `json:"sucesso"`
	Fonte       string    `json:"fonte"` // "cache" ou "online"
	Duracao     string    `json:"duracao"`
}

// BatchRequest representa uma requisição em lote
type BatchRequest struct {
	CNPJs    []string     `json:"cnpjs" validate:"required,min=1,max=100"`
	UseCache bool         `json:"use_cache"`
	Options  BatchOptions `json:"options"`
}

// BatchOptions contém opções para processamento em lote
type BatchOptions struct {
	MaxConcurrent int `json:"max_concurrent"`
	Timeout       int `json:"timeout"` // em segundos
}

// BatchResponse representa a resposta de uma consulta em lote
type BatchResponse struct {
	Results []CNPJResult `json:"results"`
	Stats   BatchStats   `json:"stats"`
}

// CNPJResult representa o resultado de uma consulta individual
type CNPJResult struct {
	CNPJ   string    `json:"cnpj"`
	Data   *CNPJData `json:"data,omitempty"`
	Error  string    `json:"error,omitempty"`
	Status string    `json:"status"` // "success", "error", "cached"
}

// BatchStats contém estatísticas do processamento em lote
type BatchStats struct {
	Total     int           `json:"total"`
	Success   int           `json:"success"`
	Errors    int           `json:"errors"`
	Cached    int           `json:"cached"`
	Duration  time.Duration `json:"duration"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
}

// WorkerStats contém estatísticas dos workers
type WorkerStats struct {
	Workers WorkerInfo `json:"workers"`
	Queue   QueueInfo  `json:"queue"`
	Cache   CacheInfo  `json:"cache"`
	System  SystemInfo `json:"system"`
}

// WorkerInfo contém informações dos workers
type WorkerInfo struct {
	Active int `json:"active"`
	Idle   int `json:"idle"`
	Total  int `json:"total"`
}

// QueueInfo contém informações da fila
type QueueInfo struct {
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Completed  int `json:"completed"`
}

// CacheInfo contém informações do cache
type CacheInfo struct {
	HitRate float64 `json:"hit_rate"`
	Size    int     `json:"size"`
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
}

// SystemInfo contém informações do sistema
type SystemInfo struct {
	Uptime    string `json:"uptime"`
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
}

// Job representa um trabalho para o worker pool
type Job struct {
	ID       string          `json:"id"`
	CNPJ     string          `json:"cnpj"`
	UseCache bool            `json:"use_cache"`
	Created  time.Time       `json:"created"`
	Started  time.Time       `json:"started"`
	Finished time.Time       `json:"finished"`
	Result   chan CNPJResult `json:"-"`
}

// Config representa a configuração da aplicação
type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Workers      WorkersConfig      `mapstructure:"workers"`
	SolveCaptcha SolveCaptchaConfig `mapstructure:"solvecaptcha"`
	RateLimit    RateLimitConfig    `mapstructure:"ratelimit"`
	Browser      BrowserConfig      `mapstructure:"browser"`
	Logging      LoggingConfig      `mapstructure:"logging"`
	LogLevel     string             `mapstructure:"log_level"` // Mantido para compatibilidade
}

// ServerConfig contém configurações do servidor
type ServerConfig struct {
	Port         int  `mapstructure:"port"`
	Prefork      bool `mapstructure:"prefork"`
	ReadTimeout  int  `mapstructure:"read_timeout"`
	WriteTimeout int  `mapstructure:"write_timeout"`
	IdleTimeout  int  `mapstructure:"idle_timeout"`
}

// WorkersConfig contém configurações dos workers
type WorkersConfig struct {
	Count          int `mapstructure:"count"`
	MaxConcurrent  int `mapstructure:"max_concurrent"`
	TimeoutSeconds int `mapstructure:"timeout_seconds"`
}

// Cache removido - sempre busca direta no site da Receita Federal

// SolveCaptchaConfig contém configurações da API SolveCaptcha
type SolveCaptchaConfig struct {
	APIKey         string `mapstructure:"api_key"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
	MaxRetries     int    `mapstructure:"max_retries"`
}

// RateLimitConfig contém configurações de rate limiting
type RateLimitConfig struct {
	RequestsPerMinute int `mapstructure:"requests_per_minute"`
}

// BrowserConfig contém configurações do browser
type BrowserConfig struct {
	PageTimeoutSeconds       int `mapstructure:"page_timeout_seconds"`
	NavigationTimeoutSeconds int `mapstructure:"navigation_timeout_seconds"`
	ElementTimeoutSeconds    int `mapstructure:"element_timeout_seconds"`
	MaxIdleMinutes           int `mapstructure:"max_idle_minutes"`
}

// LoggingConfig configurações de logging
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"`
	FilePath   string `mapstructure:"file_path"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
	Sampling   bool   `mapstructure:"sampling"`
}

// ErrorResponse representa uma resposta de erro
type ErrorResponse struct {
	Error   string    `json:"error"`
	Message string    `json:"message,omitempty"`
	Code    int       `json:"code"`
	Details string    `json:"details,omitempty"`
	Time    time.Time `json:"time"`
}

// StatusResponse representa a resposta do endpoint de status
type StatusResponse struct {
	Workers WorkerInfo `json:"workers"`
	Queue   QueueInfo  `json:"queue"`
	Cache   CacheInfo  `json:"cache"`
	System  SystemInfo `json:"system"`
}

// HealthResponse representa a resposta do health check
type HealthResponse struct {
	Status  string    `json:"status"`
	Time    time.Time `json:"time"`
	Version string    `json:"version"`
	Uptime  string    `json:"uptime"`
}

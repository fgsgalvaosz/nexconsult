package models

import (
	"time"
)

// CNPJRequest represents a CNPJ consultation request
type CNPJRequest struct {
	CNPJ string `json:"cnpj" binding:"required" validate:"required,len=14" example:"11222333000181"`
}

// CNPJResponse represents the response from CNPJ consultation
type CNPJResponse struct {
	CNPJ                      string       `json:"cnpj" example:"11.222.333/0001-81"`
	RazaoSocial               string       `json:"razao_social" example:"EMPRESA EXEMPLO LTDA"`
	NomeFantasia              string       `json:"nome_fantasia,omitempty" example:"Empresa Exemplo"`
	Situacao                  string       `json:"situacao" example:"ATIVA"`
	DataSituacao              string       `json:"data_situacao" example:"03/11/2005"`
	MotivoSituacao            string       `json:"motivo_situacao" example:"SEM MOTIVO"`
	SituacaoEspecial          string       `json:"situacao_especial,omitempty"`
	DataSituacaoEspecial      string       `json:"data_situacao_especial,omitempty"`
	TipoEmpresa               string       `json:"tipo_empresa" example:"MATRIZ"`
	DataInicioAtividade       string       `json:"data_inicio_atividade" example:"03/11/2005"`
	CNAEPrincipal             CNAEInfo     `json:"cnae_principal"`
	CNAESecundarias           []CNAEInfo   `json:"cnae_secundarias,omitempty"`
	NaturezaJuridica          string       `json:"natureza_juridica" example:"206-2 - SOCIEDADE EMPRESÁRIA LIMITADA"`
	Endereco                  EnderecoInfo `json:"endereco"`
	Telefones                 []string     `json:"telefones,omitempty"`
	Email                     string       `json:"email,omitempty"`
	CapitalSocial             string       `json:"capital_social,omitempty" example:"1000000,00"`
	Porte                     string       `json:"porte" example:"DEMAIS"`
	EnteFederativoResponsavel string       `json:"ente_federativo_responsavel,omitempty"`
	QualificacaoResponsavel   string       `json:"qualificacao_responsavel,omitempty"`
	Socios                    []SocioInfo  `json:"socios,omitempty"`
	ConsultadoEm              time.Time    `json:"consultado_em" example:"2024-01-15T10:30:00Z"`
	Cache                     bool         `json:"cache" example:"false"`
	TempoConsulta             int64        `json:"tempo_consulta_ms" example:"2500"`
}

// CNAEInfo represents CNAE information
type CNAEInfo struct {
	Codigo    string `json:"codigo" example:"6201-5/00"`
	Descricao string `json:"descricao" example:"Desenvolvimento de programas de computador sob encomenda"`
}

// EnderecoInfo represents address information
type EnderecoInfo struct {
	Logradouro  string `json:"logradouro" example:"RUA EXEMPLO"`
	Numero      string `json:"numero" example:"123"`
	Complemento string `json:"complemento,omitempty" example:"SALA 456"`
	Bairro      string `json:"bairro" example:"CENTRO"`
	CEP         string `json:"cep" example:"01234-567"`
	Municipio   string `json:"municipio" example:"SÃO PAULO"`
	UF          string `json:"uf" example:"SP"`
}

// SocioInfo represents partner/shareholder information
type SocioInfo struct {
	Nome                      string `json:"nome" example:"JOÃO DA SILVA"`
	CPFCNPJSocio              string `json:"cpf_cnpj_socio,omitempty" example:"***123456**"`
	QualificacaoSocio         string `json:"qualificacao_socio" example:"49-SÓCIO-ADMINISTRADOR"`
	DataEntradaSociedade      string `json:"data_entrada_sociedade,omitempty" example:"03/11/2005"`
	PaisOrigem                string `json:"pais_origem,omitempty" example:"BRASIL"`
	RepresentanteLegal        string `json:"representante_legal,omitempty"`
	QualificacaoRepresentante string `json:"qualificacao_representante,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error     string    `json:"error" example:"Invalid CNPJ format"`
	Message   string    `json:"message" example:"CNPJ must contain exactly 14 digits"`
	Code      string    `json:"code,omitempty" example:"INVALID_CNPJ"`
	Timestamp time.Time `json:"timestamp" example:"2024-01-15T10:30:00Z"`
	Path      string    `json:"path" example:"/api/v1/cnpj/11222333000181"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string                 `json:"status" example:"healthy"`
	Timestamp time.Time              `json:"timestamp" example:"2024-01-15T10:30:00Z"`
	Version   string                 `json:"version" example:"2.0.0"`
	Services  map[string]ServiceInfo `json:"services"`
	Uptime    string                 `json:"uptime" example:"2h30m45s"`
}

// ServiceInfo represents individual service health
type ServiceInfo struct {
	Status         string    `json:"status" example:"healthy"`
	LastCheck      time.Time `json:"last_check" example:"2024-01-15T10:30:00Z"`
	ResponseTimeMs int64     `json:"response_time_ms" example:"150"`
	Error          string    `json:"error,omitempty"`
}

// MetricsResponse represents metrics response
type MetricsResponse struct {
	Requests    RequestsMetrics    `json:"requests"`
	Performance PerformanceMetrics `json:"performance"`
	Cache       CacheMetrics       `json:"cache"`
	Browser     BrowserMetrics     `json:"browser"`
	System      SystemMetrics      `json:"system"`
	Timestamp   time.Time          `json:"timestamp" example:"2024-01-15T10:30:00Z"`
}

// RequestsMetrics represents request metrics
type RequestsMetrics struct {
	Total       int64   `json:"total" example:"1500"`
	Success     int64   `json:"success" example:"1450"`
	Errors      int64   `json:"errors" example:"50"`
	SuccessRate float64 `json:"success_rate" example:"96.67"`
}

// PerformanceMetrics represents performance metrics
type PerformanceMetrics struct {
	AvgResponseTimeMs int64 `json:"avg_response_time_ms" example:"2500"`
	P95ResponseTimeMs int64 `json:"p95_response_time_ms" example:"5200"`
	P99ResponseTimeMs int64 `json:"p99_response_time_ms" example:"8100"`
}

// CacheMetrics represents cache metrics
type CacheMetrics struct {
	HitRate float64 `json:"hit_rate" example:"85.5"`
	Hits    int64   `json:"hits" example:"1240"`
	Misses  int64   `json:"misses" example:"210"`
	Size    int64   `json:"size" example:"15000"`
}

// BrowserMetrics represents browser metrics
type BrowserMetrics struct {
	ActiveBrowsers int `json:"active_browsers" example:"8"`
	TotalBrowsers  int `json:"total_browsers" example:"15"`
	QueueSize      int `json:"queue_size" example:"3"`
}

// SystemMetrics represents system metrics
type SystemMetrics struct {
	CPUUsage    float64 `json:"cpu_usage" example:"45.2"`
	MemoryUsage float64 `json:"memory_usage" example:"512.5"`
	Goroutines  int     `json:"goroutines" example:"125"`
}

// ValidationError represents validation error details
type ValidationError struct {
	Field   string `json:"field" example:"cnpj"`
	Message string `json:"message" example:"CNPJ must contain exactly 14 digits"`
	Value   string `json:"value" example:"123456789"`
}

// BatchRequest represents a batch CNPJ consultation request
type BatchRequest struct {
	CNPJs []string `json:"cnpjs" binding:"required,min=1,max=100" example:"[\"11222333000181\",\"11333444000172\"]"`
}

// BatchResponse represents a batch CNPJ consultation response
type BatchResponse struct {
	Results    []BatchResult `json:"results"`
	Total      int           `json:"total" example:"2"`
	Success    int           `json:"success" example:"2"`
	Errors     int           `json:"errors" example:"0"`
	DurationMs int64         `json:"duration_ms" example:"5200"`
	Timestamp  time.Time     `json:"timestamp" example:"2024-01-15T10:30:00Z"`
}

// BatchResult represents individual result in batch response
type BatchResult struct {
	CNPJ       string        `json:"cnpj" example:"11222333000181"`
	Success    bool          `json:"success" example:"true"`
	Data       *CNPJResponse `json:"data,omitempty"`
	Error      string        `json:"error,omitempty"`
	DurationMs int64         `json:"duration_ms" example:"2500"`
}

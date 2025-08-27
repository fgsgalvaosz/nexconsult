package models

import "time"

// StandardResponse representa a estrutura padrão unificada para todas as respostas da API
// @Description Estrutura padrão unificada de resposta para todos os endpoints
type StandardResponse struct {
	// Status da operação (success, error, warning, info)
	// @example "success"
	Status string `json:"status" example:"success"`

	// Mensagem descritiva da operação
	// @example "Consulta realizada com sucesso"
	Message string `json:"message" example:"Consulta realizada com sucesso"`

	// Dados retornados pela operação (apenas quando status = success)
	Data interface{} `json:"data,omitempty"`

	// Detalhes do erro (apenas quando status = error)
	Error *ErrorDetails `json:"error,omitempty"`

	// Metadados da resposta
	Meta *ResponseMeta `json:"meta"`
}

// ErrorDetails contém informações detalhadas sobre erros
type ErrorDetails struct {
	// Código do erro
	// @example "INVALID_CNPJ"
	Code string `json:"code" example:"INVALID_CNPJ"`

	// Mensagem do erro
	// @example "CNPJ inválido. Deve conter 14 dígitos"
	Message string `json:"message" example:"CNPJ inválido. Deve conter 14 dígitos"`

	// Detalhes adicionais do erro
	Details interface{} `json:"details,omitempty"`
}

// ResponseMeta contém metadados da resposta
type ResponseMeta struct {
	// Timestamp da resposta em formato ISO 8601
	// @example "2025-08-25T17:25:30.468715-03:00"
	Timestamp time.Time `json:"timestamp" example:"2025-08-25T17:25:30.468715-03:00"`

	// Tempo de execução da operação
	// @example "1.234s"
	ExecutionTime string `json:"execution_time,omitempty" example:"1.234s"`

	// ID da requisição para rastreamento
	// @example "req_123456789"
	RequestID string `json:"request_id,omitempty" example:"req_123456789"`

	// Versão da API
	// @example "v1"
	Version string `json:"version,omitempty" example:"v1"`
}

// Constantes para status padronizados
const (
	StatusSuccess = "success"
	StatusError   = "error"
	StatusWarning = "warning"
	StatusInfo    = "info"
)

// Constantes para códigos de erro padronizados
const (
	ErrorCodeInvalidCNPJ     = "INVALID_CNPJ"
	ErrorCodeCNPJNotFound    = "CNPJ_NOT_FOUND"
	ErrorCodeInternalError   = "INTERNAL_ERROR"
	ErrorCodeRateLimit       = "RATE_LIMIT_EXCEEDED"
	ErrorCodeInvalidRequest  = "INVALID_REQUEST"
	ErrorCodeCaptchaError    = "CAPTCHA_ERROR"
	ErrorCodeExtractionError = "EXTRACTION_ERROR"
)

// NewSuccessResponse cria uma resposta de sucesso padronizada
func NewSuccessResponse(message string, data interface{}) *StandardResponse {
	return &StandardResponse{
		Status:  StatusSuccess,
		Message: message,
		Data:    data,
		Meta: &ResponseMeta{
			Timestamp: time.Now(),
			Version:   "v1",
		},
	}
}

// NewErrorResponse cria uma resposta de erro padronizada
func NewErrorResponse(code, message string, details interface{}) *StandardResponse {
	return &StandardResponse{
		Status:  StatusError,
		Message: "Erro na operação",
		Error: &ErrorDetails{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: &ResponseMeta{
			Timestamp: time.Now(),
			Version:   "v1",
		},
	}
}

// NewWarningResponse cria uma resposta de aviso padronizada
func NewWarningResponse(message string, data interface{}) *StandardResponse {
	return &StandardResponse{
		Status:  StatusWarning,
		Message: message,
		Data:    data,
		Meta: &ResponseMeta{
			Timestamp: time.Now(),
			Version:   "v1",
		},
	}
}

// NewInfoResponse cria uma resposta informativa padronizada
func NewInfoResponse(message string, data interface{}) *StandardResponse {
	return &StandardResponse{
		Status:  StatusInfo,
		Message: message,
		Data:    data,
		Meta: &ResponseMeta{
			Timestamp: time.Now(),
			Version:   "v1",
		},
	}
}

// SetExecutionTime define o tempo de execução na resposta
func (r *StandardResponse) SetExecutionTime(duration time.Duration) {
	if r.Meta != nil {
		r.Meta.ExecutionTime = duration.String()
	}
}

// SetRequestID define o ID da requisição na resposta
func (r *StandardResponse) SetRequestID(requestID string) {
	if r.Meta != nil {
		r.Meta.RequestID = requestID
	}
}

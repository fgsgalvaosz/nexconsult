package models

import (
	"time"
)

// APIResponse representa a resposta padrão da API
// @Description Estrutura padrão de resposta para todos os endpoints
type APIResponse struct {
	// Indica se a operação foi bem-sucedida
	// @example true
	Success bool `json:"success" example:"true"`
	// Mensagem explicativa da operação (apenas em caso de sucesso)
	// @example "Consulta realizada com sucesso"
	Message string `json:"message,omitempty" example:"Consulta realizada com sucesso"`
	// Dados retornados pela operação
	Data interface{} `json:"data,omitempty"`
	// Mensagem de erro (apenas em caso de falha)
	// @example "CNPJ inválido"
	Error string `json:"error,omitempty" example:"CNPJ inválido"`
	// Timestamp da resposta em formato ISO 8601
	// @example "2025-08-25T17:25:30.468715-03:00"
	Timestamp time.Time `json:"timestamp" example:"2025-08-25T17:25:30.468715-03:00"`
}

// SintegraResponse representa a resposta específica do Sintegra
// @Description Dados estruturados retornados pela consulta no Sintegra MA
type SintegraResponse struct {
	// CNPJ consultado
	// @example "38139407000177"
	CNPJ string `json:"cnpj" example:"38139407000177"`
	// Status da consulta (sucesso, erro_captcha, erro_extracao)
	// @example "sucesso"
	Status string `json:"status" example:"sucesso"`
	// URL da página de resultado no Sintegra
	// @example "https://sistemas1.sefaz.ma.gov.br/sintegra/..."
	URL string `json:"url" example:"https://sistemas1.sefaz.ma.gov.br/sintegra/..."`
	// Dados estruturados da empresa
	Data *SintegraData `json:"data"`
	// Tempo de execução da consulta (formato: "45s" ou "2m30s")
	// @example "45s"
	ExecutionTime string `json:"execution_time" example:"45s"`
	// Timestamp da consulta
	// @example "2025-08-25T17:25:30.468715-03:00"
	Timestamp time.Time `json:"timestamp" example:"2025-08-25T17:25:30.468715-03:00"`
	// Indica se o CAPTCHA foi resolvido automaticamente
	// @example true
	CaptchaSolved bool `json:"captcha_solved" example:"true"`
}

// SintegraData representa os dados estruturados da consulta
type SintegraData struct {
	CGC                   string          `json:"cgc"`
	InscricaoEstadual     string          `json:"inscricao_estadual"`
	RazaoSocial           string          `json:"razao_social"`
	RegimeApuracao        string          `json:"regime_apuracao"`
	Endereco              *EnderecoData   `json:"endereco"`
	CNAEPrincipal         string          `json:"cnae_principal"`
	CNAESecundarios       []CNAEData      `json:"cnae_secundarios"`
	SituacaoCadastral     string          `json:"situacao_cadastral"`
	DataSituacaoCadastral string          `json:"data_situacao_cadastral"`
	Obrigacoes            *ObrigacoesData `json:"obrigacoes"`
	DataConsulta          string          `json:"data_consulta"`
	NumeroConsulta        string          `json:"numero_consulta"`
	Observacao            string          `json:"observacao"`
}

// EnderecoData representa os dados de endereço
type EnderecoData struct {
	Logradouro  string `json:"logradouro"`
	Numero      string `json:"numero"`
	Complemento string `json:"complemento"`
	Bairro      string `json:"bairro"`
	Municipio   string `json:"municipio"`
	UF          string `json:"uf"`
	CEP         string `json:"cep"`
	DDD         string `json:"ddd"`
	Telefone    string `json:"telefone"`
}

// CNAEData representa dados de CNAE
type CNAEData struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
}

// ObrigacoesData representa dados de obrigações fiscais
type ObrigacoesData struct {
	NFeAPartirDe string `json:"nfe_a_partir_de"`
	EDFAPartirDe string `json:"edf_a_partir_de"`
	CTEAPartirDe string `json:"cte_a_partir_de"`
}

// HealthResponse representa resposta do health check
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

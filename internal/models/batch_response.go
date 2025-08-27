package models

import "time"

// BatchSintegraResponse representa a resposta de uma consulta em lote
// @Description Dados estruturados retornados pela consulta em lote no Sintegra MA
type BatchSintegraResponse struct {
	// Número total de CNPJs processados
	// @example 3
	Total int `json:"total" example:"3"`

	// Número de consultas bem-sucedidas
	// @example 2
	SuccessCount int `json:"success_count" example:"2"`

	// Número de consultas com erro
	// @example 1
	ErrorCount int `json:"error_count" example:"1"`

	// Lista de CNPJs inválidos que não foram processados
	// @example ["123456"]
	InvalidCNPJs []string `json:"invalid_cnpjs,omitempty" example:"[\"123456\"]"`

	// Resultados das consultas bem-sucedidas
	// Mapa onde a chave é o CNPJ e o valor é o resultado da consulta
	Results map[string]*SintegraResponse `json:"results"`

	// Erros das consultas que falharam
	// Mapa onde a chave é o CNPJ e o valor é a mensagem de erro
	Errors map[string]string `json:"errors,omitempty"`

	// Tempo total de execução do lote (formato: "45s" ou "2m30s")
	// @example "1m15s"
	ExecutionTime string `json:"execution_time" example:"1m15s"`

	// Timestamp da consulta
	// @example "2025-08-25T17:25:30.468715-03:00"
	Timestamp time.Time `json:"timestamp" example:"2025-08-25T17:25:30.468715-03:00"`
}

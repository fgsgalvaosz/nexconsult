package models

import (
	"regexp"
	"time"
)

// StatusRequest representa uma requisição para verificar o status de uma consulta
type StatusRequest struct {
	// CNPJ da empresa para verificar status
	CNPJ string `json:"cnpj" validate:"required"`
}

// StatusResponse representa a resposta do status de uma consulta
type StatusResponse struct {
	// CNPJ consultado
	CNPJ string `json:"cnpj"`

	// Status da consulta (em_andamento, concluida, erro, nao_encontrada)
	Status string `json:"status"`

	// Tempo estimado para conclusão (em segundos), apenas quando status=em_andamento
	TempoEstimado int `json:"tempo_estimado,omitempty"`

	// Mensagem adicional sobre o status
	Mensagem string `json:"mensagem"`

	// Timestamp da verificação
	Timestamp time.Time `json:"timestamp"`
}

// CleanCNPJ remove caracteres não numéricos do CNPJ
func (r *StatusRequest) CleanCNPJ() {
	re := regexp.MustCompile(`[^0-9]`)
	r.CNPJ = re.ReplaceAllString(r.CNPJ, "")
}

// ValidateCNPJ verifica se o CNPJ é válido
func (r *StatusRequest) ValidateCNPJ() bool {
	return len(r.CNPJ) == 14 && ValidarCNPJ(r.CNPJ)
}

func ValidarCNPJ(s string) bool {
	panic("unimplemented")
}

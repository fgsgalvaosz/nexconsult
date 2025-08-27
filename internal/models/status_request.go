package models

// ConsultaStatusRequest representa uma requisição para verificar o status de uma consulta
// @Description Estrutura de dados para verificar o status de uma consulta por CNPJ
type ConsultaStatusRequest struct {
	// CNPJ da empresa para verificar status (apenas números, 14 dígitos)
	// @example "38139407000177"
	CNPJ string `json:"cnpj" validate:"required,len=14" example:"38139407000177"`
}

// ConsultaStatusResponse representa a resposta do status de uma consulta
// @Description Dados sobre o status atual de uma consulta por CNPJ
type ConsultaStatusResponse struct {
	// CNPJ consultado
	// @example "38139407000177"
	CNPJ string `json:"cnpj" example:"38139407000177"`

	// Status da consulta (em_andamento, concluida, erro, nao_encontrada)
	// @example "em_andamento"
	Status string `json:"status" example:"em_andamento"`

	// Tempo estimado para conclusão (em segundos), apenas quando status=em_andamento
	// @example 15
	TempoEstimado int `json:"tempo_estimado,omitempty" example:"15"`

	// Mensagem adicional sobre o status
	// @example "Consulta em processamento. Aguarde."
	Mensagem string `json:"mensagem" example:"Consulta em processamento. Aguarde."`
}

package dto

import "nexconsult/internal/service"

// ErrorResponse representa uma resposta de erro da API
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// SuccessResponse representa uma resposta de sucesso da API
type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Message string      `json:"message,omitempty"`
}

// ConsultaResponse representa a resposta de uma consulta SINTEGRA
type ConsultaResponse struct {
	CNPJ              string         `json:"cnpj"`
	InscricaoEstadual string         `json:"inscricao_estadual"`
	RazaoSocial       string         `json:"razao_social"`
	RegimeApuracao    string         `json:"regime_apuracao"`
	Logradouro        string         `json:"logradouro"`
	Numero            string         `json:"numero"`
	Complemento       string         `json:"complemento"`
	Bairro            string         `json:"bairro"`
	Municipio         string         `json:"municipio"`
	UF                string         `json:"uf"`
	CEP               string         `json:"cep"`
	DDD               string         `json:"ddd"`
	Telefone          string         `json:"telefone"`
	CNAEPrincipal     string         `json:"cnae_principal"`
	CNAEsSecundarios  []service.CNAE `json:"cnaes_secundarios"`
	SituacaoCadastral string         `json:"situacao_cadastral"`
	DataSituacao      string         `json:"data_situacao"`
	NFeAPartirDe      string         `json:"nfe_a_partir_de"`
	EDFAPartirDe      string         `json:"edf_a_partir_de"`
	CTEAPartirDe      string         `json:"cte_a_partir_de"`
	DataConsulta      string         `json:"data_consulta"`
	NumeroConsulta    string         `json:"numero_consulta"`
	Observacao        string         `json:"observacao"`
}

// ToConsultaResponse converte SintegraResult para ConsultaResponse
func ToConsultaResponse(result *service.SintegraResult) *ConsultaResponse {
	return &ConsultaResponse{
		CNPJ:              result.CNPJ,
		RazaoSocial:       result.RazaoSocial,
		SituacaoCadastral: result.Situacao,
		DataConsulta:      result.DataConsulta,
	}
}

// ToConsultaResponseFromData converte SintegraData para ConsultaResponse
func ToConsultaResponseFromData(data *service.SintegraData) *ConsultaResponse {
	return &ConsultaResponse{
		CNPJ:              data.CNPJ,
		InscricaoEstadual: data.InscricaoEstadual,
		RazaoSocial:       data.RazaoSocial,
		RegimeApuracao:    data.RegimeApuracao,
		Logradouro:        data.Logradouro,
		Numero:            data.Numero,
		Complemento:       data.Complemento,
		Bairro:            data.Bairro,
		Municipio:         data.Municipio,
		UF:                data.UF,
		CEP:               data.CEP,
		DDD:               data.DDD,
		Telefone:          data.Telefone,
		CNAEPrincipal:     data.CNAEPrincipal,
		CNAEsSecundarios:  data.CNAEsSecundarios,
		SituacaoCadastral: data.SituacaoCadastral,
		DataSituacao:      data.DataSituacao,
		NFeAPartirDe:      data.NFeAPartirDe,
		EDFAPartirDe:      data.EDFAPartirDe,
		CTEAPartirDe:      data.CTEAPartirDe,
		DataConsulta:      data.DataConsulta,
		NumeroConsulta:    data.NumeroConsulta,
		Observacao:        data.Observacao,
	}
}

package models

import "strings"

// SintegraRequest representa uma requisição para consulta no Sintegra MA
// @Description Estrutura de dados para requisição de consulta CNPJ
type SintegraRequest struct {
	// CNPJ da empresa para consulta (apenas números, 14 dígitos)
	// @example "38139407000177"
	CNPJ string `json:"cnpj" validate:"required,len=14" example:"38139407000177"`
}

// ValidateCNPJ valida se o CNPJ está no formato correto
func (r *SintegraRequest) ValidateCNPJ() bool {
	// Remove caracteres não numéricos
	cnpj := strings.ReplaceAll(r.CNPJ, ".", "")
	cnpj = strings.ReplaceAll(cnpj, "/", "")
	cnpj = strings.ReplaceAll(cnpj, "-", "")
	
	// Atualiza o CNPJ limpo
	r.CNPJ = cnpj
	
	if len(cnpj) != 14 {
		return false
	}
	
	// Verificar se todos são dígitos
	for _, char := range cnpj {
		if char < '0' || char > '9' {
			return false
		}
	}
	
	return true
}

// CleanCNPJ remove formatação do CNPJ
func (r *SintegraRequest) CleanCNPJ() {
	cnpj := strings.ReplaceAll(r.CNPJ, ".", "")
	cnpj = strings.ReplaceAll(cnpj, "/", "")
	cnpj = strings.ReplaceAll(cnpj, "-", "")
	r.CNPJ = cnpj
}
package models

import "strings"

// BatchSintegraRequest representa uma requisição para consulta em lote no Sintegra MA
// @Description Estrutura de dados para requisição de consulta em lote de CNPJs
type BatchSintegraRequest struct {
	// Lista de CNPJs para consulta (cada CNPJ deve ter 14 dígitos numéricos)
	// @example ["38139407000177","27394162000170"]
	CNPJs []string `json:"cnpjs" validate:"required,min=1,max=10" example:"[\"38139407000177\",\"27394162000170\"]"`
}

// ValidateAndCleanCNPJs valida e limpa todos os CNPJs da requisição
// Retorna uma lista de CNPJs válidos e uma lista de CNPJs inválidos
func (r *BatchSintegraRequest) ValidateAndCleanCNPJs() ([]string, []string) {
	validCNPJs := []string{}
	invalidCNPJs := []string{}

	for _, cnpj := range r.CNPJs {
		// Limpar CNPJ
		cnpj = strings.ReplaceAll(cnpj, ".", "")
		cnpj = strings.ReplaceAll(cnpj, "/", "")
		cnpj = strings.ReplaceAll(cnpj, "-", "")

		// Validar CNPJ
		if len(cnpj) != 14 {
			invalidCNPJs = append(invalidCNPJs, cnpj)
			continue
		}

		// Verificar se todos são dígitos
		isValid := true
		for _, char := range cnpj {
			if char < '0' || char > '9' {
				isValid = false
				break
			}
		}

		if isValid {
			validCNPJs = append(validCNPJs, cnpj)
		} else {
			invalidCNPJs = append(invalidCNPJs, cnpj)
		}
	}

	return validCNPJs, invalidCNPJs
}

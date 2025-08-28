package extractor

import (
	"regexp"
	"strings"

	"nexconsult/internal/logger"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
)

// SintegraData representa os dados estruturados do SINTEGRA
type SintegraData struct {
	CNPJ              string `json:"cnpj"`
	InscricaoEstadual string `json:"inscricao_estadual"`
	RazaoSocial       string `json:"razao_social"`
	RegimeApuracao    string `json:"regime_apuracao"`
	Logradouro        string `json:"logradouro"`
	Numero            string `json:"numero"`
	Complemento       string `json:"complemento"`
	Bairro            string `json:"bairro"`
	Municipio         string `json:"municipio"`
	UF                string `json:"uf"`
	CEP               string `json:"cep"`
	DDD               string `json:"ddd"`
	Telefone          string `json:"telefone"`
	CNAEPrincipal     string `json:"cnae_principal"`
	CNAEsSecundarios  []CNAE `json:"cnaes_secundarios"`
	SituacaoCadastral string `json:"situacao_cadastral"`
	DataSituacao      string `json:"data_situacao"`
	NFeAPartirDe      string `json:"nfe_a_partir_de"`
	EDFAPartirDe      string `json:"edf_a_partir_de"`
	CTEAPartirDe      string `json:"cte_a_partir_de"`
	DataConsulta      string `json:"data_consulta"`
	NumeroConsulta    string `json:"numero_consulta"`
	Observacao        string `json:"observacao"`
}

// CNAE representa um código CNAE
type CNAE struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
}

// HTMLExtractor extrai dados do HTML do SINTEGRA
type HTMLExtractor struct {
	textCleanRegex *regexp.Regexp
	logger         logger.Logger
}

// NewHTMLExtractor cria uma nova instância do extrator HTML
func NewHTMLExtractor() *HTMLExtractor {
	return &HTMLExtractor{
		textCleanRegex: regexp.MustCompile(`\s+`),
		logger:         logger.GetLogger().With(logger.String("component", "extractor")),
	}
}

// ExtractDataFromHTML extrai dados estruturados do HTML
func (e *HTMLExtractor) ExtractDataFromHTML(htmlContent string) (*SintegraData, error) {
	e.logger.Debug("Iniciando extração de dados do HTML")

	// Parse do HTML
	doc, err := e.parseHTMLFromString(htmlContent)
	if err != nil {
		e.logger.Error("Erro ao parsear HTML", logger.Error(err))
		return nil, err
	}

	data := &SintegraData{}

	// Extrair campos básicos
	data.CNPJ = e.cleanCNPJ(e.extractFieldValue(doc, "CGC"))
	data.InscricaoEstadual = e.cleanInscricaoEstadual(e.extractFieldValue(doc, "Inscrição Estadual"))
	data.RazaoSocial = e.cleanText(e.extractFieldValue(doc, "Razão Social"))
	data.RegimeApuracao = e.extractFieldValue(doc, "Regime Apuração")

	// Extrair endereço
	data.Logradouro = e.extractFieldValue(doc, "Logradouro")
	data.Numero = e.extractFieldValue(doc, "Número")
	data.Complemento = e.extractFieldValue(doc, "Complemento")
	data.Bairro = e.extractFieldValue(doc, "Bairro")
	data.Municipio = e.extractFieldValue(doc, "Município")
	data.UF = e.extractFieldValue(doc, "UF")
	data.CEP = e.extractFieldValue(doc, "CEP")

	// Extrair contato
	data.DDD = e.extractFieldValue(doc, "DDD")
	data.Telefone = e.extractFieldValue(doc, "Telefone")

	// Tentar extrair DDD do telefone se estiver vazio
	if data.DDD == "" && strings.Contains(data.Telefone, "(") && strings.Contains(data.Telefone, ")") {
		parts := strings.Split(data.Telefone, ")")
		if len(parts) >= 2 {
			data.DDD = strings.TrimSpace(strings.ReplaceAll(parts[0], "(", ""))
			data.Telefone = strings.TrimSpace(parts[1])
		}
	}

	// Extrair informações cadastrais
	data.CNAEPrincipal = e.extractFieldValue(doc, "CNAE Principal")
	data.SituacaoCadastral = e.extractFieldValue(doc, "Situação Cadastral Vigente")
	data.DataSituacao = e.extractFieldValue(doc, "Data desta Situação Cadastral")
	data.NFeAPartirDe = e.extractFieldValue(doc, "NFe a partir de")
	data.EDFAPartirDe = e.extractFieldValue(doc, "EDF a partir de")
	data.CTEAPartirDe = e.extractFieldValue(doc, "CTE a partir de")
	data.DataConsulta = e.extractFieldValue(doc, "Data da Consulta")
	data.NumeroConsulta = e.extractFieldValue(doc, "Número da Consulta")
	data.Observacao = e.extractObservacao(doc)

	// Extrair CNAEs secundários
	data.CNAEsSecundarios = e.extractCNAEsFromText(doc)

	e.logger.Debug("Extração de dados concluída",
		logger.String("cnpj", data.CNPJ),
		logger.String("razaoSocial", data.RazaoSocial),
		logger.Int("cnaesSecundarios", len(data.CNAEsSecundarios)))

	return data, nil
}

// extractFieldValue extrai valor de um campo específico
func (e *HTMLExtractor) extractFieldValue(doc *goquery.Document, fieldName string) string {
	var value string

	// Buscar em spans com classe texto_negrito
	doc.Find("span.texto_negrito").Each(func(i int, sel *goquery.Selection) {
		if strings.Contains(sel.Text(), fieldName) {
			// Procurar o próximo span.texto na mesma linha (td)
			parent := sel.Parent()
			nextTd := parent.Next()
			if nextTd.Length() > 0 {
				textSpan := nextTd.Find("span.texto")
				if textSpan.Length() > 0 {
					value = strings.TrimSpace(textSpan.Text())
				}
			}
		}
	})

	// Se não encontrou, buscar em spans com classe menu_lateral3
	if value == "" {
		doc.Find("span.menu_lateral3").Each(func(i int, sel *goquery.Selection) {
			if strings.Contains(sel.Text(), fieldName) {
				parent := sel.Parent()
				nextTd := parent.Next()
				if nextTd.Length() > 0 {
					textSpan := nextTd.Find("span.texto")
					if textSpan.Length() > 0 {
						value = strings.TrimSpace(textSpan.Text())
					}
				}
			}
		})
	}

	// Se não encontrou, buscar em spans com classe menu_lateral6
	if value == "" {
		doc.Find("span.menu_lateral6").Each(func(i int, sel *goquery.Selection) {
			if strings.Contains(sel.Text(), fieldName) {
				parent := sel.Parent()
				nextTd := parent.Next()
				if nextTd.Length() > 0 {
					textSpan := nextTd.Find("span.texto")
					if textSpan.Length() > 0 {
						value = strings.TrimSpace(textSpan.Text())
					}
				}
			}
		})
	}

	return value
}

// extractObservacao extrai observações do documento
func (e *HTMLExtractor) extractObservacao(doc *goquery.Document) string {
	var observacao string
	doc.Find("span.texto").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		if strings.HasPrefix(text, "Observação:") {
			observacao = strings.TrimSpace(text)
		}
	})
	return observacao
}

// extractCNAEsFromText extrai CNAEs secundários da tabela
func (e *HTMLExtractor) extractCNAEsFromText(doc *goquery.Document) []CNAE {
	var cnaes []CNAE

	// Buscar na tabela de CNAEs secundários
	doc.Find("table#j_id6\\:idlista tbody tr").Each(func(i int, row *goquery.Selection) {
		// Pular o cabeçalho se existir
		if row.HasClass("rich-table-header") || row.HasClass("rich-table-header-continue") {
			return
		}

		var codigo, descricao string

		// Extrair código (primeira coluna)
		row.Find("td").First().Find("span.textoPequeno").Each(func(j int, cell *goquery.Selection) {
			codigo = strings.TrimSpace(cell.Text())
		})

		// Extrair descrição (segunda coluna)
		row.Find("td").Last().Find("span.textoPequeno").Each(func(j int, cell *goquery.Selection) {
			descricao = strings.TrimSpace(cell.Text())
		})

		// Adicionar CNAE se ambos os campos foram encontrados
		if codigo != "" && descricao != "" {
			cnaes = append(cnaes, CNAE{
				Codigo:    codigo,
				Descricao: descricao,
			})
		}
	})

	return cnaes
}

// parseHTMLFromString faz parse do HTML a partir de string
func (e *HTMLExtractor) parseHTMLFromString(htmlContent string) (*goquery.Document, error) {
	reader := strings.NewReader(htmlContent)
	utf8Reader, err := charset.NewReader(reader, "")
	if err != nil {
		// Se falhar, tentar sem conversão de charset
		reader = strings.NewReader(htmlContent)
		return goquery.NewDocumentFromReader(reader)
	}

	return goquery.NewDocumentFromReader(utf8Reader)
}

// cleanText limpa texto removendo espaços extras
func (e *HTMLExtractor) cleanText(text string) string {
	return strings.TrimSpace(e.textCleanRegex.ReplaceAllString(text, " "))
}

// cleanCNPJ limpa formatação do CNPJ
func (e *HTMLExtractor) cleanCNPJ(cnpj string) string {
	cnpj = strings.ReplaceAll(cnpj, ".", "")
	cnpj = strings.ReplaceAll(cnpj, "/", "")
	cnpj = strings.ReplaceAll(cnpj, "-", "")
	return strings.TrimSpace(cnpj)
}

// cleanInscricaoEstadual limpa formatação da inscrição estadual
func (e *HTMLExtractor) cleanInscricaoEstadual(ie string) string {
	ie = strings.ReplaceAll(ie, ".", "")
	ie = strings.ReplaceAll(ie, "-", "")
	return strings.TrimSpace(ie)
}

// ExtractErrorMessage extrai mensagem de erro do HTML da página
func (e *HTMLExtractor) ExtractErrorMessage(htmlContent string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		e.logger.Warn("Erro ao parsear HTML para extração de erro", logger.Error(err))
		return ""
	}

	// Procurar por mensagens de erro nos seletores conhecidos
	errorSelectors := []string{
		"#form1\\:msgs > div",
		".pf-messages-warn",
		".pf-messages-error",
		".pf-messages-warn-detail",
		".pf-messages-error-detail",
	}

	for _, selector := range errorSelectors {
		errorElement := doc.Find(selector).First()
		if errorElement.Length() > 0 {
			// Retornar o texto exato da mensagem de erro, sem modificações
			errorText := strings.TrimSpace(errorElement.Text())
			if errorText != "" {
				e.logger.Debug("Mensagem de erro encontrada",
					logger.String("selector", selector),
					logger.String("message", errorText))
				return errorText
			}
		}
	}

	e.logger.Debug("Nenhuma mensagem de erro encontrada")
	return ""
}

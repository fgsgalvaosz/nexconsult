package service

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog"
)

const (
	// Seletores CSS para extração de dados
	selectorTableCell = "td"
	selectorTableRow  = "tr"

	// Padrões de texto para identificação de campos
	patternCGC               = "CGC"
	patternInscricaoEstadual = "Inscrição Estadual"
	patternRazaoSocial       = "Razão Social"
	patternRegimeApuracao    = "Regime de Apuração"
	patternLogradouro        = "Logradouro"
	patternNumero            = "Número"
	patternComplemento       = "Complemento"
	patternBairro            = "Bairro"
	patternMunicipio         = "Município"
	patternUF                = "UF"
	patternCEP               = "CEP"
	patternDDD               = "DDD"
	patternTelefone          = "Telefone"
	patternCNAEPrincipal     = "CNAE Principal"
	patternSituacaoCadastral = "Situação Cadastral"
	patternDataSituacao      = "Data da Situação Cadastral"
	patternNFeAPartirDe      = "NFe a partir de"
	patternEDFAPartirDe      = "EDF a partir de"
	patternCTEAPartirDe      = "CTe a partir de"
	patternDataConsulta      = "Data da Consulta"
	patternNumeroConsulta    = "Número da Consulta"
	patternObservacao        = "Observação"

	// Tamanhos máximos para validação
	maxFileSize = 10 * 1024 * 1024 // 10MB
)

// SintegraExtractor extrai dados estruturados do HTML do Sintegra
type SintegraExtractor struct {
	logger zerolog.Logger
}

// NewSintegraExtractor cria uma nova instância do extrator
func NewSintegraExtractor(logger zerolog.Logger) *SintegraExtractor {
	if logger.GetLevel() == zerolog.Disabled {
		logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}
	return &SintegraExtractor{
		logger: logger,
	}
}

// ExtractFromFile lê um arquivo HTML e extrai os dados estruturados
func (e *SintegraExtractor) ExtractFromFile(filePath string) (*SintegraData, error) {
	e.logger.Info().Str("file_path", filePath).Msg("Extraindo dados do arquivo HTML")

	if err := e.validateFilePath(filePath); err != nil {
		return nil, err
	}

	content, err := e.readHTMLFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler arquivo: %w", err)
	}

	return e.ExtractFromHTML(string(content))
}

// validateFilePath valida o caminho do arquivo
func (e *SintegraExtractor) validateFilePath(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("caminho do arquivo não pode ser vazio")
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("arquivo não encontrado: %s", filePath)
		}
		return fmt.Errorf("erro ao acessar arquivo: %w", err)
	}

	if info.Size() == 0 {
		return fmt.Errorf("arquivo está vazio: %s", filePath)
	}

	if info.Size() > maxFileSize {
		return fmt.Errorf("arquivo muito grande (%d bytes), máximo permitido: %d bytes", info.Size(), maxFileSize)
	}

	return nil
}

// readHTMLFile lê o conteúdo do arquivo HTML
func (e *SintegraExtractor) readHTMLFile(filePath string) ([]byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		e.logger.Error().Err(err).Str("file_path", filePath).Msg("Erro ao ler arquivo")
		return nil, err
	}

	e.logger.Debug().Int("size_bytes", len(content)).Msg("Arquivo lido com sucesso")
	return content, nil
}

// ExtractFromHTML recebe o HTML completo da página de detalhes e retorna a
// estrutura SintegraData preenchida.
func (e *SintegraExtractor) ExtractFromHTML(html string) (*SintegraData, error) {
	e.logger.Info().Msg("Iniciando extração de dados do HTML")

	if err := e.validateHTML(html); err != nil {
		return nil, err
	}

	doc, err := e.parseHTML(html)
	if err != nil {
		return nil, fmt.Errorf("erro ao parsear HTML: %w", err)
	}

	data := e.initializeSintegraData()

	// Executar extrações em ordem lógica
	extractionFuncs := []func(*goquery.Document, *SintegraData){
		e.extractIdentificacao,
		e.extractEndereco,
		e.extractCNAE,
		e.extractSituacao,
		e.extractObrigacoes,
		e.extractMetadados,
	}

	for _, extractFunc := range extractionFuncs {
		extractFunc(doc, data)
	}

	e.logger.Info().
		Str("cgc", data.CGC).
		Str("razao_social", data.RazaoSocial).
		Msg("Extração concluída")

	return data, nil
}

// validateHTML valida o conteúdo HTML
func (e *SintegraExtractor) validateHTML(html string) error {
	if html == "" {
		return fmt.Errorf("HTML não pode ser vazio")
	}

	if len(html) < 100 {
		return fmt.Errorf("HTML muito pequeno, possivelmente inválido")
	}

	return nil
}

// initializeSintegraData inicializa a estrutura de dados
func (e *SintegraExtractor) initializeSintegraData() *SintegraData {
	return &SintegraData{
		Endereco:        &EnderecoData{},
		CNAESecundarios: make([]CNAEData, 0),
		Obrigacoes:      &ObrigacoesData{},
	}
}

// parseHTML converte string HTML em documento goquery
func (e *SintegraExtractor) parseHTML(html string) (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		e.logger.Error().Err(err).Msg("Erro ao parsear HTML")
		return nil, err
	}
	return doc, nil
}

// --- Implementações de extração com goquery ---

func (e *SintegraExtractor) extractIdentificacao(doc *goquery.Document, data *SintegraData) {
	e.logger.Debug().Msg("Extraindo dados de identificação")

	// Mapeamento de padrões para campos
	identificationFields := map[string]*string{
		patternCGC:               &data.CGC,
		patternInscricaoEstadual: &data.InscricaoEstadual,
		patternRazaoSocial:       &data.RazaoSocial,
		patternRegimeApuracao:    &data.RegimeApuracao,
	}

	e.extractFieldsFromTable(doc, identificationFields)

	// Manter extração específica do CGC por regex
	doc.Find("td").Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if text == "" {
			return
		}
		e.extractCGC(text, data)
	})
}

// extractCGC extrai o CGC/CNPJ do texto
func (e *SintegraExtractor) extractCGC(text string, data *SintegraData) {
	if matches := regexp.MustCompile(`(\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2})`).FindStringSubmatch(text); len(matches) > 1 {
		data.CGC = strings.TrimSpace(matches[1])
		e.logger.Debug().Str("cgc", data.CGC).Msg("CGC extraído")
	}
}

// parseCNAE converte texto de CNAE em estrutura CNAEData
func (e *SintegraExtractor) extractEndereco(doc *goquery.Document, data *SintegraData) {
	e.logger.Debug().Msg("Extraindo dados de endereço")

	// Mapeamento de padrões para campos de endereço
	addressFields := map[string]*string{
		patternLogradouro:  &data.Endereco.Logradouro,
		patternNumero:      &data.Endereco.Numero,
		patternComplemento: &data.Endereco.Complemento,
		patternBairro:      &data.Endereco.Bairro,
		patternMunicipio:   &data.Endereco.Municipio,
		patternUF:          &data.Endereco.UF,
		patternCEP:         &data.Endereco.CEP,
		patternDDD:         &data.Endereco.DDD,
		patternTelefone:    &data.Endereco.Telefone,
	}

	e.extractFieldsFromTable(doc, addressFields)
}

// getNextCellValue obtém o valor da próxima célula
func (e *SintegraExtractor) extractCNAE(doc *goquery.Document, data *SintegraData) {
	e.logger.Debug().Msg("Extraindo dados de CNAE")

	// Principal
	doc.Find(selectorTableCell).Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if strings.Contains(text, patternCNAEPrincipal) {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				cnae := strings.TrimSpace(nextTd.Text())
				if cnae != "" && !strings.Contains(cnae, "CNAE") {
					data.CNAEPrincipal = cnae
					e.logger.Debug().Str("cnae_principal", cnae).Msg("CNAE Principal extraído")
				}
			}
		}
	})

	// Secundários
	data.CNAESecundarios = []CNAEData{}
	doc.Find(selectorTableRow).Each(func(i int, row *goquery.Selection) {
		cells := row.Find(selectorTableCell)
		if cells.Length() >= 2 {
			firstCell := strings.TrimSpace(cells.First().Text())
			secondCell := strings.TrimSpace(cells.Eq(1).Text())
			if matches := regexp.MustCompile(`^(\d{7})$`).FindStringSubmatch(firstCell); len(matches) > 1 {
				cnae := CNAEData{Codigo: matches[1], Descricao: secondCell}
				data.CNAESecundarios = append(data.CNAESecundarios, cnae)
				e.logger.Debug().Str("cnae_secundario", cnae.Codigo).Msg("CNAE Secundário extraído")
			}
		}
	})
}

func (e *SintegraExtractor) extractSituacao(doc *goquery.Document, data *SintegraData) {
	e.logger.Debug().Msg("Extraindo dados de situação")

	// Mapeamento de padrões para campos de situação
	situationFields := map[string]*string{
		patternSituacaoCadastral: &data.SituacaoCadastral,
		patternDataSituacao:      &data.DataSituacaoCadastral,
	}

	e.extractFieldsFromTable(doc, situationFields)

	// Manter lógica específica para situação cadastral
	doc.Find(selectorTableCell).Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if strings.Contains(text, "Situação Cadastral Vigente") || strings.Contains(text, "Situa") && strings.Contains(text, "Cadastral") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				sit := strings.TrimSpace(nextTd.Text())
				if sit != "" && !strings.Contains(sit, "Situa") {
					data.SituacaoCadastral = sit
					e.logger.Debug().Str("situacao_cadastral", sit).Msg("Situação Cadastral extraída")
				}
			}
		}
		if strings.Contains(text, "Data desta Situação") || strings.Contains(text, "Data desta Situa") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				v := strings.TrimSpace(nextTd.Text())
				if v != "" && !strings.Contains(v, "Data") {
					data.DataSituacaoCadastral = v
					e.logger.Debug().Str("data_situacao_cadastral", v).Msg("Data da Situação Cadastral extraída")
				}
			}
		}
	})
}

func (e *SintegraExtractor) extractObrigacoes(doc *goquery.Document, data *SintegraData) {
	e.logger.Debug().Msg("Extraindo dados de obrigações")

	if data.Obrigacoes == nil {
		data.Obrigacoes = &ObrigacoesData{}
	}

	// Mapeamento de padrões para campos de obrigações
	obligationFields := map[string]*string{
		patternNFeAPartirDe: &data.Obrigacoes.NFeAPartirDe,
		patternEDFAPartirDe: &data.Obrigacoes.EDFAPartirDe,
		patternCTEAPartirDe: &data.Obrigacoes.CTEAPartirDe,
	}

	e.extractFieldsFromTable(doc, obligationFields)
}

func (e *SintegraExtractor) extractMetadados(doc *goquery.Document, data *SintegraData) {
	e.logger.Debug().Msg("Extraindo metadados")

	// Mapeamento de padrões para metadados
	metadataFields := map[string]*string{
		patternDataConsulta:   &data.DataConsulta,
		patternNumeroConsulta: &data.NumeroConsulta,
		patternObservacao:     &data.Observacao,
	}

	e.extractFieldsFromTable(doc, metadataFields)

	// Manter lógica específica para campos com variações
	doc.Find(selectorTableCell).Each(func(i int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if strings.Contains(text, "Número da Consulta") || strings.Contains(text, "mero da Consulta") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				v := strings.TrimSpace(nextTd.Text())
				if v != "" && !strings.Contains(v, "mero") {
					data.NumeroConsulta = v
					e.logger.Debug().Str("numero_consulta", v).Msg("Número da Consulta extraído")
				}
			}
		}
		if strings.Contains(text, "Observação") || strings.Contains(text, "Observa") {
			nextTd := sel.Next()
			if nextTd.Length() > 0 {
				v := strings.TrimSpace(nextTd.Text())
				if v != "" && !strings.Contains(v, "Observa") {
					data.Observacao = v
					e.logger.Debug().Str("observacao", v).Msg("Observação extraída")
				}
			}
		}
	})
}

// extractFieldsFromTable extrai campos de uma tabela usando mapeamento
func (e *SintegraExtractor) extractFieldsFromTable(doc *goquery.Document, fieldMap map[string]*string) {
	doc.Find(selectorTableCell).Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())

		for pattern, fieldPtr := range fieldMap {
			if strings.Contains(text, pattern) {
				if nextTd := s.Next(); nextTd.Length() > 0 {
					value := strings.TrimSpace(nextTd.Text())
					if value != "" {
						*fieldPtr = value
						e.logger.Debug().
							Str("pattern", pattern).
							Str("value", value).
							Msg("Campo extraído")
					}
				}
				break
			}
		}
	})
}

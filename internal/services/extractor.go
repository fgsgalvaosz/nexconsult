package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/nexconsult/cnpj-api/internal/models"
	"github.com/sirupsen/logrus"
)

// ExtractorService handles data extraction from HTML
type ExtractorService struct {
	logger *logrus.Logger
}

// NewExtractorService creates a new extractor service
func NewExtractorService(logger *logrus.Logger) *ExtractorService {
	return &ExtractorService{
		logger: logger,
	}
}

// ExtractCNPJData extracts CNPJ data from HTML content (based on Node.js extractorService)
func (e *ExtractorService) ExtractCNPJData(html, cnpj string) (*models.CNPJResponse, error) {
	e.logger.Debug("Starting CNPJ data extraction")

	// Parse HTML document
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Check if we're on the result page
	if !e.isResultPage(doc) {
		return nil, fmt.Errorf("not on result page - extraction failed")
	}

	response := &models.CNPJResponse{
		CNPJ: e.formatCNPJ(cnpj),
	}

	// Extract basic company information
	if err := e.extractBasicInfo(doc, response); err != nil {
		e.logger.WithError(err).Warn("Failed to extract basic info")
	}

	// Extract address information
	if err := e.extractAddress(doc, response); err != nil {
		e.logger.WithError(err).Warn("Failed to extract address")
	}

	// Extract business information
	if err := e.extractBusinessInfo(doc, response); err != nil {
		e.logger.WithError(err).Warn("Failed to extract business info")
	}

	// Extract contact information
	if err := e.extractContact(doc, response); err != nil {
		e.logger.WithError(err).Warn("Failed to extract contact info")
	}

	// Extract registration status
	if err := e.extractRegistrationStatus(doc, response); err != nil {
		e.logger.WithError(err).Warn("Failed to extract registration status")
	}

	// Extract CNAE information
	if err := e.extractCNAE(doc, response); err != nil {
		e.logger.WithError(err).Warn("Failed to extract CNAE info")
	}

	e.logger.WithFields(logrus.Fields{
		"cnpj":         response.CNPJ,
		"razao_social": response.RazaoSocial,
		"situacao":     response.Situacao,
	}).Info("CNPJ data extraction completed")

	return response, nil
}

// isResultPage checks if we're on the result page
func (e *ExtractorService) isResultPage(doc *goquery.Document) bool {
	// Check for specific elements that indicate we're on the result page
	return doc.Find("font").Length() > 0 || 
		   doc.Find("table").Length() > 0 ||
		   strings.Contains(doc.Text(), "NÚMERO DE INSCRIÇÃO")
}

// extractBasicInfo extracts basic company information
func (e *ExtractorService) extractBasicInfo(doc *goquery.Document, response *models.CNPJResponse) error {
	// Extract using font elements (like Node.js version)
	fontElements := doc.Find("font")
	
	fontElements.Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		
		// Extract Razão Social
		if strings.Contains(text, "RAZÃO SOCIAL") || strings.Contains(text, "NOME EMPRESARIAL") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.RazaoSocial = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Nome Fantasia
		if strings.Contains(text, "NOME FANTASIA") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				fantasia := strings.TrimSpace(nextFont.Text())
				if fantasia != "***" && fantasia != "" {
					response.NomeFantasia = fantasia
				}
			}
		}
		
		// Extract Natureza Jurídica
		if strings.Contains(text, "NATUREZA JURÍDICA") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.NaturezaJuridica = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Capital Social
		if strings.Contains(text, "CAPITAL SOCIAL") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.CapitalSocial = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Porte
		if strings.Contains(text, "PORTE") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.Porte = strings.TrimSpace(nextFont.Text())
			}
		}
	})
	
	return nil
}

// extractAddress extracts address information
func (e *ExtractorService) extractAddress(doc *goquery.Document, response *models.CNPJResponse) error {
	fontElements := doc.Find("font")
	
	fontElements.Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		
		// Extract Logradouro
		if strings.Contains(text, "LOGRADOURO") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.Endereco.Logradouro = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Número
		if strings.Contains(text, "NÚMERO") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.Endereco.Numero = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Complemento
		if strings.Contains(text, "COMPLEMENTO") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				complemento := strings.TrimSpace(nextFont.Text())
				if complemento != "***" && complemento != "" {
					response.Endereco.Complemento = complemento
				}
			}
		}
		
		// Extract CEP
		if strings.Contains(text, "CEP") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.Endereco.CEP = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Bairro
		if strings.Contains(text, "BAIRRO") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.Endereco.Bairro = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Município
		if strings.Contains(text, "MUNICÍPIO") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.Endereco.Municipio = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract UF
		if strings.Contains(text, "UF") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.Endereco.UF = strings.TrimSpace(nextFont.Text())
			}
		}
	})
	
	return nil
}

// extractBusinessInfo extracts business information
func (e *ExtractorService) extractBusinessInfo(doc *goquery.Document, response *models.CNPJResponse) error {
	fontElements := doc.Find("font")
	
	fontElements.Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		
		// Extract Data de Início de Atividade
		if strings.Contains(text, "DATA DE INÍCIO DE ATIVIDADE") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.DataInicioAtividade = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Tipo (MATRIZ/FILIAL)
		if strings.Contains(text, "MATRIZ") || strings.Contains(text, "FILIAL") {
			response.TipoEmpresa = strings.TrimSpace(text)
		}
	})
	
	return nil
}

// extractContact extracts contact information
func (e *ExtractorService) extractContact(doc *goquery.Document, response *models.CNPJResponse) error {
	fontElements := doc.Find("font")
	
	fontElements.Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		
		// Extract Telefone
		if strings.Contains(text, "TELEFONE") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				telefone := strings.TrimSpace(nextFont.Text())
				if telefone != "***" && telefone != "" {
					response.Telefone = telefone
				}
			}
		}
		
		// Extract Email
		if strings.Contains(text, "ENDEREÇO ELETRÔNICO") || strings.Contains(text, "EMAIL") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				email := strings.TrimSpace(nextFont.Text())
				if email != "***" && email != "" {
					response.Email = email
				}
			}
		}
	})
	
	return nil
}

// extractRegistrationStatus extracts registration status information
func (e *ExtractorService) extractRegistrationStatus(doc *goquery.Document, response *models.CNPJResponse) error {
	fontElements := doc.Find("font")
	
	fontElements.Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		
		// Extract Situação Cadastral
		if strings.Contains(text, "SITUAÇÃO CADASTRAL") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.Situacao = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Data da Situação Cadastral
		if strings.Contains(text, "DATA DA SITUAÇÃO CADASTRAL") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.DataSituacao = strings.TrimSpace(nextFont.Text())
			}
		}
		
		// Extract Motivo da Situação Cadastral
		if strings.Contains(text, "MOTIVO DE SITUAÇÃO CADASTRAL") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				response.MotivoSituacao = strings.TrimSpace(nextFont.Text())
			}
		}
	})
	
	return nil
}

// extractCNAE extracts CNAE information
func (e *ExtractorService) extractCNAE(doc *goquery.Document, response *models.CNPJResponse) error {
	fontElements := doc.Find("font")
	
	fontElements.Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		
		// Extract CNAE Principal
		if strings.Contains(text, "CÓDIGO E DESCRIÇÃO DA ATIVIDADE ECONÔMICA PRINCIPAL") {
			if nextFont := s.Next(); nextFont.Length() > 0 {
				cnaeText := strings.TrimSpace(nextFont.Text())
				if codigo, descricao := e.parseCNAE(cnaeText); codigo != "" {
					response.CNAEPrincipal = models.CNAEInfo{
						Codigo:    codigo,
						Descricao: descricao,
					}
				}
			}
		}
	})
	
	return nil
}

// parseCNAE parses CNAE code and description
func (e *ExtractorService) parseCNAE(cnaeText string) (string, string) {
	// Pattern: "12.34-5/67 - Description"
	re := regexp.MustCompile(`^(\d{2}\.\d{2}-\d/\d{2})\s*-\s*(.+)$`)
	matches := re.FindStringSubmatch(cnaeText)
	
	if len(matches) == 3 {
		return matches[1], strings.TrimSpace(matches[2])
	}
	
	return "", cnaeText
}

// formatCNPJ formats CNPJ with dots and slashes
func (e *ExtractorService) formatCNPJ(cnpj string) string {
	// Remove all non-digits
	digits := regexp.MustCompile(`\D`).ReplaceAllString(cnpj, "")
	
	if len(digits) != 14 {
		return cnpj
	}
	
	// Format as XX.XXX.XXX/XXXX-XX
	return fmt.Sprintf("%s.%s.%s/%s-%s",
		digits[0:2],
		digits[2:5],
		digits[5:8],
		digits[8:12],
		digits[12:14],
	)
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"nexconsult/internal/config"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
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

type CNAE struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
}

type SolveCaptchaResponse struct {
	ErrorId          int    `json:"errorId"`
	TaskId           string `json:"taskId"`
	Status           int    `json:"status"`
	ID               string `json:"request"`
	Error            string `json:"error_text,omitempty"`
	ErrorCode        string `json:"errorCode,omitempty"`
	ErrorDescription string `json:"errorDescription,omitempty"`
}

type CaptchaResult struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
}

type SintegraResult struct {
	CNPJ         string
	RazaoSocial  string
	Situacao     string
	DataConsulta string
	Error        error
}

type SintegraService struct {
	config            *config.Config
	lastExtractedData *SintegraData
}

func NewSintegraService(cfg *config.Config) *SintegraService {
	return &SintegraService{
		config: cfg,
	}
}

func (s *SintegraService) ScrapeCNPJ(cnpj string) (*SintegraResult, error) {
	result := &SintegraResult{
		CNPJ:         cnpj,
		DataConsulta: time.Now().Format("2006-01-02 15:04:05"),
	}

	if s.config.DebugMode {
		log.Printf("[DEBUG] Iniciando scraping para CNPJ: %s", cnpj)
	}

	// Configurar opções do Chrome
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", s.config.Headless),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.WindowSize(1920, 1080),
	)

	// Criar contexto com configurações personalizadas
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, time.Duration(s.config.Timeout)*time.Second)
	defer cancel()

	var htmlContent string

	if s.config.DebugMode {
		log.Printf("[DEBUG] Navegando para %s", s.config.SintegraURL)
	}

	// Seguir exatamente o fluxo do teste Playwright
	err := chromedp.Run(ctx,
		// Navegar para a página
		chromedp.Navigate(s.config.SintegraURL),
		// Aguardar carregamento
		chromedp.Sleep(2*time.Second),
		// Clicar no radio button CPF/CNPJ
		chromedp.Click(`td:nth-of-type(2) > label`, chromedp.ByQuery),
		// Clicar no campo CNPJ
		chromedp.Click(`#form1\:cpfCnpj`, chromedp.ByQuery),
		// Digitar o CNPJ
		chromedp.SendKeys(`#form1\:cpfCnpj`, cnpj, chromedp.ByQuery),
		// Aguardar um pouco
		chromedp.Sleep(1*time.Second),
	)

	if err != nil {
		return nil, fmt.Errorf("erro durante preenchimento inicial: %v", err)
	}

	if s.config.DebugMode {
		log.Printf("[DEBUG] CNPJ preenchido, aguardando CAPTCHA...")
	}

	// Aguardar e resolver CAPTCHA
	err = chromedp.Run(ctx,
		// Aguardar qualquer elemento de CAPTCHA aparecer
		chromedp.Sleep(3*time.Second), // Aguardar carregamento
		chromedp.ActionFunc(func(ctx context.Context) error {
			if s.config.DebugMode {
				log.Printf("[DEBUG] Procurando por elementos de CAPTCHA...")
			}
			// Tentar diferentes seletores de CAPTCHA
			var found bool
			// Verificar se existe reCAPTCHA
			err := chromedp.Run(ctx, chromedp.WaitVisible(`iframe[src*="recaptcha"]`, chromedp.ByQuery))
			if err == nil {
				found = true
				if s.config.DebugMode {
					log.Printf("[DEBUG] reCAPTCHA iframe encontrado")
				}
			} else {
				// Tentar outros seletores
				err = chromedp.Run(ctx, chromedp.WaitVisible(`[data-sitekey]`, chromedp.ByQuery))
				if err == nil {
					found = true
					if s.config.DebugMode {
						log.Printf("[DEBUG] Elemento com data-sitekey encontrado")
					}
				}
			}
			if !found {
				return fmt.Errorf("nenhum elemento de CAPTCHA encontrado")
			}
			return nil
		}),
		// Resolver CAPTCHA
		chromedp.ActionFunc(func(ctx context.Context) error {
			if s.config.DebugMode {
				log.Printf("[DEBUG] CAPTCHA encontrado, iniciando resolução...")
			}
			return s.solveCaptcha(ctx)
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("erro durante resolução do CAPTCHA: %v", err)
	}

	if s.config.DebugMode {
		log.Printf("[DEBUG] CAPTCHA resolvido, clicando no botão consultar...")
	}

	// Clicar no botão consultar e aguardar resultado
	err = chromedp.Run(ctx,
		// Clicar no botão consultar
		chromedp.Click(`#form1\:pnlPrincipal4 input:nth-of-type(2)`, chromedp.ByQuery),
		// Aguardar navegação para página de lista
		chromedp.Sleep(3*time.Second),
		// Aguardar página de lista carregar
		chromedp.WaitVisible(`#j_id6\:pnlCadastro`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if s.config.DebugMode {
				log.Printf("[DEBUG] Página de lista carregada, clicando no ícone de consulta...")
			}
			return nil
		}),
		// Clicar no ícone de consulta para ir para página de detalhes
		chromedp.Click(`#j_id6\:pnlCadastro img`, chromedp.ByQuery),
		// Aguardar navegação para página de detalhes
		chromedp.Sleep(3*time.Second),
		// Aguardar página de detalhes carregar
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if s.config.DebugMode {
				log.Printf("[DEBUG] Página de detalhes carregada, extraindo HTML...")
			}
			return nil
		}),
		chromedp.OuterHTML(`html`, &htmlContent, chromedp.ByQuery),
	)

	if err != nil {
		return nil, fmt.Errorf("erro no scraping: %v", err)
	}

	// HTML salvo apenas em modo debug (desabilitado para produção)
	// if s.config.DebugMode {
	//     timestamp := time.Now().Format("20060102_150405")
	//     filename := fmt.Sprintf("sintegra_result_%s_%s.html", cnpj, timestamp)
	//     if err := os.WriteFile(filename, []byte(htmlContent), 0644); err != nil {
	//         log.Printf("[DEBUG] Erro ao salvar HTML: %v", err)
	//     } else {
	//         log.Printf("[DEBUG] HTML salvo em: %s", filename)
	//     }
	// }

	data, err := s.ExtractDataFromHTML(htmlContent)
	if err != nil {
		return nil, fmt.Errorf("erro na extração: %v", err)
	}

	// Armazenar dados completos para acesso posterior
	s.lastExtractedData = data

	result.RazaoSocial = data.RazaoSocial
	result.Situacao = data.SituacaoCadastral

	return result, nil
}

// GetLastExtractedData retorna os últimos dados extraídos
func (s *SintegraService) GetLastExtractedData() *SintegraData {
	return s.lastExtractedData
}

func (s *SintegraService) ExtractDataFromHTML(htmlContent string) (*SintegraData, error) {
	f, err := os.CreateTemp("", "temp_*.html")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	_, err = f.WriteString(htmlContent)
	if err != nil {
		return nil, err
	}
	f.Close()

	doc, err := s.loadDocumentFromFile(f.Name())
	if err != nil {
		return nil, fmt.Errorf("erro ao parsear HTML: %v", err)
	}

	data := &SintegraData{}

	data.CNPJ = strings.ReplaceAll(strings.ReplaceAll(s.extractFieldValue(doc, "CGC"), ".", ""), "/", "")
	data.CNPJ = strings.ReplaceAll(data.CNPJ, "-", "")
	data.InscricaoEstadual = strings.ReplaceAll(strings.ReplaceAll(s.extractFieldValue(doc, "Inscrição Estadual"), ".", ""), "-", "")
	data.RazaoSocial = s.cleanText(s.extractFieldValue(doc, "Razão Social"))
	data.RegimeApuracao = s.extractFieldValue(doc, "Regime Apuração")
	data.Logradouro = s.extractFieldValue(doc, "Logradouro")
	data.Numero = s.extractFieldValue(doc, "Número")
	data.Complemento = s.extractFieldValue(doc, "Complemento")
	data.Bairro = s.extractFieldValue(doc, "Bairro")
	data.Municipio = s.extractFieldValue(doc, "Município")
	data.UF = s.extractFieldValue(doc, "UF")
	data.CEP = s.extractFieldValue(doc, "CEP")

	data.DDD = s.extractFieldValue(doc, "DDD")
	data.Telefone = s.extractFieldValue(doc, "Telefone")

	// Tentar extrair DDD do telefone se estiver vazio e telefone tiver formato (XX)XXXXXXXX
	if data.DDD == "" && strings.Contains(data.Telefone, "(") && strings.Contains(data.Telefone, ")") {
		parts := strings.Split(data.Telefone, ")")
		if len(parts) >= 2 {
			data.DDD = strings.TrimSpace(strings.ReplaceAll(parts[0], "(", ""))
			data.Telefone = strings.TrimSpace(parts[1])
		}
	}

	data.CNAEPrincipal = s.extractFieldValue(doc, "CNAE Principal")
	data.SituacaoCadastral = s.extractFieldValue(doc, "Situação Cadastral Vigente")
	data.DataSituacao = s.extractFieldValue(doc, "Data desta Situação Cadastral")
	data.NFeAPartirDe = s.extractFieldValue(doc, "NFe a partir de")
	data.EDFAPartirDe = s.extractFieldValue(doc, "EDF a partir de")
	data.CTEAPartirDe = s.extractFieldValue(doc, "CTE a partir de")
	data.DataConsulta = s.extractFieldValue(doc, "Data da Consulta")
	data.NumeroConsulta = s.extractFieldValue(doc, "Número da Consulta")
	data.Observacao = s.extractObservacao(doc)

	data.CNAEsSecundarios = s.extractCNAEsFromText(doc)

	return data, nil
}

func (s *SintegraService) solveCaptcha(ctx context.Context) error {
	if s.config.DebugMode {
		log.Println("[DEBUG] Iniciando resolução do CAPTCHA")
	}

	var siteKey string
	err := chromedp.Run(ctx,
		chromedp.AttributeValue(`[data-sitekey]`, "data-sitekey", &siteKey, nil, chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("erro ao obter site key: %v", err)
	}

	if s.config.DebugMode {
		log.Printf("[DEBUG] Site key obtida: %s", siteKey)
	}

	captchaToken, err := s.requestCaptchaSolution(siteKey)
	if err != nil {
		return fmt.Errorf("erro ao resolver CAPTCHA: %v", err)
	}

	if s.config.DebugMode {
		log.Printf("[DEBUG] Token do CAPTCHA obtido: %s", captchaToken[:50]+"...")
	}

	err = chromedp.Run(ctx,
		chromedp.Evaluate(fmt.Sprintf(`
			var textarea = document.getElementById('g-recaptcha-response');
			if (!textarea) {
				textarea = document.createElement('textarea');
				textarea.id = 'g-recaptcha-response';
				textarea.name = 'g-recaptcha-response';
				textarea.style.display = 'none';
				document.body.appendChild(textarea);
			}
			textarea.value = '%s';
			if (typeof grecaptcha !== 'undefined' && grecaptcha.getResponse) {
				grecaptcha.getResponse = function() { return '%s'; };
			}
		`, captchaToken, captchaToken), nil),
	)

	if err != nil {
		return fmt.Errorf("erro ao injetar token: %v", err)
	}

	return nil
}

func (s *SintegraService) requestCaptchaSolution(siteKey string) (string, error) {
	if s.config.DebugMode {
		log.Printf("[DEBUG] Enviando CAPTCHA para resolução...")
	}

	// Payload para SolveCaptcha API usando form data
	payload := url.Values{
		"key":       {s.config.SolveCaptchaAPIKey},
		"method":    {"userrecaptcha"},
		"googlekey": {siteKey},
		"pageurl":   {s.config.SintegraURL},
		"json":      {"1"},
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.PostForm("https://api.solvecaptcha.com/in.php", payload)
	if err != nil {
		return "", fmt.Errorf("erro ao enviar CAPTCHA: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta: %v", err)
	}

	if s.config.DebugMode {
		log.Printf("[DEBUG] Resposta da API in.php: %s", string(body))
	}

	var result SolveCaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("erro ao parsear resposta: %v (resposta: %s)", err, string(body))
	}

	if result.Status != 1 {
		return "", fmt.Errorf("erro na API SolveCaptcha: %s", result.ID)
	}

	taskId := result.ID
	if taskId == "" {
		return "", fmt.Errorf("taskId vazio recebido da API")
	}

	if s.config.DebugMode {
		log.Printf("[DEBUG] CAPTCHA enviado com ID: %s. Aguardando resolução...", taskId)
	}

	// Polling com timeout de 5 minutos (60 tentativas x 5s)
	for i := 0; i < 60; i++ {
		if s.config.DebugMode && i%6 == 0 { // Log a cada 30 segundos
			log.Printf("[DEBUG] Aguardando CAPTCHA... Tentativa %d/60 (%.0f segundos)", i+1, float64(i*5))
		}

		time.Sleep(5 * time.Second)
		token, err := s.getCaptchaResult(taskId)
		if err == nil {
			if s.config.DebugMode {
				log.Printf("[DEBUG] CAPTCHA resolvido com sucesso após %.0f segundos!", float64(i*5))
			}
			return token, nil
		}

		// Se erro não for "ainda não resolvido", reportar
		if !strings.Contains(err.Error(), "ainda não resolvido") && s.config.DebugMode {
			log.Printf("[DEBUG] Erro no polling: %v", err)
		}
	}

	return "", fmt.Errorf("timeout de 5 minutos aguardando solução do CAPTCHA (ID: %s)", taskId)
}

func (s *SintegraService) getCaptchaResult(captchaID string) (string, error) {
	// URL para verificar resultado na SolveCaptcha API
	url := fmt.Sprintf("https://api.solvecaptcha.com/res.php?key=%s&action=get&id=%s&json=1", s.config.SolveCaptchaAPIKey, captchaID)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("erro na requisição: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta: %v", err)
	}

	if s.config.DebugMode {
		log.Printf("[DEBUG] Resposta da API res.php: %s", string(body))
	}

	var result SolveCaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("erro ao parsear resposta: %v", err)
	}

	if result.Status == 1 {
		if result.ID == "" {
			return "", fmt.Errorf("token vazio recebido")
		}
		return result.ID, nil
	}

	if result.Status == 0 {
		return "", fmt.Errorf("CAPTCHA ainda não resolvido")
	}

	return "", fmt.Errorf("erro no resultado do CAPTCHA: %s", result.ID)
}

func (s *SintegraService) loadDocumentFromFile(path string) (*goquery.Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	utf8Reader, err := charset.NewReader(f, "")
	if err != nil {
		f.Close()
		f2, err2 := os.Open(path)
		if err2 != nil {
			return nil, err2
		}
		defer f2.Close()
		return goquery.NewDocumentFromReader(f2)
	}

	return goquery.NewDocumentFromReader(utf8Reader)
}

func (s *SintegraService) extractFieldValue(doc *goquery.Document, fieldName string) string {
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

func (s *SintegraService) extractObservacao(doc *goquery.Document) string {
	var observacao string
	doc.Find("span.texto").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		if strings.HasPrefix(text, "Observação:") {
			observacao = strings.TrimSpace(text)
		}
	})
	return observacao
}

func (s *SintegraService) extractCNAEsFromText(doc *goquery.Document) []CNAE {
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

// cleanText remove espaços extras e normaliza o texto
func (s *SintegraService) cleanText(text string) string {
	// Remove espaços extras entre palavras
	re := regexp.MustCompile(`\s+`)
	cleaned := re.ReplaceAllString(strings.TrimSpace(text), " ")
	return cleaned
}

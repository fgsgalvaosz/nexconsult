const fs = require('fs').promises;
const { JSDOM } = require('jsdom');
const { extractorLogger } = require('../utils/logger');

class ExtractorService {
    /**
     * Extract CNPJ data from HTML file
     * @param {string} caminhoArquivo - Path to HTML file
     * @returns {Promise<Object>} Extracted CNPJ data
     */
    async extrairDadosCNPJ(caminhoArquivo) {
        const startTime = Date.now();
        try {
            extractorLogger.info('Starting data extraction', { file: caminhoArquivo });
            const html = await fs.readFile(caminhoArquivo, 'utf-8');

            // Optimize JSDOM creation with minimal features
            const dom = new JSDOM(html, {
                features: {
                    FetchExternalResources: false,
                    ProcessExternalResources: false,
                    SkipExternalResources: true
                }
            });
            const document = dom.window.document;

            const dados = {};

            // Extract CNPJ and type (MATRIZ/FILIAL) with improved detection
            const allText = document.body.textContent;
            const cnpjMatch = allText.match(/(\d{2}\.\d{3}\.\d{3}\/\d{4}-\d{2})/);
            if (cnpjMatch) {
                dados.cnpj = cnpjMatch[1];
                console.log(`ExtractorService: CNPJ found: ${dados.cnpj}`);
            }

            // Check if it's MATRIZ or FILIAL
            if (allText.includes('MATRIZ')) {
                dados.tipo = 'MATRIZ';
            } else if (allText.includes('FILIAL')) {
                dados.tipo = 'FILIAL';
            }
            console.log(`ExtractorService: Type: ${dados.tipo || 'Not found'}`);
            
            // Extract basic data using robust search with detailed logging
            console.log('ExtractorService: Extracting basic company data...');
            dados.dataAbertura = this.extrairPorLabel(document, 'DATA DE ABERTURA');
            console.log(`ExtractorService: Data de Abertura: ${dados.dataAbertura}`);

            dados.nomeEmpresarial = this.limparTexto(this.extrairPorLabel(document, 'NOME EMPRESARIAL'));
            console.log(`ExtractorService: Nome Empresarial: ${dados.nomeEmpresarial}`);

            dados.nomeFantasia = this.limparTexto(this.extrairPorLabel(document, 'TÍTULO DO ESTABELECIMENTO'));
            console.log(`ExtractorService: Nome Fantasia: ${dados.nomeFantasia}`);

            dados.porte = this.extrairPorLabel(document, 'PORTE');
            console.log(`ExtractorService: Porte: ${dados.porte}`);
            
            // Extract main economic activity
            console.log('ExtractorService: Extracting economic activities...');
            dados.atividadePrincipal = this.limparTexto(this.extrairPorLabel(document, 'CÓDIGO E DESCRIÇÃO DA ATIVIDADE ECONÔMICA PRINCIPAL'));
            console.log(`ExtractorService: Atividade Principal: ${dados.atividadePrincipal}`);

            // Extract secondary activities
            dados.atividadesSecundarias = this.extrairAtividadesSecundarias(document);
            console.log(`ExtractorService: Atividades Secundárias: ${dados.atividadesSecundarias.length} found`);
            dados.atividadesSecundarias.forEach((atividade, index) => {
                console.log(`ExtractorService: Atividade Secundária ${index + 1}: ${atividade}`);
            });

            // Extract legal nature
            dados.naturezaJuridica = this.limparTexto(this.extrairPorLabel(document, 'CÓDIGO E DESCRIÇÃO DA NATUREZA JURÍDICA'));
            console.log(`ExtractorService: Natureza Jurídica: ${dados.naturezaJuridica}`);
            
            // Extract address with detailed logging and improved logic
            console.log('ExtractorService: Extracting address information...');
            dados.endereco = {
                logradouro: this.limparTexto(this.extrairPorLabel(document, 'LOGRADOURO')),
                numero: this.extrairNumeroEndereco(document),
                complemento: this.extrairComplemento(document),
                cep: this.limparTexto(this.extrairPorLabel(document, 'CEP')),
                bairro: this.limparTexto(this.extrairPorLabel(document, 'BAIRRO/DISTRITO')),
                municipio: this.limparTexto(this.extrairPorLabel(document, 'MUNICÍPIO')),
                uf: this.extrairPorLabel(document, 'UF')
            };

            console.log(`ExtractorService: Endereço completo:`);
            console.log(`  - Logradouro: ${dados.endereco.logradouro}`);
            console.log(`  - Número: ${dados.endereco.numero}`);
            console.log(`  - Complemento: ${dados.endereco.complemento}`);
            console.log(`  - CEP: ${dados.endereco.cep}`);
            console.log(`  - Bairro: ${dados.endereco.bairro}`);
            console.log(`  - Município: ${dados.endereco.municipio}`);
            console.log(`  - UF: ${dados.endereco.uf}`);
            
            // Extract contact information
            console.log('ExtractorService: Extracting contact information...');
            dados.contato = {
                email: this.extrairEmail(document),
                telefone: this.limparTexto(this.extrairPorLabel(document, 'TELEFONE'))
            };
            console.log(`ExtractorService: Email: ${dados.contato.email}`);
            console.log(`ExtractorService: Telefone: ${dados.contato.telefone}`);

            // Extract registration status
            console.log('ExtractorService: Extracting registration status...');
            dados.situacaoCadastral = {
                situacao: this.extrairSituacaoCadastral(document),
                data: this.extrairPorLabel(document, 'DATA DA SITUAÇÃO CADASTRAL'),
                motivo: this.extrairMotivoCadastral(document)
            };
            console.log(`ExtractorService: Situação Cadastral: ${dados.situacaoCadastral.situacao}`);
            console.log(`ExtractorService: Data da Situação: ${dados.situacaoCadastral.data}`);
            console.log(`ExtractorService: Motivo: ${dados.situacaoCadastral.motivo}`);

            // Extract special situation
            console.log('ExtractorService: Extracting special situation...');
            dados.situacaoEspecial = {
                situacao: this.limparTexto(this.extrairPorLabel(document, 'SITUAÇÃO ESPECIAL')),
                data: this.extrairPorLabel(document, 'DATA DA SITUAÇÃO ESPECIAL')
            };
            console.log(`ExtractorService: Situação Especial: ${dados.situacaoEspecial.situacao}`);
            console.log(`ExtractorService: Data da Situação Especial: ${dados.situacaoEspecial.data}`);
            
            // Extract EFR (Ente Federativo Responsável)
            console.log('ExtractorService: Extracting federal entity...');
            dados.enteFederativo = this.limparTexto(this.extrairPorLabel(document, 'ENTE FEDERATIVO RESPONSÁVEL'));
            console.log(`ExtractorService: Ente Federativo: ${dados.enteFederativo}`);

            // Extract emission date
            console.log('ExtractorService: Extracting emission date...');
            const dataEmissaoMatch = html.match(/Emitido no dia <b>(\d{2}\/\d{2}\/\d{4})<\/b> às <b>\s*(\d{2}:\d{2}:\d{2})\s*<\/b>/);
            if (dataEmissaoMatch) {
                dados.dataEmissao = {
                    data: dataEmissaoMatch[1],
                    hora: dataEmissaoMatch[2]
                };
                console.log(`ExtractorService: Data de Emissão: ${dados.dataEmissao.data} às ${dados.dataEmissao.hora}`);
            } else {
                console.log('ExtractorService: Data de emissão não encontrada');
            }

            // Extract additional fields that might be missing
            console.log('ExtractorService: Extracting additional fields...');

            // Capital social (if available)
            dados.capitalSocial = this.limparTexto(this.extrairPorLabel(document, 'CAPITAL SOCIAL'));
            if (dados.capitalSocial) {
                console.log(`ExtractorService: Capital Social: ${dados.capitalSocial}`);
            }

            // Quadro societário (if available)
            dados.quadroSocietario = this.extrairQuadroSocietario(document);
            if (dados.quadroSocietario && dados.quadroSocietario.length > 0) {
                console.log(`ExtractorService: Quadro Societário: ${dados.quadroSocietario.length} sócios encontrados`);
            }

            // Summary log
            const extractionTime = Date.now() - startTime;
            extractorLogger.info('Data extraction completed successfully', {
                cnpj: dados.cnpj,
                nomeEmpresarial: dados.nomeEmpresarial,
                situacao: dados.situacaoCadastral?.situacao,
                atividadesSecundarias: dados.atividadesSecundarias?.length || 0,
                extractionTime,
                fieldsExtracted: Object.keys(dados).length
            });

            return dados;
            
        } catch (error) {
            const extractionTime = Date.now() - startTime;
            extractorLogger.error('Error extracting CNPJ data', {
                file: caminhoArquivo,
                error: error.message,
                stack: error.stack,
                extractionTime
            });
            throw new Error(`Failed to extract data: ${error.message}`);
        }
    }

    /**
     * Extract data by label using robust search with multiple strategies
     * @param {Document} document - DOM document
     * @param {string} label - Label to search for
     * @returns {string} Extracted text
     */
    extrairPorLabel(document, label) {
        // Optimized: Reduce logging for performance
        // console.log(`ExtractorService: Searching for label: "${label}"`);

        // Strategy 1: Search in font elements (optimized)
        const fonts = Array.from(document.querySelectorAll('font'));
        const labelFont = fonts.find(font => {
            const text = font.textContent.trim().toUpperCase();
            return text.includes(label.toUpperCase());
        });

        if (labelFont) {
            // console.log(`ExtractorService: Found label "${label}" in font element`);
            const cell = labelFont.closest('td');
            if (cell) {
                // Search for bold elements in the same cell
                const boldElements = cell.querySelectorAll('b, font b');
                for (let bold of boldElements) {
                    const text = this.limparTexto(bold.textContent);
                    if (text && text.toUpperCase() !== label.toUpperCase() && text.length > 0) {
                        // console.log(`ExtractorService: Found value for "${label}": "${text}"`);
                        return text;
                    }
                }

                // If no bold elements found, try to get text from next sibling or parent
                const allText = cell.textContent;
                const labelIndex = allText.toUpperCase().indexOf(label.toUpperCase());
                if (labelIndex !== -1) {
                    const afterLabel = allText.substring(labelIndex + label.length).trim();
                    const lines = afterLabel.split('\n').map(line => line.trim()).filter(line => line.length > 0);
                    if (lines.length > 0 && lines[0] !== label) {
                        console.log(`ExtractorService: Found value for "${label}" via text parsing: "${lines[0]}"`);
                        return this.limparTexto(lines[0]);
                    }
                }
            }
        }

        // Strategy 2: Search in all text content with regex
        const allText = document.body.textContent;
        const labelRegex = new RegExp(label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '\\s*([^\n]+)', 'i');
        const match = allText.match(labelRegex);
        if (match && match[1]) {
            const value = this.limparTexto(match[1]);
            if (value && value.length > 0) {
                console.log(`ExtractorService: Found value for "${label}" via regex: "${value}"`);
                return value;
            }
        }

        console.log(`ExtractorService: No value found for label: "${label}"`);
        return '';
    }

    /**
     * Extract secondary activities
     * @param {Document} document - DOM document
     * @returns {Array} Array of secondary activities
     */
    extrairAtividadesSecundarias(document) {
        const atividadesSecundarias = [];
        const fonts = Array.from(document.querySelectorAll('font'));
        const secLabel = fonts.find(font => 
            font.textContent.trim().includes('CÓDIGO E DESCRIÇÃO DAS ATIVIDADES ECONÔMICAS SECUNDÁRIAS')
        );
        
        if (secLabel) {
            const cell = secLabel.closest('td');
            if (cell) {
                const boldFonts = cell.querySelectorAll('b');
                boldFonts.forEach(bold => {
                    const atividade = this.limparTexto(bold.textContent);
                    if (atividade && atividade.match(/^\d{2}\.\d{2}-\d-\d{2}/)) {
                        atividadesSecundarias.push(atividade);
                    }
                });
            }
        }
        return atividadesSecundarias;
    }

    /**
     * Extract quadro societário (partners/shareholders)
     * @param {Document} document - DOM document
     * @returns {Array} Array of partners
     */
    extrairQuadroSocietario(document) {
        const socios = [];
        const fonts = Array.from(document.querySelectorAll('font'));

        // Look for patterns that might indicate partners information
        const sociosLabels = [
            'SÓCIO',
            'ADMINISTRADOR',
            'REPRESENTANTE',
            'DIRETOR',
            'PRESIDENTE'
        ];

        sociosLabels.forEach(label => {
            const labelFont = fonts.find(font =>
                font.textContent.trim().toUpperCase().includes(label)
            );

            if (labelFont) {
                const cell = labelFont.closest('td');
                if (cell) {
                    const boldElements = cell.querySelectorAll('b');
                    boldElements.forEach(bold => {
                        const texto = this.limparTexto(bold.textContent);
                        if (texto && texto.length > 3 && !sociosLabels.some(l => texto.toUpperCase().includes(l))) {
                            socios.push({
                                nome: texto,
                                cargo: label
                            });
                        }
                    });
                }
            }
        });

        return socios;
    }

    /**
     * Extract address number with improved logic
     * @param {Document} document - DOM document
     * @returns {string} Address number
     */
    extrairNumeroEndereco(document) {
        const numeroRaw = this.extrairPorLabel(document, 'NÚMERO');
        // If it contains CNPJ pattern, it's wrong - return empty
        if (numeroRaw && numeroRaw.match(/\d{2}\.\d{3}\.\d{3}\/\d{4}-\d{2}/)) {
            return '';
        }
        return this.limparTexto(numeroRaw);
    }

    /**
     * Extract complement with improved logic
     * @param {Document} document - DOM document
     * @returns {string} Address complement
     */
    extrairComplemento(document) {
        const complemento = this.extrairPorLabel(document, 'COMPLEMENTO');
        // If it's just asterisks, return empty
        if (complemento && complemento.match(/^\*+$/)) {
            return '';
        }
        return this.limparTexto(complemento);
    }

    /**
     * Extract email with improved logic
     * @param {Document} document - DOM document
     * @returns {string} Email address
     */
    extrairEmail(document) {
        const email = this.extrairPorLabel(document, 'ENDEREÇO ELETRÔNICO');
        // If it contains "TELEFONE" or other non-email text, return empty
        if (email && (email.includes('TELEFONE') || !email.includes('@'))) {
            return '';
        }
        return this.limparTexto(email);
    }

    /**
     * Extract cadastral situation with improved logic
     * @param {Document} document - DOM document
     * @returns {string} Cadastral situation
     */
    extrairSituacaoCadastral(document) {
        const situacao = this.extrairPorLabel(document, 'SITUAÇÃO CADASTRAL');
        // If it contains "COMPROVANTE", it's likely the page title, not the status
        if (situacao && situacao.includes('COMPROVANTE')) {
            // Try to find "ATIVA" or other status in the document
            const allText = document.body.textContent;
            if (allText.includes('ATIVA')) {
                return 'ATIVA';
            }
            return '';
        }
        return this.limparTexto(situacao);
    }

    /**
     * Extract cadastral reason with improved logic
     * @param {Document} document - DOM document
     * @returns {string} Cadastral reason
     */
    extrairMotivoCadastral(document) {
        const motivo = this.extrairPorLabel(document, 'MOTIVO DE SITUAÇÃO CADASTRAL');
        // If it contains "SITUAÇÃO ESPECIAL", it's likely wrong
        if (motivo && motivo.includes('SITUAÇÃO ESPECIAL')) {
            return '';
        }
        return this.limparTexto(motivo);
    }

    /**
     * Clean text (remove extra spaces and special characters)
     * @param {string} texto - Text to clean
     * @returns {string} Cleaned text
     */
    limparTexto(texto) {
        if (!texto) return '';
        return texto.replace(/\s+/g, ' ').trim().replace(/\*+/g, '');
    }
}

module.exports = new ExtractorService();
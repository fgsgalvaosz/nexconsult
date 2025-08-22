const fs = require('fs').promises;
const { JSDOM } = require('jsdom');
const { extractorLogger } = require('../utils/logger');

// Optimized logging system
const isDevelopment = process.env.NODE_ENV !== 'production';
const debugLog = isDevelopment ? console.log : () => {};

class ExtractorService {
    constructor() {
        // DOM element cache for performance optimization
        this.domCache = new Map();
        this.fontElementsCache = null;
        this.currentDocumentHash = null;
    }
    /**
     * Extract CNPJ data from HTML file (REFACTORED - Main orchestrator method)
     * @param {string} caminhoArquivo - Path to HTML file
     * @returns {Promise<Object>} Extracted CNPJ data
     */
    async extrairDadosCNPJ(caminhoArquivo) {
        const startTime = Date.now();
        try {
            extractorLogger.info('Starting data extraction', { file: caminhoArquivo });
            
            // Load and parse HTML document
            const { document, html } = await this.loadDocument(caminhoArquivo);
            
            // PARALLELIZATION: Extract all data using parallel specialized methods
            const [
                basicInfo,
                businessInfo,
                endereco,
                contato,
                situacaoCadastral,
                situacaoEspecial,
                additionalFields
            ] = await Promise.all([
                Promise.resolve(this.extractBasicInfo(document)),
                Promise.resolve(this.extractBusinessInfo(document)),
                Promise.resolve(this.extractAddress(document)),
                Promise.resolve(this.extractContact(document)),
                Promise.resolve(this.extractRegistrationStatus(document)),
                Promise.resolve(this.extractSpecialSituation(document)),
                Promise.resolve(this.extractAdditionalFields(document, html))
            ]);

            const dados = {
                ...basicInfo,
                ...businessInfo,
                endereco,
                contato,
                situacaoCadastral,
                situacaoEspecial,
                ...additionalFields
            };

            // Log summary and return
            const extractionTime = Date.now() - startTime;
            extractorLogger.info('Data extraction completed successfully', {
                cnpj: dados.cnpj,
                nomeEmpresarial: dados.nomeEmpresarial,
                situacao: dados.situacaoCadastral?.situacao,
                atividadesSecundarias: dados.atividadesSecundarias?.length || 0,
                extractionTime,
                fieldsExtracted: Object.keys(dados).length
            });

            // MEMORY OPTIMIZATION: Clean up DOM cache after extraction
            this.clearDOMCache();

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
     * Load and parse HTML document with optimized JSDOM and caching
     * @param {string} caminhoArquivo - Path to HTML file
     * @returns {Object} Document and HTML content
     */
    async loadDocument(caminhoArquivo) {
        const html = await fs.readFile(caminhoArquivo, 'utf-8');

        // Create document hash for cache invalidation
        const documentHash = this.createDocumentHash(html);

        // Clear cache if document changed
        if (this.currentDocumentHash !== documentHash) {
            this.clearDOMCache();
            this.currentDocumentHash = documentHash;
        }

        // MEMORY OPTIMIZED JSDOM creation with minimal features
        const dom = new JSDOM(html, {
            features: {
                FetchExternalResources: false,
                ProcessExternalResources: false,
                SkipExternalResources: true
            },
            // Memory optimization options
            resources: 'usable',
            runScripts: 'outside-only',
            pretendToBeVisual: false,
            storageQuota: 0 // Disable storage to save memory
        });

        const document = dom.window.document;

        // Pre-cache commonly used elements for performance
        this.preCacheCommonElements(document);

        // Store DOM reference for cleanup
        this.currentDOM = dom;

        return { document, html };
    }

    /**
     * Create a simple hash for document content
     * @param {string} html - HTML content
     * @returns {string} Document hash
     */
    createDocumentHash(html) {
        // Simple hash based on content length and first/last characters
        return `${html.length}_${html.charAt(0)}_${html.charAt(html.length - 1)}`;
    }

    /**
     * Pre-cache commonly used DOM elements
     * @param {Document} document - DOM document
     */
    preCacheCommonElements(document) {
        // Cache font elements (most commonly used)
        this.fontElementsCache = Array.from(document.querySelectorAll('font'));

        // Cache table cells for faster navigation
        this.domCache.set('tableCells', Array.from(document.querySelectorAll('td')));

        // Cache bold elements for activities extraction
        this.domCache.set('boldElements', Array.from(document.querySelectorAll('b')));

        debugLog(`ExtractorService: Pre-cached ${this.fontElementsCache.length} font elements and ${this.domCache.get('tableCells').length} table cells`);
    }

    /**
     * Clear DOM cache and cleanup JSDOM for memory optimization
     */
    clearDOMCache() {
        // Clear caches
        this.domCache.clear();
        this.fontElementsCache = null;
        this.currentDocumentHash = null;

        // MEMORY OPTIMIZATION: Cleanup JSDOM window and document
        if (this.currentDOM) {
            try {
                // Close the JSDOM window to free memory
                this.currentDOM.window.close();
                this.currentDOM = null;
            } catch (error) {
                debugLog('ExtractorService: Error closing JSDOM window:', error.message);
            }
        }

        // Force garbage collection if available (Node.js with --expose-gc)
        if (global.gc) {
            global.gc();
            debugLog('ExtractorService: Forced garbage collection');
        }

        debugLog('ExtractorService: DOM cache and memory cleaned up');
    }

    /**
     * Get memory usage statistics
     * @returns {Object} Memory usage info
     */
    getMemoryStats() {
        const memUsage = process.memoryUsage();
        return {
            heapUsed: Math.round(memUsage.heapUsed / 1024 / 1024) + ' MB',
            heapTotal: Math.round(memUsage.heapTotal / 1024 / 1024) + ' MB',
            external: Math.round(memUsage.external / 1024 / 1024) + ' MB',
            rss: Math.round(memUsage.rss / 1024 / 1024) + ' MB',
            cacheSize: this.domCache.size,
            hasFontCache: !!this.fontElementsCache,
            hasDOM: !!this.currentDOM
        };
    }

    /**
     * Extract basic company information (CNPJ, type)
     * @param {Document} document - DOM document
     * @returns {Object} Basic info object
     */
    extractBasicInfo(document) {
        const allText = document.body.textContent;
        const dados = {};

        // Extract CNPJ
        const cnpjMatch = allText.match(/(\d{2}\.\d{3}\.\d{3}\/\d{4}-\d{2})/);
        if (cnpjMatch) {
            dados.cnpj = cnpjMatch[1];
            debugLog(`ExtractorService: CNPJ found: ${dados.cnpj}`);
        }

        // Check if it's MATRIZ or FILIAL
        if (allText.includes('MATRIZ')) {
            dados.tipo = 'MATRIZ';
        } else if (allText.includes('FILIAL')) {
            dados.tipo = 'FILIAL';
        }
        debugLog(`ExtractorService: Type: ${dados.tipo || 'Not found'}`);

        return dados;
    }

    /**
     * Extract business information (name, activities, legal nature)
     * @param {Document} document - DOM document
     * @returns {Object} Business info object
     */
    extractBusinessInfo(document) {
        debugLog('ExtractorService: Extracting business information...');
        
        const dados = {
            dataAbertura: this.extrairPorLabel(document, 'DATA DE ABERTURA'),
            nomeEmpresarial: this.limparTexto(this.extrairPorLabel(document, 'NOME EMPRESARIAL')),
            nomeFantasia: this.limparTexto(this.extrairPorLabel(document, 'TÍTULO DO ESTABELECIMENTO')),
            porte: this.extrairPorLabel(document, 'PORTE'),
            atividadePrincipal: this.limparTexto(this.extrairPorLabel(document, 'CÓDIGO E DESCRIÇÃO DA ATIVIDADE ECONÔMICA PRINCIPAL')),
            atividadesSecundarias: this.extrairAtividadesSecundarias(document),
            naturezaJuridica: this.limparTexto(this.extrairPorLabel(document, 'CÓDIGO E DESCRIÇÃO DA NATUREZA JURÍDICA'))
        };

        // Debug logging (only in development)
        debugLog(`ExtractorService: Nome Empresarial: ${dados.nomeEmpresarial}`);
        debugLog(`ExtractorService: Atividades Secundárias: ${dados.atividadesSecundarias.length} found`);

        return dados;
    }

    /**
     * Extract address information
     * @param {Document} document - DOM document
     * @returns {Object} Address object
     */
    extractAddress(document) {
        debugLog('ExtractorService: Extracting address information...');
        
        const endereco = {
            logradouro: this.limparTexto(this.extrairPorLabel(document, 'LOGRADOURO')),
            numero: this.extrairNumeroEndereco(document),
            complemento: this.extrairComplemento(document),
            cep: this.limparTexto(this.extrairPorLabel(document, 'CEP')),
            bairro: this.limparTexto(this.extrairPorLabel(document, 'BAIRRO/DISTRITO')),
            municipio: this.limparTexto(this.extrairPorLabel(document, 'MUNICÍPIO')),
            uf: this.extrairPorLabel(document, 'UF')
        };

        debugLog(`ExtractorService: Address extracted - ${endereco.logradouro}, ${endereco.numero}`);
        return endereco;
    }

    /**
     * Extract contact information
     * @param {Document} document - DOM document
     * @returns {Object} Contact object
     */
    extractContact(document) {
        debugLog('ExtractorService: Extracting contact information...');
        
        const contato = {
            email: this.extrairEmail(document),
            telefone: this.limparTexto(this.extrairPorLabel(document, 'TELEFONE'))
        };

        debugLog(`ExtractorService: Contact extracted - ${contato.email}, ${contato.telefone}`);
        return contato;
    }

    /**
     * Extract registration status information
     * @param {Document} document - DOM document
     * @returns {Object} Registration status object
     */
    extractRegistrationStatus(document) {
        debugLog('ExtractorService: Extracting registration status...');
        
        const situacao = {
            situacao: this.extrairSituacaoCadastral(document),
            data: this.extrairPorLabel(document, 'DATA DA SITUAÇÃO CADASTRAL'),
            motivo: this.extrairMotivoCadastral(document)
        };

        debugLog(`ExtractorService: Registration status: ${situacao.situacao}`);
        return situacao;
    }

    /**
     * Extract special situation information
     * @param {Document} document - DOM document
     * @returns {Object} Special situation object
     */
    extractSpecialSituation(document) {
        debugLog('ExtractorService: Extracting special situation...');
        
        return {
            situacao: this.limparTexto(this.extrairPorLabel(document, 'SITUAÇÃO ESPECIAL')),
            data: this.extrairPorLabel(document, 'DATA DA SITUAÇÃO ESPECIAL')
        };
    }

    /**
     * Extract additional fields (capital, partners, EFR, emission date)
     * @param {Document} document - DOM document
     * @param {string} html - Raw HTML content
     * @returns {Object} Additional fields object
     */
    extractAdditionalFields(document, html) {
        debugLog('ExtractorService: Extracting additional fields...');
        
        const dados = {
            enteFederativo: this.limparTexto(this.extrairPorLabel(document, 'ENTE FEDERATIVO RESPONSÁVEL')),
            capitalSocial: this.limparTexto(this.extrairPorLabel(document, 'CAPITAL SOCIAL')),
            quadroSocietario: this.extrairQuadroSocietario(document)
        };

        // Extract emission date from HTML
        const dataEmissaoMatch = html.match(/Emitido no dia <b>(\d{2}\/\d{2}\/\d{4})<\/b> às <b>\s*(\d{2}:\d{2}:\d{2})\s*<\/b>/);
        if (dataEmissaoMatch) {
            dados.dataEmissao = {
                data: dataEmissaoMatch[1],
                hora: dataEmissaoMatch[2]
            };
            debugLog(`ExtractorService: Data de Emissão: ${dados.dataEmissao.data} às ${dados.dataEmissao.hora}`);
        }

        if (dados.quadroSocietario?.length > 0) {
            debugLog(`ExtractorService: Quadro Societário: ${dados.quadroSocietario.length} sócios encontrados`);
        }

        return dados;
    }

    /**
     * Extract data by label using optimized cached search strategies
     * @param {Document} document - DOM document
     * @param {string} label - Label to search for
     * @returns {string} Extracted text
     */
    extrairPorLabel(document, label) {
        // Strategy 1: Search in cached font elements (OPTIMIZED)
        const fonts = this.fontElementsCache || Array.from(document.querySelectorAll('font'));
        const labelFont = fonts.find(font => {
            const text = font.textContent.trim().toUpperCase();
            return text.includes(label.toUpperCase());
        });

        if (labelFont) {
            const cell = labelFont.closest('td');
            if (cell) {
                const nextCell = cell.nextElementSibling;
                if (nextCell) {
                    const value = this.limparTexto(nextCell.textContent);
                    if (value && value.length > 0) {
                        return value;
                    }
                }
            }
        }

        // Strategy 2: Search in cached table cells (OPTIMIZED)
        const tableCells = this.domCache.get('tableCells');
        if (tableCells) {
            for (const cell of tableCells) {
                const cellText = cell.textContent.trim().toUpperCase();
                if (cellText.includes(label.toUpperCase())) {
                    const nextCell = cell.nextElementSibling;
                    if (nextCell) {
                        const value = this.limparTexto(nextCell.textContent);
                        if (value && value.length > 0) {
                            return value;
                        }
                    }
                }
            }
        }

        // Strategy 3: Search in all text content with regex (fallback)
        const allText = document.body.textContent;
        const labelRegex = new RegExp(label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '\\s*([^\n]+)', 'i');
        const match = allText.match(labelRegex);
        if (match && match[1]) {
            const value = this.limparTexto(match[1]);
            if (value && value.length > 0) {
                return value;
            }
        }

        return '';
    }

    /**
     * Extract secondary activities (CACHE OPTIMIZED)
     * @param {Document} document - DOM document
     * @returns {Array} Array of secondary activities
     */
    extrairAtividadesSecundarias(document) {
        const atividadesSecundarias = [];
        const fonts = this.fontElementsCache || Array.from(document.querySelectorAll('font'));
        const secLabel = fonts.find(font =>
            font.textContent.trim().includes('CÓDIGO E DESCRIÇÃO DAS ATIVIDADES ECONÔMICAS SECUNDÁRIAS')
        );

        if (secLabel) {
            const cell = secLabel.closest('td');
            if (cell) {
                // Use cached bold elements if available
                const boldElements = this.domCache.get('boldElements');
                const boldFonts = boldElements ?
                    boldElements.filter(b => cell.contains(b)) :
                    cell.querySelectorAll('b');

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
     * Extract partners information (CACHE OPTIMIZED)
     * @param {Document} document - DOM document
     * @returns {Array} Array of partners
     */
    extrairQuadroSocietario(document) {
        const socios = [];
        const fonts = this.fontElementsCache || Array.from(document.querySelectorAll('font'));

        // Look for patterns that might indicate partners information
        const sociosLabels = [
            'SÓCIO',
            'ADMINISTRADOR',
            'RESPONSÁVEL',
            'REPRESENTANTE LEGAL'
        ];

        fonts.forEach(font => {
            const text = font.textContent.trim();
            if (sociosLabels.some(label => text.toUpperCase().includes(label))) {
                const cell = font.closest('td');
                if (cell) {
                    const nextCell = cell.nextElementSibling;
                    if (nextCell) {
                        const socioInfo = this.limparTexto(nextCell.textContent);
                        if (socioInfo && socioInfo.length > 3) {
                            socios.push(socioInfo);
                        }
                    }
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
     * Clean text (remove extra spaces and special characters) - OPTIMIZED
     * @param {string} texto - Text to clean
     * @returns {string} Cleaned text
     */
    limparTexto(texto) {
        if (!texto) return '';
        return texto.replace(/\s+/g, ' ').trim().replace(/\*+/g, '');
    }
}

module.exports = new ExtractorService();

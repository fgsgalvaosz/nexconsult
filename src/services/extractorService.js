const fs = require('fs').promises;
const { JSDOM } = require('jsdom');
const path = require('path');

class ExtractorService {
    /**
     * Extract CNPJ data from HTML file
     * @param {string} caminhoArquivo - Path to HTML file
     * @returns {Promise<Object>} Extracted CNPJ data
     */
    async extrairDadosCNPJ(caminhoArquivo) {
        try {
            console.log(`ExtractorService: Reading HTML file: ${caminhoArquivo}`);
            const html = await fs.readFile(caminhoArquivo, 'utf-8');
            
            const dom = new JSDOM(html);
            const document = dom.window.document;
            
            const dados = {};
            
            // Extract CNPJ and type (MATRIZ/FILIAL)
            const allText = document.body.textContent;
            const cnpjMatch = allText.match(/(\d{2}\.\d{3}\.\d{3}\/\d{4}-\d{2})/);
            if (cnpjMatch) {
                dados.cnpj = cnpjMatch[1];
            }
            
            // Check if it's MATRIZ or FILIAL
            if (allText.includes('MATRIZ')) {
                dados.tipo = 'MATRIZ';
            } else if (allText.includes('FILIAL')) {
                dados.tipo = 'FILIAL';
            }
            
            // Extract basic data using robust search
            dados.dataAbertura = this.extrairPorLabel(document, 'DATA DE ABERTURA');
            dados.nomeEmpresarial = this.limparTexto(this.extrairPorLabel(document, 'NOME EMPRESARIAL'));
            dados.nomeFantasia = this.limparTexto(this.extrairPorLabel(document, 'TÍTULO DO ESTABELECIMENTO'));
            dados.porte = this.extrairPorLabel(document, 'PORTE');
            
            // Extract main economic activity
            dados.atividadePrincipal = this.limparTexto(this.extrairPorLabel(document, 'CÓDIGO E DESCRIÇÃO DA ATIVIDADE ECONÔMICA PRINCIPAL'));
            
            // Extract secondary activities
            dados.atividadesSecundarias = this.extrairAtividadesSecundarias(document);
            
            // Extract legal nature
            dados.naturezaJuridica = this.limparTexto(this.extrairPorLabel(document, 'CÓDIGO E DESCRIÇÃO DA NATUREZA JURÍDICA'));
            
            // Extract address
            dados.endereco = {
                logradouro: this.limparTexto(this.extrairPorLabel(document, 'LOGRADOURO')),
                numero: this.limparTexto(this.extrairPorLabel(document, 'NÚMERO')),
                complemento: this.limparTexto(this.extrairPorLabel(document, 'COMPLEMENTO')),
                cep: this.limparTexto(this.extrairPorLabel(document, 'CEP')),
                bairro: this.limparTexto(this.extrairPorLabel(document, 'BAIRRO/DISTRITO')),
                municipio: this.limparTexto(this.extrairPorLabel(document, 'MUNICÍPIO')),
                uf: this.extrairPorLabel(document, 'UF')
            };
            
            // Extract contact information
            dados.contato = {
                email: this.limparTexto(this.extrairPorLabel(document, 'ENDEREÇO ELETRÔNICO')),
                telefone: this.limparTexto(this.extrairPorLabel(document, 'TELEFONE'))
            };
            
            // Extract registration status
            dados.situacaoCadastral = {
                situacao: this.limparTexto(this.extrairPorLabel(document, 'SITUAÇÃO CADASTRAL')),
                data: this.extrairPorLabel(document, 'DATA DA SITUAÇÃO CADASTRAL'),
                motivo: this.limparTexto(this.extrairPorLabel(document, 'MOTIVO DE SITUAÇÃO CADASTRAL'))
            };
            
            // Extract special situation
            dados.situacaoEspecial = {
                situacao: this.limparTexto(this.extrairPorLabel(document, 'SITUAÇÃO ESPECIAL')),
                data: this.extrairPorLabel(document, 'DATA DA SITUAÇÃO ESPECIAL')
            };
            
            // Extract EFR
            dados.enteFederativo = this.limparTexto(this.extrairPorLabel(document, 'ENTE FEDERATIVO RESPONSÁVEL'));
            
            // Extract emission date
            const dataEmissaoMatch = html.match(/Emitido no dia <b>(\d{2}\/\d{2}\/\d{4})<\/b> às <b>\s*(\d{2}:\d{2}:\d{2})\s*<\/b>/);
            if (dataEmissaoMatch) {
                dados.dataEmissao = {
                    data: dataEmissaoMatch[1],
                    hora: dataEmissaoMatch[2]
                };
            }
            
            console.log(`ExtractorService: Successfully extracted data for CNPJ: ${dados.cnpj}`);
            return dados;
            
        } catch (error) {
            console.error(`ExtractorService: Error extracting data from ${caminhoArquivo}:`, error);
            throw new Error(`Failed to extract data: ${error.message}`);
        }
    }

    /**
     * Extract data by label using robust search
     * @param {Document} document - DOM document
     * @param {string} label - Label to search for
     * @returns {string} Extracted text
     */
    extrairPorLabel(document, label) {
        const fonts = Array.from(document.querySelectorAll('font'));
        const labelFont = fonts.find(font => {
            const text = font.textContent.trim().toUpperCase();
            return text.includes(label.toUpperCase());
        });
        
        if (labelFont) {
            const cell = labelFont.closest('td');
            if (cell) {
                // Search for bold elements in the same cell
                const boldElements = cell.querySelectorAll('b, font b');
                for (let bold of boldElements) {
                    const text = this.limparTexto(bold.textContent);
                    if (text && text.toUpperCase() !== label.toUpperCase()) {
                        return text;
                    }
                }
            }
        }
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
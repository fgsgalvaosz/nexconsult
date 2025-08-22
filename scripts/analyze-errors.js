#!/usr/bin/env node

/**
 * 🔍 Analisador de Páginas de Erro
 * 
 * Analisa os HTMLs salvos para identificar padrões de erro
 */

const fs = require('fs').promises;
const path = require('path');
const { JSDOM } = require('jsdom');

/**
 * Extrair informações de debug do HTML
 */
function extractDebugInfo(htmlContent) {
    const debugMatch = htmlContent.match(/<!--\s*=== DEBUG INFO ===\s*([\s\S]*?)\s*===================\s*-->/);
    if (!debugMatch) return null;
    
    const debugText = debugMatch[1];
    const info = {};
    
    debugText.split('\n').forEach(line => {
        const match = line.match(/^([^:]+):\s*(.+)$/);
        if (match) {
            const key = match[1].trim();
            const value = match[2].trim();
            
            if (key === 'Page Errors') {
                try {
                    info[key] = JSON.parse(value);
                } catch (e) {
                    info[key] = value;
                }
            } else {
                info[key] = value;
            }
        }
    });
    
    return info;
}

/**
 * Analisar conteúdo da página
 */
function analyzePage(htmlContent) {
    const dom = new JSDOM(htmlContent);
    const document = dom.window.document;
    
    const analysis = {
        title: document.title,
        hasForm: !!document.querySelector('form'),
        hasCaptcha: !!document.querySelector('.h-captcha'),
        hasErrorMessages: [],
        cnpjField: null,
        submitButton: null,
        pageType: 'unknown'
    };
    
    // Verificar mensagens de erro
    const errorSelectors = [
        '#msgErroCaptcha',
        '#msgErro',
        '.alert-danger',
        '.error-message',
        '[class*="erro"]'
    ];
    
    errorSelectors.forEach(selector => {
        const elements = document.querySelectorAll(selector);
        elements.forEach(element => {
            if (element && element.textContent.trim()) {
                analysis.hasErrorMessages.push({
                    selector: selector,
                    text: element.textContent.trim()
                });
            }
        });
    });
    
    // Verificar campo CNPJ
    const cnpjField = document.querySelector('#cnpj');
    if (cnpjField) {
        analysis.cnpjField = {
            value: cnpjField.value,
            disabled: cnpjField.disabled,
            readonly: cnpjField.readOnly
        };
    }
    
    // Verificar botão submit
    const submitButton = document.querySelector('button.btn-primary, input[type="submit"]');
    if (submitButton) {
        analysis.submitButton = {
            text: submitButton.textContent || submitButton.value,
            disabled: submitButton.disabled
        };
    }
    
    // Determinar tipo de página
    if (document.querySelector('table')) {
        analysis.pageType = 'result';
    } else if (document.querySelector('.h-captcha')) {
        analysis.pageType = 'form';
    } else if (analysis.hasErrorMessages.length > 0) {
        analysis.pageType = 'error';
    }
    
    return analysis;
}

/**
 * Analisar todos os arquivos de erro
 */
async function analyzeAllErrors() {
    try {
        console.log('🔍 Analisando páginas de erro...\n');
        
        const debugDir = path.join(process.cwd(), 'debug');
        
        // Verificar se diretório existe
        try {
            await fs.access(debugDir);
        } catch (error) {
            console.log('❌ Diretório debug/ não encontrado. Execute algumas consultas com erro primeiro.');
            return;
        }
        
        const files = await fs.readdir(debugDir);
        const htmlFiles = files.filter(file => file.endsWith('.html'));
        
        if (htmlFiles.length === 0) {
            console.log('❌ Nenhum arquivo HTML encontrado no diretório debug/');
            return;
        }
        
        console.log(`📊 Encontrados ${htmlFiles.length} arquivos para análise\n`);
        
        const results = [];
        
        for (const file of htmlFiles) {
            const filePath = path.join(debugDir, file);
            const content = await fs.readFile(filePath, 'utf-8');
            
            const debugInfo = extractDebugInfo(content);
            const pageAnalysis = analyzePage(content);
            
            results.push({
                filename: file,
                debugInfo,
                analysis: pageAnalysis
            });
        }
        
        // Gerar relatório
        console.log('=' .repeat(80));
        console.log('📋 RELATÓRIO DE ANÁLISE DE ERROS');
        console.log('=' .repeat(80));
        
        // Agrupar por tipo de erro
        const errorTypes = {};
        const cnpjErrors = {};
        
        results.forEach(result => {
            const { debugInfo, analysis, filename } = result;
            
            if (debugInfo) {
                const cnpj = debugInfo.CNPJ;
                const errorMsg = debugInfo['Error Message'];
                
                // Agrupar por tipo de erro
                if (!errorTypes[errorMsg]) {
                    errorTypes[errorMsg] = [];
                }
                errorTypes[errorMsg].push({ cnpj, filename, analysis });
                
                // Agrupar por CNPJ
                if (!cnpjErrors[cnpj]) {
                    cnpjErrors[cnpj] = [];
                }
                cnpjErrors[cnpj].push({ errorMsg, filename, analysis });
            }
        });
        
        // Mostrar tipos de erro
        console.log('\n🚨 TIPOS DE ERRO ENCONTRADOS:');
        Object.entries(errorTypes).forEach(([errorType, occurrences]) => {
            console.log(`\n❌ ${errorType}`);
            console.log(`   Ocorrências: ${occurrences.length}`);
            console.log(`   CNPJs afetados: ${occurrences.map(o => o.cnpj).join(', ')}`);

            // Mostrar análise da primeira ocorrência
            const firstOccurrence = occurrences[0];
            if (firstOccurrence.analysis.hasErrorMessages.length > 0) {
                console.log(`   Mensagens na página:`);
                firstOccurrence.analysis.hasErrorMessages.forEach(msg => {
                    console.log(`     - ${msg.text}`);
                });
            }

            // Classificar por padrão
            if (errorType.includes('Esclarecimentos adicionais')) {
                console.log(`   🔍 Tipo: CNPJ não encontrado na Receita Federal`);
                console.log(`   💡 Ação: Verificar se CNPJ existe ou está ativo`);
            } else if (errorType.includes('captcha')) {
                console.log(`   🔍 Tipo: Problema com captcha`);
                console.log(`   💡 Ação: Verificar API SolveCaptcha`);
            } else if (errorType.includes('timeout')) {
                console.log(`   🔍 Tipo: Timeout de processamento`);
                console.log(`   💡 Ação: Aumentar timeout ou verificar performance`);
            }
        });
        
        // Mostrar CNPJs problemáticos
        console.log('\n📊 CNPJs COM PROBLEMAS:');
        Object.entries(cnpjErrors).forEach(([cnpj, errors]) => {
            console.log(`\n🔢 CNPJ: ${cnpj}`);
            console.log(`   Erros: ${errors.length}`);
            errors.forEach(error => {
                console.log(`   - ${error.errorMsg}`);
            });
        });
        
        // Salvar relatório detalhado
        const reportData = {
            timestamp: new Date().toISOString(),
            totalFiles: htmlFiles.length,
            errorTypes,
            cnpjErrors,
            detailedResults: results
        };
        
        const reportPath = path.join(debugDir, `analysis-report-${Date.now()}.json`);
        await fs.writeFile(reportPath, JSON.stringify(reportData, null, 2));
        
        console.log(`\n💾 Relatório detalhado salvo em: ${reportPath}`);
        console.log('\n✅ Análise concluída!');
        
    } catch (error) {
        console.error('❌ Erro durante análise:', error.message);
    }
}

/**
 * Função principal
 */
async function main() {
    await analyzeAllErrors();
}

// Executar se chamado diretamente
if (require.main === module) {
    main().catch(console.error);
}

module.exports = { analyzeAllErrors, extractDebugInfo, analyzePage };

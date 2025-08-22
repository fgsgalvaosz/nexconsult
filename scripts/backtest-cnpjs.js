#!/usr/bin/env node

/**
 * 🧪 CNPJ API Backtest Script
 * 
 * Testa múltiplos CNPJs para avaliar performance e confiabilidade do sistema
 */

const axios = require('axios');
const fs = require('fs').promises;
const path = require('path');

// Configurações
const API_BASE_URL = 'http://localhost:3000';
const API_KEY = 'bd238cb2bace2dd234e32a8df23486f1';
const DELAY_BETWEEN_REQUESTS = 2000; // 2 segundos entre requisições
const REQUEST_TIMEOUT = 120000; // 2 minutos timeout por requisição

// CNPJs para teste
const TEST_CNPJS = [
    '11365521000169',
    '12309631000176', 
    '36785023000104',
    '36808587000107',
    '36928735000127',
    '37482317000111',
    '45726608000136',
    '46860504000182',
    '50186570000196',
    '57135656000139',
    '57282645000181',
    '57446879000117',
    '60015464000101',
    '60920523000188',
    '61216770000160',
    '52399222000122',
    '38010714000153',
    '51476314000104',
    '49044302000150',
    '26461528000151'
];

// Estatísticas globais
const stats = {
    total: 0,
    success: 0,
    failed: 0,
    cached: 0,
    errors: {},
    totalTime: 0,
    averageTime: 0,
    results: []
};

/**
 * Formatar CNPJ para exibição
 */
function formatCNPJ(cnpj) {
    return cnpj.replace(/(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})/, '$1.$2.$3/$4-$5');
}

/**
 * Formatar tempo em formato legível
 */
function formatTime(ms) {
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    return `${(ms / 60000).toFixed(1)}min`;
}

/**
 * Fazer requisição para consultar CNPJ
 */
async function consultarCNPJ(cnpj) {
    const startTime = Date.now();
    
    try {
        console.log(`\n🔍 Testando CNPJ: ${formatCNPJ(cnpj)}`);
        
        const response = await axios.post(`${API_BASE_URL}/api/cnpj/consultar`, {
            cnpj: cnpj,
            apiKey: API_KEY
        }, {
            timeout: REQUEST_TIMEOUT,
            headers: {
                'Content-Type': 'application/json'
            }
        });
        
        const duration = Date.now() - startTime;
        const data = response.data;
        
        // Verificar se é cache hit (resposta muito rápida)
        const isCached = duration < 100; // Menos de 100ms = cache hit
        
        const result = {
            cnpj: cnpj,
            cnpjFormatted: formatCNPJ(cnpj),
            success: true,
            cached: isCached,
            duration: duration,
            durationFormatted: formatTime(duration),
            statusCode: response.status,
            nomeEmpresarial: data.nomeEmpresarial || 'N/A',
            situacao: data.situacaoCadastral?.situacao || 'N/A',
            dataFields: Object.keys(data).length,
            timestamp: new Date().toISOString()
        };
        
        console.log(`✅ Sucesso! ${result.nomeEmpresarial}`);
        console.log(`   ⏱️  Tempo: ${result.durationFormatted} ${isCached ? '(Cache Hit)' : '(Nova Consulta)'}`);
        console.log(`   📊 Situação: ${result.situacao}`);
        console.log(`   📋 Campos extraídos: ${result.dataFields}`);
        
        return result;
        
    } catch (error) {
        const duration = Date.now() - startTime;
        const errorType = error.response?.status || error.code || 'UNKNOWN_ERROR';
        const errorMessage = error.response?.data?.message || error.message;
        
        const result = {
            cnpj: cnpj,
            cnpjFormatted: formatCNPJ(cnpj),
            success: false,
            cached: false,
            duration: duration,
            durationFormatted: formatTime(duration),
            statusCode: error.response?.status || 0,
            errorType: errorType,
            errorMessage: errorMessage,
            timestamp: new Date().toISOString()
        };
        
        console.log(`❌ Erro! ${errorType}`);
        console.log(`   ⏱️  Tempo: ${result.durationFormatted}`);
        console.log(`   💬 Mensagem: ${errorMessage}`);
        
        return result;
    }
}

/**
 * Aguardar delay entre requisições
 */
function delay(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Executar backtest completo
 */
async function runBacktest() {
    console.log('🚀 Iniciando Backtest do Sistema CNPJ API');
    console.log(`📊 Total de CNPJs para testar: ${TEST_CNPJS.length}`);
    console.log(`⏱️  Delay entre requisições: ${DELAY_BETWEEN_REQUESTS}ms`);
    console.log(`⏰ Timeout por requisição: ${REQUEST_TIMEOUT}ms`);
    console.log('=' .repeat(60));
    
    const startTime = Date.now();
    
    for (let i = 0; i < TEST_CNPJS.length; i++) {
        const cnpj = TEST_CNPJS[i];
        
        console.log(`\n[${i + 1}/${TEST_CNPJS.length}] Progresso: ${((i / TEST_CNPJS.length) * 100).toFixed(1)}%`);
        
        const result = await consultarCNPJ(cnpj);
        
        // Atualizar estatísticas
        stats.total++;
        stats.results.push(result);
        
        if (result.success) {
            stats.success++;
            if (result.cached) {
                stats.cached++;
            }
        } else {
            stats.failed++;
            const errorType = result.errorType;
            stats.errors[errorType] = (stats.errors[errorType] || 0) + 1;
        }
        
        stats.totalTime += result.duration;
        
        // Aguardar antes da próxima requisição (exceto na última)
        if (i < TEST_CNPJS.length - 1) {
            console.log(`⏳ Aguardando ${DELAY_BETWEEN_REQUESTS}ms antes da próxima consulta...`);
            await delay(DELAY_BETWEEN_REQUESTS);
        }
    }
    
    const totalTestTime = Date.now() - startTime;
    stats.averageTime = stats.totalTime / stats.total;
    
    // Gerar relatório
    await generateReport(totalTestTime);
}

/**
 * Gerar relatório detalhado
 */
async function generateReport(totalTestTime) {
    console.log('\n' + '='.repeat(60));
    console.log('📊 RELATÓRIO FINAL DO BACKTEST');
    console.log('='.repeat(60));
    
    // Estatísticas gerais
    console.log('\n📈 ESTATÍSTICAS GERAIS:');
    console.log(`   Total de testes: ${stats.total}`);
    console.log(`   ✅ Sucessos: ${stats.success} (${((stats.success / stats.total) * 100).toFixed(1)}%)`);
    console.log(`   ❌ Falhas: ${stats.failed} (${((stats.failed / stats.total) * 100).toFixed(1)}%)`);
    console.log(`   ⚡ Cache hits: ${stats.cached} (${((stats.cached / stats.total) * 100).toFixed(1)}%)`);
    console.log(`   ⏱️  Tempo médio: ${formatTime(stats.averageTime)}`);
    console.log(`   🕐 Tempo total do teste: ${formatTime(totalTestTime)}`);
    
    // Tipos de erro
    if (Object.keys(stats.errors).length > 0) {
        console.log('\n❌ TIPOS DE ERRO:');
        Object.entries(stats.errors).forEach(([errorType, count]) => {
            console.log(`   ${errorType}: ${count} ocorrências`);
        });
    }
    
    // Performance por categoria
    const successResults = stats.results.filter(r => r.success);
    const cachedResults = successResults.filter(r => r.cached);
    const newResults = successResults.filter(r => !r.cached);
    
    console.log('\n⚡ PERFORMANCE POR CATEGORIA:');
    if (cachedResults.length > 0) {
        const avgCached = cachedResults.reduce((sum, r) => sum + r.duration, 0) / cachedResults.length;
        console.log(`   Cache hits: ${formatTime(avgCached)} (média de ${cachedResults.length} consultas)`);
    }
    if (newResults.length > 0) {
        const avgNew = newResults.reduce((sum, r) => sum + r.duration, 0) / newResults.length;
        console.log(`   Novas consultas: ${formatTime(avgNew)} (média de ${newResults.length} consultas)`);
    }
    
    // Salvar relatório detalhado
    const reportData = {
        timestamp: new Date().toISOString(),
        summary: {
            total: stats.total,
            success: stats.success,
            failed: stats.failed,
            cached: stats.cached,
            successRate: (stats.success / stats.total) * 100,
            cacheHitRate: (stats.cached / stats.total) * 100,
            averageTime: stats.averageTime,
            totalTestTime: totalTestTime
        },
        errors: stats.errors,
        results: stats.results
    };
    
    const reportPath = path.join(__dirname, '..', 'logs', `backtest-report-${Date.now()}.json`);
    await fs.writeFile(reportPath, JSON.stringify(reportData, null, 2));
    
    console.log(`\n💾 Relatório detalhado salvo em: ${reportPath}`);
    console.log('\n🎉 Backtest concluído!');
}

/**
 * Função principal
 */
async function main() {
    try {
        // Verificar se o servidor está rodando
        console.log('🔍 Verificando se o servidor está rodando...');
        await axios.get(`${API_BASE_URL}/health`, { timeout: 5000 });
        console.log('✅ Servidor está rodando!');
        
        await runBacktest();
        
    } catch (error) {
        if (error.code === 'ECONNREFUSED') {
            console.error('❌ Erro: Servidor não está rodando!');
            console.error('   Execute "npm start" em outro terminal primeiro.');
        } else {
            console.error('❌ Erro inesperado:', error.message);
        }
        process.exit(1);
    }
}

// Executar se chamado diretamente
if (require.main === module) {
    main().catch(console.error);
}

module.exports = { runBacktest, consultarCNPJ };

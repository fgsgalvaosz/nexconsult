#!/usr/bin/env node

const fs = require('fs').promises;
const path = require('path');

/**
 * Script para visualizar arquivos de debug (HTML e screenshots)
 * Uso: node scripts/view-debug-files.js [cnpj]
 */

async function listDebugFiles(cnpjFilter = null) {
    try {
        const debugDir = 'debug';
        
        // Verificar se diretório existe
        try {
            await fs.access(debugDir);
        } catch (error) {
            console.log('📁 Diretório debug não encontrado. Nenhum erro foi capturado ainda.');
            return;
        }
        
        const files = await fs.readdir(debugDir);
        
        // Filtrar arquivos por CNPJ se especificado
        const filteredFiles = cnpjFilter 
            ? files.filter(file => file.includes(cnpjFilter))
            : files;
        
        if (filteredFiles.length === 0) {
            if (cnpjFilter) {
                console.log(`📁 Nenhum arquivo de debug encontrado para CNPJ: ${cnpjFilter}`);
            } else {
                console.log('📁 Nenhum arquivo de debug encontrado.');
            }
            return;
        }
        
        // Agrupar arquivos por CNPJ e timestamp
        const groupedFiles = {};
        
        filteredFiles.forEach(file => {
            const match = file.match(/(error|screenshot)-(\d+)-(.+)\.(html|png)$/);
            if (match) {
                const [, type, cnpj, timestamp, ext] = match;
                const key = `${cnpj}-${timestamp}`;
                
                if (!groupedFiles[key]) {
                    groupedFiles[key] = {
                        cnpj,
                        timestamp: timestamp.replace(/-/g, ':').replace('T', ' ').slice(0, -4),
                        files: {}
                    };
                }
                
                groupedFiles[key].files[type] = {
                    filename: file,
                    path: path.join(debugDir, file),
                    size: 0 // Will be filled later
                };
            }
        });
        
        // Obter tamanhos dos arquivos
        for (const group of Object.values(groupedFiles)) {
            for (const fileInfo of Object.values(group.files)) {
                try {
                    const stats = await fs.stat(fileInfo.path);
                    fileInfo.size = stats.size;
                } catch (error) {
                    fileInfo.size = 0;
                }
            }
        }
        
        // Mostrar resultados
        console.log('\n🔍 ARQUIVOS DE DEBUG ENCONTRADOS:\n');
        
        const sortedGroups = Object.entries(groupedFiles)
            .sort(([a], [b]) => b.localeCompare(a)); // Mais recentes primeiro
        
        sortedGroups.forEach(([key, group], index) => {
            console.log(`${index + 1}. CNPJ: ${group.cnpj}`);
            console.log(`   📅 Data: ${group.timestamp}`);
            
            if (group.files.error) {
                const sizeKB = (group.files.error.size / 1024).toFixed(1);
                console.log(`   📄 HTML: ${group.files.error.filename} (${sizeKB} KB)`);
            }
            
            if (group.files.screenshot) {
                const sizeKB = (group.files.screenshot.size / 1024).toFixed(1);
                console.log(`   📸 Screenshot: ${group.files.screenshot.filename} (${sizeKB} KB)`);
            }
            
            console.log('');
        });
        
        // Mostrar estatísticas
        const totalFiles = filteredFiles.length;
        const htmlFiles = filteredFiles.filter(f => f.endsWith('.html')).length;
        const pngFiles = filteredFiles.filter(f => f.endsWith('.png')).length;
        
        console.log('📊 ESTATÍSTICAS:');
        console.log(`   Total de arquivos: ${totalFiles}`);
        console.log(`   Arquivos HTML: ${htmlFiles}`);
        console.log(`   Screenshots PNG: ${pngFiles}`);
        console.log(`   Grupos de erro: ${Object.keys(groupedFiles).length}`);
        
        // Mostrar comandos úteis
        console.log('\n💡 COMANDOS ÚTEIS:');
        console.log('   Ver arquivos de um CNPJ específico:');
        console.log('   node scripts/view-debug-files.js 12309631000176');
        console.log('');
        console.log('   Abrir screenshot no navegador (Linux/Mac):');
        console.log('   xdg-open debug/screenshot-*.png  # Linux');
        console.log('   open debug/screenshot-*.png      # Mac');
        console.log('');
        console.log('   Limpar arquivos antigos:');
        console.log('   rm debug/*');
        
    } catch (error) {
        console.error('❌ Erro ao listar arquivos de debug:', error.message);
        process.exit(1);
    }
}

// Executar script
const cnpjFilter = process.argv[2];
listDebugFiles(cnpjFilter);

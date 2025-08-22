#!/bin/bash

# 🧪 Script para executar backtest dos CNPJs
# 
# Uso: ./scripts/run-backtest.sh

echo "🚀 Iniciando Backtest do Sistema CNPJ API"
echo "=========================================="

# Verificar se Node.js está instalado
if ! command -v node &> /dev/null; then
    echo "❌ Node.js não encontrado. Instale o Node.js primeiro."
    exit 1
fi

# Verificar se o diretório de logs existe
if [ ! -d "logs" ]; then
    echo "📁 Criando diretório de logs..."
    mkdir -p logs
fi

# Verificar se o servidor está rodando
echo "🔍 Verificando se o servidor está rodando..."
if ! curl -s http://localhost:3000/health > /dev/null; then
    echo "❌ Servidor não está rodando!"
    echo "   Execute 'npm start' em outro terminal primeiro."
    exit 1
fi

echo "✅ Servidor está rodando!"
echo ""

# Executar o backtest
echo "🧪 Executando backtest..."
node scripts/backtest-cnpjs.js

echo ""
echo "✅ Backtest concluído!"
echo "📊 Verifique os logs em ./logs/ para relatórios detalhados"

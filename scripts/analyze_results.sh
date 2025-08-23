#!/bin/bash

# Script de Análise de Resultados - NexConsult
# Analisa e gera relatórios dos testes de performance

set -e

# Cores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# Função para mostrar ajuda
show_help() {
    echo -e "${BLUE}📊 Analisador de Resultados - NexConsult${NC}"
    echo -e "${BLUE}=======================================${NC}"
    echo ""
    echo "Uso: $0 [arquivo_resultado.json]"
    echo ""
    echo "Se nenhum arquivo for especificado, analisa o mais recente em performance_results/"
}

# Função para encontrar arquivo mais recente
find_latest_result() {
    local latest=$(ls -t performance_results/performance_test_*.json 2>/dev/null | head -1)
    if [ -z "$latest" ]; then
        echo -e "${RED}❌ Nenhum arquivo de resultado encontrado${NC}"
        echo -e "${YELLOW}💡 Execute primeiro: ./scripts/performance_test.sh${NC}"
        exit 1
    fi
    echo "$latest"
}

# Função para analisar resultados
analyze_results() {
    local file=$1
    
    if [ ! -f "$file" ]; then
        echo -e "${RED}❌ Arquivo não encontrado: $file${NC}"
        exit 1
    fi
    
    echo -e "${BLUE}📊 Analisando resultados: $(basename $file)${NC}"
    echo -e "${BLUE}================================================${NC}"
    echo ""
    
    # Verifica se jq está disponível
    if ! command -v jq &> /dev/null; then
        echo -e "${YELLOW}⚠️  jq não encontrado. Instalando...${NC}"
        if command -v apt-get &> /dev/null; then
            sudo apt-get update && sudo apt-get install -y jq
        elif command -v yum &> /dev/null; then
            sudo yum install -y jq
        else
            echo -e "${RED}❌ Não foi possível instalar jq automaticamente${NC}"
            exit 1
        fi
    fi
    
    # Extrai dados
    local results=$(cat "$file")
    
    echo -e "${GREEN}📈 Resumo Geral:${NC}"
    echo -e "${GREEN}===============${NC}"
    
    # Tabela de resultados
    printf "%-12s %-10s %-10s %-12s %-12s %-10s\n" "Concurrent" "Requests" "Success%" "Avg Time" "Req/Sec" "Errors"
    printf "%-12s %-10s %-10s %-12s %-12s %-10s\n" "----------" "--------" "--------" "---------" "--------" "------"
    
    echo "$results" | jq -r '.[] | 
        "\(.concurrent)\t\(.total_requests)\t\(.success_rate | tonumber | . * 100 / 100 | tostring + "%")\t\(.avg_response_time | tonumber | . | tostring + "s")\t\(.requests_per_second | tonumber | . | tostring)\t\(.error_count)"' | 
    while IFS=$'\t' read -r concurrent requests success_rate avg_time req_sec errors; do
        printf "%-12s %-10s %-10s %-12s %-12s %-10s\n" "$concurrent" "$requests" "$success_rate" "$avg_time" "$req_sec" "$errors"
    done
    
    echo ""
    
    # Análise detalhada
    echo -e "${PURPLE}🔍 Análise Detalhada:${NC}"
    echo -e "${PURPLE}====================${NC}"
    
    # Melhor performance
    local best_rps=$(echo "$results" | jq -r 'max_by(.requests_per_second) | .requests_per_second')
    local best_concurrent=$(echo "$results" | jq -r 'max_by(.requests_per_second) | .concurrent')
    echo -e "🏆 Melhor throughput: ${GREEN}${best_rps} req/s${NC} com ${CYAN}${best_concurrent}${NC} concurrent"
    
    # Menor tempo de resposta
    local best_time=$(echo "$results" | jq -r 'min_by(.avg_response_time) | .avg_response_time')
    local best_time_concurrent=$(echo "$results" | jq -r 'min_by(.avg_response_time) | .concurrent')
    echo -e "⚡ Menor tempo médio: ${GREEN}${best_time}s${NC} com ${CYAN}${best_time_concurrent}${NC} concurrent"
    
    # Taxa de sucesso
    local worst_success=$(echo "$results" | jq -r 'min_by(.success_rate) | .success_rate')
    local worst_success_concurrent=$(echo "$results" | jq -r 'min_by(.success_rate) | .concurrent')
    if (( $(echo "$worst_success < 95" | bc -l) )); then
        echo -e "⚠️  Menor taxa de sucesso: ${RED}${worst_success}%${NC} com ${CYAN}${worst_success_concurrent}${NC} concurrent"
    else
        echo -e "✅ Taxa de sucesso: ${GREEN}Todas acima de 95%${NC}"
    fi
    
    echo ""
    
    # Recomendações
    echo -e "${CYAN}💡 Recomendações:${NC}"
    echo -e "${CYAN}=================${NC}"
    
    # Analisa escalabilidade
    local concurrent_1=$(echo "$results" | jq -r '.[] | select(.concurrent == 1) | .requests_per_second // 0')
    local concurrent_max=$(echo "$results" | jq -r 'max_by(.concurrent) | .requests_per_second')
    local concurrent_max_level=$(echo "$results" | jq -r 'max_by(.concurrent) | .concurrent')
    
    if (( $(echo "$concurrent_max > $concurrent_1 * 2" | bc -l) )); then
        echo -e "🚀 Sistema escala bem: throughput aumenta ${GREEN}$(echo "scale=1; $concurrent_max / $concurrent_1" | bc -l)x${NC} com concorrência"
        echo -e "   Recomendação: Use ${GREEN}${best_concurrent}${NC} workers para melhor performance"
    else
        echo -e "⚠️  Escalabilidade limitada: considere otimizações"
        echo -e "   Possíveis gargalos: captcha, browser pool, ou rate limiting"
    fi
    
    # Analisa tempo de resposta
    local avg_response=$(echo "$results" | jq -r '[.[].avg_response_time] | add / length')
    if (( $(echo "$avg_response < 5" | bc -l) )); then
        echo -e "⚡ Tempo de resposta: ${GREEN}Excelente${NC} (média: $(printf "%.2f" $avg_response)s)"
    elif (( $(echo "$avg_response < 15" | bc -l) )); then
        echo -e "👍 Tempo de resposta: ${YELLOW}Bom${NC} (média: $(printf "%.2f" $avg_response)s)"
    else
        echo -e "🐌 Tempo de resposta: ${RED}Lento${NC} (média: $(printf "%.2f" $avg_response)s)"
        echo -e "   Considere: otimizar captcha, aumentar browser pool, ou usar cache"
    fi
    
    # Analisa erros
    local total_errors=$(echo "$results" | jq -r '[.[].error_count] | add')
    if [ "$total_errors" -gt 0 ]; then
        echo -e "❌ Erros detectados: ${RED}${total_errors}${NC} total"
        echo -e "   Verifique logs para identificar causas (timeout, captcha, etc.)"
    else
        echo -e "✅ Sem erros: ${GREEN}Sistema estável${NC}"
    fi
    
    echo ""
    
    # Gera gráfico ASCII simples
    echo -e "${BLUE}📊 Gráfico de Performance (Req/Sec):${NC}"
    echo -e "${BLUE}====================================${NC}"
    
    local max_rps=$(echo "$results" | jq -r 'max_by(.requests_per_second) | .requests_per_second')
    
    echo "$results" | jq -r '.[] | "\(.concurrent) \(.requests_per_second)"' | sort -n | while read concurrent rps; do
        local bar_length=$(echo "scale=0; $rps * 50 / $max_rps" | bc -l)
        local bar=$(printf "%*s" "$bar_length" | tr ' ' '█')
        printf "%2s concurrent: %-50s %.2f req/s\n" "$concurrent" "$bar" "$rps"
    done
    
    echo ""
    echo -e "${GREEN}✅ Análise concluída!${NC}"
}

# Função principal
main() {
    local result_file=""
    
    if [ $# -eq 0 ]; then
        result_file=$(find_latest_result)
    elif [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
        show_help
        exit 0
    else
        result_file="$1"
    fi
    
    analyze_results "$result_file"
}

# Executa função principal
main "$@"

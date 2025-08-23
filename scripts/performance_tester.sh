#!/bin/bash

# NexConsult Performance Tester - Script Otimizado
# Teste de performance completo com análise integrada
# Combina funcionalidades de teste rápido, completo e análise

set -e

# Configurações padrão
API_BASE_URL="${API_BASE_URL:-http://localhost:3000/api/v1}"
OUTPUT_DIR="performance_results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

# Cores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# CNPJs para teste (otimizada - 50 CNPJs válidos)
CNPJS=(
    "11365521000169" "12309631000176" "36785023000104" "36808587000107"
    "36928735000127" "37482317000111" "45726608000136" "46860504000182"
    "50186570000196" "57135656000139" "57282645000181" "57446879000117"
    "60015464000101" "60920523000188" "61216770000160" "52399222000122"
    "38010714000153" "51476314000104" "49044302000150" "26461528000151"
    "39521518000106" "48390941000105" "16014960400177" "34555461000142"
    "41173405000109" "48743464000114" "61180301000139" "47724108000190"
    "53523583000100" "60113178000170" "44458587000152" "58741540000106"
    "30288513000100" "55064526000127" "46787246000156" "52847783000147"
    "50391153000185" "03382803000146" "44751305000100" "54790003000103"
    "58360581000152" "36181093000145" "49368535000109" "32918995000160"
    "49173066000172" "39698506000151" "56419239000155" "53704399000158"
    "51867294000194" "44223079000195"
)

# Função para mostrar ajuda
show_help() {
    echo -e "${BLUE}� NexConsult Performance Tester${NC}"
    echo -e "${BLUE}=================================${NC}"
    echo ""
    echo "Uso: $0 [modo] [opções]"
    echo ""
    echo "Modos:"
    echo "  quick                  Teste rápido (20 req, 5 concurrent)"
    echo "  full                   Teste completo (múltiplos níveis)"
    echo "  analyze [arquivo]      Analisa resultados existentes"
    echo ""
    echo "Opções:"
    echo "  -u, --url URL          URL base da API"
    echo "  -c, --concurrent N     Níveis de concorrência (ex: 1,5,10)"
    echo "  -n, --requests N       Número de requisições por teste"
    echo "  -t, --timeout N        Timeout por requisição (segundos)"
    echo "  -o, --output DIR       Diretório de saída"
    echo "  -h, --help             Mostra esta ajuda"
    echo ""
    echo "Exemplos:"
    echo "  $0 quick                           # Teste rápido"
    echo "  $0 full -c 1,5,10 -n 30          # Teste personalizado"
    echo "  $0 analyze                         # Analisa último resultado"
    echo "  $0 analyze results.json            # Analisa arquivo específico"
}

# Função para verificar dependências
check_dependencies() {
    local missing=()

    if ! command -v curl &> /dev/null; then
        missing+=("curl")
    fi

    if ! command -v bc &> /dev/null; then
        missing+=("bc")
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        echo -e "${RED}❌ Dependências faltando: ${missing[*]}${NC}"
        echo -e "${YELLOW}💡 Instale com: sudo apt-get install ${missing[*]}${NC}"
        exit 1
    fi
}

# Função para verificar API
check_api() {
    echo -e "${BLUE}� Verificando API...${NC}"

    if ! curl -s --max-time 10 "${API_BASE_URL}/status" > /dev/null; then
        echo -e "${RED}❌ API não disponível em ${API_BASE_URL}${NC}"
        echo -e "${YELLOW}💡 Execute: make run${NC}"
        exit 1
    fi

    echo -e "${GREEN}✅ API disponível${NC}"
}

# Função para fazer requisição
make_request() {
    local cnpj=$1
    local timeout=${2:-60}
    local start_time=$(date +%s.%N)

    local response=$(curl -s -w "%{http_code}" --max-time "$timeout" \
        "${API_BASE_URL}/cnpj/${cnpj}" -o /dev/null 2>/dev/null || echo "000")

    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc -l)

    echo "${response},${duration}"
}

# Função para teste rápido
run_quick_test() {
    local concurrent=${1:-5}
    local requests=${2:-20}
    local timeout=${3:-60}

    echo -e "${BLUE}🚀 Teste Rápido${NC}"
    echo -e "${BLUE}===============${NC}"
    echo -e "Concorrência: ${CYAN}${concurrent}${NC}"
    echo -e "Requisições: ${CYAN}${requests}${NC}"
    echo ""

    local temp_file="/tmp/nexconsult_quick_${TIMESTAMP}.txt"
    > "$temp_file"

    local start_time=$(date +%s.%N)

    # Executa requisições
    for ((i=0; i<requests; i++)); do
        local cnpj=${CNPJS[$((i % ${#CNPJS[@]}))]}

        # Controla concorrência
        while [ $(jobs -r | wc -l) -ge $concurrent ]; do
            sleep 0.1
        done

        (make_request "$cnpj" "$timeout" >> "$temp_file") &
    done

    # Aguarda conclusão
    wait

    local end_time=$(date +%s.%N)
    local total_time=$(echo "$end_time - $start_time" | bc -l)

    # Calcula estatísticas
    local success_count=$(awk -F',' '$1 == 200 { count++ } END { print count+0 }' "$temp_file")
    local error_count=$(awk -F',' '$1 != 200 { count++ } END { print count+0 }' "$temp_file")
    local avg_time=$(awk -F',' '$1 == 200 { sum += $2; count++ } END { if(count > 0) print sum/count; else print 0 }' "$temp_file")
    local min_time=$(awk -F',' '$1 == 200 { if(min == "" || $2 < min) min = $2 } END { print min+0 }' "$temp_file")
    local max_time=$(awk -F',' '$1 == 200 { if($2 > max) max = $2 } END { print max+0 }' "$temp_file")
    local rps=$(echo "scale=2; $requests / $total_time" | bc -l)

    # Exibe resultados
    echo -e "${GREEN}📊 Resultados:${NC}"
    echo -e "${GREEN}===============${NC}"
    echo -e "Total: ${CYAN}${requests}${NC} | Sucessos: ${GREEN}${success_count}${NC} | Erros: ${RED}${error_count}${NC}"
    echo -e "Taxa de sucesso: ${GREEN}$(echo "scale=1; $success_count * 100 / $requests" | bc -l)%${NC}"
    echo ""
    echo -e "Tempo total: ${CYAN}$(printf "%.2f" $total_time)s${NC}"
    echo -e "Tempo médio: ${CYAN}$(printf "%.2f" $avg_time)s${NC}"
    echo -e "Tempo min/max: ${CYAN}$(printf "%.2f" $min_time)s${NC} / ${CYAN}$(printf "%.2f" $max_time)s${NC}"
    echo -e "Req/segundo: ${CYAN}$(printf "%.2f" $rps)${NC}"

    # Avaliação
    if (( $(echo "$avg_time < 5" | bc -l) )); then
        echo -e "Performance: ${GREEN}🚀 Excelente${NC}"
    elif (( $(echo "$avg_time < 10" | bc -l) )); then
        echo -e "Performance: ${YELLOW}⚡ Boa${NC}"
    elif (( $(echo "$avg_time < 20" | bc -l) )); then
        echo -e "Performance: ${YELLOW}⚠️  Aceitável${NC}"
    else
        echo -e "Performance: ${RED}🐌 Lenta${NC}"
    fi

    rm -f "$temp_file"
}

# Função para teste completo
run_full_test() {
    local concurrent_levels=${1:-"1,5,10,20"}
    local requests=${2:-50}
    local timeout=${3:-60}

    echo -e "${BLUE}🧪 Teste Completo${NC}"
    echo -e "${BLUE}=================${NC}"
    echo -e "Níveis: ${CYAN}${concurrent_levels}${NC}"
    echo -e "Requisições por nível: ${CYAN}${requests}${NC}"
    echo ""

    mkdir -p "$OUTPUT_DIR"
    local results_file="${OUTPUT_DIR}/performance_test_${TIMESTAMP}.json"
    local log_file="${OUTPUT_DIR}/performance_test_${TIMESTAMP}.log"

    echo "[]" > "$results_file"

    IFS=',' read -ra LEVELS <<< "$concurrent_levels"

    for level in "${LEVELS[@]}"; do
        echo -e "${YELLOW}🔄 Testando ${level} requisições simultâneas...${NC}"

        local temp_file="/tmp/nexconsult_full_${level}_${TIMESTAMP}.txt"
        > "$temp_file"

        local start_time=$(date +%s.%N)

        # Executa requisições
        for ((i=0; i<requests; i++)); do
            local cnpj=${CNPJS[$((i % ${#CNPJS[@]}))]}

            while [ $(jobs -r | wc -l) -ge $level ]; do
                sleep 0.1
            done

            (make_request "$cnpj" "$timeout" >> "$temp_file") &
        done

        wait

        local end_time=$(date +%s.%N)
        local total_time=$(echo "$end_time - $start_time" | bc -l)

        # Calcula estatísticas
        local success_count=$(awk -F',' '$1 == 200 { count++ } END { print count+0 }' "$temp_file")
        local error_count=$(awk -F',' '$1 != 200 { count++ } END { print count+0 }' "$temp_file")
        local avg_time=$(awk -F',' '$1 == 200 { sum += $2; count++ } END { if(count > 0) print sum/count; else print 0 }' "$temp_file")
        local min_time=$(awk -F',' '$1 == 200 { if(min == "" || $2 < min) min = $2 } END { print min+0 }' "$temp_file")
        local max_time=$(awk -F',' '$1 == 200 { if($2 > max) max = $2 } END { print max+0 }' "$temp_file")
        local rps=$(echo "scale=2; $requests / $total_time" | bc -l)
        local success_rate=$(echo "scale=3; $success_count / $requests" | bc -l)

        # Adiciona ao JSON
        local result=$(cat <<EOF
{
  "concurrent": $level,
  "total_requests": $requests,
  "success_count": $success_count,
  "error_count": $error_count,
  "success_rate": $success_rate,
  "total_time": $total_time,
  "avg_response_time": $avg_time,
  "min_response_time": $min_time,
  "max_response_time": $max_time,
  "requests_per_second": $rps,
  "timestamp": "$(date -Iseconds)"
}
EOF
)

        # Atualiza arquivo JSON
        local temp_json="/tmp/temp_results_${TIMESTAMP}.json"
        jq ". += [$result]" "$results_file" > "$temp_json" 2>/dev/null || {
            # Fallback se jq não estiver disponível
            echo "[$result]" > "$results_file"
        }
        [ -f "$temp_json" ] && mv "$temp_json" "$results_file"

        echo -e "  ✅ ${level} concurrent: ${success_count}/${requests} sucessos ($(printf "%.1f" $(echo "$success_rate * 100" | bc -l))%) - $(printf "%.2f" $avg_time)s avg"

        rm -f "$temp_file"
    done

    echo ""
    echo -e "${GREEN}✅ Teste completo finalizado!${NC}"
    echo -e "Resultados salvos em: ${CYAN}${results_file}${NC}"
    echo ""

    # Análise automática
    analyze_results "$results_file"
}

# Função para análise de resultados
analyze_results() {
    local file=${1:-""}

    if [ -z "$file" ]; then
        # Encontra arquivo mais recente
        file=$(ls -t "${OUTPUT_DIR}"/performance_test_*.json 2>/dev/null | head -1)
        if [ -z "$file" ]; then
            echo -e "${RED}❌ Nenhum arquivo de resultado encontrado${NC}"
            echo -e "${YELLOW}💡 Execute primeiro: $0 full${NC}"
            exit 1
        fi
    fi

    if [ ! -f "$file" ]; then
        echo -e "${RED}❌ Arquivo não encontrado: $file${NC}"
        exit 1
    fi

    echo -e "${BLUE}📊 Análise de Resultados${NC}"
    echo -e "${BLUE}========================${NC}"
    echo -e "Arquivo: ${CYAN}$(basename "$file")${NC}"
    echo ""

    # Verifica se jq está disponível
    if ! command -v jq &> /dev/null; then
        echo -e "${YELLOW}⚠️  jq não encontrado. Instalando...${NC}"
        sudo apt-get update && sudo apt-get install -y jq >/dev/null 2>&1
    fi

    # Tabela de resultados
    echo -e "${GREEN}📈 Resumo:${NC}"
    printf "%-12s %-10s %-10s %-12s %-12s %-10s\n" "Concurrent" "Requests" "Success%" "Avg Time" "Req/Sec" "Errors"
    printf "%-12s %-10s %-10s %-12s %-12s %-10s\n" "----------" "--------" "--------" "---------" "--------" "------"

    if command -v jq &> /dev/null; then
        jq -r '.[] | "\(.concurrent)\t\(.total_requests)\t\((.success_rate * 100) | floor)%\t\(.avg_response_time | . * 100 | floor / 100)s\t\(.requests_per_second | . * 100 | floor / 100)\t\(.error_count)"' "$file" | \
        while IFS=$'\t' read -r concurrent requests success_rate avg_time req_sec errors; do
            printf "%-12s %-10s %-10s %-12s %-12s %-10s\n" "$concurrent" "$requests" "$success_rate" "$avg_time" "$req_sec" "$errors"
        done
    else
        echo -e "${YELLOW}⚠️  jq não disponível - análise limitada${NC}"
    fi

    echo ""

    # Recomendações
    echo -e "${PURPLE}💡 Recomendações:${NC}"
    if command -v jq &> /dev/null; then
        local best_rps=$(jq -r '[.[] | .requests_per_second] | max' "$file")
        local best_concurrent=$(jq -r '.[] | select(.requests_per_second == ([.[] | .requests_per_second] | max)) | .concurrent' "$file")
        local avg_response=$(jq -r '[.[] | .avg_response_time] | add / length' "$file")

        echo -e "• Melhor performance: ${GREEN}${best_concurrent} concurrent${NC} (${best_rps} req/s)"

        if (( $(echo "$avg_response < 5" | bc -l) )); then
            echo -e "• Performance geral: ${GREEN}Excelente${NC} (< 5s médio)"
        elif (( $(echo "$avg_response < 10" | bc -l) )); then
            echo -e "• Performance geral: ${YELLOW}Boa${NC} (5-10s médio)"
        elif (( $(echo "$avg_response < 20" | bc -l) )); then
            echo -e "• Performance geral: ${YELLOW}Aceitável${NC} (10-20s médio)"
            echo -e "• Considere otimizar captcha ou browser pool"
        else
            echo -e "• Performance geral: ${RED}Lenta${NC} (> 20s médio)"
            echo -e "• Recomenda-se investigar gargalos"
        fi
    fi
}

# Função principal
main() {
    local mode=""
    local concurrent_levels="1,5,10,20"
    local requests=50
    local timeout=60

    # Parse argumentos
    while [[ $# -gt 0 ]]; do
        case $1 in
            quick|full|analyze)
                mode="$1"
                shift
                ;;
            -u|--url)
                API_BASE_URL="$2"
                shift 2
                ;;
            -c|--concurrent)
                concurrent_levels="$2"
                shift 2
                ;;
            -n|--requests)
                requests="$2"
                shift 2
                ;;
            -t|--timeout)
                timeout="$2"
                shift 2
                ;;
            -o|--output)
                OUTPUT_DIR="$2"
                shift 2
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                if [ -z "$mode" ]; then
                    mode="$1"
                elif [ "$mode" = "analyze" ] && [ -z "${analyze_file:-}" ]; then
                    analyze_file="$1"
                else
                    echo -e "${RED}❌ Opção desconhecida: $1${NC}"
                    show_help
                    exit 1
                fi
                shift
                ;;
        esac
    done

    # Modo padrão
    if [ -z "$mode" ]; then
        mode="quick"
    fi

    # Verifica dependências
    check_dependencies

    case $mode in
        quick)
            check_api
            run_quick_test 5 20 "$timeout"
            ;;
        full)
            check_api
            run_full_test "$concurrent_levels" "$requests" "$timeout"
            ;;
        analyze)
            analyze_results "${analyze_file:-}"
            ;;
        *)
            echo -e "${RED}❌ Modo inválido: $mode${NC}"
            show_help
            exit 1
            ;;
    esac
}

# Executa se chamado diretamente
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
    
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

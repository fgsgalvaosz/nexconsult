#!/bin/bash

# Script de Teste de Performance - NexConsult
# Testa consultas simultâneas de CNPJ para avaliar performance

set -e

# Configurações
API_BASE_URL="${API_BASE_URL:-http://localhost:3000/api/v1}"
OUTPUT_DIR="performance_results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
RESULTS_FILE="${OUTPUT_DIR}/performance_test_${TIMESTAMP}.json"
LOG_FILE="${OUTPUT_DIR}/performance_test_${TIMESTAMP}.log"

# Cores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Lista de CNPJs para teste
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
    "51867294000194" "44223079000195" "30699240000197" "35340137000170"
    "40002731000190" "52164912000100" "34606066000141" "58103865000163"
    "55330786000105" "26114962000165" "39530947000140" "55570277000141"
    "44461394000150" "50029654000116" "52645697000151" "52645932000195"
    "33552154000145" "57810337000181" "43996591000101" "58329861000106"
    "33618781000131" "41646909000107" "50824718000170" "42115775000152"
    "60112305000117" "38139407000177" "53648206000199" "40779544000118"
    "48123272000105" "35948057000100" "37633558000114" "35805623000116"
    "49542636000154" "55419982000142" "35636409000183" "49858909000174"
    "22932054000128" "29754242000152" "58189115000156" "60664100000144"
    "40770892000124" "44169485000117" "42200740000111" "51897234000114"
    "54079267000145" "47693349000110" "30664867000102" "53577741000104"
    "50115777000170" "14704059000175" "34576167000117" "31785334000141"
)

# Função para exibir ajuda
show_help() {
    echo -e "${BLUE}🧪 Script de Teste de Performance - NexConsult${NC}"
    echo -e "${BLUE}================================================${NC}"
    echo ""
    echo "Uso: $0 [opções]"
    echo ""
    echo "Opções:"
    echo "  -u, --url URL          URL base da API (padrão: http://localhost:3000/api/v1)"
    echo "  -c, --concurrent N     Número de requisições simultâneas (padrão: 1,5,10,20)"
    echo "  -n, --requests N       Número total de requisições por teste (padrão: 50)"
    echo "  -t, --timeout N        Timeout por requisição em segundos (padrão: 60)"
    echo "  -o, --output DIR       Diretório de saída (padrão: performance_results)"
    echo "  -h, --help             Mostra esta ajuda"
    echo ""
    echo "Exemplos:"
    echo "  $0                                    # Teste padrão"
    echo "  $0 -c 5,10,15 -n 30                 # Teste com 5,10,15 concurrent e 30 requests"
    echo "  $0 -u http://prod.example.com/api/v1 # Teste em produção"
}

# Função para verificar se a API está disponível
check_api() {
    echo -e "${BLUE}🔍 Verificando disponibilidade da API...${NC}"
    
    if ! curl -s --max-time 10 "${API_BASE_URL}/status" > /dev/null; then
        echo -e "${RED}❌ API não está disponível em ${API_BASE_URL}${NC}"
        echo -e "${YELLOW}💡 Certifique-se de que o servidor está rodando:${NC}"
        echo -e "${YELLOW}   make run${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✅ API disponível${NC}"
}

# Função para fazer uma requisição e medir tempo
make_request() {
    local cnpj=$1
    local start_time=$(date +%s.%N)
    
    local response=$(curl -s -w "%{http_code}|%{time_total}" \
        --max-time 60 \
        "${API_BASE_URL}/cnpj/${cnpj}" 2>/dev/null || echo "000|0")
    
    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc -l)
    
    local http_code=$(echo "$response" | tail -1 | cut -d'|' -f1)
    local curl_time=$(echo "$response" | tail -1 | cut -d'|' -f2)
    
    echo "${cnpj},${http_code},${duration},${curl_time}"
}

# Função para executar teste de performance
run_performance_test() {
    local concurrent=$1
    local total_requests=$2
    local timeout=$3
    
    echo -e "${PURPLE}🚀 Executando teste: ${concurrent} concurrent, ${total_requests} requests${NC}"
    
    local temp_file=$(mktemp)
    local start_time=$(date +%s.%N)
    
    # Executa requisições em paralelo
    for ((i=0; i<total_requests; i++)); do
        local cnpj=${CNPJS[$((i % ${#CNPJS[@]}))]}
        
        # Controla concorrência
        while [ $(jobs -r | wc -l) -ge $concurrent ]; do
            sleep 0.1
        done
        
        make_request "$cnpj" >> "$temp_file" &
    done
    
    # Aguarda todas as requisições terminarem
    wait
    
    local end_time=$(date +%s.%N)
    local total_duration=$(echo "$end_time - $start_time" | bc -l)
    
    # Processa resultados
    local success_count=$(awk -F',' '$2 == 200 { count++ } END { print count+0 }' "$temp_file")
    local error_count=$(awk -F',' '$2 != 200 { count++ } END { print count+0 }' "$temp_file")
    local avg_response_time=$(awk -F',' '$2 == 200 { sum += $3; count++ } END { if(count > 0) print sum/count; else print 0 }' "$temp_file")
    local min_response_time=$(awk -F',' '$2 == 200 { if(min == "" || $3 < min) min = $3 } END { print min+0 }' "$temp_file")
    local max_response_time=$(awk -F',' '$2 == 200 { if($3 > max) max = $3 } END { print max+0 }' "$temp_file")
    local requests_per_second=$(echo "scale=2; $total_requests / $total_duration" | bc -l)
    
    # Salva resultados detalhados
    cat "$temp_file" >> "${OUTPUT_DIR}/detailed_${TIMESTAMP}.csv"
    
    # Cria resultado JSON
    local result=$(cat <<EOF
{
    "concurrent": $concurrent,
    "total_requests": $total_requests,
    "success_count": $success_count,
    "error_count": $error_count,
    "success_rate": $(echo "scale=2; $success_count * 100 / $total_requests" | bc -l),
    "total_duration": $total_duration,
    "avg_response_time": $avg_response_time,
    "min_response_time": $min_response_time,
    "max_response_time": $max_response_time,
    "requests_per_second": $requests_per_second,
    "timestamp": "$(date -Iseconds)"
}
EOF
)
    
    echo "$result"
    
    # Exibe resumo
    echo -e "${GREEN}📊 Resultados:${NC}"
    echo -e "   Sucessos: ${GREEN}${success_count}${NC}/${total_requests} ($(echo "scale=1; $success_count * 100 / $total_requests" | bc -l)%)"
    echo -e "   Erros: ${RED}${error_count}${NC}"
    echo -e "   Tempo total: ${CYAN}$(printf "%.2f" $total_duration)s${NC}"
    echo -e "   Tempo médio: ${CYAN}$(printf "%.2f" $avg_response_time)s${NC}"
    echo -e "   Req/seg: ${CYAN}$(printf "%.2f" $requests_per_second)${NC}"
    echo ""
    
    rm "$temp_file"
}

# Função principal
main() {
    local concurrent_levels="1,5,10,20"
    local total_requests=50
    local timeout=60
    
    # Parse argumentos
    while [[ $# -gt 0 ]]; do
        case $1 in
            -u|--url)
                API_BASE_URL="$2"
                shift 2
                ;;
            -c|--concurrent)
                concurrent_levels="$2"
                shift 2
                ;;
            -n|--requests)
                total_requests="$2"
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
                echo -e "${RED}❌ Opção desconhecida: $1${NC}"
                show_help
                exit 1
                ;;
        esac
    done
    
    # Cria diretório de saída
    mkdir -p "$OUTPUT_DIR"
    
    # Verifica dependências
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}❌ curl não encontrado${NC}"
        exit 1
    fi
    
    if ! command -v bc &> /dev/null; then
        echo -e "${RED}❌ bc não encontrado${NC}"
        exit 1
    fi
    
    # Verifica API
    check_api
    
    echo -e "${BLUE}🧪 Iniciando testes de performance${NC}"
    echo -e "${BLUE}===================================${NC}"
    echo -e "API: ${CYAN}${API_BASE_URL}${NC}"
    echo -e "CNPJs disponíveis: ${CYAN}${#CNPJS[@]}${NC}"
    echo -e "Níveis de concorrência: ${CYAN}${concurrent_levels}${NC}"
    echo -e "Requisições por teste: ${CYAN}${total_requests}${NC}"
    echo ""
    
    # Inicia log
    echo "Performance Test Started: $(date)" > "$LOG_FILE"
    echo "cnpj,http_code,duration,curl_time" > "${OUTPUT_DIR}/detailed_${TIMESTAMP}.csv"
    
    # Executa testes
    local results="["
    local first=true
    
    IFS=',' read -ra LEVELS <<< "$concurrent_levels"
    for level in "${LEVELS[@]}"; do
        if [ "$first" = true ]; then
            first=false
        else
            results+=","
        fi
        
        local result=$(run_performance_test "$level" "$total_requests" "$timeout")
        results+="$result"
    done
    
    results+="]"
    
    # Salva resultados finais
    echo "$results" > "$RESULTS_FILE"
    
    echo -e "${GREEN}✅ Teste concluído!${NC}"
    echo -e "📄 Resultados salvos em: ${CYAN}${RESULTS_FILE}${NC}"
    echo -e "📄 Log detalhado em: ${CYAN}${LOG_FILE}${NC}"
    echo -e "📄 Dados detalhados em: ${CYAN}${OUTPUT_DIR}/detailed_${TIMESTAMP}.csv${NC}"
}

# Executa função principal
main "$@"

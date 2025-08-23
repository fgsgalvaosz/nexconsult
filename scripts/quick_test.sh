#!/bin/bash

# Script de Teste Rápido - NexConsult
# Teste simples e rápido de performance

set -e

# Configurações
API_BASE_URL="${API_BASE_URL:-http://localhost:3000/api/v1}"
CONCURRENT="${CONCURRENT:-5}"
REQUESTS="${REQUESTS:-20}"

# Cores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# CNPJs para teste rápido (primeiros 20)
CNPJS=(
    "11365521000169" "12309631000176" "36785023000104" "36808587000107"
    "36928735000127" "37482317000111" "45726608000136" "46860504000182"
    "50186570000196" "57135656000139" "57282645000181" "57446879000117"
    "60015464000101" "60920523000188" "61216770000160" "52399222000122"
    "38010714000153" "51476314000104" "49044302000150" "26461528000151"
)

echo -e "${BLUE}🚀 Teste Rápido de Performance - NexConsult${NC}"
echo -e "${BLUE}===========================================${NC}"
echo ""
echo -e "API: ${CYAN}${API_BASE_URL}${NC}"
echo -e "Concorrência: ${CYAN}${CONCURRENT}${NC}"
echo -e "Requisições: ${CYAN}${REQUESTS}${NC}"
echo ""

# Verifica se a API está disponível
echo -e "${BLUE}🔍 Verificando API...${NC}"
if ! curl -s --max-time 5 "${API_BASE_URL}/status" > /dev/null; then
    echo -e "${RED}❌ API não disponível. Execute: make run${NC}"
    exit 1
fi
echo -e "${GREEN}✅ API disponível${NC}"
echo ""

# Função para fazer requisição
make_request() {
    local cnpj=$1
    local start=$(date +%s.%N)
    
    local response=$(curl -s -w "%{http_code}" --max-time 30 \
        "${API_BASE_URL}/cnpj/${cnpj}" -o /dev/null 2>/dev/null || echo "000")
    
    local end=$(date +%s.%N)
    local duration=$(echo "$end - $start" | bc -l)
    
    printf "CNPJ: %s | Status: %s | Tempo: %.2fs\n" "$cnpj" "$response" "$duration"
    echo "$response,$duration" >> /tmp/quick_test_results.txt
}

# Limpa arquivo de resultados
> /tmp/quick_test_results.txt

echo -e "${YELLOW}🧪 Executando ${REQUESTS} requisições com ${CONCURRENT} simultâneas...${NC}"
echo ""

start_time=$(date +%s.%N)

# Executa requisições
for ((i=0; i<REQUESTS; i++)); do
    cnpj=${CNPJS[$((i % ${#CNPJS[@]}))]}
    
    # Controla concorrência
    while [ $(jobs -r | wc -l) -ge $CONCURRENT ]; do
        sleep 0.1
    done
    
    make_request "$cnpj" &
done

# Aguarda todas terminarem
wait

end_time=$(date +%s.%N)
total_time=$(echo "$end_time - $start_time" | bc -l)

echo ""
echo -e "${GREEN}📊 Resultados:${NC}"
echo -e "${GREEN}===============${NC}"

# Calcula estatísticas
success_count=$(awk -F',' '$1 == 200 { count++ } END { print count+0 }' /tmp/quick_test_results.txt)
error_count=$(awk -F',' '$1 != 200 { count++ } END { print count+0 }' /tmp/quick_test_results.txt)
avg_time=$(awk -F',' '$1 == 200 { sum += $2; count++ } END { if(count > 0) print sum/count; else print 0 }' /tmp/quick_test_results.txt)
min_time=$(awk -F',' '$1 == 200 { if(min == "" || $2 < min) min = $2 } END { print min+0 }' /tmp/quick_test_results.txt)
max_time=$(awk -F',' '$1 == 200 { if($2 > max) max = $2 } END { print max+0 }' /tmp/quick_test_results.txt)
rps=$(echo "scale=2; $REQUESTS / $total_time" | bc -l)

echo -e "Total de requisições: ${CYAN}${REQUESTS}${NC}"
echo -e "Sucessos: ${GREEN}${success_count}${NC}"
echo -e "Erros: ${RED}${error_count}${NC}"
echo -e "Taxa de sucesso: ${GREEN}$(echo "scale=1; $success_count * 100 / $REQUESTS" | bc -l)%${NC}"
echo ""
echo -e "Tempo total: ${CYAN}$(printf "%.2f" $total_time)s${NC}"
echo -e "Tempo médio: ${CYAN}$(printf "%.2f" $avg_time)s${NC}"
echo -e "Tempo mínimo: ${CYAN}$(printf "%.2f" $min_time)s${NC}"
echo -e "Tempo máximo: ${CYAN}$(printf "%.2f" $max_time)s${NC}"
echo ""
echo -e "Requisições/segundo: ${CYAN}$(printf "%.2f" $rps)${NC}"

# Avaliação de performance
if (( $(echo "$avg_time < 5" | bc -l) )); then
    echo -e "Performance: ${GREEN}🚀 Excelente${NC}"
elif (( $(echo "$avg_time < 10" | bc -l) )); then
    echo -e "Performance: ${YELLOW}⚡ Boa${NC}"
elif (( $(echo "$avg_time < 20" | bc -l) )); then
    echo -e "Performance: ${YELLOW}⚠️  Aceitável${NC}"
else
    echo -e "Performance: ${RED}🐌 Lenta${NC}"
fi

# Limpa arquivo temporário
rm -f /tmp/quick_test_results.txt

echo ""
echo -e "${BLUE}💡 Para teste completo: ./scripts/performance_test.sh${NC}"

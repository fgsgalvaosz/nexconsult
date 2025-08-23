# 🧪 Scripts de Teste de Performance

Scripts para testar a performance da API NexConsult com consultas simultâneas de CNPJ.

## 📁 Scripts Disponíveis

### 🚀 `quick_test.sh` - Teste Rápido
Teste simples e rápido para verificação básica de performance.

```bash
# Teste padrão (5 concurrent, 20 requests)
./scripts/quick_test.sh

# Personalizar configurações
CONCURRENT=10 REQUESTS=50 ./scripts/quick_test.sh

# Testar API remota
API_BASE_URL=https://api.exemplo.com/v1 ./scripts/quick_test.sh
```

**Características:**
- ⚡ Execução rápida (1-2 minutos)
- 📊 Resultados em tempo real
- 🎯 Ideal para verificações rápidas

### 🔬 `performance_test.sh` - Teste Completo
Teste abrangente com múltiplos níveis de concorrência e análise detalhada.

```bash
# Teste padrão (1,5,10,20 concurrent, 50 requests cada)
./scripts/performance_test.sh

# Teste personalizado
./scripts/performance_test.sh -c 5,10,15,20 -n 100

# Teste em produção
./scripts/performance_test.sh -u https://api.prod.com/v1 -c 1,5,10

# Ajuda completa
./scripts/performance_test.sh --help
```

**Opções:**
- `-c, --concurrent N`: Níveis de concorrência (ex: 1,5,10,20)
- `-n, --requests N`: Número de requests por teste
- `-t, --timeout N`: Timeout por request (segundos)
- `-u, --url URL`: URL base da API
- `-o, --output DIR`: Diretório de saída

**Características:**
- 📊 Múltiplos cenários de teste
- 💾 Resultados salvos em JSON
- 📈 Métricas detalhadas
- 🔍 Logs completos

### 📊 `analyze_results.sh` - Análise de Resultados
Analisa e gera relatórios dos resultados dos testes.

```bash
# Analisa resultado mais recente
./scripts/analyze_results.sh

# Analisa arquivo específico
./scripts/analyze_results.sh performance_results/performance_test_20231201_143022.json
```

**Características:**
- 📈 Tabelas de performance
- 🏆 Identificação de melhores configurações
- 💡 Recomendações automáticas
- 📊 Gráficos ASCII

## 🎯 Lista de CNPJs de Teste

Os scripts utilizam uma lista de **95 CNPJs reais** para testes:

```
11365521000169, 12309631000176, 36785023000104, 36808587000107,
36928735000127, 37482317000111, 45726608000136, 46860504000182,
... (total de 95 CNPJs)
```

## 📊 Métricas Coletadas

### Métricas Básicas
- **Requests por segundo (RPS)**: Throughput do sistema
- **Tempo de resposta**: Médio, mínimo e máximo
- **Taxa de sucesso**: Percentual de requests bem-sucedidas
- **Taxa de erro**: Percentual de falhas

### Métricas Avançadas
- **Escalabilidade**: Como performance varia com concorrência
- **Estabilidade**: Consistência dos tempos de resposta
- **Gargalos**: Identificação de limitações do sistema

## 🚀 Fluxo de Teste Recomendado

### 1. Verificação Rápida
```bash
# Certifique-se que a API está rodando
make run

# Teste rápido
./scripts/quick_test.sh
```

### 2. Teste Completo
```bash
# Teste abrangente
./scripts/performance_test.sh

# Analise os resultados
./scripts/analyze_results.sh
```

### 3. Teste Customizado
```bash
# Para ambiente de produção
./scripts/performance_test.sh \
  -u https://api.prod.com/v1 \
  -c 1,2,5,10 \
  -n 30 \
  -t 120
```

## 📁 Estrutura de Saída

```
performance_results/
├── performance_test_20231201_143022.json    # Resultados principais
├── performance_test_20231201_143022.log     # Log de execução
└── detailed_20231201_143022.csv             # Dados detalhados
```

### Formato JSON dos Resultados
```json
[
  {
    "concurrent": 5,
    "total_requests": 50,
    "success_count": 48,
    "error_count": 2,
    "success_rate": 96.0,
    "total_duration": 45.23,
    "avg_response_time": 8.45,
    "min_response_time": 3.21,
    "max_response_time": 15.67,
    "requests_per_second": 1.11,
    "timestamp": "2023-12-01T14:30:22-03:00"
  }
]
```

## 🔧 Dependências

### Obrigatórias
- `curl`: Para fazer requisições HTTP
- `bc`: Para cálculos matemáticos
- `bash`: Shell script

### Opcionais
- `jq`: Para análise de JSON (instalado automaticamente)

### Instalação no Ubuntu/Debian
```bash
sudo apt-get update
sudo apt-get install curl bc jq
```

## 💡 Dicas de Uso

### Performance Esperada
- **Excelente**: < 5s tempo médio, > 2 req/s
- **Boa**: 5-10s tempo médio, 1-2 req/s  
- **Aceitável**: 10-20s tempo médio, 0.5-1 req/s
- **Lenta**: > 20s tempo médio, < 0.5 req/s

### Interpretação dos Resultados
- **Alta concorrência com baixo RPS**: Gargalo no captcha ou browser pool
- **Muitos erros**: Timeouts ou problemas de conectividade
- **Tempos inconsistentes**: Instabilidade do serviço de captcha

### Otimizações Sugeridas
1. **Aumentar browser pool**: Se RPS não escala com concorrência
2. **Otimizar captcha**: Se tempo médio > 15s
3. **Implementar cache**: Para CNPJs consultados recentemente
4. **Rate limiting**: Para evitar bloqueios da Receita Federal

## 🐛 Troubleshooting

### API não disponível
```bash
# Verifique se o servidor está rodando
make run

# Teste conectividade
curl http://localhost:3000/api/v1/status
```

### Erros de timeout
- Aumente o timeout: `-t 120`
- Reduza concorrência: `-c 1,2,5`
- Verifique logs do servidor

### Resultados inconsistentes
- Execute múltiplos testes
- Verifique carga do sistema
- Monitore recursos (CPU, memória)

## 📈 Exemplos de Uso

### Teste de Carga Básico
```bash
./scripts/performance_test.sh -c 1,5,10 -n 30
```

### Teste de Stress
```bash
./scripts/performance_test.sh -c 10,20,30,50 -n 100 -t 180
```

### Teste de Produção
```bash
./scripts/performance_test.sh \
  -u https://api.nexconsult.com/v1 \
  -c 1,2,5 \
  -n 20 \
  -t 60
```

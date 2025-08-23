# 🏢 CNPJ Consultor

Sistema de consulta automatizada de CNPJs na Receita Federal com resolução automática de captcha.

## ✨ Características

- 🚀 **Alta Performance**: Pool de workers com browsers otimizados
- 🤖 **Captcha Automático**: Resolução via SolveCaptcha.com
- 🔄 **Busca Direta**: Sempre consulta dados atualizados da Receita Federal
- 📊 **API REST**: Interface simples e documentada
- 🛡️ **Rate Limiting**: Controle de requisições
- 📈 **Monitoramento**: Estatísticas em tempo real

## 🚀 Início Rápido

### Pré-requisitos

- Go 1.21+
- Chave API do SolveCaptcha.com

### Instalação

```bash
# Clone o repositório
git clone <repo-url>
cd nexconsult

# Configure as variáveis de ambiente
export SOLVECAPTCHA_API_KEY="sua-chave-aqui"

# Compile e execute
go build -o cnpj-consultor .
./cnpj-consultor
```

### Uso da API

```bash
# Consultar CNPJ
curl "http://localhost:3000/api/v1/cnpj/38139407000177"

# Verificar status do sistema
curl "http://localhost:3000/api/v1/status"
```

## 📁 Estrutura do Projeto

```
nexconsult/
├── main.go           # Aplicação principal
├── browser.go        # Gerenciamento de browsers e extração
├── worker.go         # Pool de workers
├── captcha.go        # Cliente SolveCaptcha
├── config.go         # Configurações
├── types.go          # Tipos e estruturas
├── legacy/           # Código Python (referência)
│   ├── main.py
│   ├── cnpj_consultor_v2.py
│   └── requirements.txt
└── docs/             # Documentação
```

## ⚙️ Configuração

### Variáveis de Ambiente

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `PORT` | 3000 | Porta do servidor |
| `WORKERS_COUNT` | 5 | Número de workers |
| `SOLVECAPTCHA_API_KEY` | - | Chave da API SolveCaptcha |
| `LOG_LEVEL` | info | Nível de log (debug, info, warn, error) |
| `RATE_LIMIT_RPM` | 100 | Requisições por minuto |

### Configuração Avançada

```bash
# Browser
export BROWSER_PAGE_TIMEOUT=30
export BROWSER_NAV_TIMEOUT=30
export BROWSER_ELEMENT_TIMEOUT=15

# Workers
export MAX_CONCURRENT=10
export WORKER_TIMEOUT=300

# Captcha
export CAPTCHA_TIMEOUT=300
export CAPTCHA_MAX_RETRIES=3
```

## 📊 API Reference

### GET /api/v1/cnpj/{cnpj}

Consulta dados de um CNPJ.

**Parâmetros:**
- `cnpj`: CNPJ com ou sem formatação

**Resposta:**
```json
{
  "cnpj": "38.139.407/0001-77",
  "razao_social": "FERRAZ AUTO CENTER LTDA",
  "situacao": "ATIVA",
  "data_situacao": "18/08/2020",
  "endereco": {
    "logradouro": "R GUANABARA",
    "numero": "377",
    "cidade": "IMPERATRIZ",
    "uf": "MA",
    "cep": "65903-270"
  },
  "atividades": [...],
  "comprovante": {
    "emitido_em": "23/08/2025 às 10:45:56"
  }
}
```

### GET /api/v1/status

Retorna estatísticas do sistema.

**Resposta:**
```json
{
  "jobs": {
    "pending": 0,
    "processing": 0,
    "completed": 15
  },
  "workers": {
    "total": 5,
    "active": 2
  },
  "system": {
    "uptime": "2h30m15s",
    "version": "1.0.0"
  }
}
```

## 🔧 Desenvolvimento

### Compilação

```bash
go build -o cnpj-consultor .
```

### Testes

```bash
go test ./...
```

### Logs

```bash
# Debug detalhado
export LOG_LEVEL=debug
./cnpj-consultor

# Apenas erros
export LOG_LEVEL=error
./cnpj-consultor
```

## 📈 Performance

- **Primeira consulta**: ~30-40s (inclui resolução de captcha)
- **Throughput**: ~100 consultas/hora (limitado pelo captcha)
- **Concorrência**: 5 workers simultâneos
- **Memória**: ~50MB por worker

## 🛠️ Arquitetura

### Componentes

1. **API Server**: Fiber HTTP server
2. **Worker Pool**: Gerencia workers concorrentes
3. **Browser Manager**: Pool de browsers Chrome/Chromium
4. **Captcha Client**: Interface com SolveCaptcha.com
5. **CNPJ Extractor**: Extração de dados da Receita Federal

### Fluxo de Processamento

1. Requisição HTTP recebida
2. Job criado e enviado para worker pool
3. Worker obtém browser do pool
4. Navega para site da Receita Federal
5. Resolve captcha automaticamente
6. Submete formulário e extrai dados
7. Retorna dados estruturados

## 📝 Licença

MIT License - veja LICENSE para detalhes.

## 🤝 Contribuição

1. Fork o projeto
2. Crie uma branch para sua feature
3. Commit suas mudanças
4. Push para a branch
5. Abra um Pull Request

## 📞 Suporte

Para dúvidas ou problemas, abra uma issue no GitHub.

# CNPJ API

API para consulta de CNPJs na Receita Federal do Brasil, construída com Go e Gin.

## 🚀 Características

- **Alta Performance**: Construída com Go para máxima eficiência
- **Cache Inteligente**: Redis para cache com fallback em memória
- **Pool de Browsers**: Gerenciamento automático de browsers Chrome/Chromium
- **Rate Limiting**: Proteção contra abuso da API
- **Documentação Swagger**: Interface interativa para testes
- **Monitoramento**: Health checks e métricas detalhadas
- **Consulta em Lote**: Suporte para múltiplas consultas simultâneas

## 📋 Pré-requisitos

- Go 1.21+
- Docker e Docker Compose
- Make (opcional, mas recomendado)

## 🛠️ Configuração de Desenvolvimento

### 1. Clone o repositório
```bash
git clone <repository-url>
cd cnpj-api
```

### 2. Configuração completa (recomendado)
```bash
make setup
```

Este comando irá:
- Instalar dependências Go
- Instalar ferramentas de desenvolvimento
- Iniciar serviços Docker (Redis, PostgreSQL)
- Gerar documentação Swagger

### 3. Configuração manual (alternativa)

#### Instalar dependências
```bash
make deps
```

#### Instalar ferramentas de desenvolvimento
```bash
make dev-tools
```

#### Iniciar serviços de desenvolvimento
```bash
make docker-up
```

#### Gerar documentação Swagger
```bash
make swagger
```

## 🏃‍♂️ Executando a Aplicação

### Desenvolvimento com Hot Reload
```bash
make dev
```

### Execução simples
```bash
make run
```

### Início rápido (serviços + aplicação)
```bash
make start
```

## 🐳 Serviços de Desenvolvimento

O Docker Compose inclui:

- **Redis** (localhost:6379) - Cache
- **PostgreSQL** (localhost:5432) - Banco de dados
- **Redis Commander** (http://localhost:8081) - Interface web para Redis
- **pgAdmin** (http://localhost:8082) - Interface web para PostgreSQL
  - Email: admin@cnpj-api.com
  - Senha: admin123

### Comandos Docker
```bash
# Iniciar serviços
make docker-up

# Parar serviços
make docker-down

# Ver logs
make docker-logs

# Limpeza completa
make docker-clean
```

## 📚 Documentação da API

Após iniciar a aplicação, acesse:
- **Swagger UI**: http://localhost:8080/swagger/index.html
- **API Base**: http://localhost:8080/api/v1

### Endpoints Principais

#### Consulta CNPJ
```bash
GET /api/v1/cnpj/{cnpj}
```

#### Consulta em Lote
```bash
POST /api/v1/cnpj/batch
Content-Type: application/json

{
  "cnpjs": ["11222333000181", "11333444000172"]
}
```

#### Health Check
```bash
GET /health
```

#### Métricas
```bash
GET /metrics
```

## 🔧 Comandos Make Disponíveis

```bash
make help          # Mostra todos os comandos disponíveis
make build         # Compila a aplicação
make run           # Compila e executa
make dev           # Modo desenvolvimento com hot reload
make test          # Executa testes
make test-coverage # Testes com relatório de cobertura
make clean         # Limpa artefatos de build
make fmt           # Formata código Go
make lint          # Executa linter
make security      # Verificações de segurança
make swagger       # Gera documentação Swagger
```

## ⚙️ Configuração

A aplicação usa variáveis de ambiente definidas no arquivo `.env`. As principais configurações:

### Servidor
- `PORT`: Porta da aplicação (padrão: 8080)
- `ENVIRONMENT`: Ambiente (development/production)

### Redis
- `REDIS_HOST`: Host do Redis (padrão: localhost)
- `REDIS_PORT`: Porta do Redis (padrão: 6379)

### PostgreSQL
- `DB_HOST`: Host do banco (padrão: localhost)
- `DB_PORT`: Porta do banco (padrão: 5432)
- `DB_USER`: Usuário do banco
- `DB_PASSWORD`: Senha do banco

### CNPJ Service
- `SOLVE_CAPTCHA_API_KEY`: Chave da API SolveCaptcha
- `CNPJ_TIMEOUT`: Timeout para consultas (segundos)

### Rate Limiting
- `RATE_LIMIT_RPM`: Requests por minuto (padrão: 1000)
- `RATE_LIMIT_BURST`: Burst size (padrão: 50)

## 🧪 Testes

```bash
# Executar todos os testes
make test

# Testes com cobertura
make test-coverage

# Teste de endpoint específico
curl http://localhost:8080/health
```

## 📊 Monitoramento

### Health Checks
- `/health` - Status geral da aplicação
- `/health/ready` - Readiness probe
- `/health/live` - Liveness probe

### Métricas
- `/metrics` - Métricas da aplicação em JSON

### Logs
A aplicação gera logs estruturados em JSON com informações detalhadas sobre:
- Requests HTTP
- Performance
- Erros
- Cache hits/misses
- Status dos browsers

## 🔒 Segurança

- Rate limiting por IP
- Headers de segurança (CSP, HSTS, etc.)
- Validação de entrada
- Sanitização de dados
- Autenticação por token para endpoints administrativos

## 🚀 Deploy

Para produção, configure as variáveis de ambiente apropriadas e:

```bash
# Build para produção
go build -o cnpj-api cmd/api/main.go

# Executar
./cnpj-api
```

## 🤝 Contribuição

1. Fork o projeto
2. Crie uma branch para sua feature (`git checkout -b feature/AmazingFeature`)
3. Commit suas mudanças (`git commit -m 'Add some AmazingFeature'`)
4. Push para a branch (`git push origin feature/AmazingFeature`)
5. Abra um Pull Request

## 📝 Licença

Este projeto está sob a licença MIT. Veja o arquivo `LICENSE` para detalhes.

## 🆘 Suporte

Para suporte, abra uma issue no repositório ou entre em contato com a equipe de desenvolvimento.

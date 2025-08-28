# NexConsult API

API para consulta de dados de empresas no SINTEGRA do Maranhão.

## 🚀 Funcionalidades

- ✅ Consulta automática de CNPJ no SINTEGRA-MA
- ✅ Resolução automática de CAPTCHA via SolveCaptcha.com
- ✅ Extração completa de dados empresariais
- ✅ API REST com resposta em JSON
- ✅ Validação de CNPJ
- ✅ Health check para monitoramento

## 📋 Dados Extraídos

- **Identificação**: CNPJ, Inscrição Estadual, Razão Social
- **Endereço**: Logradouro, Número, Complemento, Bairro, Município, UF, CEP
- **Contato**: DDD, Telefone
- **Atividades**: CNAE Principal, CNAEs Secundários
- **Situação**: Status Cadastral, Data da Situação
- **Obrigações**: NFe, EDF, CTE
- **Metadados**: Data da Consulta, Observações

## 🛠️ Instalação

### Pré-requisitos

- Go 1.21+
- Chave da API SolveCaptcha.com

### Configuração

1. Clone o repositório:
```bash
git clone <repository-url>
cd nexconsult
```

2. Configure as variáveis de ambiente:
```bash
cp .env.example .env
# Edite o arquivo .env com sua chave da API
```

3. Instale as dependências:
```bash
go mod download
```

4. Compile a aplicação:
```bash
go build -o bin/nexconsult-api cmd/server/main.go
```

5. Execute:
```bash
./bin/nexconsult-api
```

## 🐳 Docker

### Build e execução com Docker:

```bash
# Build da imagem
docker build -t nexconsult-api .

# Execução
docker run -p 3000:3000 -e CAPTCHA_API_KEY=sua_chave_aqui nexconsult-api
```

### Docker Compose:

```bash
# Definir variável de ambiente
export CAPTCHA_API_KEY=sua_chave_aqui

# Executar
docker-compose up -d
```

## 📡 API Endpoints

### Health Check
```
GET /health
```

**Resposta:**
```json
{
  "status": "ok",
  "service": "nexconsult-api",
  "version": "1.0.0"
}
```

### Consulta CNPJ
```
GET /api/v1/consulta/{cnpj}
```

**Exemplo:**
```bash
curl http://localhost:3000/api/v1/consulta/34194865000158
```

**Resposta de Sucesso:**
```json
{
  "success": true,
  "data": {
    "cnpj": "34194865000158",
    "inscricao_estadual": "126089817",
    "razao_social": "S E L DE SOUZA SUARES VEICULOS",
    "regime_apuracao": "SIMPLES NACIONAL",
    "logradouro": "RUA PERNAMBUCO",
    "numero": "757",
    "complemento": "LETRA A",
    "bairro": "CENTRO",
    "municipio": "IMPERATRIZ",
    "uf": "MA",
    "cep": "65903320",
    "telefone": "91777746",
    "cnae_principal": "4512902 - COMÉRCIO SOB CONSIGNAÇÃO DE VEÍCULOS AUTOMOTORES",
    "cnaes_secundarios": [
      {
        "codigo": "4511102",
        "descricao": "COMÉRCIO A VAREJO DE AUTOMÓVEIS, CAMIONETAS E UTILITÁRIOS USADOS"
      }
    ],
    "situacao_cadastral": "HABILITADO",
    "data_situacao": "20/09/2023",
    "data_consulta": "27/08/2025"
  },
  "message": "Consulta realizada com sucesso"
}
```

**Resposta de Erro:**
```json
{
  "error": "Validation Error",
  "message": "CNPJ deve ter 14 dígitos"
}
```

## ⚙️ Variáveis de Ambiente

| Variável | Descrição | Padrão |
|----------|-----------|---------|
| `PORT` | Porta do servidor | `3000` |
| `CAPTCHA_API_KEY` | Chave da API SolveCaptcha.com | **obrigatório** |
| `HEADLESS` | Executar browser em modo headless | `true` |
| `DEBUG_MODE` | Ativar logs de debug | `false` |

## 🔧 Monitoramento

A API inclui health check em `/health` para monitoramento:

```bash
# Verificar status
curl http://localhost:3000/health
```

## 📝 Logs

Os logs incluem:
- Requisições HTTP (via middleware)
- Início/fim de consultas
- Erros de scraping
- Debug (quando habilitado)

## 🚨 Limitações

- Requer chave válida da API SolveCaptcha.com
- Dependente da disponibilidade do site SINTEGRA-MA
- Taxa de consultas limitada pela API de CAPTCHA

## 📄 Licença

Este projeto é licenciado sob a MIT License.

## 🤝 Contribuição

1. Fork o projeto
2. Crie uma branch para sua feature
3. Commit suas mudanças
4. Push para a branch
5. Abra um Pull Request

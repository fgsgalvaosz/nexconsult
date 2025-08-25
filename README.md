# Sintegra MA Scraper

Scraper automatizado para consulta de CNPJ no sistema Sintegra do Maranhão, com resolução automática de reCAPTCHA usando a API SolveCaptcha.

## Características

- ✅ **Headless: FALSE** - Navegador visível para debug e acompanhamento
- 🤖 **Resolução automática de CAPTCHA** via SolveCaptcha API
- 🔄 **Fallback manual** quando API não está disponível
- 📊 **Logging estruturado** com zerolog
- 💾 **Resultados em JSON** com timestamp
- ⚡ **Arquitetura limpa** seguindo padrões do NexConsult

## Pré-requisitos

- Go 1.21+
- Google Chrome instalado
- Conta SolveCaptcha (opcional, mas recomendado)

## Instalação

1. **Clone ou baixe o projeto:**
```bash
git clone <repo-url>
cd nexconsult
```

2. **Instale as dependências:**
```bash
go mod tidy
```

3. **Configure a API key (opcional):**
```bash
cp .env.example .env
# Edite o .env e adicione sua chave da SolveCaptcha
```

## Configuração da API SolveCaptcha

1. Acesse: https://solvecaptcha.com/
2. Crie uma conta e obtenha sua API key
3. Configure a variável de ambiente:

```bash
# Windows
set SOLVECAPTCHA_API_KEY=sua_chave_aqui

# Linux/Mac
export SOLVECAPTCHA_API_KEY=sua_chave_aqui
```

## Uso

### Execução básica:
```bash
go run main.go
```

### Compilar e executar:
```bash
go build -o sintegra-scraper .
./sintegra-scraper
```

## Funcionamento

1. **Inicialização**: Abre Chrome em modo visível (headless=false)
2. **Navegação**: Acessa o portal Sintegra MA
3. **Preenchimento**: Insere o CNPJ de teste (38139407000177)
4. **CAPTCHA**: 
   - Se API configurada: resolve automaticamente
   - Se não: pausa para resolução manual
5. **Consulta**: Submete o formulário e navega pelos resultados
6. **Extração**: Coleta dados da página de detalhes
7. **Resultado**: Salva em arquivo JSON com timestamp

## Estrutura de Resultados

```json
{
  "cnpj": "38139407000177",
  "status": "sucesso",
  "url": "https://sistemas1.sefaz.ma.gov.br/sintegra/...",
  "data": {
    "campo_0": "Razão Social: EMPRESA EXEMPLO LTDA",
    "campo_1": "CNPJ: 38.139.407/0001-77",
    "razao_social": "EMPRESA EXEMPLO LTDA",
    "situacao": "Ativa"
  },
  "execution_time": "45.2s",
  "timestamp": "2024-01-15T10:30:00Z",
  "captcha_solved": true
}
```

## Logs e Monitoramento

O sistema utiliza logging estruturado com níveis:
- `INFO`: Operações normais
- `WARN`: Avisos (ex: API CAPTCHA indisponível)
- `ERROR`: Erros recuperáveis
- `FATAL`: Erros que interrompem a execução

## Troubleshooting

### Chrome não encontrado
```bash
# Windows: Instale o Chrome ou adicione ao PATH
# Linux: sudo apt install google-chrome-stable
```

### CAPTCHA não resolve automaticamente
- Verifique se `SOLVECAPTCHA_API_KEY` está configurada
- Verifique se há créditos na conta SolveCaptcha
- O script pausará para resolução manual se necessário

### Timeout na navegação
- Verifique conexão com internet
- Site pode estar indisponível
- Aumente timeout na configuração se necessário

## Arquitetura

```
main.go
├── Config              # Configurações via env
├── CaptchaSolver      # Integração SolveCaptcha API
├── SintegraMAScraper  # Automação Rod/Chrome
├── Logger             # Logging estruturado
└── Models             # Estruturas de dados
```

## Segurança

- ✅ API keys via variáveis de ambiente
- ✅ Não exposição de credenciais no código
- ✅ User-agent realístico
- ✅ Argumentos Chrome otimizados

## Licença

Seguindo padrões do projeto NexConsult.

---

**Desenvolvido seguindo as especificações do projeto NexConsult**
- Arquitetura limpa
- Logging estruturado com zerolog
- Configuração dinâmica via environment
- Integração SolveCaptcha API
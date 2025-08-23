# Consulta CNPJ - Receita Federal

Este projeto implementa uma automação para consultar CNPJs no site da Receita Federal, baseado nos passos de gravação fornecidos.

## Funcionalidades

- Automação completa da consulta de CNPJ
- Resolução automática de hCaptcha (quando possível)
- Extração dos dados do resultado
- Logging detalhado das operações
- Suporte a modo headless

## Requisitos

- Python 3.7+
- Chrome/Chromium instalado
- Dependências listadas em `requirements.txt`

## Instalação

1. Clone ou baixe este projeto
2. Instale as dependências:

```bash
pip install -r requirements.txt
```

## Uso

### Uso Básico

```python
from cnpj_consulta import CNPJConsulta

# Cria uma instância do consultor
consultor = CNPJConsulta(headless=False)

try:
    # Consulta um CNPJ
    resultado = consultor.consultar_cnpj("38139407000177")
    
    if resultado:
        print("Consulta realizada com sucesso!")
        print(f"URL: {resultado['url_resultado']}")
    else:
        print("Falha na consulta")
        
finally:
    consultor.fechar()
```

### Executar o exemplo

```bash
python cnpj_consulta.py
```

## Estrutura do Projeto

```
.
├── cnpj_consulta.py    # Classe principal para consulta
├── requirements.txt    # Dependências do projeto
└── README.md          # Este arquivo
```

## Como Funciona

O script segue os passos da gravação fornecida:

1. **Configuração do navegador**: Define viewport de 1100x633px
2. **Navegação**: Acessa a URL da Receita Federal com o CNPJ
3. **Resolução de Captcha**: Tenta resolver o hCaptcha automaticamente
4. **Consulta**: Clica no botão "CONSULTAR"
5. **Extração**: Coleta os dados da página de resultado

## Limitações e Considerações

- **hCaptcha**: A resolução automática pode não funcionar sempre. Em casos reais, pode ser necessária intervenção manual
- **Rate Limiting**: O site da Receita Federal pode ter limitações de taxa
- **Mudanças no Site**: Se o site mudar sua estrutura, o script pode precisar de ajustes
- **Uso Responsável**: Use com moderação e respeite os termos de uso do site

## Personalização

### Modo Headless

Para executar sem interface gráfica:

```python
consultor = CNPJConsulta(headless=True)
```

### Extração de Dados Específicos

Você pode modificar o método `_extrair_dados_resultado()` para extrair campos específicos como:
- Razão social
- Nome fantasia
- Situação cadastral
- Data de abertura
- Endereço
- Atividade principal

## Troubleshooting

### Chrome não encontrado
Certifique-se de que o Chrome está instalado no sistema.

### Timeout errors
Aumente os tempos de espera se a conexão estiver lenta.

### Captcha não resolvido
Em alguns casos, pode ser necessário resolver o captcha manualmente.

## Contribuição

Sinta-se à vontade para contribuir com melhorias, correções de bugs ou novas funcionalidades.

## Aviso Legal

Este projeto é apenas para fins educacionais e de automação pessoal. Certifique-se de respeitar os termos de uso do site da Receita Federal e use com responsabilidade.

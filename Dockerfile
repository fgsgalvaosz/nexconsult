# Build stage
FROM golang:1.23-alpine AS builder

# Instalar dependências necessárias
RUN apk add --no-cache git ca-certificates tzdata

# Definir diretório de trabalho
WORKDIR /app

# Copiar arquivos de dependências
COPY go.mod go.sum ./

# Download das dependências
RUN go mod download

# Copiar código fonte
COPY . .

# Build da aplicação
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Estágio de produção - usando imagem com Chrome pré-configurado
FROM ghcr.io/puppeteer/puppeteer:21.6.1

# A imagem do Puppeteer já tem as dependências necessárias

# Usar o usuário pptruser que já existe na imagem do Puppeteer
USER pptruser

# Definir diretório de trabalho
WORKDIR /home/pptruser

# Copiar binário da aplicação
COPY --from=builder /app/main .
COPY --from=builder /app/.env.example .

# Definir variáveis de ambiente
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/bin/chromium-browser
ENV DISPLAY=:99
ENV HOME=/home/appuser

# Expor porta da aplicação
EXPOSE 8080

# Mudar para usuário não-root
USER appuser

# Comando para executar a aplicação
CMD ["./main"]
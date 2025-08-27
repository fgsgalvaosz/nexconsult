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

# Estágio de produção - usando Alpine com Chromium
FROM alpine:latest

# Instalar dependências necessárias incluindo Chromium
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    chromium \
    nss \
    freetype \
    freetype-dev \
    harfbuzz \
    ttf-freefont \
    && rm -rf /var/cache/apk/*

# Criar usuário não-root
RUN addgroup -g 1001 -S appuser && \
    adduser -S appuser -u 1001 -G appuser

# Definir diretório de trabalho
WORKDIR /home/appuser

# Copiar binário da aplicação
COPY --from=builder /app/main .
COPY --from=builder /app/.env.example .

# Definir variáveis de ambiente
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/bin/chromium-browser
ENV DISPLAY=:99
ENV HOME=/home/appuser

# Ajustar permissões
RUN chown -R appuser:appuser /home/appuser

# Expor porta da aplicação
EXPOSE 8080

# Mudar para usuário não-root
USER appuser

# Comando para executar a aplicação
CMD ["./main"]
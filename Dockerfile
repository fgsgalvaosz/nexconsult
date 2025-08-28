# Build stage
FROM golang:1.24-alpine AS builder

# Instalar dependências necessárias
RUN apk add --no-cache git ca-certificates tzdata

# Definir diretório de trabalho
WORKDIR /app

# Copiar arquivos de dependências
COPY go.mod go.sum ./

# Baixar dependências
RUN go mod download

# Copiar código fonte
COPY . .

# Build da aplicação
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o nexconsult-api cmd/server/main.go

# Production stage
FROM alpine:latest

# Instalar dependências de runtime
RUN apk --no-cache add ca-certificates chromium

# Criar usuário não-root
RUN addgroup -g 1001 -S nexconsult && \
    adduser -S nexconsult -u 1001 -G nexconsult

# Definir diretório de trabalho
WORKDIR /app

# Copiar binário da aplicação
COPY --from=builder /app/nexconsult-api .

# Copiar arquivos de configuração
COPY --from=builder /app/.env.example .env

# Definir permissões
RUN chown -R nexconsult:nexconsult /app
USER nexconsult

# Expor porta
EXPOSE 3000

# Definir variáveis de ambiente
ENV PORT=3000
ENV HEADLESS=true

# Comando de inicialização
CMD ["./nexconsult-api"]

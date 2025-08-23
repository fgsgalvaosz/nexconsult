# Multi-stage build para otimizar tamanho da imagem
FROM golang:1.21-alpine AS builder

# Instala dependências necessárias
RUN apk add --no-cache git ca-certificates tzdata

# Define diretório de trabalho
WORKDIR /app

# Copia arquivos de dependências
COPY go.mod go.sum ./

# Baixa dependências
RUN go mod download

# Copia código fonte
COPY . .

# Compila a aplicação
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o nexconsult ./cmd/main.go

# Imagem final mínima
FROM alpine:latest

# Instala certificados CA e timezone
RUN apk --no-cache add ca-certificates tzdata

# Cria usuário não-root
RUN adduser -D -s /bin/sh nexconsult

# Define diretório de trabalho
WORKDIR /app

# Copia binário da etapa de build
COPY --from=builder /app/nexconsult .

# Copia arquivo de configuração se existir
COPY --from=builder /app/.env.example .env

# Muda proprietário dos arquivos
RUN chown -R nexconsult:nexconsult /app

# Muda para usuário não-root
USER nexconsult

# Expõe porta
EXPOSE 3000

# Define comando padrão
CMD ["./nexconsult"]

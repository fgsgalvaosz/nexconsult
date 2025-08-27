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

# Instalar dependências necessárias incluindo Chromium e Xvfb
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    chromium \
    nss \
    freetype \
    freetype-dev \
    harfbuzz \
    ttf-freefont \
    xvfb \
    dbus \
    && rm -rf /var/cache/apk/*

# Criar usuário não-root
RUN addgroup -g 1001 -S appuser && \
    adduser -S appuser -u 1001 -G appuser

# Definir diretório de trabalho
WORKDIR /home/appuser

# Copiar binário da aplicação
COPY --from=builder /app/main .
COPY --from=builder /app/.env.example .

# Criar script de inicialização
RUN echo '#!/bin/sh' > start.sh && \
    echo 'echo "Iniciando Xvfb..."' >> start.sh && \
    echo 'Xvfb :99 -screen 0 1024x768x24 -ac +extension GLX +render -noreset &' >> start.sh && \
    echo 'XVFB_PID=$!' >> start.sh && \
    echo 'export DISPLAY=:99' >> start.sh && \
    echo 'echo "Aguardando Xvfb inicializar..."' >> start.sh && \
    echo 'sleep 3' >> start.sh && \
    echo 'echo "Verificando se Xvfb está rodando..."' >> start.sh && \
    echo 'if ! kill -0 $XVFB_PID 2>/dev/null; then' >> start.sh && \
    echo '  echo "ERRO: Xvfb falhou ao iniciar"' >> start.sh && \
    echo '  exit 1' >> start.sh && \
    echo 'fi' >> start.sh && \
    echo 'echo "Xvfb iniciado com sucesso (PID: $XVFB_PID)"' >> start.sh && \
    echo 'echo "DISPLAY=$DISPLAY"' >> start.sh && \
    echo 'exec "$@"' >> start.sh && \
    chmod +x start.sh

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

# Comando para executar a aplicação com Xvfb
CMD ["./start.sh", "./main"]
# Makefile para NexConsult - CNPJ Consultor API
# Seguindo as boas práticas definidas em .augment/rules/rule-boas-praticas.md

# Variáveis
APP_NAME := nexconsult
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Diretórios
BIN_DIR := bin
CMD_DIR := cmd
INTERNAL_DIR := internal
DOCS_DIR := docs

# Configurações Go
GO := go
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
GOFLAGS := -ldflags="-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Cores para output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

.PHONY: help build run clean test lint fmt vet deps swagger docker dev prod install

# Target padrão
.DEFAULT_GOAL := help

help: ## Mostra esta mensagem de ajuda
	@echo "$(BLUE)NexConsult - CNPJ Consultor API$(NC)"
	@echo "$(BLUE)================================$(NC)"
	@echo ""
	@echo "$(GREEN)Comandos disponíveis:$(NC)"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  $(YELLOW)%-15s$(NC) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""

build: clean fmt vet ## Compila a aplicação
	@echo "$(BLUE)🔨 Compilando $(APP_NAME)...$(NC)"
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(APP_NAME) $(CMD_DIR)/main.go
	@echo "$(GREEN)✅ Build concluído: $(BIN_DIR)/$(APP_NAME)$(NC)"

run: build ## Executa a aplicação
	@echo "$(BLUE)🚀 Executando $(APP_NAME)...$(NC)"
	./$(BIN_DIR)/$(APP_NAME)

dev: ## Executa em modo desenvolvimento (com hot reload)
	@echo "$(BLUE)🔥 Modo desenvolvimento...$(NC)"
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "$(YELLOW)⚠️  Air não encontrado. Instalando...$(NC)"; \
		$(GO) install github.com/cosmtrek/air@latest; \
		air; \
	fi

clean: ## Remove arquivos de build
	@echo "$(BLUE)🧹 Limpando arquivos de build...$(NC)"
	@rm -rf $(BIN_DIR)
	@echo "$(GREEN)✅ Limpeza concluída$(NC)"

test: ## Executa todos os testes
	@echo "$(BLUE)🧪 Executando testes...$(NC)"
	$(GO) test -v -race -coverprofile=coverage.out ./...
	@echo "$(GREEN)✅ Testes concluídos$(NC)"

test-coverage: test ## Executa testes com relatório de cobertura
	@echo "$(BLUE)📊 Gerando relatório de cobertura...$(NC)"
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✅ Relatório gerado: coverage.html$(NC)"

lint: ## Executa linter (golangci-lint)
	@echo "$(BLUE)🔍 Executando linter...$(NC)"
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "$(YELLOW)⚠️  golangci-lint não encontrado. Instalando...$(NC)"; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v1.55.2; \
		golangci-lint run; \
	fi
	@echo "$(GREEN)✅ Linter concluído$(NC)"

fmt: ## Formata o código
	@echo "$(BLUE)🎨 Formatando código...$(NC)"
	$(GO) fmt ./...
	@echo "$(GREEN)✅ Formatação concluída$(NC)"

vet: ## Executa go vet
	@echo "$(BLUE)🔎 Executando go vet...$(NC)"
	$(GO) vet ./...
	@echo "$(GREEN)✅ Go vet concluído$(NC)"

deps: ## Baixa e organiza dependências
	@echo "$(BLUE)📦 Baixando dependências...$(NC)"
	$(GO) mod download
	$(GO) mod tidy
	@echo "$(GREEN)✅ Dependências atualizadas$(NC)"

swagger: ## Gera documentação Swagger
	@echo "$(BLUE)📚 Gerando documentação Swagger...$(NC)"
	@if command -v swag > /dev/null; then \
		swag init -g $(CMD_DIR)/main.go -o $(DOCS_DIR); \
	else \
		echo "$(YELLOW)⚠️  swag não encontrado. Instalando...$(NC)"; \
		$(GO) install github.com/swaggo/swag/cmd/swag@latest; \
		swag init -g $(CMD_DIR)/main.go -o $(DOCS_DIR); \
	fi
	@echo "$(GREEN)✅ Documentação Swagger gerada$(NC)"

install: ## Instala ferramentas de desenvolvimento
	@echo "$(BLUE)🛠️  Instalando ferramentas de desenvolvimento...$(NC)"
	$(GO) install github.com/cosmtrek/air@latest
	$(GO) install github.com/swaggo/swag/cmd/swag@latest
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v1.55.2
	@echo "$(GREEN)✅ Ferramentas instaladas$(NC)"

docker-build: ## Constrói imagem Docker
	@echo "$(BLUE)🐳 Construindo imagem Docker...$(NC)"
	docker build -t $(APP_NAME):$(VERSION) .
	docker tag $(APP_NAME):$(VERSION) $(APP_NAME):latest
	@echo "$(GREEN)✅ Imagem Docker construída$(NC)"

docker-run: docker-build ## Executa container Docker
	@echo "$(BLUE)🐳 Executando container Docker...$(NC)"
	docker run --rm -p 3000:3000 --env-file .env $(APP_NAME):latest

release: clean ## Prepara release (build para múltiplas plataformas)
	@echo "$(BLUE)🚀 Preparando release...$(NC)"
	@mkdir -p $(BIN_DIR)/release
	# Linux AMD64
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/release/$(APP_NAME)-linux-amd64 $(CMD_DIR)/main.go
	# Linux ARM64
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/release/$(APP_NAME)-linux-arm64 $(CMD_DIR)/main.go
	# Windows AMD64
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/release/$(APP_NAME)-windows-amd64.exe $(CMD_DIR)/main.go
	# macOS AMD64
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/release/$(APP_NAME)-darwin-amd64 $(CMD_DIR)/main.go
	# macOS ARM64
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/release/$(APP_NAME)-darwin-arm64 $(CMD_DIR)/main.go
	@echo "$(GREEN)✅ Release preparado em $(BIN_DIR)/release/$(NC)"

check: fmt vet lint test ## Executa todas as verificações (fmt, vet, lint, test)
	@echo "$(GREEN)✅ Todas as verificações passaram$(NC)"

ci: deps check build ## Pipeline de CI (usado em GitHub Actions)
	@echo "$(GREEN)✅ Pipeline de CI concluído$(NC)"

info: ## Mostra informações do projeto
	@echo "$(BLUE)📋 Informações do Projeto$(NC)"
	@echo "$(BLUE)========================$(NC)"
	@echo "Nome: $(APP_NAME)"
	@echo "Versão: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Go Version: $(shell $(GO) version)"
	@echo "GOOS: $(GOOS)"
	@echo "GOARCH: $(GOARCH)"

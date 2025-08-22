# CNPJ API Makefile

# Variables
APP_NAME=cnpj-api
BINARY_NAME=bin/$(APP_NAME)
MAIN_PATH=cmd/api/main.go
DOCKER_COMPOSE_FILE=docker-compose.dev.yml

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
BLUE=\033[0;34m
NC=\033[0m # No Color

.PHONY: help build run dev test clean deps docker-up docker-down docker-logs swagger

# Default target
help: ## Show this help message
	@echo "$(BLUE)CNPJ API Development Commands$(NC)"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "$(GREEN)%-15s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build the application
build: ## Build the application binary
	@echo "$(YELLOW)Building $(APP_NAME)...$(NC)"
	@go build -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "$(GREEN)Build completed: $(BINARY_NAME)$(NC)"

# Run the application locally
run: build ## Build and run the application
	@echo "$(YELLOW)Starting $(APP_NAME)...$(NC)"
	@./$(BINARY_NAME)

# Development mode with hot reload (requires air)
dev: ## Run in development mode with hot reload
	@echo "$(YELLOW)Starting development server...$(NC)"
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "$(RED)Air not found. Installing...$(NC)"; \
		go install github.com/cosmtrek/air@latest; \
		air; \
	fi

# Install dependencies
deps: ## Install Go dependencies
	@echo "$(YELLOW)Installing dependencies...$(NC)"
	@go mod download
	@go mod tidy
	@echo "$(GREEN)Dependencies installed$(NC)"

# Install development tools
dev-tools: ## Install development tools
	@echo "$(YELLOW)Installing development tools...$(NC)"
	@go install github.com/swaggo/swag/cmd/swag@latest
	@go install github.com/cosmtrek/air@latest
	@echo "$(GREEN)Development tools installed$(NC)"

# Generate Swagger documentation
swagger: ## Generate Swagger documentation
	@echo "$(YELLOW)Generating Swagger documentation...$(NC)"
	@swag init -g $(MAIN_PATH) -o docs --parseDependency --parseInternal
	@echo "$(GREEN)Swagger documentation generated$(NC)"

# Run tests
test: ## Run tests
	@echo "$(YELLOW)Running tests...$(NC)"
	@go test -v ./...

# Run tests with coverage
test-coverage: ## Run tests with coverage report
	@echo "$(YELLOW)Running tests with coverage...$(NC)"
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

# Clean build artifacts
clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@echo "$(GREEN)Clean completed$(NC)"

# Docker commands for development services
docker-up: ## Start development services (Redis, PostgreSQL)
	@echo "$(YELLOW)Starting development services...$(NC)"
	@docker-compose -f $(DOCKER_COMPOSE_FILE) up -d
	@echo "$(GREEN)Development services started$(NC)"
	@echo "$(BLUE)Services available:$(NC)"
	@echo "  - Redis: localhost:6379"
	@echo "  - PostgreSQL: localhost:5432"
	@echo "  - Redis Commander: http://localhost:8081"
	@echo "  - pgAdmin: http://localhost:8082 (admin@cnpj-api.com / admin123)"

docker-down: ## Stop development services
	@echo "$(YELLOW)Stopping development services...$(NC)"
	@docker-compose -f $(DOCKER_COMPOSE_FILE) down
	@echo "$(GREEN)Development services stopped$(NC)"

docker-logs: ## Show logs from development services
	@docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f

docker-clean: ## Stop and remove all containers, networks, and volumes
	@echo "$(YELLOW)Cleaning up Docker resources...$(NC)"
	@docker-compose -f $(DOCKER_COMPOSE_FILE) down -v --remove-orphans
	@echo "$(GREEN)Docker cleanup completed$(NC)"

# Database commands
db-migrate: ## Run database migrations (placeholder)
	@echo "$(YELLOW)Running database migrations...$(NC)"
	@echo "$(BLUE)Migrations not implemented yet$(NC)"

# Full development setup
setup: deps dev-tools docker-up swagger ## Complete development setup
	@echo "$(GREEN)Development environment setup completed!$(NC)"
	@echo ""
	@echo "$(BLUE)Next steps:$(NC)"
	@echo "  1. Run 'make dev' to start the development server"
	@echo "  2. Visit http://localhost:8080/swagger for API documentation"
	@echo "  3. Use 'make docker-logs' to monitor service logs"

# Quick start for development
start: docker-up run ## Quick start: services + application

# Format code
fmt: ## Format Go code
	@echo "$(YELLOW)Formatting code...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)Code formatted$(NC)"

# Lint code
lint: ## Lint Go code
	@echo "$(YELLOW)Linting code...$(NC)"
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "$(RED)golangci-lint not found. Install it from https://golangci-lint.run/$(NC)"; \
	fi

# Security check
security: ## Run security checks
	@echo "$(YELLOW)Running security checks...$(NC)"
	@if command -v gosec > /dev/null; then \
		gosec ./...; \
	else \
		echo "$(RED)gosec not found. Installing...$(NC)"; \
		go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest; \
		gosec ./...; \
	fi

# NexConsult API Makefile

# Variáveis
APP_NAME=nexconsult-api
BINARY_DIR=bin
DOCKER_IMAGE=nexconsult-api
DOCKER_REGISTRY?=docker.io
DOCKER_USERNAME?=your-username
VERSION?=latest
FULL_IMAGE_NAME=$(DOCKER_REGISTRY)/$(DOCKER_USERNAME)/$(DOCKER_IMAGE)

# Comandos Go
.PHONY: build
build:
	@echo "🔨 Building application..."
	@mkdir -p $(BINARY_DIR)
	@go build -o $(BINARY_DIR)/$(APP_NAME) cmd/server/main.go
	@echo "✅ Build completed: $(BINARY_DIR)/$(APP_NAME)"

.PHONY: run
run: build
	@echo "🚀 Starting application..."
	@./$(BINARY_DIR)/$(APP_NAME)

.PHONY: test
test:
	@echo "🧪 Running tests..."
	@go test -v ./...

.PHONY: clean
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf $(BINARY_DIR)
	@rm -f *.html *.log
	@echo "✅ Clean completed"

.PHONY: deps
deps:
	@echo "📦 Installing dependencies..."
	@go mod download
	@go mod tidy

# Comandos Docker
.PHONY: docker-build
docker-build:
	@echo "🐳 Building Docker image..."
	@docker build -t $(DOCKER_IMAGE):$(VERSION) .
	@echo "✅ Docker image built: $(DOCKER_IMAGE):$(VERSION)"

.PHONY: docker-build-hub
docker-build-hub:
	@echo "🐳 Building Docker image for Docker Hub..."
	@docker build -t $(FULL_IMAGE_NAME):$(VERSION) -t $(FULL_IMAGE_NAME):latest .
	@echo "✅ Docker image built: $(FULL_IMAGE_NAME):$(VERSION)"

.PHONY: docker-tag
docker-tag:
	@echo "🏷️  Tagging Docker image for Docker Hub..."
	@docker tag $(DOCKER_IMAGE):$(VERSION) $(FULL_IMAGE_NAME):$(VERSION)
	@docker tag $(DOCKER_IMAGE):$(VERSION) $(FULL_IMAGE_NAME):latest
	@echo "✅ Image tagged: $(FULL_IMAGE_NAME):$(VERSION) and $(FULL_IMAGE_NAME):latest"

.PHONY: docker-push
docker-push:
	@echo "📤 Pushing Docker image to Docker Hub..."
	@docker push $(FULL_IMAGE_NAME):$(VERSION)
	@docker push $(FULL_IMAGE_NAME):latest
	@echo "✅ Image pushed to Docker Hub: $(FULL_IMAGE_NAME)"

.PHONY: docker-login
docker-login:
	@echo "🔐 Logging into Docker Hub..."
	@docker login $(DOCKER_REGISTRY)

.PHONY: docker-publish
docker-publish: docker-build-hub docker-push
	@echo "🚀 Docker image published to Docker Hub!"

.PHONY: docker-run
docker-run:
	@echo "🐳 Running Docker container..."
	@docker run -p 3000:3000 --env-file .env $(DOCKER_IMAGE):$(VERSION)

.PHONY: docker-compose-up
docker-compose-up:
	@echo "🐳 Starting with Docker Compose..."
	@docker-compose up -d
	@echo "✅ Services started"

.PHONY: docker-compose-down
docker-compose-down:
	@echo "🐳 Stopping Docker Compose services..."
	@docker-compose down
	@echo "✅ Services stopped"

.PHONY: docker-compose-logs
docker-compose-logs:
	@docker-compose logs -f

# Comandos de desenvolvimento
.PHONY: dev
dev:
	@echo "🔄 Starting development mode..."
	@go run cmd/server/main.go

.PHONY: lint
lint:
	@echo "🔍 Running linter..."
	@golangci-lint run

.PHONY: format
format:
	@echo "🎨 Formatting code..."
	@go fmt ./...
	@goimports -w .

# Comandos de deploy
.PHONY: deploy-build
deploy-build: clean deps test build
	@echo "🚀 Production build completed"

.PHONY: health-check
health-check:
	@echo "🏥 Checking API health..."
	@curl -f http://localhost:3000/health || echo "❌ Health check failed"

# Help
.PHONY: help
help:
	@echo "NexConsult API - Available commands:"
	@echo ""
	@echo "Development:"
	@echo "  make build          - Build the application"
	@echo "  make run            - Build and run the application"
	@echo "  make dev            - Run in development mode"
	@echo "  make test           - Run tests"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Install dependencies"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build        - Build Docker image"
	@echo "  make docker-build-hub    - Build Docker image for Docker Hub"
	@echo "  make docker-tag          - Tag image for Docker Hub"
	@echo "  make docker-push         - Push image to Docker Hub"
	@echo "  make docker-login        - Login to Docker Hub"
	@echo "  make docker-publish      - Build and publish to Docker Hub"
	@echo "  make docker-run          - Run Docker container"
	@echo "  make docker-compose-up   - Start with Docker Compose"
	@echo "  make docker-compose-down - Stop Docker Compose"
	@echo "  make docker-compose-logs - View logs"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint           - Run linter"
	@echo "  make format         - Format code"
	@echo ""
	@echo "Deploy:"
	@echo "  make deploy-build   - Production build"
	@echo "  make health-check   - Check API health"

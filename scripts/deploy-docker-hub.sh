#!/bin/bash

# Script para deploy da imagem NexConsult no Docker Hub
# Uso: ./scripts/deploy-docker-hub.sh [versão] [username]

set -e

# Cores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Função para log colorido
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Verificar se Docker está instalado e rodando
check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker não está instalado!"
        exit 1
    fi

    if ! docker info &> /dev/null; then
        log_error "Docker não está rodando!"
        exit 1
    fi

    log_success "Docker está disponível"
}

# Verificar se está logado no Docker Hub
check_docker_login() {
    if ! docker info | grep -q "Username:"; then
        log_warning "Você não está logado no Docker Hub"
        log_info "Executando docker login..."
        docker login
    else
        log_success "Já está logado no Docker Hub"
    fi
}

# Função principal
main() {
    local version=${1:-"latest"}
    local username=${2:-""}
    
    log_info "🚀 Iniciando deploy da imagem NexConsult para Docker Hub"
    echo "=================================================="
    
    # Verificações iniciais
    check_docker
    check_docker_login
    
    # Solicitar username se não fornecido
    if [ -z "$username" ]; then
        echo -n "Digite seu username do Docker Hub: "
        read username
        if [ -z "$username" ]; then
            log_error "Username é obrigatório!"
            exit 1
        fi
    fi
    
    log_info "Configurações:"
    echo "  - Versão: $version"
    echo "  - Username: $username"
    echo "  - Imagem: $username/nexconsult-api"
    echo ""
    
    # Confirmar antes de prosseguir
    echo -n "Deseja continuar? (y/N): "
    read confirm
    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        log_warning "Deploy cancelado pelo usuário"
        exit 0
    fi
    
    # Executar build e push
    log_info "Executando build da imagem..."
    DOCKER_USERNAME="$username" VERSION="$version" make docker-build-hub
    
    log_info "Fazendo push para Docker Hub..."
    DOCKER_USERNAME="$username" VERSION="$version" make docker-push
    
    log_success "🎉 Deploy concluído com sucesso!"
    echo ""
    echo "Sua imagem está disponível em:"
    echo "  - docker pull $username/nexconsult-api:$version"
    echo "  - docker pull $username/nexconsult-api:latest"
    echo ""
    echo "Para usar a imagem:"
    echo "  docker run -p 3000:3000 -e CAPTCHA_API_KEY=sua_chave $username/nexconsult-api:$version"
}

# Verificar se o script está sendo executado do diretório correto
if [ ! -f "Makefile" ] || [ ! -f "Dockerfile" ]; then
    log_error "Execute este script a partir do diretório raiz do projeto!"
    exit 1
fi

# Executar função principal
main "$@"

#!/bin/bash

# Script para deploy do NexConsult API no Docker Swarm
# Uso: ./deploy-stack.sh

echo "🚀 Fazendo deploy do NexConsult API..."

# Criar volume se não existir
docker volume create nexconsult_logs 2>/dev/null || true

# Criar rede se não existir
docker network create --driver overlay --attachable Gacont 2>/dev/null || true

# Deploy do stack
docker stack deploy -c docker-stack.yml nexconsult

echo "✅ Deploy concluído!"
echo ""
echo "Verificar status: docker stack services nexconsult"
echo "Ver logs: docker service logs nexconsult_nexconsult-api"
echo "Acessar API: https://nexconsult-api.gacont.com.br/health"

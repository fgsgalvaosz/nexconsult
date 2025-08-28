#!/bin/bash

# Script para deploy completo: Traefik + Portainer + NexConsult API
# Baseado no SetupOrion

echo "🚀 Iniciando deploy completo dos stacks..."
echo "=================================================="

# Verificar se está em um swarm
if ! docker info --format '{{.Swarm.LocalNodeState}}' | grep -q "active"; then
    echo "❌ Este node não está em um Docker Swarm ativo!"
    echo "Para inicializar um swarm, execute: docker swarm init"
    exit 1
fi

echo "✅ Docker Swarm está ativo"

# Criar rede se não existir
if ! docker network ls --filter name=Gacont --format "{{.Name}}" | grep -q "^Gacont$"; then
    echo "🔧 Criando rede 'Gacont'..."
    docker network create --driver overlay --attachable Gacont
    echo "✅ Rede 'Gacont' criada"
else
    echo "✅ Rede 'Gacont' já existe"
fi

# Criar volumes necessários
echo "🔧 Criando volumes..."
docker volume create traefik_certificates 2>/dev/null || true
docker volume create portainer_data 2>/dev/null || true
docker volume create nexconsult_logs 2>/dev/null || true
echo "✅ Volumes criados"

# Deploy do Traefik (primeiro)
echo ""
echo "1️⃣ Fazendo deploy do Traefik..."
docker stack deploy -c traefik-stack.yml traefik
echo "✅ Traefik deployado"

# Aguardar Traefik inicializar
echo "⏳ Aguardando Traefik inicializar (30s)..."
sleep 30

# Deploy do Portainer
echo ""
echo "2️⃣ Fazendo deploy do Portainer..."
docker stack deploy -c portainer-stack.yml portainer
echo "✅ Portainer deployado"

# Aguardar Portainer inicializar
echo "⏳ Aguardando Portainer inicializar (30s)..."
sleep 30

# Deploy do NexConsult API
echo ""
echo "3️⃣ Fazendo deploy do NexConsult API..."
docker stack deploy -c docker-stack.yml nexconsult
echo "✅ NexConsult API deployado"

echo ""
echo "🎉 Deploy completo finalizado!"
echo "=================================================="
echo ""
echo "📋 Serviços disponíveis:"
echo "  🔀 Traefik Dashboard: https://traefik.gacont.com.br"
echo "  🐳 Portainer:        https://portainer.gacont.com.br"
echo "  🚀 NexConsult API:   https://nexconsult.gacont.com.br"
echo ""
echo "📊 Verificar status dos stacks:"
echo "  docker stack ls"
echo "  docker stack services traefik"
echo "  docker stack services portainer"
echo "  docker stack services nexconsult"
echo ""
echo "📝 Ver logs:"
echo "  docker service logs traefik_traefik"
echo "  docker service logs portainer_portainer"
echo "  docker service logs nexconsult_nexconsult-api"
echo ""
echo "🔧 Para remover tudo:"
echo "  docker stack rm nexconsult portainer traefik"

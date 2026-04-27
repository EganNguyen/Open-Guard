#!/bin/bash
set -e

# ECS Shim for LocalStack Community Edition
# This script starts Docker containers that mimic ECS Tasks

SERVICE_NAME=$1
IMAGE=$2
PORT=$3
EXTRA_ENV=$4

if [ -z "$SERVICE_NAME" ] || [ -z "$IMAGE" ]; then
    echo "Usage: $0 <service-name> <image> [port] [extra-env-file]"
    exit 1
fi

echo "🚀 Starting $SERVICE_NAME (shim) on LocalStack network..."

# Get LocalStack container name
LS_CONTAINER=$(docker ps --format '{{.Names}}' | grep localstack | head -n 1)

if [ -z "$LS_CONTAINER" ]; then
    echo "❌ LocalStack container not found. Is LocalStack running?"
    exit 1
fi

# Get LocalStack container IP
LOCALSTACK_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$LS_CONTAINER" | tr -d '\r\n ')

# Dynamically discover core data service IPs on bridge network
KAFKA_IP=$(docker inspect -f '{{.NetworkSettings.Networks.bridge.IPAddress}}' docker-kafka-1 2>/dev/null | tr -d '\r\n ')
REDIS_IP=$(docker inspect -f '{{.NetworkSettings.Networks.bridge.IPAddress}}' docker-redis-1 2>/dev/null | tr -d '\r\n ')
POSTGRES_IP=$(docker inspect -f '{{.NetworkSettings.Networks.bridge.IPAddress}}' docker-postgres-1 2>/dev/null | tr -d '\r\n ')
MONGO_IP=$(docker inspect -f '{{.NetworkSettings.Networks.bridge.IPAddress}}' docker-mongo-primary-1 2>/dev/null | tr -d '\r\n ')
CLICKHOUSE_IP=$(docker inspect -f '{{.NetworkSettings.Networks.bridge.IPAddress}}' docker-clickhouse-1 2>/dev/null | tr -d '\r\n ')

# Fallback to Gateway if not found
GATEWAY_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}' "$LS_CONTAINER" | head -n 1 | tr -d '\r\n ')

KAFKA_ADDR=${KAFKA_IP:-$GATEWAY_IP}
REDIS_ADDR=${REDIS_IP:-$GATEWAY_IP}
POSTGRES_ADDR=${POSTGRES_IP:-$GATEWAY_IP}
MONGO_ADDR=${MONGO_IP:-$GATEWAY_IP}
CLICKHOUSE_ADDR=${CLICKHOUSE_IP:-$GATEWAY_IP}

DB_URL="postgres://openguard:change-me-in-production@$POSTGRES_ADDR:5432/openguard?sslmode=disable"

# Base configuration for all microservices
ENV_ARGS=(
    -e AWS_ACCESS_KEY_ID=test
    -e AWS_SECRET_ACCESS_KEY=test
    -e AWS_REGION=us-east-1
    -e USE_AWS_SECRETS_MANAGER=true
    -e AWS_SECRETSMANAGER_ENDPOINT="http://$LOCALSTACK_IP:4566"
    -e AWS_S3_ENDPOINT="http://$LOCALSTACK_IP:4566"
    -e DATABASE_URL="$DB_URL"
    -e POLICY_DATABASE_URL="$DB_URL"
    -e REDIS_URL="redis://$REDIS_ADDR:6379"
    -e KAFKA_BROKERS="$KAFKA_ADDR:9092"
    -e MONGO_URI="mongodb://$MONGO_ADDR:27017"
    -e CLICKHOUSE_ADDR="$CLICKHOUSE_ADDR:9000"
    -e INTERNAL_API_KEY="test-api-key"
    -e INFRA_MODE=localstack
    -v "$(pwd)/infra/certs:/certs:ro"
)

# Add extra env if provided
if [ -n "$EXTRA_ENV" ] && [ -f "$EXTRA_ENV" ]; then
    ENV_ARGS+=(--env-file "$EXTRA_ENV")
fi

docker run -d \
    --name "openguard-$SERVICE_NAME" \
    --network bridge \
    "${ENV_ARGS[@]}" \
    ${PORT:+-p $PORT:$PORT} \
    "$IMAGE"

echo "✅ $SERVICE_NAME is running."

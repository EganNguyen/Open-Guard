#!/bin/bash
set -e

# ECS Shim v3 (Strict Port & Network management)
SERVICE_NAME=$1
IMAGE=$2
PORT=$3

if [ -z "$SERVICE_NAME" ] || [ -z "$IMAGE" ] || [ -z "$PORT" ]; then
    echo "Usage: $0 <service-name> <image> <port>"
    exit 1
fi

echo "🚀 Starting $SERVICE_NAME (shim) on openguard-net..."

# Discovery for LocalStack (running in openguard-net)
LS_CONTAINER=$(docker ps --format '{{.Names}}' | grep localstack | head -n 1)

# On macOS, host.docker.internal is the most reliable way to reach services mapped to host ports
# We also use explicit container names for in-network communication
HOST_DNS="host.docker.internal"

# Database and Infrastructure connection strings
DB_URL="postgres://openguard:${POSTGRES_PASSWORD:-change-me-in-production}@docker-postgres-1:5432/openguard?sslmode=disable"
API_URL="${OPENGUARD_PUBLIC_URL:-https://localhost:8080}"

# Special logic for Dashboard
if [ "$SERVICE_NAME" = "dashboard" ]; then
    API_URL="/"
fi

# Cleanup existing
docker rm -f "openguard-$SERVICE_NAME" 2>/dev/null || true

# Use explicit container names for peer discovery
# All services listen on 8080 or 8082, but map to host $PORT
docker run -d \
    --name "openguard-$SERVICE_NAME" \
    --network openguard-net \
    --restart on-failure:5 \
    --add-host host.docker.internal:host-gateway \
    -e AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-test} \
    -e AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-test} \
    -e AWS_REGION=${AWS_DEFAULT_REGION:-us-east-1} \
    -e USE_AWS_SECRETS_MANAGER=true \
    -e AWS_SECRETSMANAGER_ENDPOINT="http://$LS_CONTAINER:4566" \
    -e AWS_S3_ENDPOINT="http://$LS_CONTAINER:4566" \
    -e DATABASE_URL="$DB_URL" \
    -e POLICY_DATABASE_URL="$DB_URL" \
    -e REDIS_URL="redis://docker-redis-1:6379" \
    -e KAFKA_BROKERS="docker-kafka-1:9092" \
    -e MONGO_URI="mongodb://docker-mongo-primary-1:27017" \
    -e CLICKHOUSE_ADDR="docker-clickhouse-1:9000" \
    -e INTERNAL_API_KEY="test-api-key" \
    -e INFRA_MODE=localstack \
    -e OPENGUARD_API_URL="$API_URL" \
    -v "$(pwd)/infra/certs:/certs:ro" \
    -p "$PORT:$PORT" \
    "$IMAGE"

echo "✅ $SERVICE_NAME is running on host port $PORT."

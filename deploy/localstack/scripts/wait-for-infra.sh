#!/bin/bash
set -e

# Data Tier Health Sentinel v5 (macOS Host-optimized)
# Verifies infrastructure availability using local ports mapped from Docker

wait_for_port() {
    local port=$1
    local service=$2
    echo "⌛ Waiting for $service (localhost:$port)..."
    # Using 'nc -z' on macOS to check if the port is open
    until nc -z localhost "$port"; do
        sleep 2
    done
    echo "✅ $service is ready!"
}

# 1. Wait for standard infrastructure ports
wait_for_port 5432 "PostgreSQL"
wait_for_port 6379 "Redis"
wait_for_port 9092 "Kafka"
wait_for_port 27017 "MongoDB"
wait_for_port 8123 "ClickHouse"

echo "🚀 Local infrastructure is healthy. Proceeding with LocalStack Pro."

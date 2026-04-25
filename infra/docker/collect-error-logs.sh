#!/bin/bash

# Comprehensive error log collection script for all Docker Compose containers
# Usage: ./collect-error-logs.sh > error-report.txt

OUTPUT_FILE="container-error-report-$(date +%Y%m%d-%H%M%S).log"
TEMP_FILE=$(mktemp)

echo "================================"
echo "DOCKER COMPOSE ERROR LOG REPORT"
echo "Generated: $(date)"
echo "================================" >> "$TEMP_FILE"
echo "" >> "$TEMP_FILE"

# List of all containers from docker compose
CONTAINERS=(
  "docker-kafka-1"
  "docker-localstack-1"
  "docker-postgres-1"
  "docker-zookeeper-1"
  "docker-prometheus-1"
  "docker-mongo-secondary-1-1"
  "docker-mongo-primary-1"
  "docker-mongo-secondary-2-1"
  "docker-redis-1"
  "docker-clickhouse-1"
  "docker-jaeger-1"
  "docker-mongo-init-1"
  "docker-promtail-1"
  "docker-kafka-init-1"
  "docker-gateway-1"
  "docker-grafana-1"
  "docker-loki-1"
  "docker-task-frontend-1"
  "docker-threat-1"
  "docker-policy-1"
  "docker-iam-1"
  "docker-connector-registry-1"
  "docker-audit-1"
  "docker-compliance-1"
  "docker-dlp-1"
  "docker-webhook-delivery-1"
  "docker-control-plane-1"
  "docker-alerting-1"
  "docker-task-backend-1"
  "docker-dashboard-1"
)

# Error patterns to catch
ERROR_PATTERNS="(ERROR|FATAL|PANIC|EXCEPTION|error|exception|fatal|panic|failed|failure|WARN.*error|warning.*error)"

for container in "${CONTAINERS[@]}"; do
  echo "" >> "$TEMP_FILE"
  echo "===============================================" >> "$TEMP_FILE"
  echo "CONTAINER: $container" >> "$TEMP_FILE"
  echo "===============================================" >> "$TEMP_FILE"
  echo "" >> "$TEMP_FILE"
  
  # Get container status
  STATUS=$(docker inspect --format='{{.State.Running}}' "$container" 2>/dev/null)
  if [ "$STATUS" = "true" ]; then
    echo "[Status: RUNNING]" >> "$TEMP_FILE"
  else
    EXIT_CODE=$(docker inspect --format='{{.State.ExitCode}}' "$container" 2>/dev/null)
    echo "[Status: EXITED] [Exit Code: $EXIT_CODE]" >> "$TEMP_FILE"
  fi
  echo "" >> "$TEMP_FILE"
  
  # Collect errors
  ERRORS=$(docker logs "$container" 2>&1 | grep -iE "$ERROR_PATTERNS" | head -30)
  
  if [ -z "$ERRORS" ]; then
    echo "✓ No errors detected in logs" >> "$TEMP_FILE"
  else
    echo "⚠ ERRORS FOUND:" >> "$TEMP_FILE"
    echo "$ERRORS" >> "$TEMP_FILE"
  fi
  echo "" >> "$TEMP_FILE"
done

# Summary section
echo "" >> "$TEMP_FILE"
echo "===============================================" >> "$TEMP_FILE"
echo "SUMMARY" >> "$TEMP_FILE"
echo "===============================================" >> "$TEMP_FILE"
echo "" >> "$TEMP_FILE"

RUNNING_COUNT=$(docker ps --filter label=com.docker.compose.project=docker -q 2>/dev/null | wc -l)
EXITED_COUNT=$(docker ps -a --filter label=com.docker.compose.project=docker --filter status=exited -q 2>/dev/null | wc -l)

echo "Total Running Containers: $RUNNING_COUNT" >> "$TEMP_FILE"
echo "Total Exited Containers: $EXITED_COUNT" >> "$TEMP_FILE"
echo "" >> "$TEMP_FILE"

# Container exit status summary
echo "CONTAINER STATUS:" >> "$TEMP_FILE"
for container in "${CONTAINERS[@]}"; do
  STATUS=$(docker inspect --format='{{.State.Running}}' "$container" 2>/dev/null)
  if [ "$STATUS" = "true" ]; then
    echo "  ✓ $container [RUNNING]" >> "$TEMP_FILE"
  else
    EXIT_CODE=$(docker inspect --format='{{.State.ExitCode}}' "$container" 2>/dev/null)
    echo "  ✗ $container [EXITED] Exit Code: $EXIT_CODE" >> "$TEMP_FILE"
  fi
done

# Copy to final location
cp "$TEMP_FILE" "$OUTPUT_FILE"
cat "$TEMP_FILE"
rm "$TEMP_FILE"

echo ""
echo "Report saved to: $OUTPUT_FILE"

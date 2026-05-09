#!/usr/bin/env bash
# Pre-pull a list of container images with retries and backoff.
# This script is intended for local developer machines and CI to
# reduce transient TLS/download errors when docker compose pulls
# many images in parallel.

set -euo pipefail
IFS=$'\n\t'

RETRIES=${RETRIES:-3}
BACKOFF_BASE=${BACKOFF_BASE:-2}

# Default image list. Update this list as infra images change.
IMAGES=(
  "grafana/loki:2.8.2"
  "prom/prometheus:v2.44.0"
  "confluentinc/cp-kafka:7.4.0"
  "jaegertracing/all-in-one:1.45"
  "mongo:6.0"
  "clickhouse/clickhouse-server:23.3-alpine"
)

echo "[prepull] Starting pre-pull for ${#IMAGES[@]} images"

for img in "${IMAGES[@]}"; do
  echo "[prepull] Image: $img"
  attempt=1
  until [ $attempt -gt "$RETRIES" ]; do
    echo "[prepull] Attempt $attempt for $img"
    if docker pull "$img"; then
      echo "[prepull] Pulled $img"
      break
    fi
    attempt=$((attempt + 1))
    sleep_time=$(( BACKOFF_BASE ** (attempt - 1) ))
    echo "[prepull] Pull failed for $img, retrying in ${sleep_time}s..."
    sleep "$sleep_time"
  done
  if [ $attempt -gt "$RETRIES" ]; then
    echo "[prepull] WARN: Failed to pull $img after $RETRIES attempts — proceeding anyway"
  fi
done

echo "[prepull] Complete"

exit 0

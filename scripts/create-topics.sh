#!/bin/bash
# OpenGuard Kafka Topic Bootstrap Script
# Standardizes topic partitions and replication per spec §12.1

set -e

KAFKA_SERVER=${KAFKA_BROKERS:-localhost:9092}

# Detect number of brokers to adjust replication factor
# If we can't detect, default to 1 for safety
BROKERS=$(kafka-broker-api-versions.sh --bootstrap-server "$KAFKA_SERVER" 2>/dev/null | grep -c "id:" || echo 0)
if [ "$BROKERS" -eq 0 ]; then
  # Fallback: check if we are in docker and try to detect there
  BROKERS=$(docker exec kafka kafka-broker-api-versions.sh --bootstrap-server localhost:9092 2>/dev/null | grep -c "id:" || echo 1)
fi

REPLICATION=$([ "$BROKERS" -ge 3 ] && echo 3 || echo 1)
echo "Detected $BROKERS brokers, using replication-factor=$REPLICATION"

TOPICS=(
  "auth.events:12"
  "policy.changes:6"
  "data.access:24"
  "threat.alerts:12"
  "audit.trail:24"
  "notifications.outbound:6"
  "saga.orchestration:12"
  "outbox.dlq:3"
  "connector.events:24"
  "webhook.delivery:12"
  "webhook.dlq:3"
)

for TOPIC_DEF in "${TOPICS[@]}"; do
  TOPIC="${TOPIC_DEF%%:*}"
  PARTITIONS="${TOPIC_DEF##*:}"
  
  echo "Ensuring topic: $TOPIC ($PARTITIONS partitions, replication $REPLICATION)..."
  
  if command -v kafka-topics >/dev/null 2>&1; then
    kafka-topics --bootstrap-server "$KAFKA_SERVER" \
      --create --if-not-exists \
      --topic "$TOPIC" \
      --partitions "$PARTITIONS" \
      --replication-factor "$REPLICATION" \
      --config compression.type=lz4
  else
    # Fallback to docker exec
    docker exec kafka kafka-topics --bootstrap-server localhost:9092 \
      --create --if-not-exists \
      --topic "$TOPIC" \
      --partitions "$PARTITIONS" \
      --replication-factor "$REPLICATION" \
      --config compression.type=lz4
  fi
  echo "Created/Verified topic: $TOPIC"
done

echo "✅ Topic bootstrap complete."


#!/usr/bin/env bash

# OpenGuard Kafka Topic Bootstrap Script
# Standardizes topic partitions and replication per spec §12.1

set -e

BROKERS=${KAFKA_BROKERS:-localhost:9092}
REPLICATION=${KAFKA_REPLICATION_FACTOR:-1} # Default to 1 for dev, override to 3 in prod

# Format: "topic:partitions"
TOPICS=(
    "auth.events:12"
    "policy.changes:6"
    "data.access:24"
    "threat.alerts:12"
    "audit.trail:24"
    "connector.events:24"
    "webhook.delivery:12"
    "webhook.dlq:3"
    "notifications.outbound:6"
    "saga.orchestration:12"
    "outbox.dlq:3"
)

echo "🚀 Bootstrapping OpenGuard Kafka Topics..."

for topic_spec in "${TOPICS[@]}"; do
    TOPIC_NAME=$(echo $topic_spec | cut -d':' -f1)
    PARTITIONS=$(echo $topic_spec | cut -d':' -f2)
    
    echo "Creating topic: $TOPIC_NAME ($PARTITIONS partitions, replication $REPLICATION)..."
    
    # We use the confluent-kafka image's CLI if available, or assume kafka-topics is in PATH
    if command -v kafka-topics >/dev/null 2>&1; then
        kafka-topics --create --bootstrap-server "$BROKERS" \
            --topic "$TOPIC_NAME" \
            --partitions "$PARTITIONS" \
            --replication-factor "$REPLICATION" \
            --if-not-exists
    else
        # Fallback to docker exec if running in local dev
        docker exec -it kafka kafka-topics --create --bootstrap-server localhost:9092 \
            --topic "$TOPIC_NAME" \
            --partitions "$PARTITIONS" \
            --replication-factor "$REPLICATION" \
            --if-not-exists || echo "⚠️ Warning: Failed to create topic $TOPIC_NAME. Is Kafka running?"
    fi
done

echo "✅ Topic bootstrap complete."

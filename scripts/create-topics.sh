#!/bin/bash
set -e

echo "Creating Kafka topics from infra/kafka/topics.json..."

# Topics
docker exec openguard-kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic auth.events --partitions 6 --config retention.ms=604800000
docker exec openguard-kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic policy.changes --partitions 3 --config retention.ms=604800000
docker exec openguard-kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic data.access --partitions 6 --config retention.ms=604800000
docker exec openguard-kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic threat.alerts --partitions 3 --config retention.ms=2592000000
docker exec openguard-kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic audit.trail --partitions 6 --config retention.ms=-1
docker exec openguard-kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic notifications.outbound --partitions 3 --config retention.ms=604800000

echo "Kafka topics created successfully."

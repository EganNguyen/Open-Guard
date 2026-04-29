# Runbook: Kafka Consumer Lag

## Context
High consumer lag means a service is falling behind in processing events (e.g. Audit, Threat Detection, Webhook Delivery).

## Steps
1. Identify the lagging consumer group using Grafana (`kafka_consumer_group_lag`).
2. Check the logs for the specific service (e.g., `audit`) for repeated errors or crashes.
3. If the service is healthy but slow, scale up the consumer deployment (increase replicas).
4. If a single partition is lagging, check for hot keys (skewed data).
5. If the issue is due to a bad message (poison pill), manually advance the consumer offset or clear the DLQ.

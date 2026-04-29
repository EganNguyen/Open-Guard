# Audit Service

**Core Intent:** Processes outbox events from Kafka and writes compliance logs to MongoDB.
**Key Files:**
- `pkg/consumer/consumer.go`: Kafka message routing.
- `pkg/repository/repository.go`: MongoDB inserts.

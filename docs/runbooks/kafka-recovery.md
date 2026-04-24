# Runbook: Kafka Recovery

Procedures for recovering from Kafka broker failure or consumer group lag.

## 1. Broker Failure
- Kafka is configured with `offsets.topic.replication.factor: 3`.
- If one broker fails, the cluster continues with 2 replicas.
- **Recovery**: Replace the failed node; Kafka will re-sync data automatically.

## 2. Consumer Group Lag
If a service (e.g. `dlp-service`) is lagging:
1. Identify lag: `kafka-consumer-groups --bootstrap-server localhost:9092 --group dlp-service --describe`
2. **Action**: Increase partitions and scale up consumer instances.
3. **Emergency Reset**: If messages are corrupt or unprocessable:
   ```bash
   kafka-consumer-groups --bootstrap-server localhost:9092 --group dlp-service --topic control.plane.events --reset-offsets --to-latest --execute
   ```

## 3. Topic Deletion Recovery
If a topic is accidentally deleted:
1. Re-create topic with proper replication.
2. If data is lost, trigger a "Re-sync" event if the services support it (e.g. audit log re-ingestion from cold storage).

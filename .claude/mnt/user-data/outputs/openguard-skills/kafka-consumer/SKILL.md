---
name: openguard-kafka-consumer
description: >
  Use this skill when writing or reviewing any Kafka consumer in OpenGuard.
  Triggers: "consume from topic", "audit consumer", "bulk writer", "manual offset
  commit", "process Kafka messages", "audit log ingestion", "ClickHouse writer",
  "hash chain", "bulk insert to MongoDB", "DLQ routing", "saga consumer", or
  any code in services/audit/pkg/consumer/, services/compliance/, services/threat/,
  services/alerting/, or services/dlp/ that reads from Kafka. The manual offset
  commit contract is the most critical correctness invariant in the system —
  violating it causes permanent, undetectable audit gaps.
---

# OpenGuard Kafka Consumer Skill

The audit guarantee of OpenGuard depends entirely on a single invariant:
**Kafka offsets are committed ONLY after a successful downstream write.**
Every consumer in this codebase must honor this contract without exception.

---

## 1. The Offset Commit Contract

```
Consumer reads message batch
  → Process batch (write to MongoDB / ClickHouse / PostgreSQL / Redis)
    → On SUCCESS:  CommitOffsets()
    → On FAILURE:  do NOT commit — retry or route to DLQ
```

**Why this matters:** If the service crashes after a successful write but before
offset commit, the messages are reprocessed on restart. This is safe because all
downstream writes are idempotent (MongoDB `event_id` unique index, Kafka idempotent
producer, ClickHouse dedup). If you commit offsets before the write and then crash,
those events are gone forever — undetectable audit gaps.

**Never use auto-commit mode.** Never commit inside the processing loop before
verifying the write succeeded.

---

## 2. Consumer Skeleton (Mandatory Structure)

Every consumer follows this structure. Adapt the processing logic; keep the shell:

```go
// pkg/consumer/consumer.go
package consumer

import (
    "context"
    "log/slog"
    "time"

    "github.com/confluentinc/confluent-kafka-go/kafka"
    "golang.org/x/sync/errgroup"
)

type Consumer struct {
    client     *kafka.Consumer
    writer     BulkWriter       // interface defined in THIS package
    logger     *slog.Logger
    maxDocs    int
    flushAfter time.Duration
}

func NewConsumer(cfg ConsumerConfig, writer BulkWriter, logger *slog.Logger) *Consumer {
    if writer == nil {
        panic("NewConsumer: writer is required")
    }
    c, err := kafka.NewConsumer(&kafka.ConfigMap{
        "bootstrap.servers":        cfg.Brokers,
        "group.id":                 cfg.GroupID,
        "auto.offset.reset":        "earliest",
        "enable.auto.commit":       false,   // MANDATORY: manual commit mode
        "enable.auto.offset.store": false,   // Don't store offsets automatically
    })
    if err != nil {
        panic(fmt.Sprintf("create kafka consumer: %v", err))
    }
    return &Consumer{
        client:     c,
        writer:     writer,
        logger:     logger,
        maxDocs:    cfg.MaxDocs,
        flushAfter: cfg.FlushAfter,
    }
}

func (c *Consumer) Run(ctx context.Context) error {
    if err := c.client.Subscribe(c.topic, nil); err != nil {
        return fmt.Errorf("subscribe: %w", err)
    }
    defer func() {
        if err := c.client.Close(); err != nil {
            c.logger.ErrorContext(ctx, "consumer close failed", "error", err)
        }
    }()

    ticker := time.NewTicker(c.flushAfter)
    defer ticker.Stop()

    var buffer []kafka.Message

    for {
        select {
        case <-ctx.Done():
            // Flush remaining buffer before exit
            if len(buffer) > 0 {
                if err := c.flushAndCommit(ctx, buffer); err != nil {
                    c.logger.ErrorContext(ctx, "final flush failed", "error", err)
                }
            }
            return ctx.Err()

        case <-ticker.C:
            if len(buffer) > 0 {
                if err := c.flushAndCommit(ctx, buffer); err != nil {
                    c.logger.ErrorContext(ctx, "flush failed, will retry on next tick", "error", err)
                    continue // do NOT clear buffer — retry next tick
                }
                buffer = buffer[:0]
            }

        default:
            msg, err := c.client.ReadMessage(10 * time.Millisecond)
            if err != nil {
                if err.(kafka.Error).Code() == kafka.ErrTimedOut {
                    continue // no message available; non-fatal
                }
                c.logger.ErrorContext(ctx, "read message failed", "error", err)
                continue
            }
            buffer = append(buffer, *msg)
            if len(buffer) >= c.maxDocs {
                if err := c.flushAndCommit(ctx, buffer); err != nil {
                    c.logger.ErrorContext(ctx, "flush at capacity failed", "error", err)
                    continue
                }
                buffer = buffer[:0]
            }
        }
    }
}

// flushAndCommit writes the batch downstream then commits offsets.
// The order is critical: write FIRST, commit AFTER.
// If write fails: return error, do NOT commit. Caller retries.
// If commit fails: log error. Messages may be reprocessed — that's acceptable
// because all writes are idempotent.
func (c *Consumer) flushAndCommit(ctx context.Context, msgs []kafka.Message) error {
    // Step 1: Parse and add to bulk writer
    for _, msg := range msgs {
        event, err := parseEnvelope(msg.Value)
        if err != nil {
            c.logger.ErrorContext(ctx, "failed to parse envelope, routing to DLQ",
                "offset", msg.TopicPartition.Offset,
                "error", err,
            )
            // Route unparseable messages to DLQ without blocking the batch
            c.routeToDLQ(ctx, msg, err)
            continue
        }
        c.writer.Add(event)
    }

    // Step 2: Flush to downstream store (MongoDB, ClickHouse, etc.)
    // If this fails, we return the error WITHOUT committing offsets.
    if err := c.writer.Flush(ctx); err != nil {
        return fmt.Errorf("flush batch: %w", err)  // caller will retry
    }

    // Step 3: Commit offsets ONLY after successful flush
    // Use the last message's offset from each partition
    offsets := topicPartitionOffsets(msgs)
    if _, err := c.client.CommitOffsets(offsets); err != nil {
        // Commit failed — log but don't return error.
        // Messages will be reprocessed; idempotent writes make this safe.
        c.logger.ErrorContext(ctx, "offset commit failed (will reprocess)",
            "error", err,
            "batch_size", len(msgs),
        )
    }
    return nil
}
```

---

## 3. MongoDB Bulk Writer

```go
// pkg/consumer/bulk_writer.go
package consumer

type BulkWriter struct {
    coll    *mongo.Collection
    mu      sync.Mutex
    buffer  []mongo.WriteModel
    maxDocs int
}

// Add appends a document to the buffer. Thread-safe.
// The consumer's Run loop owns the flush timing — Add never flushes.
func (b *BulkWriter) Add(event AuditEvent) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.buffer = append(b.buffer,
        mongo.NewInsertOneModel().SetDocument(event),
    )
}

// Flush writes all buffered documents to MongoDB as a single BulkWrite.
// ordered=false: continues past duplicate key errors (idempotent reprocessing).
// Returns non-nil error ONLY for non-duplicate failures.
// The consumer commits Kafka offsets AFTER this returns nil.
func (b *BulkWriter) Flush(ctx context.Context) error {
    b.mu.Lock()
    if len(b.buffer) == 0 {
        b.mu.Unlock()
        return nil
    }
    docs := b.buffer
    // Reset buffer before releasing lock — new Add() calls go to fresh buffer
    b.buffer = make([]mongo.WriteModel, 0, b.maxDocs)
    b.mu.Unlock()

    opts := options.BulkWrite().SetOrdered(false)
    _, err := b.coll.BulkWrite(ctx, docs, opts)
    if err != nil {
        var bulkErr mongo.BulkWriteException
        if errors.As(err, &bulkErr) {
            for _, we := range bulkErr.WriteErrors {
                if we.Code != 11000 { // 11000 = duplicate key (E11000)
                    // Non-duplicate error: signal failure so consumer does NOT commit
                    return fmt.Errorf("bulk write non-duplicate error: %w", err)
                }
            }
            // All failures were duplicates — idempotent; safe to commit offsets
            return nil
        }
        return fmt.Errorf("bulk write failed: %w", err)
    }
    return nil
}
```

**Why `ordered=false`:** On reprocessing after a crash, some events in the batch
were already written. With `ordered=true`, the first duplicate stops the entire
batch. With `ordered=false`, duplicates are skipped and the rest are inserted.
Error handling then filters out E11000 errors as expected, non-failures.

---

## 4. ClickHouse Bulk Writer (Compliance Service)

```go
// pkg/consumer/clickhouse_writer.go
package consumer

type ClickHouseWriter struct {
    conn    driver.Conn
    mu      sync.Mutex
    buffer  []AuditEvent
    maxRows int
}

func (w *ClickHouseWriter) Add(event AuditEvent) {
    w.mu.Lock()
    defer w.mu.Unlock()
    w.buffer = append(w.buffer, event)
}

// Flush uses the native batch API — not string-built INSERT queries.
// Kafka offsets are committed by the consumer AFTER this returns nil.
func (w *ClickHouseWriter) Flush(ctx context.Context) error {
    w.mu.Lock()
    if len(w.buffer) == 0 {
        w.mu.Unlock()
        return nil
    }
    events := w.buffer
    w.buffer = make([]AuditEvent, 0, w.maxRows)
    w.mu.Unlock()

    batch, err := w.conn.PrepareBatch(ctx, "INSERT INTO events")
    if err != nil {
        return fmt.Errorf("prepare batch: %w", err)
    }
    for _, e := range events {
        if err := batch.Append(
            e.EventID, e.Type, e.OrgID, e.ActorID,
            e.ActorType, e.OccurredAt, e.Source, string(e.Payload),
        ); err != nil {
            return fmt.Errorf("append event %s: %w", e.EventID, err)
        }
    }
    if err := batch.Send(); err != nil {
        return fmt.Errorf("send batch: %w", err)
    }
    return nil
}
```

---

## 5. Hash Chain (Audit Integrity)

The audit hash chain assigns each event a globally-ordered sequence number per org
and chains events cryptographically so tampering is detectable.

```go
// pkg/consumer/hash_chain.go
package consumer

// AssignChainSequence atomically reserves a range of sequence numbers for a batch
// of events belonging to the same org. This avoids per-event DB round trips.
//
// Implementation: findOneAndUpdate with $inc on the batch size.
// Returns: (firstSeq, prevHash, error)
//   firstSeq: the first reserved sequence number
//   prevHash: the hash of the event at (firstSeq - 1) — the chain anchor
func AssignChainSequence(
    ctx context.Context,
    chainState *mongo.Collection,
    orgID string,
    batchSize int,
) (firstSeq int64, prevHash string, err error) {
    // Pipeline update: atomically increment by batchSize, capture previous last_hash
    result := chainState.FindOneAndUpdate(
        ctx,
        bson.M{"_id": orgID},
        bson.A{
            bson.M{"$set": bson.M{
                "prev_hash": "$last_hash",  // capture before increment
                "seq":       bson.M{"$add": bson.A{"$seq", batchSize}},
            }},
        },
        options.FindOneAndUpdate().
            SetUpsert(true).
            SetReturnDocument(options.Before), // return doc BEFORE update
    )
    if result.Err() != nil && !errors.Is(result.Err(), mongo.ErrNoDocuments) {
        return 0, "", fmt.Errorf("reserve chain sequence: %w", result.Err())
    }

    var state struct {
        Seq      int64  `bson:"seq"`
        LastHash string `bson:"last_hash"`
    }
    if err := result.Decode(&state); err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
        return 0, "", fmt.Errorf("decode chain state: %w", err)
    }
    // state.Seq is the sequence BEFORE this batch was reserved
    return state.Seq + 1, state.LastHash, nil
}

// ChainHash computes the HMAC-SHA256 linking this event to the previous one.
// Input fields are fixed — changing them breaks existing chain verification.
func ChainHash(secret, prevHash string, event AuditEvent) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(prevHash))
    mac.Write([]byte(event.EventID))
    mac.Write([]byte(event.OrgID))
    mac.Write([]byte(event.Type))
    mac.Write([]byte(strconv.FormatInt(event.OccurredAt.Unix(), 10)))
    return hex.EncodeToString(mac.Sum(nil))
}

// AssignChainToEvents assigns sequence numbers and computes hashes for a batch.
// All events in the batch belong to the same org_id.
// After this function, update audit_chain_state.last_hash to the last event's hash.
func AssignChainToEvents(
    ctx context.Context,
    chainState *mongo.Collection,
    secret string,
    orgID string,
    events []AuditEvent,
) error {
    firstSeq, prevHash, err := AssignChainSequence(ctx, chainState, orgID, len(events))
    if err != nil {
        return err
    }
    for i := range events {
        events[i].ChainSeq = firstSeq + int64(i)
        events[i].ChainHash = ChainHash(secret, prevHash, events[i])
        prevHash = events[i].ChainHash
    }
    // Update the chain state's last_hash to point to the end of this batch
    _, err = chainState.UpdateOne(ctx,
        bson.M{"_id": orgID},
        bson.M{"$set": bson.M{"last_hash": prevHash}},
    )
    return err
}
```

---

## 6. Dead Letter Queue (DLQ) Routing

```go
// Route a failed message to DLQ without blocking the batch.
// DLQ failure is logged but not fatal — the original message was already
// unprocessable; losing the DLQ copy is acceptable vs. stalling the consumer.
func (c *Consumer) routeToDLQ(ctx context.Context, msg kafka.Message, reason error) {
    dlqPayload, _ := json.Marshal(map[string]any{
        "original_topic":     *msg.TopicPartition.Topic,
        "original_offset":    msg.TopicPartition.Offset,
        "original_partition": msg.TopicPartition.Partition,
        "original_payload":   string(msg.Value),
        "error":              reason.Error(),
        "failed_at":          time.Now().UTC(),
    })
    if err := c.dlqProducer.Produce(ctx, kafka.TopicOutboxDLQ, "", dlqPayload); err != nil {
        c.logger.ErrorContext(ctx, "failed to route to DLQ",
            "original_offset", msg.TopicPartition.Offset,
            "error", err,
        )
    }
}
```

---

## 7. Envelope Parsing and Schema Validation

```go
// Every consumer must validate SchemaVer before processing.
func parseEnvelope(raw []byte) (*models.EventEnvelope, error) {
    var env models.EventEnvelope
    if err := json.Unmarshal(raw, &env); err != nil {
        return nil, fmt.Errorf("unmarshal envelope: %w", err)
    }
    if env.SchemaVer != "1.0" {
        return nil, fmt.Errorf("unsupported schema version %q", env.SchemaVer)
    }
    if env.ID == "" || env.OrgID == "" || env.Type == "" {
        return nil, fmt.Errorf("envelope missing required fields: id=%q org_id=%q type=%q",
            env.ID, env.OrgID, env.Type)
    }
    return &env, nil
}
```

---

## 8. Consumer Group Registry

Use only these canonical group IDs. Adding a new consumer requires updating the
registry in `shared/kafka/topics.go` — not inventing a new string locally:

```go
const (
    GroupAudit           = "openguard-audit-v1"
    GroupThreat          = "openguard-threat-v1"
    GroupAlerting        = "openguard-alerting-v1"
    GroupCompliance      = "openguard-compliance-v1"
    GroupPolicy          = "openguard-policy-v1"
    GroupSaga            = "openguard-saga-v1"
    GroupWebhookDelivery = "openguard-webhook-delivery-v1"
)
```

All consumers set `auto.offset.reset: earliest` so replays after deployment are safe.

---

## 9. Consumer Checklist

Before submitting any Kafka consumer code:

- [ ] `enable.auto.commit: false` in consumer config
- [ ] `enable.auto.offset.store: false` in consumer config
- [ ] `CommitOffsets()` called ONLY after successful downstream write
- [ ] Flush failure returns error WITHOUT committing offsets
- [ ] `ordered=false` on MongoDB BulkWrite
- [ ] E11000 duplicate key errors filtered as non-failures
- [ ] Schema version validated before processing each message
- [ ] Unparseable messages routed to DLQ, not dropped silently
- [ ] `time.NewTicker` used for flush interval, not `time.Sleep`
- [ ] Consumer group ID is from the canonical registry in `shared/kafka/topics.go`
- [ ] Hash chain uses `NULLIF` pattern... (no, hash chain is per the HMAC fields above)
- [ ] `auto.offset.reset: earliest` so replays are safe
- [ ] Graceful shutdown: flush remaining buffer before returning from `Run()`

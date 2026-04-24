package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

type MockKafkaReader struct {
	Messages  []kafka.Message
	index     int
	Committed int
}

func (m *MockKafkaReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	if m.index >= len(m.Messages) {
		// Block until context cancelled
		<-ctx.Done()
		return kafka.Message{}, ctx.Err()
	}
	msg := m.Messages[m.index]
	// DO NOT advance index here; it's a mock behavior to simulate Fetch not advancing offset
	return msg, nil
}

func (m *MockKafkaReader) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	m.Committed += len(msgs)
	m.index += len(msgs) // Only advance on commit
	return nil
}

type MockDeliverer struct {
	DeliverFunc func(ctx context.Context, messageKey, target, payload, secret string) error
	Attempts    int
}

func (m *MockDeliverer) Deliver(ctx context.Context, messageKey, target, payload, secret string) error {
	m.Attempts++
	if m.DeliverFunc != nil {
		return m.DeliverFunc(ctx, messageKey, target, payload, secret)
	}
	return nil
}

type MockKafkaPublisher struct{}

func (m *MockKafkaPublisher) Publish(ctx context.Context, topic, key string, payload []byte) error {
	return nil
}

// TestAtLeastOnceDelivery_ProcessKilled simulates the process being killed
// (e.g. by OOM or panic) during the message delivery phase, and verifies
// that upon restart, the message is fetched and processed again because
// the offset was not committed.
func TestAtLeastOnceDelivery_ProcessKilled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	req := WebhookDeliveryRequest{Target: "http://example.com", Payload: "{}"}
	val, _ := json.Marshal(req)

	msg := kafka.Message{
		Key:   []byte("test-delivery-id"),
		Value: val,
	}

	reader := &MockKafkaReader{
		Messages: []kafka.Message{msg},
	}

	deliverer := &MockDeliverer{
		DeliverFunc: func(ctx context.Context, messageKey, target, payload, secret string) error {
			// Simulate a panic/process kill during delivery attempt
			panic("process killed during delivery")
		},
	}

	consumer := &WebhookConsumer{
		reader:    reader,
		deliverer: deliverer,
		publisher: &MockKafkaPublisher{},
		logger:    logger,
	}

	// 1. First run: simulates "Start" picking up the message, but crashing during Deliver
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Process "killed"
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		_ = consumer.Start(ctx)
	}()

	// Since it paniced before CommitMessages was called, Committed should be 0
	if reader.Committed != 0 {
		t.Fatalf("expected 0 commits, got %d", reader.Committed)
	}

	// 2. Restart process: consumer starts again. Because index wasn't advanced (no commit),
	// it will fetch the SAME message again.
	// This time we make delivery succeed.
	deliverer.DeliverFunc = func(ctx context.Context, messageKey, target, payload, secret string) error {
		return nil // Success
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// Wait a bit and then cancel to stop the consumer loop
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := consumer.Start(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("unexpected error from Start: %v", err)
	}

	// It should have been committed now
	if reader.Committed != 1 {
		t.Fatalf("expected 1 commit, got %d", reader.Committed)
	}

	// Total delivery attempts across restarts: 1 panic + 1 success = 2
	if deliverer.Attempts != 2 {
		t.Fatalf("expected 2 delivery attempts (1 from before crash, 1 after), got %d", deliverer.Attempts)
	}
}

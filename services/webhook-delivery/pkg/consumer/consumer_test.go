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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockReader struct {
	mock.Mock
}

func (m *MockReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	args := m.Called(ctx)
	return args.Get(0).(kafka.Message), args.Error(1)
}

func (m *MockReader) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	args := m.Called(ctx, msgs)
	return args.Error(0)
}

type MockDeliverer struct {
	mock.Mock
}

func (m *MockDeliverer) Deliver(ctx context.Context, messageKey, target, payload, secret string) error {
	args := m.Called(ctx, messageKey, target, payload, secret)
	return args.Error(0)
}

type MockPublisher struct {
	mock.Mock
}

func (m *MockPublisher) Publish(ctx context.Context, topic, key string, payload []byte) error {
	args := m.Called(ctx, topic, key, payload)
	return args.Error(0)
}

func TestWebhookConsumer_ProcessMessage_DLQ(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockDeliverer := new(MockDeliverer)
	mockPublisher := new(MockPublisher)
	
	c := &WebhookConsumer{
		deliverer:  mockDeliverer,
		publisher:  mockPublisher,
		logger:     logger,
		getBackoff: func(int) time.Duration { return 0 }, // Instant retry for test
	}

	ctx := context.Background()
	req := WebhookDeliveryRequest{
		Target:  "http://example.com/webhook",
		Payload: "{}",
		Secret:  "secret",
	}
	val, _ := json.Marshal(req)
	msg := kafka.Message{
		Key:   []byte("msg-1"),
		Value: val,
	}

	// Mock 5 failed attempts
	mockDeliverer.On("Deliver", mock.Anything, "msg-1", req.Target, req.Payload, req.Secret).Return(errors.New("connection reset")).Times(5)
	
	// Mock DLQ publish
	mockPublisher.On("Publish", mock.Anything, "webhook.dlq", "msg-1", mock.Anything).Return(nil)

	err := c.processMessage(ctx, msg)
	assert.NoError(t, err)

	mockDeliverer.AssertExpectations(t)
	mockPublisher.AssertExpectations(t)
}

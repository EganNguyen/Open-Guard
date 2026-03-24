package consumer_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/openguard/audit/pkg/consumer"
	"github.com/openguard/audit/pkg/models"
	sharedmodels "github.com/openguard/shared/models"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWriter struct {
	added []models.AuditEvent
}

func (m *mockWriter) Add(ctx context.Context, ev models.AuditEvent) error {
	m.added = append(m.added, ev)
	return nil
}

type mockStateRepo struct {
	lastSeq  int64
	lastHash string
}

func (m *mockStateRepo) GetLastChainState(ctx context.Context, orgID string) (int64, string, error) {
	return m.lastSeq, m.lastHash, nil
}

func TestConsumer_HandleMessage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	writer := &mockWriter{}
	repo := &mockStateRepo{lastSeq: 5, lastHash: "prev-hash"}
	secret := "test-secret"
	
	c := consumer.NewConsumer([]string{"localhost:9092"}, []string{"topic1"}, repo, writer, logger, secret)
	
	envelope := sharedmodels.EventEnvelope{
		ID:         "evt-123",
		OrgID:      "org-1",
		Type:       "test.event",
		OccurredAt: time.Now(),
		Payload:    []byte(`{"foo":"bar"}`),
	}
	val, _ := json.Marshal(envelope)
	
	msg := kafka.Message{
		Value: val,
	}
	
	err := c.HandleMessage(context.Background(), msg)
	require.NoError(t, err)
	
	assert.Len(t, writer.added, 1)
	added := writer.added[0]
	assert.Equal(t, "evt-123", added.EventID)
	assert.Equal(t, int64(6), added.ChainSeq)
	assert.Equal(t, "prev-hash", added.PrevChainHash)
	assert.NotEmpty(t, added.ChainHash)

	t.Run("unmarshal error", func(t *testing.T) {
		msg := kafka.Message{Value: []byte("invalid json")}
		err := c.HandleMessage(context.Background(), msg)
		assert.Error(t, err)
	})

	t.Run("initial event (no prev state)", func(t *testing.T) {
		repoEmpty := &mockStateRepo{lastSeq: 0, lastHash: ""}
		c2 := consumer.NewConsumer([]string{"localhost:9092"}, []string{"topic1"}, repoEmpty, writer, logger, secret)
		err := c2.HandleMessage(context.Background(), msg)
		require.NoError(t, err)
		assert.Equal(t, int64(1), writer.added[len(writer.added)-1].ChainSeq)
		assert.Equal(t, "", writer.added[len(writer.added)-1].PrevChainHash)
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := &mockStateRepoErr{}
		c3 := consumer.NewConsumer([]string{"localhost:9092"}, []string{"topic1"}, repoErr, writer, logger, secret)
		err := c3.HandleMessage(context.Background(), msg)
		assert.Error(t, err)
	})
}

type mockStateRepoErr struct {
	mockStateRepo
}

func (m *mockStateRepoErr) GetLastChainState(ctx context.Context, orgID string) (int64, string, error) {
	return 0, "", context.DeadlineExceeded
}

func TestConsumer_Stop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	repo := &mockStateRepo{}
	writer := &mockWriter{}
	secret := "test-secret"
	
	// We can't easily mock the internal kafka.Reader without a real connection,
	// but calling Stop() on a NewConsumer instance (even with dummy brokers)
	// will exercise the Close() call and code coverage.
	c := consumer.NewConsumer([]string{"localhost:9092"}, []string{"topic1"}, repo, writer, logger, secret)
	err := c.Stop()
	assert.NoError(t, err)
}

type mockWriterErr struct{}

func (m *mockWriterErr) Add(ctx context.Context, ev models.AuditEvent) error {
	return context.DeadlineExceeded
}

func TestConsumer_HandleMessage_WriterError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	writer := &mockWriterErr{}
	repo := &mockStateRepo{lastSeq: 5, lastHash: "prev-hash"}
	secret := "test-secret"
	
	c := consumer.NewConsumer([]string{"localhost:9092"}, []string{"topic1"}, repo, writer, logger, secret)
	
	envelope := sharedmodels.EventEnvelope{ID: "evt-123", OrgID: "org-1"}
	val, _ := json.Marshal(envelope)
	msg := kafka.Message{Value: val}
	
	err := c.HandleMessage(context.Background(), msg)
	assert.Error(t, err)
}

func TestConsumer_HandleMessage_DifferentOrg(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	writer := &mockWriter{}
	repo := &mockStateRepo{lastSeq: 10, lastHash: "hash-x"}
	secret := "test-secret"
	c := consumer.NewConsumer([]string{"localhost:9092"}, []string{"topic1"}, repo, writer, logger, secret)
	
	val, _ := json.Marshal(sharedmodels.EventEnvelope{ID: "e1", OrgID: "org-2"})
	c.HandleMessage(context.Background(), kafka.Message{Value: val})
	
	assert.Equal(t, int64(11), writer.added[0].ChainSeq)
}

package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

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

func (m *MockReader) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) BulkWrite(ctx context.Context, events []interface{}) error {
	args := m.Called(ctx, events)
	return args.Error(0)
}

func (m *MockRepository) ReserveSequence(ctx context.Context, orgID string, count int64) (int64, string, error) {
	args := m.Called(ctx, orgID, count)
	return args.Get(0).(int64), args.String(1), args.Error(2)
}

func (m *MockRepository) UpdateHashChainCAS(ctx context.Context, orgID, prevHash, newHash string) (bool, error) {
	args := m.Called(ctx, orgID, prevHash, newHash)
	return args.Bool(0), args.Error(1)
}

func TestFlush_Success(t *testing.T) {
	os.Setenv("AUDIT_SECRET_KEY", "test-secret")
	defer os.Unsetenv("AUDIT_SECRET_KEY")

	mockReader := new(MockReader)
	mockRepo := new(MockRepository)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	consumer := &AuditConsumer{
		reader: mockReader,
		repo:   mockRepo,
		logger: logger,
	}

	ctx := context.Background()
	orgID := "org-123"
	eventID := "evt-456"

	eventData, _ := json.Marshal(map[string]interface{}{
		"org_id":   orgID,
		"event_id": eventID,
		"data":     "test",
	})

	messages := []kafka.Message{
		{Value: eventData, Topic: "audit.trail"},
	}

	mockRepo.On("ReserveSequence", ctx, orgID, int64(1)).Return(int64(10), "prev-hash", nil)
	mockRepo.On("UpdateHashChainCAS", ctx, orgID, "prev-hash", mock.Anything).Return(true, nil)
	mockRepo.On("BulkWrite", ctx, mock.Anything).Return(nil)
	mockReader.On("CommitMessages", ctx, mock.Anything).Return(nil)

	consumer.flush(ctx, messages)

	mockRepo.AssertExpectations(t)
	mockReader.AssertExpectations(t)
}

func TestFlush_MissingOrgID(t *testing.T) {
	mockReader := new(MockReader)
	mockRepo := new(MockRepository)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	consumer := &AuditConsumer{
		reader: mockReader,
		repo:   mockRepo,
		logger: logger,
	}

	ctx := context.Background()
	eventData, _ := json.Marshal(map[string]interface{}{
		"event_id": "evt-456",
		"data":     "test",
	})

	messages := []kafka.Message{
		{Value: eventData, Topic: "audit.trail"},
	}

	// Should return early without calling repo
	consumer.flush(ctx, messages)

	mockRepo.AssertNotCalled(t, "ReserveSequence", mock.Anything, mock.Anything, mock.Anything)
}

func TestFlush_BulkWriteFailure(t *testing.T) {
	mockReader := new(MockReader)
	mockRepo := new(MockRepository)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	consumer := &AuditConsumer{
		reader: mockReader,
		repo:   mockRepo,
		logger: logger,
	}

	ctx := context.Background()
	orgID := "org-123"
	eventData, _ := json.Marshal(map[string]interface{}{
		"org_id":   orgID,
		"event_id": "evt-456",
	})

	messages := []kafka.Message{
		{Value: eventData, Topic: "audit.trail"},
	}

	mockRepo.On("ReserveSequence", ctx, orgID, int64(1)).Return(int64(10), "prev-hash", nil)
	mockRepo.On("UpdateHashChainCAS", ctx, orgID, "prev-hash", mock.Anything).Return(true, nil)
	mockRepo.On("BulkWrite", ctx, mock.Anything).Return(assert.AnError)

	consumer.flush(ctx, messages)

	// Should NOT commit messages if BulkWrite fails
	mockReader.AssertNotCalled(t, "CommitMessages", mock.Anything, mock.Anything)
}

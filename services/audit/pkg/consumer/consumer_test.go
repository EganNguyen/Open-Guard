package consumer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	_ = os.Setenv("AUDIT_SECRET_KEY", "test-secret")
	defer func() { _ = os.Unsetenv("AUDIT_SECRET_KEY") }()

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

func TestFlush_HashChainIntegrity(t *testing.T) {
	_ = os.Setenv("AUDIT_SECRET_KEY", "test-secret")
	defer func() { _ = os.Unsetenv("AUDIT_SECRET_KEY") }()

	mockReader := new(MockReader)
	mockRepo := new(MockRepository)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	consumer := &AuditConsumer{
		reader: mockReader,
		repo:   mockRepo,
		logger: logger,
	}

	ctx := context.Background()
	orgID := "org-1"

	// 1. Prepare 2 messages in a batch
	msg1Data, _ := json.Marshal(map[string]interface{}{"org_id": orgID, "event_id": "e1"})
	msg2Data, _ := json.Marshal(map[string]interface{}{"org_id": orgID, "event_id": "e2"})
	messages := []kafka.Message{
		{Value: msg1Data, Topic: "audit.trail"},
		{Value: msg2Data, Topic: "audit.trail"},
	}

	// 2. Mock expectations
	// Initial state: sequence 10, prevHash "h0"
	mockRepo.On("ReserveSequence", ctx, orgID, int64(2)).Return(int64(10), "h0", nil)

	// We capture the events passed to BulkWrite to verify hashes
	var capturedEvents []interface{}
	mockRepo.On("BulkWrite", ctx, mock.Anything).Run(func(args mock.Arguments) {
		capturedEvents = args.Get(1).([]interface{})
	}).Return(nil)

	// Final hash update (CAS)
	mockRepo.On("UpdateHashChainCAS", ctx, orgID, "h0", mock.Anything).Return(true, nil)
	mockReader.On("CommitMessages", ctx, mock.Anything).Return(nil)

	// 3. Execute
	consumer.flush(ctx, messages)

	// 4. Verify Integrity
	assert.Len(t, capturedEvents, 2)
	e1 := capturedEvents[0].(map[string]interface{})
	e2 := capturedEvents[1].(map[string]interface{})

	assert.Equal(t, int64(10), e1["sequence"])
	assert.Equal(t, int64(11), e2["sequence"])

	// Manual hash calculation to verify
	secret := "test-secret"

	// Hash 1
	mac1 := hmac.New(sha256.New, []byte(secret))
	mac1.Write([]byte("e1|h0"))
	h1 := hex.EncodeToString(mac1.Sum(nil))
	assert.Equal(t, h1, e1["integrity_hash"])

	// Hash 2
	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write([]byte("e2|" + h1))
	h2 := hex.EncodeToString(mac2.Sum(nil))
	assert.Equal(t, h2, e2["integrity_hash"])
}

func TestFlush_CASRetryOnConflict(t *testing.T) {
	_ = os.Setenv("AUDIT_SECRET_KEY", "test-secret")
	defer func() { _ = os.Unsetenv("AUDIT_SECRET_KEY") }()

	mockReader := new(MockReader)
	mockRepo := new(MockRepository)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	consumer := &AuditConsumer{
		reader: mockReader,
		repo:   mockRepo,
		logger: logger,
	}

	ctx := context.Background()
	orgID := "org-1"
	msgData, _ := json.Marshal(map[string]interface{}{"org_id": orgID, "event_id": "e1"})
	messages := []kafka.Message{{Value: msgData}}

	// First attempt: CAS fails
	mockRepo.On("ReserveSequence", ctx, orgID, int64(1)).Return(int64(10), "h0", nil).Once()
	mockRepo.On("UpdateHashChainCAS", ctx, orgID, "h0", mock.Anything).Return(false, nil).Once()

	// Second attempt: Success
	mockRepo.On("ReserveSequence", ctx, orgID, int64(1)).Return(int64(11), "h1", nil).Once()
	mockRepo.On("UpdateHashChainCAS", ctx, orgID, "h1", mock.Anything).Return(true, nil).Once()

	mockRepo.On("BulkWrite", ctx, mock.Anything).Return(nil)
	mockReader.On("CommitMessages", ctx, mock.Anything).Return(nil)

	consumer.flush(ctx, messages)

	mockRepo.AssertExpectations(t)
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
